# DeviceOS Deployment Guide

## Quick Start

```bash
# Generate default config and data directory
deviceos init

# Start the server
deviceos start

# Check server health
deviceos status
```

The server listens on `http://0.0.0.0:8080`. Default admin API key is printed in the logs or stored in the database.

## Binary Download

### Build from Source

```bash
go build -o bin/deviceos ./cmd/deviceos
```

### Install

```bash
make install
# Copies to ~/.local/bin/deviceos
```

### Cross-Compile

```bash
make release
# Produces dist/deviceos-<version>-linux-amd64
#         dist/deviceos-<version>-linux-arm64
```

## Docker

### Build

```bash
make docker-build
# Tags: deviceos:latest, deviceos:<version>
```

### Run with Docker Compose

```yaml
# docker-compose.yml (included in repo root)
version: "3.9"
services:
  deviceos:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./deviceos.yaml:/data/deviceos.yaml:ro
      - deviceos_data:/data
    environment:
      DEVICEOS_SERVER_HOST: "0.0.0.0"
      DEVICEOS_SERVER_PORT: "8080"
      DEVICEOS_STORAGE_PATH: "/data/deviceos.db"
      DEVICEOS_LOG_LEVEL: "info"
    restart: unless-stopped

volumes:
  deviceos_data:
```

```bash
make docker-run
# or
docker compose up -d
```

### Docker Environment Variables

Use environment variables for production configuration instead of mounting a config file:

```bash
docker run -d \
  --name deviceos \
  -p 8080:8080 \
  -v deviceos_data:/data \
  -e DEVICEOS_JWT_SECRET="your-strong-secret-here" \
  -e DEVICEOS_ADMIN_TOKEN="your-admin-token" \
  -e DEVICEOS_STORAGE_PATH="/data/deviceos.db" \
  deviceos:latest
```

**Important**: The Docker image uses `USER deviceos` and the working directory is `/data`. Mount a volume at `/data` for database persistence.

## Production Checklist

Before deploying to production, verify each item:

- [ ] **Change JWT secret** -- Set `modules.jwt_secret` or `DEVICEOS_JWT_SECRET` to a strong random value. Do not use the default.
- [ ] **Enable TLS** -- Set `tls_cert` and `tls_key`, or place behind a TLS-terminating reverse proxy (nginx, Caddy, Traefik).
- [ ] **Set admin API key** -- Set `modules.admin_api_key` or `DEVICEOS_ADMIN_TOKEN` to a known value.
- [ ] **Configure rate limiting** -- Set `rate_limit_rpm` to an appropriate value (e.g. 60 for general API, higher for telemetry ingestion).
- [ ] **Set telemetry TTL** -- Configure `telemetry_ttl` based on your data retention requirements.
- [ ] **Use volume mounts** -- Always mount a persistent volume or bind mount for the database file.
- [ ] **Restrict CORS origins** -- Set `allowed_origins` to your dashboard domain(s).
- [ ] **Set log level** -- Use `"warn"` or `"error"` in production to reduce log volume; use `"info"` for normal operations.
- [ ] **Enable MQTT over TLS** -- If using MQTT, place behind a TLS-terminating proxy or add TLS support via the embedded broker configuration.

## TLS

### Self-Signed Certificate (Development/Internal)

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /etc/deviceos/key.pem \
  -out /etc/deviceos/cert.pem \
  -subj "/CN=deviceos.local"
```

Then set in config:

```yaml
server:
  tls_key: "/etc/deviceos/key.pem"
  tls_cert: "/etc/deviceos/cert.pem"
