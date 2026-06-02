package alerts

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
)

func TestAlerts_ModuleBasics(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{}}
	if m.Name() != "alerts" {
		t.Fatalf("expected alerts, got %s", m.Name())
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestAlerts_Init(t *testing.T) {
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

func TestAlerts_Init_Error(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnMigrate: func(name, sql string) error { return http.ErrAbortHandler },
	}}
	if err := m.Init(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestAlerts_OnTelemetry_WithMatchingRule(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (db.RowsInterface, error) {
			return &dbtest.MockRows{
				Rows: [][]interface{}{
					{"rule_1", "high-temp", "temperature", ">", 25.0, "5m", "log", "", true, "2026-01-01T00:00:00Z"},
				},
			}, nil
		},
		OnExec: func(sql string, args []interface{}) (db.Result, error) {
			return &dbtest.MockResult{}, nil
		},
	}}
	m.OnTelemetry("dev_001", json.RawMessage(`{"temperature":30}`), json.RawMessage(`{}`))
}

func TestAlerts_OnTelemetry_NoMatch(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (db.RowsInterface, error) {
			return &dbtest.MockRows{
				Rows: [][]interface{}{
					{"rule_1", "high-temp", "temperature", ">", 50.0, "5m", "log", "", true, "2026-01-01T00:00:00Z"},
				},
			}, nil
		},
	}}
	m.OnTelemetry("dev_001", json.RawMessage(`{"temperature":30}`), json.RawMessage(`{}`))
}

func TestAlerts_OnTelemetry_BadMetrics(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{}}
	m.OnTelemetry("dev_001", json.RawMessage(`bad`), json.RawMessage(`{}`))
}
