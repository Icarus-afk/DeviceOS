# DeviceOS Configuration Reference

DeviceOS reads configuration from a YAML file (default: `deviceos.yaml`) with environment variable overrides. All environment variables use the `DEVICEOS_` prefix. Env vars take precedence over YAML values.

Generate a default config file with:

```bash
deviceos init
```

## Config File Location

```bash
deviceos start                    # uses deviceos.yaml in current directory
deviceos start --config /path/to/config.yaml
```

## YAML Reference

```yaml
server:
  host: "0.0.0.0"           # Listen address
  port: 8080                 # HTTP server port
  tls_key: ""                # Path to TLS private key (PEM)
  tls_cert: ""               # Path to TLS certificate (PEM)
  allowed_origins: []        # CORS allowed origins (e.g. ["https://app.example.com"])
  rate_limit_rpm: 0          # Rate limit: requests per minute per IP (0 = disabled)
  log_level: "info"          # Log level: debug, info, warn, error

storage:
  path: "./data/deviceos.db" # SQLite database file path
  max_open_conns: 0          # Max open connections (0 = default 25)
  max_idle_conns: 0          # Max idle connections (0 = default 5)

modules:
  jwt_secret: "dev-change-me-in-production"   # JWT signing key (MUST change in production)
  admin_api_key: ""                           # Admin API key (auto-generated if empty)
  telemetry_ttl: "720h"                       # Telemetry retention duration (0 = keep forever)
  telemetry_prune_interval: "1h"              # How often to prune expired telemetry
  mqtt:
    port: 1883                                # MQTT broker TCP port
```

## Field Details

### `server.host`
- Type: `string`
- Default: `"0.0.0.0"`
- Env: `DEVICEOS_SERVER_HOST`
- Set to `"127.0.0.1"` to bind only to localhost behind a reverse proxy.

### `server.port`
- Type: `int`
- Default: `8080`
- Env: `DEVICEOS_SERVER_PORT`
- Range: 1-65535.

### `server.tls_key`
- Type: `string`
- Default: `""` (TLS disabled)
- Env: `DEVICEOS_TLS_KEY`
- Path to PEM-encoded private key. Must be set together with `tls_cert`.

### `server.tls_cert`
- Type: `string`
- Default: `""` (TLS disabled)
- Env: `DEVICEOS_TLS_CERT`
- Path to PEM-encoded certificate. Must be set together with `tls_key`.

### `server.allowed_origins`
- Type: `[]string`
- Default: `[]` (CORS disabled)
- Env: `DEVICEOS_ALLOWED_ORIGINS` (comma-separated)
- List of allowed `Origin` header values. Set to `["*"]` to allow all origins.

### `server.rate_limit_rpm`
- Type: `int`
- Default: `0` (disabled)
- Env: `DEVICEOS_RATE_LIMIT_RPM`
- Token-bucket rate limiter per client IP. Must be non-negative. A value of 60 allows approximately 1 request per second per IP.

### `server.log_level`
- Type: `string`
- Default: `"info"`
- Env: `DEVICEOS_LOG_LEVEL`
- Valid values: `debug`, `info`, `warn`, `error`.

### `storage.path`
- Type: `string`
- Default: `"./data/deviceos.db"`
- Env: `DEVICEOS_STORAGE_PATH`
- Filesystem path for the SQLite database file. The parent directory must exist (or be created before starting).

### `storage.max_open_conns`
- Type: `int`
- Default: `0` (uses default 25)
- Env: `DEVICEOS_STORAGE_MAX_OPEN_CONNS`
- Maximum number of open connections to the database. 0 uses the internal default of 25.

### `storage.max_idle_conns`
- Type: `int`
- Default: `0` (uses default 5)
- Env: `DEVICEOS_STORAGE_MAX_IDLE_CONNS`
- Maximum number of idle connections in the pool. 0 uses the internal default of 5.

