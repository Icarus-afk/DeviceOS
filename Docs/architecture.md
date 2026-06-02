# DeviceOS Architecture Overview

## Single-Binary Philosophy

DeviceOS ships as a single Go binary with zero runtime dependencies. One process handles HTTP, WebSocket, MQTT, telemetry ingestion, alert evaluation, and the dashboard. This makes deployment trivial -- copy one file to a server, run `deviceos start`, and the entire IoT backend is operational.

The database is SQLite via `modernc.org/sqlite` -- a pure Go SQLite implementation that requires no CGO, no shared libraries, and no external database process. Everything lives in one binary.

## Module System

Every feature is a self-contained module that implements the `registry.Module` interface:

```go
type Module interface {
    Name() string
    Init(cfg any) error
    RegisterRoutes(mux any) error
    Start() error
    Stop() error
}
```

- `Name()` -- unique module identifier (e.g. "devices", "telemetry")
- `Init(cfg any)` -- run migrations, set up internal state. Receives the full `*config.Config`
- `RegisterRoutes(mux any)` -- register HTTP handlers on the server mux
- `Start()` -- start background goroutines (pruning, MQTT broker, simulator loop)
- `Stop()` -- clean shutdown of goroutines and resources

Modules are registered in `cmd/deviceos/start.go`. Importing a module and calling `r.Register(...)` enables it. Deleting that import line removes the feature entirely -- no dead code, no conditional flags, no configuration toggles. The module pattern makes DeviceOS extensible without modifying core code.

Modules communicate through:
- **Direct calls** -- e.g. telemetry module exposes `AddTelemetryHook()` so alerts and events modules can subscribe to telemetry streams
- **Events hub** -- the events module provides a typed publish/subscribe WebSocket hub for realtime event distribution
- **Shared database** -- all modules access the same SQLite database through the `db.DBClient` interface

## Core vs Modules

### Core (`internal/`)

| Package | Responsibility |
|---|---|
| `internal/config` | YAML config loading with environment variable overrides |
| `internal/server` | HTTP server with middleware chain (CORS, rate limit, auth, request ID, logging) |
| `internal/db` | SQLite wrapper, connection pool, migration system, `DBClient` interface |
| `internal/registry` | Module registry -- registration, lifecycle (Init, Start, Stop) |
| `internal/httperr` | Structured JSON error responses |
| `internal/crypto` | AES-256-GCM encryption helpers |
| `internal/ctxutil` | Context/request utilities (org ID, role, subject extraction) |
| `internal/version` | Build-time version/commit injection |
| `internal/dbtest` | Mock DB for unit testing |

### Modules (`modules/`)

| Module | Routes | Purpose |
|---|---|---|
| `auth` | `POST /api/v1/auth/login`, `POST /api/v1/auth/token` | JWT issuance, API key auth, middleware |
| `devices` | `POST/GET/PUT/DELETE /api/v1/devices` | Device CRUD, registration |
| `telemetry` | `POST/GET /api/v1/telemetry`, `GET /api/v1/ws/telemetry` | Telemetry ingest, query, WebSocket streaming, retention |
| `commands` | `POST/GET /api/v1/devices/{id}/commands`, `GET /api/v1/ws/commands` | Downlink commands to devices |
| `ota` | `POST/GET /api/v1/firmware`, `POST .../deploy`, `GET .../deployments/{id}` | Firmware upload, staged deploy, rollback |
| `alerts` | `POST/GET/PUT/DELETE /api/v1/alerts/rules`, `GET .../history` | Rule engine, condition evaluation |
| `webhooks` | `POST/GET/PUT/DELETE /api/v1/webhooks` | Outgoing webhook management |
| `fleet` | `POST/GET/DELETE /api/v1/groups`, `.../tags`, `.../group`, `.../fleet/health` | Groups, tags, health monitoring |
| `audit` | `GET /api/v1/audit` | Action logging |
| `dashboard` | `GET /dashboard`, `GET /` | Static dashboard UI (embedded HTML) |
| `tenant` | `POST/GET /api/v1/orgs`, `POST/GET .../orgs/{id}/users` | Multi-org management |
| `mqtt` | `GET /api/v1/mqtt/status` | Embedded MQTT broker (port 1883) |
| `events` | `GET /api/v1/ws/events` | Typed realtime event hub |
| `simulator` | `POST /api/v1/simulator/start`, `POST .../stop` | Device simulation for testing |

## Data Flow

