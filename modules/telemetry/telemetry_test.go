package telemetry

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func TestTelemetry_ModuleBasics(t *testing.T) {
	m := &Module{hub: NewHub()}
	if m.Name() != "telemetry" {
		t.Fatalf("expected telemetry, got %s", m.Name())
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetry_Init(t *testing.T) {
	var migrated bool
	m := &Module{
		db:  &sparkdbtest.MockDB{OnMigrate: func(name, sql string) error { migrated = true; return nil }},
		hub: NewHub(),
	}
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Fatal("expected Migrate")
	}
}

func TestTelemetry_Init_Error(t *testing.T) {
	m := &Module{
		db:  &sparkdbtest.MockDB{OnMigrate: func(name, sql string) error { return http.ErrAbortHandler }},
		hub: NewHub(),
	}
	if err := m.Init(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestHub_AddRemoveBroadcast(t *testing.T) {
	h := NewHub()
	if h == nil {
		t.Fatal("expected non-nil hub")
	}
	h.Broadcast([]byte("test")) // no clients, should not panic
}

func TestTelemetry_Ingest_CustomTimestamp(t *testing.T) {
	m := &Module{
		db:  &sparkdbtest.MockDB{OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) { return &sparkdbtest.MockResult{}, nil }},
		hub: NewHub(),
	}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/telemetry", bytes.NewReader([]byte(
		`{"device_id":"dev_001","metrics":{"t":1},"timestamp":"2026-05-14T12:00:00Z"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

var _ = http.ErrAbortHandler
var _ = httptest.NewRecorder
