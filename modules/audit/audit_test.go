package audit

import (
	"encoding/json"
	"testing"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
)

func TestAudit_Log(t *testing.T) {
	var execd bool
	m := &Module{db: &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (db.Result, error) {
			execd = true
			return nil, nil
		},
		OnMigrate: func(name, sql string) error { return nil },
	}}

	m.Log("admin", "create_device", "dev_001", json.RawMessage(`{"reason":"test"}`))
	if !execd {
		t.Fatal("expected Exec to be called")
	}
}

func TestAudit_Init(t *testing.T) {
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
		t.Fatal("expected Migrate to be called")
	}
}
