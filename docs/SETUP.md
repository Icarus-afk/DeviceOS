# Setup Guide

## Prerequisites

- **Go 1.22+** — for building from source
- **SparkDB binary** — downloaded or built separately

## Installing SparkDB

DeviceOS requires the SparkDB database server. You have several options:

### Option 1: Copy binary to project root

```bash
cp /path/to/sparkdb .
# DeviceOS will auto-discover it
```

### Option 2: Install to PATH

```bash
cp /path/to/sparkdb ~/.local/bin/
# Ensure ~/.local/bin is in PATH
```

### Option 3: Set in config

```yaml
# deviceos.yaml
sparkdb:
  bin_path: "/opt/sparkdb/sparkdb"
```

### Option 4: Set env var

```bash
export SPARKDB_BIN=/opt/sparkdb/sparkdb
export DEVICEOS_SPARKDB_BIN_PATH=/opt/sparkdb/sparkdb
```

### Binary search order

DeviceOS searches for `sparkdb` in this order:
1. `bin_path` in `deviceos.yaml`
2. `DEVICEOS_SPARKDB_BIN_PATH` env var
3. `SPARKDB_BIN` env var
4. `./sparkdb` (current directory)
5. `./bin/sparkdb`
6. `PATH`

## Building

```bash
# Build both binaries
make build        # builds bin/deviceos and bin/simload

# Or manually
go build -o bin/deviceos ./cmd/deviceos/
go build -o bin/simload ./cmd/simload/
```

## Configuration

DeviceOS loads configuration from `deviceos.yaml` by default:

```bash
# Use default config file
./bin/deviceos

# Use a different config
./bin/deviceos --config /path/to/config.yaml

# See all options
./bin/deviceos --help
```

### Configuration sources (in priority order)

1. CLI flags (`--config <path>`)
2. Environment variables (`DEVICEOS_*`)
3. YAML config file
4. Built-in defaults

See [CONFIGURATION.md](CONFIGURATION.md) for the full reference.

## Running

### Start the server

```bash
# Using make
make run

# Directly
./bin/deviceos

# With custom config
./bin/deviceos --config config/examples/deviceos.prod.yaml
```

The server starts SparkDB as a managed subprocess, runs database migrations, and begins serving HTTP. Watch the logs for progress:

```
time=... level=INFO msg="sparkdb started" addr=http://127.0.0.1:9600
time=... level=INFO msg="server starting" addr=0.0.0.0:8080
```

### Verify it's running

```bash
curl http://localhost:8080/healthz
# {"status":"ok"}
```

### Stopping

Send SIGINT (Ctrl+C) or SIGTERM. DeviceOS gracefully shuts down:
1. Stops HTTP server
2. Stops all modules
3. Closes SparkDB connection
4. Sends SIGINT to SparkDB (waits 10s, then kills)

```bash
kill <deviceos-pid>
```

## Simulating Devices

The `simload` tool registers many devices and sends telemetry:

```bash
# Register 100 devices, send telemetry, 10 concurrent workers
./bin/simload --server http://localhost:8080 --devices 100 --concurrency 10
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for more on the simulator.

## Production Considerations

- **Set a strong JWT secret** via `DEVICEOS_JWT_SECRET` env var
- **Set an admin API key** via `DEVICEOS_ADMIN_TOKEN` env var
- **Enable SparkDB auth** (`sparkdb.auth: true`)
- **Use a persistent data directory** (`sparkdb.data_dir`)
- **Configure backups** via `sparkdb.extra_config.backup`
- **Enable TLS** for the HTTP server if exposed to network
- **Run behind a reverse proxy** (nginx, Caddy) for TLS termination and rate limiting

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---|---|---|
| `sparkdb binary not found` | SparkDB not installed | Copy sparkdb binary to project root or set `bin_path` |
| `sparkdb: rate limit exceeded` | Too many requests too fast | Increase `MinRequestGap` or reduce concurrency |
| `failed to open SparkDB` | SparkDB didn't start or wrong host/port | Check SparkDB logs in data directory |
| `401 Unauthorized` | Invalid or expired JWT | Re-login with admin API key |
