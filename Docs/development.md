# DeviceOS Development Guide

## Prerequisites

- Go 1.25 or later
- No CGO required -- build with `CGO_ENABLED=0`

```bash
# Verify Go version
go version

# Build the DeviceOS binary
go build ./cmd/deviceos

# Or use the Makefile
make build
```

## Project Structure

```
cmd/
  deviceos/          Main binary (start, init, backup, restore, version, status)
  simload/           Load testing tool
  demo/              Demo setup script
internal/
  config/            YAML config loading with env overrides
  crypto/            AES-256-GCM encryption helpers
  ctxutil/           Context utilities (org ID, role, subject extraction)
  db/                SQLite wrapper, migration system, DBClient interface
  dbtest/            Mock database for unit testing
  httperr/           Structured JSON error responses
  registry/          Module registry and lifecycle management
  server/            HTTP server with middleware chain
  version/           Build-time version/commit information
modules/
  auth/              Login, JWT issuance, API key auth, middleware
  devices/           Device CRUD and registration
  telemetry/         Telemetry ingest, query, WebSocket, retention pruning
  commands/          Downlink commands to devices
  ota/               Firmware upload, staged deploy, rollback
  alerts/            Alert rule engine, condition evaluation
  webhooks/          Outgoing webhook management
  fleet/             Device groups, tags, fleet health monitoring
  audit/             Action logging
  dashboard/         Static dashboard UI (embedded HTML)
  tenant/            Multi-org management
  mqtt/              Embedded MQTT broker (mochi-mqtt)
  events/            Typed realtime event hub (WebSocket)
  simulator/         Device simulation for testing
tests/
  integration/       Integration tests
```

## Makefile Targets

| Target | Command | Description |
|---|---|---|
| `build` | `go build -o bin/DeviceOS ./cmd/deviceos` | Build the binary |
| `run` | `./bin/DeviceOS start` | Build and run |
| `run-dev` | `go run ./cmd/deviceos start` | Run directly |
| `init` | `go run ./cmd/deviceos init` | Scaffold config |
| `install` | `cp bin/DeviceOS ~/.local/bin/` | Install to PATH |
| `test` | `go test ./... -count=1` | Run all unit tests |
| `test-integration` | `go test -tags=integration ./tests/integration/` | Run integration tests |
| `lint` | `golangci-lint run ./...` | Run linter |
| `fmt` | `go fmt ./...` | Format code |
| `clean` | `rm -rf bin/ data/` | Remove build artifacts |
| `docker-build` | `docker build -t DeviceOS:latest .` | Build Docker image |
| `docker-run` | `docker compose up -d` | Run Docker Compose |
| `release` | Cross-compile for linux/amd64 and linux/arm64 | Build release binaries |

## Adding a New Module

### Step 1: Create the Module Directory

```bash
mkdir modules/foo
```

### Step 2: Implement the Module Interface

Create `modules/foo/module.go`:

```go
package foo

import (
    "fmt"
    "log/slog"
    "net/http"

    "github.com/lohtbrok/deviceos/internal/db"
    "github.com/lohtbrok/deviceos/internal/httperr"
)

type Module struct {
    db db.DBClient
}

func New(db db.DBClient) *Module {
    return &Module{db: db}
}

func (m *Module) Name() string { return "foo" }

func (m *Module) Init(cfg any) error {
    if err := m.db.Migrate("foo_v1", `CREATE TABLE IF NOT EXISTS foo (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        created_at TEXT DEFAULT (datetime('now'))
    )`); err != nil {
        return fmt.Errorf("foo: migrate: %w", err)
    }
    slog.Info("foo module initialized")
    return nil
}

func (m *Module) RegisterRoutes(mux any) error {
    r, ok := mux.(*http.ServeMux)
    if !ok {
        return fmt.Errorf("foo: unexpected mux type")
    }
    r.HandleFunc("GET /api/v1/foo", m.handleList)
    r.HandleFunc("POST /api/v1/foo", m.handleCreate)
    return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

func (m *Module) handleList(w http.ResponseWriter, r *http.Request) {
    rows, err := m.db.Query(`SELECT id, name, created_at FROM foo WHERE org_id = ?`, orgID(r))
    if err != nil {
        httperr.Internal(w, "query failed")
        return
    }
    defer rows.Close()
    // ... scan and respond
}

func (m *Module) handleCreate(w http.ResponseWriter, r *http.Request) {
    // ... decode, validate, insert
}

func orgID(r *http.Request) string {
    return r.Header.Get("X-Org-ID")
}
```

### Step 3: Add Migrations

Call `m.db.Migrate(name, sql)` in `Init()`. Use the same name-based idempotency pattern as all existing modules (e.g. `"foo_v1"`, `"foo_v2_bar"`).

```go
func (m *Module) Init(cfg any) error {
    if err := m.db.Migrate("foo_v1", `CREATE TABLE IF NOT EXISTS foo (...)`); err != nil {
        return err
    }
    // Additional versioned migrations as features evolve:
    if err := m.db.Migrate("foo_v2_org", `ALTER TABLE foo ADD COLUMN org_id TEXT`); err != nil {
        return err
    }
    return nil
}
```

**Never modify an existing migration SQL string.** Create a new named migration instead.

### Step 4: Register in `cmd/deviceos/start.go`

