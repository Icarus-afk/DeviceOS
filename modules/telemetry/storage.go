package telemetry

import (
	"encoding/json"
	"fmt"
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

func (m *Module) storeTelemetry(deviceID string, ts time.Time, metrics, metadata json.RawMessage) (int64, error) {
	res, err := m.db.Exec(
		`INSERT INTO telemetry (device_id, timestamp, metrics, metadata)
		 VALUES (?, ?, ?, ?)`,
		deviceID, ts, string(metrics), string(metadata),
	)
	if err != nil {
		return 0, fmt.Errorf("store telemetry: %w", err)
	}

	m.db.Exec(`UPDATE devices SET status='online', last_seen=? WHERE id=?`, ts, deviceID)

	return res.LastInsertId()
}
