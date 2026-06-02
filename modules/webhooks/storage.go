package webhooks

const orgMigration = `ALTER TABLE webhooks ADD COLUMN org_id TEXT NOT NULL DEFAULT '';`

const migrations = `
CREATE TABLE IF NOT EXISTS webhooks (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    url        TEXT NOT NULL,
    secret     TEXT NOT NULL DEFAULT '',
    events     TEXT NOT NULL DEFAULT '[]',
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id          TEXT PRIMARY KEY,
    webhook_id  TEXT NOT NULL,
    event       TEXT NOT NULL,
    payload     TEXT NOT NULL,
    status      TEXT NOT NULL,
    status_code INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id)
);
`
