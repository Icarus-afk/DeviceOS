package fleet

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func TestFleet_CreateGroup_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/groups", bytes.NewReader([]byte(`{"name":"fleet-a"}`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var g Group
	if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	if g.Name != "fleet-a" {
		t.Fatalf("expected fleet-a, got %s", g.Name)
	}
}

func TestFleet_CreateGroup_MissingName(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/groups", bytes.NewReader([]byte(`{}`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestFleet_CreateGroup_BadJSON(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/groups", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestFleet_CreateGroup_ExecError(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/groups", bytes.NewReader([]byte(`{"name":"fleet-a"}`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestFleet_ListGroups_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{
				Rows: [][]interface{}{
					{"grp_1", "fleet-a", "2026-01-01T00:00:00Z"},
				},
			}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/groups", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]Group
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["groups"]) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result["groups"]))
	}
}

func TestFleet_ListGroups_QueryError(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/groups", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestFleet_ListGroups_Empty(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/groups", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestFleet_DeleteGroup_Success(t *testing.T) {
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
	r := httptest.NewRequest("DELETE", "/api/v1/groups/grp_1", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestFleet_DeleteGroup_NotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{Affected: 0}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/v1/groups/nonexistent", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestFleet_DeleteGroup_ExecError(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/v1/groups/grp_1", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestFleet_AddTags_Success(t *testing.T) {
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
	r := httptest.NewRequest("POST", "/api/v1/devices/dev_001/tags", bytes.NewReader([]byte(
		`{"tags":["fleet-a","critical"]}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFleet_AddTags_DeviceNotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{Affected: 0}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices/nonexistent/tags", bytes.NewReader([]byte(
		`{"tags":["fleet-a"]}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestFleet_AddTags_BadJSON(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices/dev_001/tags", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestFleet_AddTags_ExecError(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices/dev_001/tags", bytes.NewReader([]byte(
		`{"tags":["fleet-a"]}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestFleet_SetGroup_Success(t *testing.T) {
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
	r := httptest.NewRequest("PUT", "/api/v1/devices/dev_001/group", bytes.NewReader([]byte(
		`{"group":"fleet-a"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFleet_SetGroup_NotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{Affected: 0}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/devices/nonexistent/group", bytes.NewReader([]byte(
		`{"group":"fleet-a"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestFleet_SetGroup_BadJSON(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/devices/dev_001/group", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestFleet_SetGroup_ExecError(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/devices/dev_001/group", bytes.NewReader([]byte(
		`{"group":"fleet-a"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestFleet_Health_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{
				Row: []interface{}{10, 7, 3},
			}
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/fleet/health", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["total_devices"] != float64(10) {
		t.Fatalf("expected 10, got %v", result["total_devices"])
	}
	if result["online_devices"] != float64(7) {
		t.Fatalf("expected 7, got %v", result["online_devices"])
	}
}
