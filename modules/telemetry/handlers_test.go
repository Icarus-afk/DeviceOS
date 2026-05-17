package telemetry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func TestTelemetry_Ingest_Success(t *testing.T) {
	var execd bool
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			execd = true
			return &sparkdbtest.MockResult{}, nil
		},
	}, hub: NewHub()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/telemetry", bytes.NewReader([]byte(
		`{"device_id":"dev_001","metrics":{"temp":25.5}}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !execd {
		t.Fatal("expected Exec to be called")
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp["id"]; !ok {
		t.Fatal("expected id in response")
	}
}

func TestTelemetry_Ingest_MissingDeviceID(t *testing.T) {
	m := &Module{hub: NewHub()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/telemetry", bytes.NewReader([]byte(
		`{"metrics":{"temp":25.5}}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTelemetry_Ingest_BadJSON(t *testing.T) {
	m := &Module{hub: NewHub()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/telemetry", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTelemetry_Ingest_WithHook(t *testing.T) {
	var hookCalled bool
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{}, nil
		},
	}, hub: NewHub()}
	m.SetTelemetryHook(func(deviceID string, metrics, metadata json.RawMessage) {
		hookCalled = true
	})

	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/telemetry", bytes.NewReader([]byte(
		`{"device_id":"dev_001","metrics":{"temp":25.5}}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if !hookCalled {
		t.Fatal("expected telemetry hook to be called")
	}
}

func TestTelemetry_Ingest_HookNil(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{}, nil
		},
	}, hub: NewHub()}

	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/telemetry", bytes.NewReader([]byte(
		`{"device_id":"dev_001","metrics":{"temp":25.5}}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestTelemetry_Ingest_ExecError(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return nil, http.ErrAbortHandler
		},
	}, hub: NewHub()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/telemetry", bytes.NewReader([]byte(
		`{"device_id":"dev_001","metrics":{"temp":25.5}}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestTelemetry_Query_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{
				Rows: [][]interface{}{
					{int64(1), "dev_001", "2026-01-01T00:00:00Z", `{"temp":25.5}`, `{}`},
				},
			}, nil
		},
	}, hub: NewHub()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/telemetry?device_id=dev_001", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]Telemetry
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["telemetry"]) != 1 {
		t.Fatalf("expected 1 telemetry entry, got %d", len(result["telemetry"]))
	}
}

func TestTelemetry_Query_Error(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return nil, http.ErrAbortHandler
		},
	}, hub: NewHub()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/telemetry?device_id=dev_001", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestTelemetry_Query_Empty(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{}, nil
		},
	}, hub: NewHub()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/telemetry?device_id=dev_001", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestTelemetry_Latest_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{
				Row: []interface{}{int64(1), "dev_001", "2026-01-01T00:00:00Z", `{"temp":25.5}`, `{}`},
			}
		},
	}, hub: NewHub()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/telemetry/latest?device_id=dev_001", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTelemetry_Latest_NotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{Err: http.ErrNoLocation}
		},
	}, hub: NewHub()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/telemetry/latest?device_id=dev_001", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
