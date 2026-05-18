package commands

import (
	"database/sql"
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

type Module struct {
	db        sparkdb.DBClient
	pending   sync.Map
}

func New(db sparkdb.DBClient) *Module {
	return &Module{db: db}
}

func (m *Module) Name() string { return "commands" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("commands_v1", migration); err != nil {
		return fmt.Errorf("commands: migrate: %w", err)
	}
	slog.Info("commands module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("commands: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/devices/{id}/commands", m.handleSend)
	r.HandleFunc("GET /api/v1/devices/{id}/commands", m.handleList)
	r.HandleFunc("GET /api/v1/commands/{id}", m.handleGet)
	r.HandleFunc("PUT /api/v1/commands/{id}/result", m.handleResult)
	r.HandleFunc("GET /api/v1/ws/commands", m.handleWS)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

type Command struct {
	ID         string          `json:"id"`
	DeviceID   string          `json:"device_id"`
	Command    string          `json:"command"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Status     string          `json:"status"`
	Result     json.RawMessage `json:"result,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

type SendRequest struct {
	Command string          `json:"command"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func (m *Module) handleSend(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")

	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.Command == "" {
		http.Error(w, `{"error":"command is required"}`, http.StatusBadRequest)
		return
	}

	payload := req.Payload
	if payload == nil {
		payload = json.RawMessage("{}")
	}

	cmd := Command{
		ID:        generateCmdID(),
		DeviceID:  deviceID,
		Command:   req.Command,
		Payload:   payload,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	_, err := m.db.Exec(
		`INSERT INTO commands (id, device_id, command, payload, status, created_at)
		 VALUES (?, ?, ?, ?, 'pending', ?)`,
		cmd.ID, cmd.DeviceID, cmd.Command, string(cmd.Payload), cmd.CreatedAt,
	)
	if err != nil {
		slog.Error("store command", "error", err)
		http.Error(w, `{"error":"failed to create command"}`, http.StatusInternalServerError)
		return
	}

	m.pending.Store(cmd.DeviceID, cmd)

	writeJSON(w, http.StatusCreated, cmd)
}

func (m *Module) handleList(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")

	rows, err := m.db.Query(
		`SELECT id, device_id, command, payload, status, result, created_at, completed_at
		 FROM commands WHERE device_id = ? ORDER BY created_at DESC LIMIT 50`, deviceID,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var commands []Command
	for rows.Next() {
		var c Command
		var payloadStr, resultStr sql.NullString
		if err := rows.Scan(&c.ID, &c.DeviceID, &c.Command, &payloadStr, &c.Status, &resultStr, &c.CreatedAt, &c.CompletedAt); err != nil {
			continue
		}
		if payloadStr.Valid {
			c.Payload = json.RawMessage(payloadStr.String)
		}
		if resultStr.Valid {
			c.Result = json.RawMessage(resultStr.String)
		}
		commands = append(commands, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"commands": commands})
}

func (m *Module) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var c Command
	var payloadStr, resultStr sql.NullString
	err := m.db.QueryRow(
		`SELECT id, device_id, command, payload, status, result, created_at, completed_at
		 FROM commands WHERE id = ?`, id,
	).Scan(&c.ID, &c.DeviceID, &c.Command, &payloadStr, &c.Status, &resultStr, &c.CreatedAt, &c.CompletedAt)
	if err != nil {
		http.Error(w, `{"error":"command not found"}`, http.StatusNotFound)
		return
	}
	if payloadStr.Valid {
		c.Payload = json.RawMessage(payloadStr.String)
	}
	if resultStr.Valid {
		c.Result = json.RawMessage(resultStr.String)
	}
	writeJSON(w, http.StatusOK, c)
}

type ResultRequest struct {
	Result json.RawMessage `json:"result"`
	Status string          `json:"status"`
}

func (m *Module) handleResult(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req ResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.Status == "" {
		req.Status = "completed"
	}

	now := time.Now()
	res, err := m.db.Exec(
		`UPDATE commands SET status=?, result=?, completed_at=? WHERE id=?`,
		req.Status, string(req.Result), now, id,
	)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		http.Error(w, `{"error":"command not found"}`, http.StatusNotFound)
		return
	}
	m.pending.Delete(id)
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (m *Module) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("cmd ws upgrade", "error", err)
		return
	}
	defer conn.Close()

	deviceID := r.URL.Query().Get("device_id")
	slog.Info("cmd ws connected", "device_id", deviceID)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Send pending commands
	m.pending.Range(func(key, value any) bool {
		if deviceID == "" || key.(string) == deviceID {
			cmd := value.(Command)
			data, _ := json.Marshal(cmd)
			conn.WriteMessage(websocket.TextMessage, data)
		}
		return true
	})

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if deviceID != "" {
				var cmd Command
				var pStr, rStr sql.NullString
				err := m.db.QueryRow(
					`SELECT id, device_id, command, payload, status, result, created_at, completed_at
					 FROM commands WHERE device_id = ? AND status = 'pending'
					 ORDER BY created_at ASC LIMIT 1`, deviceID,
				).Scan(&cmd.ID, &cmd.DeviceID, &cmd.Command, &pStr, &cmd.Status, &rStr, &cmd.CreatedAt, &cmd.CompletedAt)
				if err == nil {
					if pStr.Valid {
						cmd.Payload = json.RawMessage(pStr.String)
					}
					if rStr.Valid {
						cmd.Result = json.RawMessage(rStr.String)
					}
					data, _ := json.Marshal(cmd)
					conn.WriteMessage(websocket.TextMessage, data)
					m.db.Exec(`UPDATE commands SET status='delivered' WHERE id=?`, cmd.ID)
				}
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