### `modules.jwt_secret`
- Type: `string`
- Default: `"dev-change-me-in-production"`
- Env: `DEVICEOS_JWT_SECRET`
- HMAC-SHA256 secret key for signing and verifying JWT tokens. **Must be changed to a strong, unique value in production.** A warning is logged if the default value is used.

### `modules.admin_api_key`
- Type: `string`
- Default: `""` (auto-generated)
- Env: `DEVICEOS_ADMIN_TOKEN`
- Static API key for admin access. If left empty, a random key is generated on first startup and stored in the database. To find the auto-generated key, check the logs or query the `api_keys` table.

### `modules.telemetry_ttl`
- Type: `string` (Go duration)
- Default: `"720h"` (30 days)
- Env: `DEVICEOS_TELEMETRY_TTL`
- How long telemetry data is retained. Set to `"0"` to keep telemetry forever. Examples: `"24h"`, `"168h"`, `"720h"`, `"2160h"`.

### `modules.telemetry_prune_interval`
- Type: `string` (Go duration)
- Default: `"1h"`
- Env: `DEVICEOS_TELEMETRY_PRUNE_INTERVAL`
- How often the background pruner checks for and deletes expired telemetry records. Only active when `telemetry_ttl` is greater than 0.

### `modules.mqtt.port`
- Type: `int`
- Default: `1883`
- Env: N/A (use `DEVICEOS_MQTT_PORT` if needed -- check env section)
- Note: The MQTT port is currently set only from the YAML config or the `mqtt` struct in code. This is configurable within the YAML as shown.

## Environment Variables

| Variable | Overrides | Example |
|---|---|---|
| `DEVICEOS_SERVER_HOST` | `server.host` | `0.0.0.0` |
| `DEVICEOS_SERVER_PORT` | `server.port` | `8080` |
| `DEVICEOS_TLS_CERT` | `server.tls_cert` | `/etc/deviceos/cert.pem` |
| `DEVICEOS_TLS_KEY` | `server.tls_key` | `/etc/deviceos/key.pem` |
| `DEVICEOS_ALLOWED_ORIGINS` | `server.allowed_origins` | `https://app.example.com,https://admin.example.com` |
| `DEVICEOS_RATE_LIMIT_RPM` | `server.rate_limit_rpm` | `120` |
| `DEVICEOS_LOG_LEVEL` | `server.log_level` | `debug` |
| `DEVICEOS_STORAGE_PATH` | `storage.path` | `/data/deviceos.db` |
| `DEVICEOS_STORAGE_MAX_OPEN_CONNS` | `storage.max_open_conns` | `50` |
| `DEVICEOS_STORAGE_MAX_IDLE_CONNS` | `storage.max_idle_conns` | `10` |
| `DEVICEOS_JWT_SECRET` | `modules.jwt_secret` | `your-256-bit-secret-here` |
| `DEVICEOS_ADMIN_TOKEN` | `modules.admin_api_key` | `dos_prod_admin_token_abc123` |
| `DEVICEOS_TELEMETRY_TTL` | `modules.telemetry_ttl` | `168h` |
| `DEVICEOS_TELEMETRY_PRUNE_INTERVAL` | `modules.telemetry_prune_interval` | `30m` |

## Default Configuration (Generated by `deviceos init`)

Running `deviceos init` creates `deviceos.yaml` with:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  tls_key: ""
  tls_cert: ""
  allowed_origins: []
  rate_limit_rpm: 0
  log_level: "info"

storage:
  path: "./data/deviceos.db"
  max_open_conns: 0
  max_idle_conns: 0

modules:
  jwt_secret: "dev-change-me-in-production"
  admin_api_key: ""
  telemetry_ttl: "720h"
  telemetry_prune_interval: "1h"
  mqtt:
    port: 1883
```

## Validation Rules

- `server.port` must be between 1 and 65535
- `server.tls_key` and `server.tls_cert` must be provided together (or both left empty)
- `storage.path` must not be empty
- `server.rate_limit_rpm` must not be negative
- `server.log_level` must be one of: `debug`, `info`, `warn`, `error`
- If `modules.jwt_secret` is the default value, a warning is issued (not an error, for development convenience)
