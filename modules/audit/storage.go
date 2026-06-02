package audit

const migration = `
CREATE TABLE IF NOT EXISTS audit_log (
    id         TEXT PRIMARY KEY,
    actor      TEXT NOT NULL,
    action     TEXT NOT NULL,
    target     TEXT NOT NULL,
    details    TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_created
    ON audit_log(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_actor
    ON audit_log(actor);
`

const orgMigration = `ALTER TABLE audit_log ADD COLUMN org_id TEXT NOT NULL DEFAULT '';`
