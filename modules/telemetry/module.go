package telemetry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lohtbrok/deviceos/internal/sparkdb"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type TelemetryCallback func(deviceID string, metrics, metadata json.RawMessage)

type Module struct {
	db       sparkdb.DBClient
	hub      *Hub
	hook     TelemetryCallback
}

func (m *Module) SetTelemetryHook(fn TelemetryCallback) {
	m.hook = fn
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]bool)}
}

func (h *Hub) Add(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()
}

func (h *Hub) Remove(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

func (h *Hub) Broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			slog.Warn("ws broadcast", "error", err)
			conn.Close()
			go h.Remove(conn)
		}
	}
}

func New(db sparkdb.DBClient) *Module {
	return &Module{db: db, hub: NewHub()}
}

func (m *Module) Name() string { return "telemetry" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("telemetry_v1", migration); err != nil {
		return fmt.Errorf("telemetry: migrate: %w", err)
	}
	slog.Info("telemetry module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("telemetry: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/telemetry", m.handleIngest)
	r.HandleFunc("GET /api/v1/telemetry", m.handleQuery)
	r.HandleFunc("GET /api/v1/telemetry/latest", m.handleLatest)
	r.HandleFunc("GET /api/v1/ws/telemetry", m.handleWS)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

type Telemetry struct {
	ID        int64           `json:"id"`
	DeviceID  string          `json:"device_id"`
	Timestamp time.Time       `json:"timestamp"`
	Metrics   json.RawMessage `json:"metrics"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

type IngestRequest struct {
	DeviceID  string          `json:"device_id"`
	SecretKey string          `json:"secret_key"`
	Timestamp *time.Time      `json:"timestamp,omitempty"`
	Metrics   json.RawMessage `json:"metrics"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

func (m *Module) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" || req.Metrics == nil {
		http.Error(w, `{"error":"device_id and metrics are required"}`, http.StatusBadRequest)
		return
	}

	ts := time.Now()
	if req.Timestamp != nil {
		ts = *req.Timestamp
	}

	if req.Metadata == nil {
		req.Metadata = json.RawMessage("{}")
	}

	id, err := m.storeTelemetry(req.DeviceID, ts, req.Metrics, req.Metadata)
	if err != nil {
		slog.Error("store telemetry", "error", err)
		http.Error(w, `{"error":"failed to store telemetry"}`, http.StatusInternalServerError)
		return
	}

	evt, _ := json.Marshal(map[string]any{
		"event": "telemetry",
		"data": Telemetry{
			ID:        id,
			DeviceID:  req.DeviceID,
			Timestamp: ts,
			Metrics:   req.Metrics,
			Metadata:  req.Metadata,
		},
	})
	m.hub.Broadcast(evt)

	if m.hook != nil {
		m.hook(req.DeviceID, req.Metrics, req.Metadata)
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (m *Module) handleQuery(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}

	rows, err := m.db.Query(
		`SELECT id, device_id, timestamp, metrics, metadata
		 FROM telemetry WHERE device_id = ?
		 ORDER BY timestamp DESC LIMIT `+limit, deviceID,
	)
	if err != nil {
		slog.Error("query telemetry", "error", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []Telemetry
	for rows.Next() {
		var t Telemetry
		var metricsStr, metadataStr string
		if err := rows.Scan(&t.ID, &t.DeviceID, &t.Timestamp, &metricsStr, &metadataStr); err != nil {
			continue
		}
		t.Metrics = json.RawMessage(metricsStr)
		t.Metadata = json.RawMessage(metadataStr)
		results = append(results, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"telemetry": results})
}

func (m *Module) handleLatest(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")

	var t Telemetry
	var metricsStr, metadataStr string
	err := m.db.QueryRow(
		`SELECT id, device_id, timestamp, metrics, metadata
		 FROM telemetry WHERE device_id = ?
		 ORDER BY timestamp DESC LIMIT 1`, deviceID,
	).Scan(&t.ID, &t.DeviceID, &t.Timestamp, &metricsStr, &metadataStr)
	if err != nil {
		http.Error(w, `{"error":"no telemetry found"}`, http.StatusNotFound)
		return
	}
	t.Metrics = json.RawMessage(metricsStr)
	t.Metadata = json.RawMessage(metadataStr)
	writeJSON(w, http.StatusOK, t)
}

func (m *Module) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade", "error", err)
		return
	}
	m.hub.Add(conn)
	slog.Info("ws client connected")

	go func() {
		defer func() {
			m.hub.Remove(conn)
			conn.Close()
			slog.Info("ws client disconnected")
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
