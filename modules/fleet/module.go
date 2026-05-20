package fleet

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
)

type Module struct {
	db sparkdb.DBClient
}

func New(db sparkdb.DBClient) *Module {
	return &Module{db: db}
}

func (m *Module) Name() string { return "fleet" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("fleet_v1", migrations); err != nil {
		return fmt.Errorf("fleet: migrate: %w", err)
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

type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (m *Module) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var g Group
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if g.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	g.ID = fmt.Sprintf("grp_%d", time.Now().UnixNano())
	g.CreatedAt = time.Now()

	_, err := m.db.Exec(`INSERT INTO groups (id, name, created_at) VALUES (?, ?, ?)`,
		g.ID, g.Name, g.CreatedAt)
	if err != nil {
		slog.Error("create group", "error", err)
		http.Error(w, `{"error":"failed to create group"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (m *Module) handleListGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := m.db.Query(`SELECT id, name, created_at FROM groups ORDER BY name`)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var groups []Group
	for rows.Next() {
		var g Group
		rows.Scan(&g.ID, &g.Name, &g.CreatedAt)
		groups = append(groups, g)
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (m *Module) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := m.db.Exec(`DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	if a, _ := res.RowsAffected(); a == 0 {
		http.Error(w, `{"error":"group not found"}`, http.StatusNotFound)
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
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	tagsJSON, _ := json.Marshal(req.Tags)
	res, err := m.db.Exec(`UPDATE devices SET tags=? WHERE id=?`, string(tagsJSON), deviceID)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	if a, _ := res.RowsAffected(); a == 0 {
		http.Error(w, `{"error":"device not found"}`, http.StatusNotFound)
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
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	res, err := m.db.Exec(`UPDATE devices SET device_group=? WHERE id=?`, req.Group, deviceID)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	if a, _ := res.RowsAffected(); a == 0 {
		http.Error(w, `{"error":"device not found"}`, http.StatusNotFound)
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
	FROM devices`).Scan(&total, &online, &offline)

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
