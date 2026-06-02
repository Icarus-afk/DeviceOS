package ota

const orgMigration = `ALTER TABLE firmware ADD COLUMN org_id TEXT NOT NULL DEFAULT '';`

const migrations = `
CREATE TABLE IF NOT EXISTS firmware (
    id                 TEXT PRIMARY KEY,
    version            TEXT NOT NULL,
    target_device_type TEXT NOT NULL,
    checksum           TEXT NOT NULL,
    size               INTEGER NOT NULL DEFAULT 0,
    changelog          TEXT NOT NULL DEFAULT '',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS deployments (
    id              TEXT PRIMARY KEY,
    firmware_id     TEXT NOT NULL,
    target_group    TEXT NOT NULL DEFAULT '',
    rollout_percent INTEGER NOT NULL DEFAULT 100,
    status          TEXT NOT NULL DEFAULT 'in_progress',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (firmware_id) REFERENCES firmware(id)
);

CREATE TABLE IF NOT EXISTS deployment_devices (
    deployment_id TEXT NOT NULL,
    device_id     TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (deployment_id, device_id),
    FOREIGN KEY (deployment_id) REFERENCES deployments(id)
);
`
