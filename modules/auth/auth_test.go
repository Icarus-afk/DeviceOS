package auth

import (
	"net/http"
	"testing"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
)

func TestAuth_Init(t *testing.T) {
	migrated := false
	execd := false
	m := &Module{
		db: &dbtest.MockDB{
			OnMigrate: func(name, sql string) error {
				migrated = true
				return nil
			},
			OnExec: func(sql string, args []interface{}) (db.Result, error) {
				execd = true
				return &dbtest.MockResult{}, nil
			},
		},
		jwtSecret:  "test-secret",
		adminToken: "admin-token",
	}
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Fatal("expected Migrate to be called")
	}
	if !execd {
		t.Fatal("expected Exec (insert api key) to be called")
	}
}

func TestAuth_Init_Defaults(t *testing.T) {
	m := &Module{
		db: &dbtest.MockDB{
			OnMigrate: func(name, sql string) error { return nil },
			OnExec:    func(sql string, args []interface{}) (db.Result, error) { return &dbtest.MockResult{}, nil },
		},
	}
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
}

func TestAuth_New_Defaults(t *testing.T) {
	m := New(&dbtest.MockDB{}, "", "")
	if m.jwtSecret == "" {
		t.Fatal("expected default jwt secret")
	}
	if m.adminToken == "" {
		t.Fatal("expected generated admin token")
	}
}

func TestAuth_ModuleBasics(t *testing.T) {
	m := &Module{jwtSecret: "test"}
	if m.Name() != "auth" {
		t.Fatalf("expected auth, got %s", m.Name())
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestAuth_Init_MigrateError(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnMigrate: func(name, sql string) error { return http.ErrAbortHandler },
	}}
	if err := m.Init(nil); err == nil {
		t.Fatal("expected error")
	}
}

var _ = http.ErrAbortHandler