```go
import "github.com/lohtbrok/deviceos/modules/foo"

// In cmdStart():
r.Register(foo.New(database))
```

The module is now enabled. To remove it, delete the import and the `r.Register(...)` line.

### Step 5: Write Tests

```go
// modules/foo/foo_test.go
package foo

import (
    "testing"
    "github.com/lohtbrok/deviceos/internal/dbtest"
)

func TestFooModule(t *testing.T) {
    mockDB := &dbtest.MockDB{}
    m := New(mockDB)
    // ... test handlers using httptest.NewRequest / httptest.NewRecorder
}
```

## Conventions

### Code Style

- Format with `go fmt ./...` before committing
- `golangci-lint` is configured in `.golangci.yml` with gofmt, errcheck, gosimple, govet, ineffassign, staticcheck, unconvert, and unused
- **No comments in code.** Business logic should be self-documenting through clear function and variable names

### Error Handling

Use `internal/httperr` for structured HTTP error responses:

```go
httperr.BadRequest(w, "description")
httperr.NotFound(w, "description")
httperr.Internal(w, "description")
httperr.Unauthorized(w, "description")
httperr.Forbidden(w, "description")
httperr.Conflict(w, "description")
```

All errors are returned as JSON:

```json
{"error": {"code": "bad_request", "message": "description"}}
```

### Multi-Tenancy

Every table that should be tenant-scoped includes an `org_id TEXT` column. Extract the org ID from the request:

```go
func orgID(r *http.Request) string {
    return r.Header.Get("X-Org-ID")
}
```

The `internal/ctxutil` package provides convenience functions:

```go
orgID := ctxutil.OrgID(r)
role := ctxutil.Role(r)
subject := ctxutil.Subject(r)
```

### Database Access

Always access the database through the `db.DBClient` interface, never directly through `database/sql`. This allows unit testing with `dbtest.MockDB`.

```go
type DBClient interface {
    Exec(sql string, args ...interface{}) (Result, error)
    Query(sql string, args ...interface{}) (RowsInterface, error)
    QueryRow(sql string, args ...interface{}) RowInterface
    Migrate(name, sql string) error
}
```

### Module Communication

- **Telemetry hooks**: Use `telemetryMod.AddTelemetryHook(fn)` to subscribe to telemetry events
- **Events hub**: Use `eventsMod.Hub().Publish(event)` to broadcast typed events over WebSocket
- **Direct database**: Modules can query any table, but should prefer to use module-specific storage interfaces where possible

## Testing

### Unit Tests

Run all tests:

```bash
make test
# or
go test ./... -count=1
```

Use `dbtest.MockDB` to mock database interactions:

```go
mockDB := &dbtest.MockDB{
    OnExec: func(sql string, args []interface{}) (db.Result, error) {
        return &dbtest.MockResult{LastID: 1, Affected: 1}, nil
    },
    OnQuery: func(sql string, args []interface{}) (db.RowsInterface, error) {
        return &dbtest.MockRows{
            Columns: []string{"id", "name"},
            Rows:    [][]interface{}{{"1", "test"}},
        }, nil
    },
}
```

Test HTTP handlers with `net/http/httptest`:

```go
func TestListHandler(t *testing.T) {
    mockDB := &dbtest.MockDB{/* ... */}
    m := New(mockDB)

    req := httptest.NewRequest("GET", "/api/v1/foo", nil)
    rec := httptest.NewRecorder()
    m.handleList(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
}
```

### Integration Tests

```bash
make test-integration
# or
go test -tags=integration -count=1 -timeout=180s ./tests/integration/
```

Integration tests in `tests/integration/` start a full DeviceOS server with a temporary database and test end-to-end flows.

## Migration System

The migration system supports two modes:

### Simple Name-Based (Preferred)

```go
m.db.Migrate("module_v1", `CREATE TABLE IF NOT EXISTS ...`)
m.db.Migrate("module_v2_description", `ALTER TABLE ...`)
```

Each call to `Migrate` is idempotent -- if the migration name exists in `_migrations`, it is skipped.

### Versioned Migrator (For Advanced Use)

```go
migrator := m.db.NewMigrator()
migrator.Add(db.Migration{
    Version: 1,
    Name:    "create_foo",
    Up:      `CREATE TABLE foo (...)` ,
    Down:    `DROP TABLE foo`,
})
migrator.Add(db.Migration{
    Version: 2,
    Name:    "add_bar_column",
    Up:      `ALTER TABLE foo ADD COLUMN bar TEXT`,
    Down:    `ALTER TABLE foo DROP COLUMN bar`,  -- SQLite does not support DROP COLUMN, handle in Down as needed
})
migrator.Up()
```

**Never modify existing migrations.** Always append new ones.

## Build Targets

```bash
make build            # Build for current platform
make docker-build     # Build Docker image
make release          # Cross-compile for linux/amd64 and linux/arm64
```

Release binaries are placed in `dist/` with SHA256 checksums:

```
dist/deviceos-<version>-linux-amd64
dist/deviceos-<version>-linux-arm64
dist/checksums.txt
```

LDFlags inject version and commit information at build time:

```
-ldflags "-X github.com/lohtbrok/deviceos/internal/version.Version=<version>
          -X github.com/lohtbrok/deviceos/internal/version.Commit=<commit>"
```
