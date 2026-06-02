package audit

import (
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

func (m *Module) Name() string { return "audit" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("audit_v1", migration); err != nil {
		return fmt.Errorf("audit: migrate: %w", err)
	}
	if err := m.db.Migrate("audit_v2_org", orgMigration); err != nil {
		return fmt.Errorf("audit: migrate org: %w", err)
	}
	slog.Info("audit module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("audit: unexpected mux type")
	}
	r.HandleFunc("GET /api/v1/audit", m.handleQuery)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

func orgID(r *http.Request) string { return r.Header.Get("X-Org-ID") }

type Entry struct {
	ID        string          `json:"id"`
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	Target    string          `json:"target"`
	Details   json.RawMessage `json:"details,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

func (m *Module) Log(actor, action, target string, details json.RawMessage) {
	id := fmt.Sprintf("aud_%d", time.Now().UnixNano())
	m.db.Exec(
		`INSERT INTO audit_log (id, actor, action, target, details, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, actor, action, target, string(details), time.Now(),
	)
}

func (m *Module) handleQuery(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}
	rows, err := m.db.Query(
		`SELECT id, actor, action, target, details, created_at
		 FROM audit_log WHERE org_id = ? ORDER BY created_at DESC LIMIT `+limit,
		orgID(r),
	)
	if err != nil {
		httperr.Internal(w, "query failed")
		return
	}
	defer rows.Close()

	entries := make([]Entry, 0)
	for rows.Next() {
		var e Entry
		var detailsStr string
		rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &detailsStr, &e.CreatedAt)
		e.Details = json.RawMessage(detailsStr)
		entries = append(entries, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit_logs": entries})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
