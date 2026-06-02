package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
)

func TestCommands_Send_Success(t *testing.T) {
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
		`{"command":"reboot"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var cmd Command
	if err := json.Unmarshal(w.Body.Bytes(), &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Command != "reboot" {
		t.Fatalf("expected reboot, got %s", cmd.Command)
	}
	if cmd.Status != "pending" {
		t.Fatalf("expected pending, got %s", cmd.Status)
	}
}

func TestCommands_Send_MissingCommand(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices/dev_001/commands", bytes.NewReader([]byte(`{}`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCommands_Send_BadJSON(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices/dev_001/commands", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCommands_Send_ExecError(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (db.Result, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices/dev_001/commands", bytes.NewReader([]byte(
		`{"command":"reboot"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCommands_List_Success(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (db.RowsInterface, error) {
			return &dbtest.MockRows{
				Rows: [][]interface{}{
					{"cmd_1", "dev_001", "reboot", `{}`, "pending", "", "2026-01-01T00:00:00Z", nil},
				},
			}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/devices/dev_001/commands", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]Command
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["commands"]) != 1 {
		t.Fatalf("expected 1 command, got %d", len(result["commands"]))
	}
}

func TestCommands_List_QueryError(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (db.RowsInterface, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/devices/dev_001/commands", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCommands_List_Empty(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (db.RowsInterface, error) {
			return &dbtest.MockRows{}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/devices/dev_001/commands", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCommands_Get_Success(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) db.RowInterface {
			return &dbtest.MockRow{
				Row: []interface{}{"cmd_1", "dev_001", "reboot", `{}`, "pending", "", "2026-01-01T00:00:00Z", nil},
			}
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/commands/cmd_1", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var cmd Command
	if err := json.Unmarshal(w.Body.Bytes(), &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.ID != "cmd_1" {
		t.Fatalf("expected cmd_1, got %s", cmd.ID)
	}
}

func TestCommands_Get_NotFound(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) db.RowInterface {
			return &dbtest.MockRow{Err: http.ErrNoLocation}
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/commands/nonexistent", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCommands_Result_Success(t *testing.T) {
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
		`{"result":"ok","status":"completed"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCommands_Result_NotFound(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (db.Result, error) {
			return &dbtest.MockResult{Affected: 0}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/commands/nonexistent/result", bytes.NewReader([]byte(
		`{"result":"ok","status":"completed"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCommands_Result_ExecError(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (db.Result, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/commands/cmd_1/result", bytes.NewReader([]byte(
		`{"result":"ok","status":"completed"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
