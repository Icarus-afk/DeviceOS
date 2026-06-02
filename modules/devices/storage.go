package devices

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lohtbrok/deviceos/internal/db"
)

const migration = `
CREATE TABLE IF NOT EXISTS devices (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT '',
    secret_key TEXT NOT NULL,
    metadata   TEXT NOT NULL DEFAULT '{}',
    tags       TEXT NOT NULL DEFAULT '[]',
    device_group TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'offline',
    last_seen  DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

const orgMigration = `ALTER TABLE devices ADD COLUMN org_id TEXT NOT NULL DEFAULT '';`

func (m *Module) createDevice(req RegisterRequest, orgID string) (*Device, string, error) {
	id := generateID("dev")
	secret := generateSecret()

	metadata := req.Metadata
	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	tagsJSON, _ := json.Marshal(req.Tags)
	if req.Tags == nil {
		tagsJSON = []byte("[]")
	}

	now := time.Now()
	_, err := m.db.Exec(
		`INSERT INTO devices (id, name, type, secret_key, metadata, tags, device_group, org_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'offline', ?, ?)`,
		id, req.Name, req.Type, secret, string(metadata), string(tagsJSON), req.Group, orgID, now, now,
	)
	if err != nil {
		return nil, "", fmt.Errorf("insert device: %w", err)
	}

	device := &Device{
		ID:        id,
		Name:      req.Name,
		Type:      req.Type,
		OrgID:     orgID,
		Status:    "offline",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if req.Metadata != nil {
		device.Metadata = req.Metadata
	}
	device.Tags = req.Tags

	return device, secret, nil
}

func (m *Module) listDevices(orgID string) ([]Device, error) {
	var err error
	var rows db.RowsInterface
	if orgID == "" {
		rows, err = m.db.Query(
			`SELECT id, name, type, metadata, tags, device_group, org_id, status, last_seen, created_at, updated_at
			 FROM devices ORDER BY created_at DESC`,
		)
	} else {
		rows, err = m.db.Query(
			`SELECT id, name, type, metadata, tags, device_group, org_id, status, last_seen, created_at, updated_at
			 FROM devices WHERE org_id = ? ORDER BY created_at DESC`, orgID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		var metadataStr, tagsStr string
		var lastSeen time.Time
		var lastSeenNull *time.Time
		err := rows.Scan(&d.ID, &d.Name, &d.Type, &metadataStr, &tagsStr, &d.Group, &d.OrgID, &d.Status, &lastSeen, &d.CreatedAt, &d.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if !lastSeen.IsZero() {
			lastSeenNull = &lastSeen
		}
		d.Metadata = json.RawMessage(metadataStr)
		json.Unmarshal([]byte(tagsStr), &d.Tags)
		d.LastSeen = lastSeenNull
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (m *Module) getDevice(id, orgID string) (*Device, error) {
	var d Device
	var metadataStr, tagsStr string
	var lastSeen time.Time
	var lastSeenNull *time.Time
	err := m.db.QueryRow(
		`SELECT id, name, type, metadata, tags, device_group, org_id, status, last_seen, created_at, updated_at
		 FROM devices WHERE id = ?`, id,
	).Scan(&d.ID, &d.Name, &d.Type, &metadataStr, &tagsStr, &d.Group, &d.OrgID, &d.Status, &lastSeen, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if !lastSeen.IsZero() {
		lastSeenNull = &lastSeen
	}
	d.Metadata = json.RawMessage(metadataStr)
	json.Unmarshal([]byte(tagsStr), &d.Tags)
	d.LastSeen = lastSeenNull
	return &d, nil
}

func (m *Module) updateDevice(id string, req RegisterRequest, orgID string) (*Device, error) {
	metadata := req.Metadata
	if metadata == nil {
		metadata = json.RawMessage("{}")
	}
	tagsJSON, _ := json.Marshal(req.Tags)
	if req.Tags == nil {
		tagsJSON = []byte("[]")
	}

	now := time.Now()
	_, err := m.db.Exec(
		`UPDATE devices SET name=?, type=?, metadata=?, tags=?, device_group=?, updated_at=?
		 WHERE id=?`,
		req.Name, req.Type, string(metadata), string(tagsJSON), req.Group, now, id,
	)
	if err != nil {
		return nil, err
	}
	return m.getDevice(id, orgID)
}

func (m *Module) deleteDevice(id, orgID string) error {
	_, err := m.db.Exec(`DELETE FROM devices WHERE id = ?`, id)
	return err
}

func generateID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

func generateSecret() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
