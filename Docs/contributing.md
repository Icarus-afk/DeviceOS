# Contributing to DeviceOS

## Code of Conduct

Be respectful, constructive, and professional. We do not tolerate harassment, discrimination, or toxic behavior.

## Getting Started

1. Fork the repository
2. Run `make build` to verify your development environment
3. Run `make test` to verify all tests pass

## Development Workflow

### 1. Pick an Area

DeviceOS is organized into self-contained modules. Each module lives in `modules/<name>/` and implements the `registry.Module` interface. See [`Docs/development.md`](development.md) for the full module specification.

Common contribution areas:
- **New modules**: A new protocol adapter, storage backend, or integration
- **Existing modules**: Bug fixes, feature additions, test coverage
- **Core**: Performance improvements, security hardening, documentation

### 2. Make Changes

- Follow existing code patterns (same style, same test patterns)
- Use `internal/httperr` for all HTTP error responses
- Use `internal/dbtest.MockDB` for unit test database mocking
- Use `db.DBClient` interface for all database access (never `database/sql` directly)
- Add or update `org_id` scoping when adding multi-tenant data

### 3. Write Tests

- Unit tests go in the module package (e.g. `modules/foo/foo_test.go`)
- Tests must not depend on external services (no network, no database server)
- Use `net/http/httptest` for HTTP handler tests
- Use `dbtest.MockDB` for database mocking
- Integration tests go in `tests/integration/`
- Run `make test` before committing

### 4. Run Lint and Format

```bash
make fmt
make lint   # requires golangci-lint
```

The project enforces:
- `go fmt` formatting (no exceptions)
- No lint warnings
- No commented-out code
- No `TODO` or `FIXME` in committed code

### 5. Commit

- Write clear, concise commit messages
- Use conventional commit prefixes: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`
- Keep commits focused on a single concern
- Each commit should compile and pass tests independently when possible

Example commit messages:
```
feat(mqtt): add TLS support for embedded broker
fix(alerts): handle empty telemetry payload gracefully
docs(api): add curl examples for OTA deployment
refactor(db): extract migration logic into separate type
```

### 6. Submit a Pull Request

- Open a PR against `main`
- Describe what the change does and why
- Reference any related issues
- Ensure CI passes (all tests, lint, build)

## Code Style

### Imports

Group standard library, third-party, and internal imports:

```go
import (
    "context"
    "net/http"

    "github.com/gorilla/websocket"

    "github.com/lohtbrok/deviceos/internal/db"
    "github.com/lohtbrok/deviceos/internal/httperr"
)
```

### Error Handling

```go
// Good — structured, typed error
httperr.BadRequest(w, "device name is required")

// Bad — raw HTTP error
http.Error(w, "bad request", http.StatusBadRequest)
```

### Naming

- Use short, descriptive names (`db` not `database`)
- Receiver names: single letter (`m *Module`, `s *Server`)
- Test functions: `TestHandlerName` with `t *testing.T`
- Table-driven tests for multiple cases: `TestHandlerName/tc.name`

### No Comments

Code should be self-documenting through clear naming, small functions, and consistent patterns. Do not add explanatory comments. If something is unclear, refactor it.

## Testing Guidelines

### Unit Tests

- Test each handler in isolation with mocked database
- Test success and error paths
- Use table-driven tests for multiple cases

```go
func TestCreateDevice(t *testing.T) {
    tests := []struct {
        name       string
        body       string
        wantStatus int
    }{
        {"valid device", `{"name":"sensor-001"}`, http.StatusCreated},
        {"empty name", `{"name":""}`, http.StatusBadRequest},
        {"missing body", ``, http.StatusBadRequest},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            // ...
        })
    }
}
```

### Integration Tests

Integration tests in `tests/integration/` start a real DeviceOS server with a temporary SQLite database. They test end-to-end flows across multiple modules.

```bash
make test-integration
```

## Module Lifecycle

When adding a new module, ensure these lifecycle methods are implemented:

| Method | Called | Purpose |
|--------|--------|---------|
| `Name()` | Registration | Unique module identifier |
| `Init(any)` | Startup | Run migrations, initialize state |
| `RegisterRoutes(any)` | Startup | Register HTTP handlers |
| `Start()` | After init | Start background goroutines |
| `Stop()` | Shutdown | Clean shutdown of resources |

## Release Process

1. Update version in `internal/version/version.go`
2. Update `Docs/openapi.yaml` version field
3. Run full test suite: `make test && make test-integration`
4. Build release binaries: `make release`
5. Tag with version: `git tag v0.1.0`
6. Push tag: `git push origin v0.1.0`
