package devices

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

func (m *Module) Name() string { return "devices" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("devices_v1", migration); err != nil {
		return fmt.Errorf("devices: migrate v1: %w", err)
	}
	if err := m.db.Migrate("devices_v2_org", orgMigration); err != nil {
		return fmt.Errorf("devices: migrate org: %w", err)
	}
	slog.Info("devices module initialized")
	return nil
}

func orgID(r *http.Request) string {
	return r.Header.Get("X-Org-ID")
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("devices: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/devices", m.handleRegister)
	r.HandleFunc("GET /api/v1/devices", m.handleList)
	r.HandleFunc("GET /api/v1/devices/{id}", m.handleGet)
	r.HandleFunc("PUT /api/v1/devices/{id}", m.handleUpdate)
	r.HandleFunc("DELETE /api/v1/devices/{id}", m.handleDelete)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

type Device struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	SecretKey string          `json:"secret_key,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Tags      []string        `json:"tags,omitempty"`
	Group     string          `json:"group,omitempty"`
	OrgID     string          `json:"org_id,omitempty"`
	Status    string          `json:"status"`
	LastSeen  *time.Time      `json:"last_seen,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type RegisterRequest struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Tags     []string        `json:"tags,omitempty"`
	Group    string          `json:"group,omitempty"`
}

type RegisterResponse struct {
	Device    Device `json:"device"`
	SecretKey string `json:"secret_key"`
}

func (m *Module) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid request body")
		return
	}

	oid := orgID(r)
	device, secret, err := m.createDevice(req, oid)
	if err != nil {
		slog.Error("register device", "error", err)
		httperr.Internal(w, "failed to register device")
		return
	}

	writeJSON(w, http.StatusCreated, RegisterResponse{Device: *device, SecretKey: secret})
}

func (m *Module) handleList(w http.ResponseWriter, r *http.Request) {
	oid := orgID(r)
	devices, err := m.listDevices(oid)
	if err != nil {
		slog.Error("list devices", "error", err)
		httperr.Internal(w, "failed to list devices")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (m *Module) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	oid := orgID(r)
	device, err := m.getDevice(id, oid)
	if err != nil {
		httperr.NotFound(w, "device not found")
		return
	}
	writeJSON(w, http.StatusOK, device)
}

func (m *Module) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	oid := orgID(r)
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid request body")
		return
	}
	device, err := m.updateDevice(id, req, oid)
	if err != nil {
		httperr.NotFound(w, "device not found")
		return
	}
	writeJSON(w, http.StatusOK, device)
}

func (m *Module) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	oid := orgID(r)
	if err := m.deleteDevice(id, oid); err != nil {
		httperr.NotFound(w, "device not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
