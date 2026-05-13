# DeviceOS

Self-hosted IoT backend — single binary, zero external dependencies.

DeviceOS manages device authentication, telemetry ingestion, real-time dashboards, OTA firmware updates, alerting, fleet management, and multi-tenancy. It runs SparkDB as a managed subprocess — no separate database setup needed.

## Quick Start

```bash
# 1. Place the sparkdb binary next to DeviceOS
cp /path/to/sparkdb .

# 2. Build
make setup

# 3. Start
make run
# Server listening on http://0.0.0.0:8080
```

## Documentation

| File | Contents |
|---|---|
| [docs/SETUP.md](docs/SETUP.md) | Installation, config, running |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | All config options (YAML + env vars) |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | System architecture |
| [docs/API.md](docs/API.md) | Full API reference |
| [docs/MODULES.md](docs/MODULES.md) | Module system guide |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | Development guide |
| [config/examples/](config/examples/) | Example config files |

## Feature Overview

| Feature | Description |
|---|---|
| Device Auth | Register devices, issue JWT tokens, API key authentication |
| Telemetry | Ingest metrics via HTTP or WebSocket, query historical/latest data |
| Real-time Dashboard | Web dashboard with live telemetry via WebSocket |
| OTA Firmware | Upload firmware, deploy to device groups, track rollout status |
| Alerting | Define rules on metric thresholds, get notified via webhooks |
| Fleet Management | Organize devices into groups, tag devices, fleet health overview |
| Commands | Send commands to devices, track execution results |
| Webhooks | Outbound HTTP hooks on device events |
| Multi-tenancy | Organizations with users and role-based access |
| Audit Log | Track admin actions across the system |
| Device Simulator | Built-in simulator for testing and development |

## Technology

- **Backend:** Go 1.22+ (stdlib `net/http` with method-based routing)
- **Storage:** SparkDB (standalone Go database, managed subprocess)
- **Auth:** JWT (HMAC-SHA256) + API keys
- **WebSocket:** `gorilla/websocket` for real-time telemetry and commands
