# Architecture

## Overview

DeviceOS is a single-binary IoT backend that manages a SparkDB database as a subprocess. All features are implemented as self-contained modules that register HTTP routes and database migrations.

```
┌────────────────────────────────────────────────────┐
│                   deviceos binary                   │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐          │
│  │ Devices  │  │Telemetry │  │  Alerts  │  ...      │
│  │ Module   │  │ Module   │  │  Module  │           │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘           │
│       │              │              │                │
│  ┌────┴──────────────┴──────────────┴────┐           │
│  │           Module Registry             │           │
│  │  InitAll → RegisterRoutes → StopAll   │           │
│  └────────────────┬──────────────────────┘           │
│                   │                                  │
│  ┌────────────────┴──────────────────────┐           │
│  │           SparkDB Client (DB)          │           │
│  │  Query / Exec / QueryRow / Transaction │           │
│  └────────────────┬──────────────────────┘           │
│                   │                                  │
│  ┌────────────────┴──────────────────────┐           │
│  │         SparkDB Server Manager         │           │
│  │  Start (subprocess) / Stop / Health    │           │
│  └────────────────┬──────────────────────┘           │
│                   │                                  │
│  ┌────────────────┴──────────────────────┐           │
│  │           HTTP Server (mux)            │           │
│  │  http.ListenAndServe + middleware      │           │
│  └────────────────────────────────────────┘           │
└──────────────────────────────────────────────────────┘
                      │
                      │ HTTP (localhost:9600)
                      ▼
┌────────────────────────────────────────────────────┐
│               SparkDB Server (subprocess)           │
│  SQL-over-HTTP engine with auth, rate limiting     │
└────────────────────────────────────────────────────┘
```

## Startup Sequence

```
main()
  │
  ├─ config.Load("deviceos.yaml")
  │   └─ parse YAML → apply env var overrides → defaults
  │
  ├─ sparkdb.NewServer(cfg)
  │   └─ generates sparkdb.json config from DeviceOS config + extra_config
  │
  ├─ sparkSrv.Start(ctx)
  │   ├─ findSparkDBBin() → search PATH, CWD, bin_path
  │   ├─ exec.CommandContext("sparkdb start --config <path>")
  │   ├─ poll /health every 500ms (up to 15s)
  │   └─ return when SparkDB is ready
  │
  ├─ sparkdb.Open(cfg)
  │   ├─ POST /auth/login (admin:admin) → get JWT
  │   └─ fallback: POST /auth/api-key → get JWT
  │
  ├─ registry.New()
  │
  ├─ r.Register(module) × 12 modules
  │
  ├─ r.InitAll(cfg)
  │   └─ each module.Init():
  │       ├─ db.Migrate("name", sql)
  │       │   ├─ CREATE TABLE IF NOT EXISTS _migrations
  │       │   ├─ SELECT COUNT(*) FROM _migrations WHERE name = ?
  │       │   ├─ if not applied: BEGIN → exec SQL → COMMIT
  │       │   └─ INSERT INTO _migrations
  │       └─ module-specific setup (e.g. create admin API key)
  │
  ├─ r.RegisterAllRoutes(mux)
  │   └─ each module registers HTTP handlers on the mux
  │
  ├─ srv.Start()  →  http.ListenAndServe (goroutine)
  │
  ├─ await SIGINT/SIGTERM
  │
  └─ shutdown:
      ├─ r.StopAll()       → modules clean up
      ├─ db.Close()        → close SparkDB connection
      └─ sparkSrv.Stop()   → SIGINT → wait 10s → SIGKILL
```

## Module Architecture

Each feature lives in its own Go package under `modules/`. Every module implements the `Module` interface:

```go
type Module interface {
    Name() string
    Init(cfg any) error
    RegisterRoutes(mux any) error
    Start() error
    Stop() error
}
```

Modules are registered in `cmd/deviceos/main.go`. Dropping an import line removes that feature entirely. Modules communicate via the registry or direct hook wiring (e.g., telemetry → alerts).

See [MODULES.md](MODULES.md) for detailed module documentation.

## SparkDB Integration

DeviceOS treats SparkDB as a remote database accessed via HTTP REST. Unlike SQLite (embedded), SparkDB runs as a separate process.

### Request flow

```
Module → db.Query(SQL, args...)
         → throttle()       [1.3s gap enforcement]
         → POST /query      [HTTP to localhost:9600]
         → SparkDB parses SQL → executes → returns JSON
         → DeviceOS scans JSON into Go types
```

### Transaction flow

```
Module → db.Begin()
         → tx.Exec(SQL)     [buffered, no HTTP call]
         → tx.Exec(SQL)     [buffered]
         → tx.Commit()
         → POST /transaction [batch of all SQL in one HTTP call]
```

### Rate limiting

SparkDB enforces 60 req/min per user/IP. DeviceOS's client enforces a 1.3s gap (`minRequestGap`) between queries to stay under this limit. Transaction commits also count toward the limit.

### Authentication to SparkDB

1. DeviceOS tries `POST /auth/login` with `admin`/`admin`
2. If that fails, falls back to `POST /auth/api-key` with configured API key
3. All subsequent queries use the obtained JWT as `Authorization: Bearer <token>`

## HTTP Middleware

```
Request
  │
  ├─ withMiddleware: Content-Type, X-DeviceOS-Version, request logging
  │
  ├─ If path starts with /api/v1/auth/, /api/v1/ws/, /healthz, /dashboard:
  │   └─ pass through (no auth)
  │
  └─ Otherwise:
      └─ auth.Module.Middleware:
          ├─ Bearer <token> → validate JWT → set X-User-Role, X-User-Subject
          └─ ApiKey <key>   → lookup in api_keys table → set X-User-Role
```

## Data Flow: Telemetry → Alerts

```
POST /api/v1/telemetry
  → telemetry.handleIngest
    → storeTelemetry()        [INSERT + UPDATE device status]
    → telemetryMod.OnTelemetry (hook, if set)
      → alertsMod.OnTelemetry
        → loadEnabledRules()  [SELECT enabled rules]
        → for each rule:
            evaluate metrics against threshold
            if triggered: INSERT alert_event
```

## WebSocket

Two WebSocket endpoints for real-time communication:

| Endpoint | Protocol | Purpose |
|---|---|---|
| `GET /api/v1/ws/telemetry` | Any client | Broadcasts telemetry datapoints in real-time |
| `GET /api/v1/ws/commands?device_id=X` | Device | Streams pending commands to specific device |

WebSocket connections are authenticated via a token in the query string or validated via device identity.
