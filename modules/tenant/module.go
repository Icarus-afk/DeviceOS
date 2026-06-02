package tenant

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

func (m *Module) Name() string { return "tenant" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("tenant_v1", migrations); err != nil {
		return fmt.Errorf("tenant: migrate: %w", err)
	}
	slog.Info("tenant module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("tenant: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/orgs", m.handleCreate)
	r.HandleFunc("GET /api/v1/orgs", m.handleList)
	r.HandleFunc("POST /api/v1/orgs/{id}/users", m.handleInviteUser)
	r.HandleFunc("GET /api/v1/orgs/{id}/users", m.handleListUsers)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

type Org struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID        string `json:"id"`
	OrgID     string `json:"org_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

func (m *Module) handleCreate(w http.ResponseWriter, r *http.Request) {
	var org Org
	if err := json.NewDecoder(r.Body).Decode(&org); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}
	if org.Name == "" {
		httperr.BadRequest(w, "name is required")
		return
	}
	org.ID = fmt.Sprintf("org_%d", time.Now().UnixNano())
	org.CreatedAt = time.Now()

	_, err := m.db.Exec(`INSERT INTO orgs (id, name, created_at) VALUES (?, ?, ?)`,
		org.ID, org.Name, org.CreatedAt)
	if err != nil {
		slog.Error("create org", "error", err)
		httperr.Internal(w, "failed to create org")
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (m *Module) handleList(w http.ResponseWriter, r *http.Request) {
	rows, err := m.db.Query(`SELECT id, name, created_at FROM orgs ORDER BY name`)
	if err != nil {
		httperr.Internal(w, "query failed")
		return
	}
	defer rows.Close()
	orgs := make([]Org, 0)
	for rows.Next() {
		var o Org
		rows.Scan(&o.ID, &o.Name, &o.CreatedAt)
		orgs = append(orgs, o)
	}
	writeJSON(w, http.StatusOK, map[string]any{"orgs": orgs})
}

func (m *Module) handleInviteUser(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("id")
	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}
	if u.Email == "" {
		httperr.BadRequest(w, "email is required")
		return
	}
	if u.Role == "" {
		u.Role = "viewer"
	}
	u.ID = fmt.Sprintf("usr_%d", time.Now().UnixNano())
	u.OrgID = orgID
	u.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	_, err := m.db.Exec(
		`INSERT INTO org_users (id, org_id, email, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.OrgID, u.Email, u.Role, u.CreatedAt)
	if err != nil {
		httperr.Internal(w, "failed to invite user")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (m *Module) handleListUsers(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("id")
	rows, err := m.db.Query(
		`SELECT id, org_id, email, role, created_at FROM org_users WHERE org_id = ? ORDER BY email`, orgID)
	if err != nil {
		httperr.Internal(w, "query failed")
		return
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.Role, &u.CreatedAt)
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
