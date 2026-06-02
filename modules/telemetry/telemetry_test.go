package telemetry

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
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
		db:  &dbtest.MockDB{OnMigrate: func(name, sql string) error { migrated = true; return nil }},
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
		db:  &dbtest.MockDB{OnMigrate: func(name, sql string) error { return http.ErrAbortHandler }},
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
		db:  &dbtest.MockDB{OnExec: func(sql string, args []interface{}) (db.Result, error) { return &dbtest.MockResult{}, nil }},
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

func TestPrune(t *testing.T) {
	m := &Module{
		db:            &dbtest.MockDB{},
		hub:           NewHub(),
		telemetryTTL:  0,
		pruneInterval: 0,
		stopCh:        make(chan struct{}),
	}
	m.prune() // should not panic or error with mock
}

func TestPruneLoopStops(t *testing.T) {
	m := &Module{
		db:            &dbtest.MockDB{},
		hub:           NewHub(),
		telemetryTTL:  time.Hour,
		pruneInterval: time.Second,
		stopCh:        make(chan struct{}),
	}
	go m.pruneLoop()
	time.Sleep(50 * time.Millisecond)
	m.Stop()
}

var _ = http.ErrAbortHandler
var _ = httptest.NewRecorder
