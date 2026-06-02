package events

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type subscriber struct {
	conn     *websocket.Conn
	events   map[string]bool // nil or empty = all events
	mu       sync.Mutex
}

func (s *subscriber) wants(eventType string) bool {
	if s.events == nil {
		return true
	}
	return s.events[eventType]
}

func (s *subscriber) send(msg []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, msg)
}

type Hub struct {
	mu       sync.RWMutex
	subs     map[*websocket.Conn]*subscriber
}

func NewHub() *Hub {
	return &Hub{subs: make(map[*websocket.Conn]*subscriber)}
}

func (h *Hub) Subscribe(conn *websocket.Conn, eventTypes []string) *subscriber {
	var events map[string]bool
	if eventTypes != nil {
		events = make(map[string]bool, len(eventTypes))
		for _, et := range eventTypes {
			events[et] = true
		}
	}

	sub := &subscriber{conn: conn, events: events}

	h.mu.Lock()
	h.subs[conn] = sub
	h.mu.Unlock()

	return sub
}

func (h *Hub) Unsubscribe(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.subs, conn)
	h.mu.Unlock()
}

func (h *Hub) Publish(evt Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		slog.Error("events: marshal event", "type", evt.Type, "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn, sub := range h.subs {
		if !sub.wants(evt.Type) {
			continue
		}
		if err := sub.send(data); err != nil {
			slog.Warn("events: write", "error", err)
			conn.Close()
			go h.Unsubscribe(conn)
		}
	}
}

func (h *Hub) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

type Module struct {
	hub    *Hub
	stopCh chan struct{}
}

func New() *Module {
	return &Module{
		hub:    NewHub(),
		stopCh: make(chan struct{}),
	}
}

func (m *Module) Hub() *Hub { return m.hub }

func (m *Module) Name() string { return "events" }

func (m *Module) Init(cfg any) error {
	slog.Info("events module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("events: unexpected mux type")
	}
	r.HandleFunc("GET /api/v1/ws/events", m.handleWS)
	return nil
}

func (m *Module) Start() error { return nil }

func (m *Module) Stop() error {
	if m.stopCh != nil {
		close(m.stopCh)
	}
	return nil
}

func (m *Module) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("events: ws upgrade", "error", err)
		return
	}

	eventTypes := parseEventsParam(r.URL.Query().Get("events"))
	sub := m.hub.Subscribe(conn, eventTypes)
	slog.Info("events: client connected",
		"events", eventTypes,
		"count", m.hub.Len(),
	)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer func() {
			m.hub.Unsubscribe(conn)
			conn.Close()
			slog.Info("events: client disconnected",
				"count", m.hub.Len(),
			)
		}()

		for {
			select {
			case <-ticker.C:
				if err := sub.send([]byte(`{"type":"ping"}`)); err != nil {
					return
				}
			case <-m.stopCh:
				return
			default:
				_, _, err := conn.ReadMessage()
				if err != nil {
					return
				}
			}
		}
	}()
}

func parseEventsParam(raw string) []string {
	if raw == "" {
		return nil
	}
	var types []string
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			if i > start {
				types = append(types, raw[start:i])
			}
			start = i + 1
		}
	}
	return types
}