```
Device/Client
    |
    v
HTTP/WS/MQTT request
    |
    v
Server middleware chain (outer to inner):
  1. Request ID injection (X-Request-ID)
  2. Logging (method, path, status, duration)
  3. Version header (X-DeviceOS-Version)
  4. Auth middleware (JWT Bearer or ApiKey validation)
  5. Rate limiter (token bucket per IP)
  6. CORS (origin validation, preflight handling)
    |
    v
http.ServeMux path matching
    |
    v
Module handler (e.g. telemetry.handleIngest)
    |
    v
db.DBClient interface (SQLite via modernc.org/sqlite)
```

For authenticated routes, the auth middleware injects `X-User-Role`, `X-User-Subject`, `X-Device-ID`, and `X-Org-ID` headers into the request context after validating the JWT or API key. Public routes bypass auth (healthz, login, WebSocket telemetry/commands endpoints).

## Storage

### SQLite via modernc.org/sqlite

- Pure Go implementation -- no CGO, no system SQLite library
- WAL journal mode with `synchronous=NORMAL` for read concurrency
- `busy_timeout=5000ms` to handle concurrent writes
- `foreign_keys=ON` for referential integrity
- `cache_size=-8192` (8MB page cache) and `temp_store=MEMORY`

### Migration System

Each module calls `db.Migrate(name, sql)` in its `Init()` method. The migration system:

1. Creates a `_migrations` tracking table if it does not exist
2. Checks if the migration name has already been applied
3. If not, runs the SQL in a transaction and records the migration
4. Never modifies existing migrations -- additions only

Migrations use a simple name-based idempotency key (e.g. `"telemetry_v1"`, `"telemetry_v2_org"`). The `Migrator` type also supports versioned migrations with up/down rollback.

### Multi-Tenancy

Tenant isolation uses an `org_id` column pattern. Every multi-tenant table includes an `org_id TEXT` column. Module handlers extract the org ID from the `X-Org-ID` request header and filter queries accordingly. When `X-Org-ID` is empty, the request operates in a single-tenant context.

## Real-Time Communication

### WebSocket Hub (telemetry module)

The telemetry module maintains a WebSocket hub. When telemetry is ingested, it broadcasts the data to all connected WebSocket clients. This provides real-time dashboard updates without polling.

### Events Hub (events module)

The events module provides a typed publish/subscribe hub over WebSocket (`/api/v1/ws/events`). Clients can filter by event type using the `?events=type1,type2` query parameter. The hub supports:
- `telemetry` events (published by telemetry module hooks)
- Arbitrary event types published by other modules or external systems
- 30-second ping keepalive

### MQTT Broker (mqtt module)

An embedded MQTT broker (via `mochi-mqtt`) runs on port 1883. Devices authenticate using their device ID and secret key (standard MQTT username/password). Devices publish telemetry to `deviceos/{device_id}/telemetry`. The broker hooks into the telemetry storage pipeline, providing an alternative to HTTP ingestion.

## Security

| Layer | Mechanism |
|---|---|
| Authentication | JWT (HS256) for user and device tokens, API keys for admin access |
| Auth middleware | Supports `Authorization: Bearer <jwt>` and `Authorization: ApiKey <key>` |
| TLS | Optional server-side TLS with minimum TLS 1.2 |
| CORS | Configurable allowed origins, preflight handling |
| Rate limiting | Token-bucket per IP, configurable RPM |
| Request tracing | `X-Request-ID` header on every request |
| Encryption | AES-256-GCM in `internal/crypto` for sensitive field encryption |

## Key Design Decisions

### Why Go?

- Single-binary deployment without a runtime dependency (contrast: Python, Node.js)
- Excellent concurrency primitives for handling thousands of device connections
- Fast compilation and small binary size
- Strong standard library -- no framework dependency
- Cross-compilation to ARM for edge deployment

### Why SQLite?

- Zero operational overhead -- no database server to install, configure, or tune
- WAL mode provides sufficient read concurrency for IoT workloads (thousands of devices)
- Single file makes backup trivial (`deviceos backup`)
- `modernc.org/sqlite` eliminates CGO dependency
- DeviceOS targets fleet sizes where SQLite is appropriate (thousands, not millions of devices)

### Why a Module System?

- Features can be added independently without touching core
- Import-line registration means zero configuration overhead -- either the feature is compiled in or it is not
- Modules are self-contained testable units with their own migrations
- External contributors can add modules without deep knowledge of the entire codebase
- The registry interface provides a clean lifecycle contract

### Why an Embedded MQTT Broker Instead of External Dependency?

- Eliminates the operational complexity of running and maintaining a separate Mosquitto/EMQX instance
- Single-binary philosophy extends to messaging
- The embedded broker (mochi-mqtt) is production-grade and handles thousands of concurrent connections
- Tight integration with telemetry storage via Go hooks instead of bridge scripts

