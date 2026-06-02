package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/httperr"
)

type Module struct {
	db db.DBClient
}

func New(db db.DBClient) *Module {
	return &Module{db: db}
}

func (m *Module) Name() string { return "webhooks" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("webhooks_v1", migrations); err != nil {
		return fmt.Errorf("webhooks: migrate: %w", err)
	}
	if err := m.db.Migrate("webhooks_v2_org", orgMigration); err != nil {
		return fmt.Errorf("webhooks: migrate org: %w", err)
	}
	slog.Info("webhooks module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("webhooks: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/webhooks", m.handleCreate)
	r.HandleFunc("GET /api/v1/webhooks", m.handleList)
	r.HandleFunc("PUT /api/v1/webhooks/{id}", m.handleUpdate)
	r.HandleFunc("DELETE /api/v1/webhooks/{id}", m.handleDelete)
	r.HandleFunc("GET /api/v1/webhooks/{id}/deliveries", m.handleDeliveries)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

type Webhook struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Secret    string   `json:"secret,omitempty"`
	Events    []string `json:"events"`
	Enabled   bool     `json:"enabled"`
	CreatedAt string   `json:"created_at"`
}

type Delivery struct {
	ID         string `json:"id"`
	WebhookID  string `json:"webhook_id"`
	Event      string `json:"event"`
	Payload    string `json:"payload"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code"`
	CreatedAt  string `json:"created_at"`
}

func orgID(r *http.Request) string { return r.Header.Get("X-Org-ID") }

func (m *Module) handleCreate(w http.ResponseWriter, r *http.Request) {
	var wh Webhook
	if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}
	if wh.Name == "" || wh.URL == "" {
		httperr.BadRequest(w, "name and url are required")
		return
	}
	wh.ID = fmt.Sprintf("wh_%d", time.Now().UnixNano())
	wh.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	oid := orgID(r)

	events, _ := json.Marshal(wh.Events)
	_, err := m.db.Exec(
		`INSERT INTO webhooks (id, name, url, secret, events, enabled, created_at, org_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		wh.ID, wh.Name, wh.URL, wh.Secret, string(events), wh.Enabled, wh.CreatedAt, oid,
	)
	if err != nil {
		httperr.Internal(w, "failed to create webhook")
		return
	}
	writeJSON(w, http.StatusCreated, wh)
}

func (m *Module) handleList(w http.ResponseWriter, r *http.Request) {
	oid := orgID(r)
	rows, err := m.db.Query(
		`SELECT id, name, url, secret, events, enabled, created_at FROM webhooks WHERE org_id = ? ORDER BY created_at DESC`, oid,
	)
	if err != nil {
		httperr.Internal(w, "query failed")
		return
	}
	defer rows.Close()

	hooks := make([]Webhook, 0)
	for rows.Next() {
		var wh Webhook
		var eventsStr string
		rows.Scan(&wh.ID, &wh.Name, &wh.URL, &wh.Secret, &eventsStr, &wh.Enabled, &wh.CreatedAt)
		json.Unmarshal([]byte(eventsStr), &wh.Events)
		hooks = append(hooks, wh)
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": hooks})
}

func (m *Module) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	oid := orgID(r)
	var wh Webhook
	if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}
	events, _ := json.Marshal(wh.Events)
	res, err := m.db.Exec(
		`UPDATE webhooks SET name=?, url=?, secret=?, events=?, enabled=? WHERE id=? AND org_id=?`,
		wh.Name, wh.URL, wh.Secret, string(events), wh.Enabled, id, oid,
	)
	if err != nil {
		httperr.Internal(w, "update failed")
		return
	}
	if a, _ := res.RowsAffected(); a == 0 {
		httperr.NotFound(w, "webhook not found")
		return
	}
	wh.ID = id
	writeJSON(w, http.StatusOK, wh)
}

func (m *Module) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	oid := orgID(r)
	res, err := m.db.Exec(`DELETE FROM webhooks WHERE id = ? AND org_id = ?`, id, oid)
	if err != nil || func() int64 { a, _ := res.RowsAffected(); return a }() == 0 {
		httperr.NotFound(w, "webhook not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) handleDeliveries(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	oid := orgID(r)
	rows, err := m.db.Query(
		`SELECT d.id, d.webhook_id, d.event, d.payload, d.status, d.status_code, d.created_at
		 FROM webhook_deliveries d JOIN webhooks w ON w.id = d.webhook_id
		 WHERE d.webhook_id = ? AND w.org_id = ? ORDER BY d.created_at DESC LIMIT 50`, id, oid,
	)
	if err != nil {
		httperr.Internal(w, "query failed")
		return
	}
	defer rows.Close()

	deliveries := make([]Delivery, 0)
	for rows.Next() {
		var d Delivery
		rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.Payload, &d.Status, &d.StatusCode, &d.CreatedAt)
		deliveries = append(deliveries, d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": deliveries})
}

func (m *Module) Deliver(event string, payload []byte, whs []Webhook) {
	for _, wh := range whs {
		if !wh.Enabled {
			continue
		}
		if !contains(wh.Events, event) {
			continue
		}
		go m.deliver(wh, event, payload)
	}
}

func (m *Module) deliver(wh Webhook, event string, payload []byte) {
	id := fmt.Sprintf("del_%d", time.Now().UnixNano())
	resp, err := http.Post(wh.URL, "application/json", bytes.NewReader(payload))
	status := "success"
	code := 0
	if err != nil {
		status = "failed"
	} else {
		code = resp.StatusCode
		if code >= 400 {
			status = "failed"
		}
		resp.Body.Close()
	}
	m.db.Exec(
		`INSERT INTO webhook_deliveries (id, webhook_id, event, payload, status, status_code, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, wh.ID, event, string(payload), status, code, time.Now().UTC().Format(time.RFC3339),
	)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
