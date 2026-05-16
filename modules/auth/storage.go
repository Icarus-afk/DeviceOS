package auth

import (
	"crypto/rand"
	"encoding/hex"
)

const migration = `
CREATE TABLE IF NOT EXISTS api_keys (
    key        TEXT PRIMARY KEY,
    role       TEXT NOT NULL DEFAULT 'admin',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func generateAPIKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "dos_" + hex.EncodeToString(b)
}
