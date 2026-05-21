package audit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func TestAudit_Query_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{
				Rows: [][]interface{}{
					{"aud_1", "admin", "create_device", "dev_001", `{}`, "2026-01-01T00:00:00Z"},
				},
			}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/audit", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]Entry
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["audit_logs"]) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result["audit_logs"]))
	}
}

func TestAudit_Query_Error(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/audit", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestAudit_Query_Empty(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/audit", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
