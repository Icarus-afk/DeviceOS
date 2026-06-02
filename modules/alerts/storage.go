package alerts

const migrations = `
CREATE TABLE IF NOT EXISTS alert_rules (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    metric         TEXT NOT NULL,
    operator       TEXT NOT NULL,
    threshold      REAL NOT NULL DEFAULT 0,
    duration       TEXT NOT NULL DEFAULT '',
    channel        TEXT NOT NULL DEFAULT 'log',
    channel_target TEXT NOT NULL DEFAULT '',
    enabled        INTEGER NOT NULL DEFAULT 1,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS alert_events (
    id         TEXT PRIMARY KEY,
    rule_id    TEXT NOT NULL,
    rule_name  TEXT NOT NULL,
    device_id  TEXT NOT NULL,
    metric     TEXT NOT NULL,
    value      REAL NOT NULL,
    message    TEXT NOT NULL,
    severity   TEXT NOT NULL DEFAULT 'warning',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (rule_id) REFERENCES alert_rules(id)
);

CREATE INDEX IF NOT EXISTS idx_alert_events_created
    ON alert_events(created_at DESC);
`

const orgMigration = `
ALTER TABLE alert_rules ADD COLUMN org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_events ADD COLUMN org_id TEXT NOT NULL DEFAULT '';
`
