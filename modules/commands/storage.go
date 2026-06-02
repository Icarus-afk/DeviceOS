package commands

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const migration = `
CREATE TABLE IF NOT EXISTS commands (
    id          TEXT PRIMARY KEY,
    device_id   TEXT NOT NULL,
    command     TEXT NOT NULL,
    payload     TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    result      TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_commands_device
    ON commands(device_id, created_at DESC);
`

const orgMigration = `ALTER TABLE commands ADD COLUMN org_id TEXT NOT NULL DEFAULT '';`

func generateCmdID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("cmd_%s", hex.EncodeToString(b))
}
