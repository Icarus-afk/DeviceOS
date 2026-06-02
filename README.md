# DeviceOS

Self-hosted IoT backend — single binary, zero external dependencies.

DeviceOS manages device authentication, telemetry ingestion, real-time dashboards, OTA firmware updates, alerting, fleet management, and multi-tenancy. Embedded SQLite storage — no separate database needed.

## Quick Start

```bash
# Build
make build

# Init default config
make init

# Start
make run
# Server listening on http://0.0.0.0:8080
```

## Documentation

| File | Contents |
|------|----------|
| [Docs/architecture.md](Docs/architecture.md) | System architecture |
| [Docs/configuration.md](Docs/configuration.md) | All config options (YAML + env vars) |
| [Docs/deployment.md](Docs/deployment.md) | Deployment guide |
| [Docs/development.md](Docs/development.md) | Module development guide |
| [Docs/openapi.yaml](Docs/openapi.yaml) | API specification |

## Feature Overview

| Feature | Description |
|---------|-------------|
| Device Auth | Register devices, issue JWT tokens, API key authentication |
| Telemetry | Ingest metrics via HTTP, MQTT or WebSocket, query historical/latest data |
| Real-time Events | Typed WebSocket event stream with per-client filtering |
| OTA Firmware | Upload firmware, deploy to device groups, track rollout status |
| Alerting | Define rules on metric thresholds, get notified via webhooks |
| Fleet Management | Organize devices into groups, tag devices, fleet health overview |
| Commands | Send commands to devices, track execution results |
| Webhooks | Outbound HTTP hooks on device events |
| Multi-tenancy | Organizations with users and role-based access |
| Audit Log | Track admin actions across the system |
| Device Simulator | Built-in simulator for testing and development |

## Technology

- **Backend:** Go 1.25+ (stdlib `net/http` with method-based routing)
- **Storage:** SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Auth:** JWT (HMAC-SHA256) + API keys
- **WebSocket:** `gorilla/websocket` for real-time telemetry and events
- **MQTT:** Embedded broker via `mochi-mqtt` (pure Go, no CGO)
