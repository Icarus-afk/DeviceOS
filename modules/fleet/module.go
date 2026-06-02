package fleet

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

func (m *Module) Name() string { return "fleet" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("fleet_v1", migrations); err != nil {
		return fmt.Errorf("fleet: migrate: %w", err)
	}
	if err := m.db.Migrate("fleet_v2_org", orgMigration); err != nil {
		return fmt.Errorf("fleet: migrate org: %w", err)
	}
	slog.Info("fleet module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("fleet: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/groups", m.handleCreateGroup)
	r.HandleFunc("GET /api/v1/groups", m.handleListGroups)
	r.HandleFunc("DELETE /api/v1/groups/{id}", m.handleDeleteGroup)
	r.HandleFunc("POST /api/v1/devices/{id}/tags", m.handleAddTags)
	r.HandleFunc("PUT /api/v1/devices/{id}/group", m.handleSetGroup)
	r.HandleFunc("GET /api/v1/fleet/health", m.handleHealth)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

func orgID(r *http.Request) string { return r.Header.Get("X-Org-ID") }

type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (m *Module) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var g Group
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}
	if g.Name == "" {
		httperr.BadRequest(w, "name is required")
		return
	}
	g.ID = fmt.Sprintf("grp_%d", time.Now().UnixNano())
	g.CreatedAt = time.Now()

	_, err := m.db.Exec(`INSERT INTO groups (id, name, created_at, org_id) VALUES (?, ?, ?, ?)`,
		g.ID, g.Name, g.CreatedAt, orgID(r))
	if err != nil {
		slog.Error("create group", "error", err)
		httperr.Internal(w, "failed to create group")
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (m *Module) handleListGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := m.db.Query(`SELECT id, name, created_at FROM groups WHERE org_id = ? ORDER BY name`, orgID(r))
	if err != nil {
		httperr.Internal(w, "query failed")
		return
	}
	defer rows.Close()
	groups := make([]Group, 0)
	for rows.Next() {
		var g Group
		rows.Scan(&g.ID, &g.Name, &g.CreatedAt)
		groups = append(groups, g)
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (m *Module) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := m.db.Exec(`DELETE FROM groups WHERE id = ? AND org_id = ?`, id, orgID(r))
	if err != nil {
		httperr.Internal(w, "delete failed")
		return
	}
	if a, _ := res.RowsAffected(); a == 0 {
		httperr.NotFound(w, "group not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type TagsRequest struct {
	Tags []string `json:"tags"`
}

func (m *Module) handleAddTags(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	var req TagsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}
	tagsJSON, _ := json.Marshal(req.Tags)
	res, err := m.db.Exec(`UPDATE devices SET tags=? WHERE id=? AND org_id=?`, string(tagsJSON), deviceID, orgID(r))
	if err != nil {
		httperr.Internal(w, "update failed")
		return
	}
	if a, _ := res.RowsAffected(); a == 0 {
		httperr.NotFound(w, "device not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": req.Tags})
}

type GroupRequest struct {
	Group string `json:"group"`
}

func (m *Module) handleSetGroup(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	var req GroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}
	res, err := m.db.Exec(`UPDATE devices SET device_group=? WHERE id=? AND org_id=?`, req.Group, deviceID, orgID(r))
	if err != nil {
		httperr.Internal(w, "update failed")
		return
	}
	if a, _ := res.RowsAffected(); a == 0 {
		httperr.NotFound(w, "device not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"group": req.Group})
}

func (m *Module) handleHealth(w http.ResponseWriter, r *http.Request) {
	var total, online, offline int
	m.db.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN status='online' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status='offline' THEN 1 ELSE 0 END), 0)
	FROM devices WHERE org_id = ?`, orgID(r)).Scan(&total, &online, &offline)

	writeJSON(w, http.StatusOK, map[string]any{
		"total_devices":   total,
		"online_devices":  online,
		"offline_devices": offline,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