```

### Let's Encrypt (Production)

Use `certbot` with a reverse proxy (nginx/Caddy) that terminates TLS and forwards to DeviceOS on localhost:8080. DeviceOS does not include built-in ACME support.

Example nginx configuration:

```nginx
server {
    listen 443 ssl;
    server_name deviceos.example.com;

    ssl_certificate /etc/letsencrypt/live/deviceos.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/deviceos.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## Backup and Restore

### Backup

```bash
# Creates backup-<timestamp>.db.gz in current directory
deviceos backup

# Specify output path
deviceos backup --output /backups/deviceos-2026-06-01.db.gz
```

The backup command:
1. Opens the database in read-only mode
2. Creates a consistent snapshot using `VACUUM INTO`
3. Compresses the snapshot with gzip
4. Writes the compressed file to the output path

### Restore

```bash
deviceos restore backup-2026-06-01.db.gz
```

The restore command:
1. Decompresses the backup file
2. Runs `PRAGMA integrity_check` on the restored database
3. Prompts for confirmation
4. Backs up the current database to `<path>.bak`
5. Replaces the current database with the restored copy

**Restore requires the server to be stopped.**

## Multi-Tenancy

DeviceOS supports organizational multi-tenancy. Each organization (org) is isolated by an `org_id` column pattern on all data tables.

### Setting Up Orgs

```bash
# Create an org via the API
curl -X POST http://localhost:8080/api/v1/orgs \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "Acme Corp"}'

# Invite a user to the org
curl -X POST http://localhost:8080/api/v1/orgs/<org-id>/users \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"email": "user@acme.com", "role": "admin"}'
```

### Using X-Org-ID

All data operations scoped to an org use the `X-Org-ID` header:

```bash
# Register a device under org
curl -X POST http://localhost:8080/api/v1/devices \
  -H "Authorization: Bearer <jwt>" \
  -H "X-Org-ID: <org-id>" \
  -H "Content-Type: application/json" \
  -d '{"name": "sensor-001", "type": "temp-sensor"}'

# Query telemetry scoped to org
curl http://localhost:8080/api/v1/telemetry?device_id=sensor-001 \
  -H "Authorization: Bearer <jwt>" \
  -H "X-Org-ID: <org-id>"
```

When `X-Org-ID` is empty or absent, the request operates in a single-tenant context.

## Systemd Service Unit

Create `/etc/systemd/system/deviceos.service`:

```ini
[Unit]
Description=DeviceOS IoT Backend
After=network.target

[Service]
Type=simple
User=deviceos
Group=deviceos
WorkingDirectory=/opt/deviceos
ExecStart=/usr/local/bin/deviceos start --config /opt/deviceos/deviceos.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

# Security hardening
PrivateTmp=true
ProtectSystem=full
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable deviceos
sudo systemctl start deviceos
sudo systemctl status deviceos
```

## Performance Tuning

### SQLite Page Size

The database uses SQLite defaults with WAL mode and tuned pragmas set in `internal/db/db.go`:

| Pragma | Value | Effect |
|---|---|---|
| `journal_mode` | WAL | Write-Ahead Logging for concurrent reads |
| `synchronous` | NORMAL | Balance between durability and write speed |
| `busy_timeout` | 5000 | Wait up to 5s on locked database |
| `foreign_keys` | ON | Enforce referential integrity |
| `cache_size` | -8192 | 8MB page cache |
| `temp_store` | MEMORY | Store temp tables/indexes in memory |

### Connection Pool Settings

- `max_open_conns` -- default 25. Increase for higher concurrency. SQLite is single-writer, so too many connections will queue on write locks.
- `max_idle_conns` -- default 5. Keep idle connections ready to avoid reconnection overhead.
- `conn_max_lifetime` -- 5 minutes (hardcoded). Connections are recycled to prevent staleness.

### MQTT QoS Levels

The embedded MQTT broker supports QoS 0 and QoS 1. QoS 0 (fire-and-forget) is recommended for telemetry ingestion to minimize overhead. QoS 1 (at-least-once) is appropriate for command delivery.

### Operating System Tuning

For production deployments with high device counts:

```bash
# Increase max open file descriptors
echo "fs.file-max = 100000" >> /etc/sysctl.conf

# In systemd service unit
LimitNOFILE=65536
```

## Monitoring

### Health Check

```bash
curl http://localhost:8080/healthz
# {"status":"ok","version":"0.1.0-seed","uptime":"5m30s","modules":{"alerts":"ok","auth":"ok",...}}
```

### MQTT Status

```bash
curl http://localhost:8080/api/v1/mqtt/status
# {"status":"running","port":1883,"connected":42}
```

### Fleet Health

```bash
curl http://localhost:8080/api/v1/fleet/health \
  -H "Authorization: Bearer <jwt>"
# {"total_devices":150,"online_devices":142,"offline_devices":8}
```
