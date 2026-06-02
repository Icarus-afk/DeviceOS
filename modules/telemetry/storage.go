package telemetry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

const migration = `
CREATE TABLE IF NOT EXISTS telemetry (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    metrics   TEXT NOT NULL,
    metadata  TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_telemetry_device_ts
    ON telemetry(device_id, timestamp DESC);
`

const orgMigration = `ALTER TABLE telemetry ADD COLUMN org_id TEXT NOT NULL DEFAULT '';`

func (m *Module) storeTelemetry(deviceID string, ts time.Time, metrics, metadata json.RawMessage, orgID string) (int64, error) {
	res, err := m.db.Exec(
		`INSERT INTO telemetry (device_id, org_id, timestamp, metrics, metadata)
		 VALUES (?, ?, ?, ?, ?)`,
		deviceID, orgID, ts, string(metrics), string(metadata),
	)
	if err != nil {
		return 0, fmt.Errorf("store telemetry: %w", err)
	}

	m.db.Exec(`UPDATE devices SET status='online', last_seen=? WHERE id=?`, ts, deviceID)

	return res.LastInsertId()
}

func (m *Module) pruneLoop() {
	ticker := time.NewTicker(m.pruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.prune()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Module) prune() {
	cutoff := time.Now().Add(-m.telemetryTTL)
	res, err := m.db.Exec(
		`DELETE FROM telemetry WHERE timestamp < ?`,
		cutoff,
	)
	if err != nil {
		slog.Error("telemetry prune", "error", err)
		return
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		slog.Info("telemetry prune deleted old records", "count", affected, "cutoff", cutoff)
	}
}
