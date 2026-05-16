package devices

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func newDeviceModule() *Module {
	return &Module{db: &sparkdbtest.MockDB{}}
}

func registerRoutes(t *testing.T, m *Module) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}
	return mux
}

func jsonBody(v any) *bytes.Reader {
	data, _ := json.Marshal(v)
	return bytes.NewReader(data)
}

func TestDevices_Register_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{}, nil
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices", jsonBody(RegisterRequest{
		Name: "sensor-01", Type: "temp-sensor",
	}))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp RegisterResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Device.Name != "sensor-01" {
		t.Fatalf("expected sensor-01, got %s", resp.Device.Name)
	}
	if resp.Device.Status != "offline" {
		t.Fatalf("expected offline, got %s", resp.Device.Status)
	}
	if resp.SecretKey == "" {
		t.Fatal("expected non-empty secret_key")
	}
}

func TestDevices_Register_BadRequest(t *testing.T) {
	m := newDeviceModule()
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices", bytes.NewReader([]byte(`{invalid`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDevices_List_Success(t *testing.T) {
	now := time.Now()
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{
				Rows: [][]interface{}{
					{"dev_001", "sensor-01", "temp-sensor", `{"loc":"dhaka"}`, `["a"]`, "group-1", "online", now, now, now},
				},
			}, nil
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/devices", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]Device
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(result["devices"]) != 1 {
		t.Fatalf("expected 1 device, got %d", len(result["devices"]))
	}
	if result["devices"][0].ID != "dev_001" {
		t.Fatalf("expected dev_001, got %s", result["devices"][0].ID)
	}
}

func TestDevices_List_Empty(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{}, nil
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/devices", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDevices_List_QueryError(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/devices", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestDevices_Get_Success(t *testing.T) {
	now := time.Now()
	m := &Module{db: &sparkdbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{
				Row: []interface{}{"dev_001", "sensor-01", "temp-sensor", `{}`, `[]`, "", "online", now, now, now},
			}
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/devices/dev_001", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var dev Device
	if err := json.Unmarshal(w.Body.Bytes(), &dev); err != nil {
		t.Fatalf("json: %v", err)
	}
	if dev.ID != "dev_001" {
		t.Fatalf("expected dev_001, got %s", dev.ID)
	}
}

func TestDevices_Get_NotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{Err: http.ErrNoLocation}
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/devices/nonexistent", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDevices_Update_Success(t *testing.T) {
	now := time.Now()
	callCount := 0
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			callCount++
			return &sparkdbtest.MockResult{}, nil
		},
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{
				Row: []interface{}{"dev_001", "updated", "sensor", `{}`, `[]`, "", "online", now, now, now},
			}
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/devices/dev_001", jsonBody(RegisterRequest{
		Name: "updated", Type: "sensor",
	}))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var dev Device
	if err := json.Unmarshal(w.Body.Bytes(), &dev); err != nil {
		t.Fatalf("json: %v", err)
	}
	if dev.Name != "updated" {
		t.Fatalf("expected updated, got %s", dev.Name)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 Exec call, got %d", callCount)
	}
}

func TestDevices_Update_BadRequest(t *testing.T) {
	m := newDeviceModule()
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/devices/dev_001", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDevices_Update_NotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{}, nil
		},
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{Err: http.ErrNoLocation}
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/devices/dev_001", jsonBody(RegisterRequest{
		Name: "updated", Type: "sensor",
	}))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDevices_Delete_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{}, nil
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/v1/devices/dev_001", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestDevices_Register_ExecError(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return nil, http.ErrAbortHandler
		},
	}}
	mux := registerRoutes(t, m)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices", jsonBody(RegisterRequest{
		Name: "sensor-01", Type: "temp-sensor",
	}))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
