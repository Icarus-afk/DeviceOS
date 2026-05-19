package webhooks

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func TestWebhooks_ModuleBasics(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	if m.Name() != "webhooks" {
		t.Fatalf("expected webhooks, got %s", m.Name())
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestWebhooks_Init(t *testing.T) {
	var migrated bool
	m := &Module{db: &sparkdbtest.MockDB{
		OnMigrate: func(name, sql string) error {
			migrated = true
			return nil
		},
	}}
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Fatal("expected Migrate")
	}
}

func TestWebhooks_Init_Error(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnMigrate: func(name, sql string) error { return http.ErrAbortHandler },
	}}
	if err := m.Init(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Fatal("expected true")
	}
	if contains([]string{"a", "b", "c"}, "d") {
		t.Fatal("expected false")
	}
	if contains(nil, "a") {
		t.Fatal("expected false for nil slice")
	}
}

func TestWebhooks_HandleDelete_ExecErrorCheck(t *testing.T) {
	handlerCalled := false
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{Affected: 1}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/v1/webhooks/wh_1", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	_ = handlerCalled
}
