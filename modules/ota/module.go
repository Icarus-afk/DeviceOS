package ota

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/httperr"
)

type Module struct {
	db       db.DBClient
	storeDir string
}

func New(db db.DBClient) *Module {
	return &Module{db: db, storeDir: "data/firmware"}
}

func (m *Module) Name() string { return "ota" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("ota_v1", migrations); err != nil {
		return fmt.Errorf("ota: migrate: %w", err)
	}
	if err := m.db.Migrate("ota_v2_org", orgMigration); err != nil {
		return fmt.Errorf("ota: migrate org: %w", err)
	}
	if err := os.MkdirAll(m.storeDir, 0755); err != nil {
		return fmt.Errorf("ota: mkdir: %w", err)
	}
	slog.Info("ota module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("ota: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/firmware", m.handleUpload)
	r.HandleFunc("GET /api/v1/firmware", m.handleList)
	r.HandleFunc("GET /api/v1/firmware/{id}", m.handleGet)
	r.HandleFunc("POST /api/v1/firmware/{id}/deploy", m.handleDeploy)
	r.HandleFunc("GET /api/v1/deployments/{id}", m.handleDeploymentStatus)
	r.HandleFunc("PUT /api/v1/deployments/{id}/device-status", m.handleDeviceStatus)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

type Firmware struct {
	ID               string    `json:"id"`
	Version          string    `json:"version"`
	TargetDeviceType string    `json:"target_device_type"`
	Checksum         string    `json:"checksum"`
	Size             int64     `json:"size"`
	Changelog        string    `json:"changelog,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type Deployment struct {
	ID             string        `json:"id"`
	FirmwareID     string        `json:"firmware_id"`
	TargetGroup    string        `json:"target_group"`
	RolloutPercent int           `json:"rollout_percent"`
	Status         string        `json:"status"`
	DeviceStates   []DeviceState `json:"device_states,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}

type DeviceState struct {
	DeviceID string `json:"device_id"`
	Status   string `json:"status"`
}

type firmwareUploadRequest struct {
	Version          string `json:"version"`
	TargetDeviceType string `json:"target_device_type"`
	Changelog        string `json:"changelog,omitempty"`
}

func orgID(r *http.Request) string { return r.Header.Get("X-Org-ID") }

func (m *Module) handleUpload(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	oid := orgID(r)
	var (
		version    string
		targetType string
		changelog  string
		data       []byte
	)

	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			httperr.BadRequest(w, "file too large")
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			httperr.BadRequest(w, "file required")
			return
		}
		defer file.Close()

		version = r.FormValue("version")
		targetType = r.FormValue("target_device_type")
		changelog = r.FormValue("changelog")

		data, err = io.ReadAll(file)
		if err != nil {
			httperr.Internal(w, "failed to read file")
			return
		}
	} else {
		var req firmwareUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httperr.BadRequest(w, "invalid request body")
			return
		}
		version = req.Version
		targetType = req.TargetDeviceType
		changelog = req.Changelog
	}

	if version == "" || targetType == "" {
		httperr.BadRequest(w, "version and target_device_type required")
		return
	}

	var id, checksum string
	var size int64
	if len(data) > 0 {
		hash := sha256.Sum256(data)
		checksum = hex.EncodeToString(hash[:])
		id = fmt.Sprintf("fw_%s", checksum[:12])
		size = int64(len(data))

		dst := filepath.Join(m.storeDir, id)
		if err := os.WriteFile(dst, data, 0644); err != nil {
			httperr.Internal(w, "failed to store file")
			return
		}
	} else {
		id = fmt.Sprintf("fw_%d", time.Now().UnixNano())
		checksum = "metadata-only"
	}

	fw := Firmware{
		ID:               id,
		Version:          version,
		TargetDeviceType: targetType,
		Checksum:         checksum,
		Size:             size,
		Changelog:        changelog,
		CreatedAt:        time.Now(),
	}

	_, err := m.db.Exec(
		`INSERT INTO firmware (id, version, target_device_type, checksum, size, changelog, created_at, org_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		fw.ID, fw.Version, fw.TargetDeviceType, fw.Checksum, fw.Size, fw.Changelog, fw.CreatedAt, oid,
	)
	if err != nil {
		if len(data) > 0 {
			os.Remove(filepath.Join(m.storeDir, id))
		}
		httperr.Internal(w, "failed to register firmware")
		return
	}

	writeJSON(w, http.StatusCreated, fw)
}

func (m *Module) handleList(w http.ResponseWriter, r *http.Request) {
	oid := orgID(r)
	rows, err := m.db.Query(
		`SELECT id, version, target_device_type, checksum, size, changelog, created_at
		 FROM firmware WHERE org_id = ? ORDER BY created_at DESC`, oid,
	)
	if err != nil {
		slog.Error("list firmware", "error", err)
		httperr.Internal(w, "query failed")
		return
	}
	defer rows.Close()

	list := make([]Firmware, 0)
	for rows.Next() {
		var f Firmware
		rows.Scan(&f.ID, &f.Version, &f.TargetDeviceType, &f.Checksum, &f.Size, &f.Changelog, &f.CreatedAt)
		list = append(list, f)
	}
	writeJSON(w, http.StatusOK, map[string]any{"firmware": list})
}

func (m *Module) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	oid := orgID(r)
	var f Firmware
	err := m.db.QueryRow(
		`SELECT id, version, target_device_type, checksum, size, changelog, created_at
		 FROM firmware WHERE id = ? AND org_id = ?`, id, oid,
	).Scan(&f.ID, &f.Version, &f.TargetDeviceType, &f.Checksum, &f.Size, &f.Changelog, &f.CreatedAt)
	if err != nil {
		httperr.NotFound(w, "firmware not found")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

type DeployRequest struct {
	TargetGroup    string `json:"target_group"`
	RolloutPercent int    `json:"rollout_percent"`
}

func (m *Module) handleDeploy(w http.ResponseWriter, r *http.Request) {
	firmwareID := r.PathValue("id")

	var req DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}
	if req.RolloutPercent <= 0 || req.RolloutPercent > 100 {
		req.RolloutPercent = 100
	}

	deployID := fmt.Sprintf("dep_%d", time.Now().UnixNano())
	_, err := m.db.Exec(
		`INSERT INTO deployments (id, firmware_id, target_group, rollout_percent, status, created_at)
		 VALUES (?, ?, ?, ?, 'in_progress', ?)`,
		deployID, firmwareID, req.TargetGroup, req.RolloutPercent, time.Now(),
	)
	if err != nil {
		httperr.Internal(w, "failed to create deployment")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id": deployID, "firmware_id": firmwareID,
		"status": "in_progress", "rollout_percent": req.RolloutPercent,
	})
}

func (m *Module) handleDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var d Deployment
	err := m.db.QueryRow(
		`SELECT id, firmware_id, target_group, rollout_percent, status, created_at
		 FROM deployments WHERE id = ?`, id,
	).Scan(&d.ID, &d.FirmwareID, &d.TargetGroup, &d.RolloutPercent, &d.Status, &d.CreatedAt)
	if err != nil {
		httperr.NotFound(w, "deployment not found")
		return
	}

	rows, err := m.db.Query(
		`SELECT device_id, status FROM deployment_devices WHERE deployment_id = ?`, id,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ds DeviceState
			rows.Scan(&ds.DeviceID, &ds.Status)
			d.DeviceStates = append(d.DeviceStates, ds)
		}
	}

	writeJSON(w, http.StatusOK, d)
}

type DeviceStatusRequest struct {
	DeviceID string `json:"device_id"`
	Status   string `json:"status"`
}

func (m *Module) handleDeviceStatus(w http.ResponseWriter, r *http.Request) {
	deployID := r.PathValue("id")
	var req DeviceStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}

	_, err := m.db.Exec(
		`INSERT INTO deployment_devices (deployment_id, device_id, status)
		 VALUES (?, ?, ?)
		 ON CONFLICT(deployment_id, device_id) DO UPDATE SET status=excluded.status`,
		deployID, req.DeviceID, req.Status,
	)
	if err != nil {
		httperr.Internal(w, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "recorded"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
