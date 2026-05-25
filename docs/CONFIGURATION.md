# Configuration Reference

DeviceOS is configured via a YAML file (default: `deviceos.yaml`) with environment variable overrides.

## Configuration Sources

Settings are applied in this order (later sources override earlier ones):

1. **Built-in defaults** (see below)
2. **YAML config file** (`deviceos.yaml` or `--config <path>`)
3. **Environment variables** (`DEVICEOS_*`)

## Full Config Reference

```yaml
server:
  # HTTP server bind address
  # Env: DEVICEOS_SERVER_HOST
  # Default: "0.0.0.0"
  host: "0.0.0.0"

  # HTTP server port
  # Env: DEVICEOS_SERVER_PORT
  # Default: 8080
  port: 8080

sparkdb:
  # Path to SparkDB binary. Auto-discovered if empty (see SETUP.md).
  # Env: DEVICEOS_SPARKDB_BIN_PATH or SPARKDB_BIN
  # Default: "" (auto-discover)
  bin_path: ""

  # SparkDB server host (SparkDB binds to this internally)
  # Env: DEVICEOS_SPARKDB_HOST
  # Default: "127.0.0.1"
  host: "127.0.0.1"

  # SparkDB server port
  # Env: DEVICEOS_SPARKDB_PORT
  # Default: 9600
  port: 9600

  # SparkDB database name
  # Env: DEVICEOS_SPARKDB_DATABASE
  # Default: "deviceos"
  database: "deviceos"

  # SparkDB data directory (for persistence)
  # Env: DEVICEOS_SPARKDB_DATA_DIR
  # Default: "./data/sparkdb"
  data_dir: "./data/sparkdb"

  # SparkDB API key (for automated auth)
  # Env: DEVICEOS_SPARKDB_API_KEY
  api_key: ""

  # Enable SparkDB authentication
  # Env: DEVICEOS_SPARKDB_AUTH
  # Default: false
  auth: false

  # WAL mode for the database. Disable for very low-disk environments.
  # Env: DEVICEOS_SPARKDB_WAL_MODE
  # Default: true
  wal_mode: true

  # Maximum database connections
  # Env: DEVICEOS_SPARKDB_MAX_CONNECTIONS
  # Default: 100
  max_connections: 100

  # Extra SparkDB config keys — deep-merged into the generated
  # sparkdb.json config file. Supports any SparkDB config section.
  extra_config:
    # Backup settings
    backup:
      schedule: "0 3 * * *"     # daily at 3am
      keep_count: 30

    # TLS for SparkDB's internal server
    tls:
      enabled: false

    # Encryption at rest
    encryption:
      enabled: false

    # Replication
    replication:
      role: standalone          # primary | replica | standalone

modules:
  # JWT signing secret. CHANGE IN PRODUCTION.
  # Env: DEVICEOS_JWT_SECRET
  # Default: "dev-change-me-in-production" (WARNING: insecure)
  jwt_secret: ""

  # Bootstrap admin API key. Auto-generated if empty (printed on first start).
  # Env: DEVICEOS_ADMIN_TOKEN
  # Default: "" (auto-generated)
  admin_api_key: ""
```

## Environment Variable Reference

| Env Var | Maps To | Default |
|---|---|---|
| `DEVICEOS_SERVER_HOST` | `server.host` | `0.0.0.0` |
| `DEVICEOS_SERVER_PORT` | `server.port` | `8080` |
| `DEVICEOS_SPARKDB_BIN_PATH` | `sparkdb.bin_path` | empty |
| `SPARKDB_BIN` | `sparkdb.bin_path` *(backward compat)* | empty |
| `DEVICEOS_SPARKDB_HOST` | `sparkdb.host` | `127.0.0.1` |
| `DEVICEOS_SPARKDB_PORT` | `sparkdb.port` | `9600` |
| `DEVICEOS_SPARKDB_DATABASE` | `sparkdb.database` | `deviceos` |
| `DEVICEOS_SPARKDB_DATA_DIR` | `sparkdb.data_dir` | `./data/sparkdb` |
| `DEVICEOS_SPARKDB_API_KEY` | `sparkdb.api_key` | empty |
| `DEVICEOS_SPARKDB_AUTH` | `sparkdb.auth` | `false` |
| `DEVICEOS_SPARKDB_WAL_MODE` | `sparkdb.wal_mode` | `true` |
| `DEVICEOS_SPARKDB_MAX_CONNECTIONS` | `sparkdb.max_connections` | `100` |
| `DEVICEOS_JWT_SECRET` | `modules.jwt_secret` | `dev-change-me-in-production` |
| `DEVICEOS_ADMIN_TOKEN` | `modules.admin_api_key` | auto-generated |

## Example Configs

See `config/examples/` for ready-to-use config files:

| File | Use Case |
|---|---|
| `deviceos.dev.yaml` | Development (defaults, explicit admin token) |
| `deviceos.prod.yaml` | Production (auth enabled, secrets via env vars) |
| `deviceos.minimal.yaml` | Minimal (just required fields) |

## Rate Limiting

The SparkDB client enforces a minimum 1.3s gap between queries to stay under SparkDB's 60 req/min rate limit. This means:
- Device registration: ~1 device/second
- Telemetry ingestion with alerts: ~0.3 datapoints/second
- Simple queries: ~0.77 queries/second

This is a SparkDB server-side limitation, not a DeviceOS limitation.
