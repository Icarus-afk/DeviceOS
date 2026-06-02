package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
)

func TestCommands_ModuleBasics(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{}}
	if m.Name() != "commands" {
		t.Fatalf("expected commands, got %s", m.Name())
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestCommands_Init(t *testing.T) {
	var migrated bool
	m := &Module{db: &dbtest.MockDB{
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

func TestCommands_Init_Error(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnMigrate: func(name, sql string) error { return http.ErrAbortHandler },
	}}
	if err := m.Init(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestCommands_Send_WithPayload(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (db.Result, error) {
			return &dbtest.MockResult{}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices/dev_001/commands", bytes.NewReader([]byte(
		`{"command":"update-fw","payload":{"version":"2.0"}}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCommands_Result_DefaultStatus(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (db.Result, error) {
			return &dbtest.MockResult{Affected: 1}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/commands/cmd_1/result", bytes.NewReader([]byte(
		`{"result":"ok"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
