package ota

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func TestOTA_UploadJSON_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{}, nil
		},
	}, storeDir: t.TempDir()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/firmware", bytes.NewReader([]byte(
		`{"version":"1.0","target_device_type":"sensor"}`,
	)))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var fw Firmware
	if err := json.Unmarshal(w.Body.Bytes(), &fw); err != nil {
		t.Fatal(err)
	}
	if fw.Version != "1.0" {
		t.Fatalf("expected 1.0, got %s", fw.Version)
	}
}

func TestOTA_UploadJSON_MissingFields(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/firmware", bytes.NewReader([]byte(
		`{"version":"1.0"}`,
	)))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestOTA_UploadJSON_BadRequest(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/firmware", bytes.NewReader([]byte(`{bad`)))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestOTA_UploadJSON_ExecError(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return nil, http.ErrAbortHandler
		},
	}, storeDir: t.TempDir()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/firmware", bytes.NewReader([]byte(
		`{"version":"1.0","target_device_type":"sensor"}`,
	)))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestOTA_List_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{
				Rows: [][]interface{}{
					{"fw_001", "1.0", "sensor", "abc123", int64(1024), "init", "2026-01-01T00:00:00Z"},
				},
			}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/firmware", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]Firmware
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["firmware"]) != 1 {
		t.Fatalf("expected 1 firmware, got %d", len(result["firmware"]))
	}
}

func TestOTA_List_QueryError(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/firmware", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestOTA_List_Empty(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/firmware", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestOTA_Get_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{
				Row: []interface{}{"fw_001", "1.0", "sensor", "abc123", int64(1024), "init", "2026-01-01T00:00:00Z"},
			}
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/firmware/fw_001", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var fw Firmware
	if err := json.Unmarshal(w.Body.Bytes(), &fw); err != nil {
		t.Fatal(err)
	}
	if fw.ID != "fw_001" {
		t.Fatalf("expected fw_001, got %s", fw.ID)
	}
}

func TestOTA_Get_NotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{Err: http.ErrNoLocation}
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/firmware/nonexistent", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestOTA_Deploy_Success(t *testing.T) {
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
	r := httptest.NewRequest("POST", "/api/v1/firmware/fw_001/deploy", bytes.NewReader([]byte(
		`{"target_group":"all","rollout_percent":100}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOTA_Deploy_BadJSON(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/firmware/fw_001/deploy", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestOTA_Deploy_ExecError(t *testing.T) {
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
	r := httptest.NewRequest("POST", "/api/v1/firmware/fw_001/deploy", bytes.NewReader([]byte(
		`{"target_group":"all","rollout_percent":100}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestOTA_DeploymentStatus_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{
				Row: []interface{}{"dep_001", "fw_001", "all", 100, "in_progress", "2026-01-01T00:00:00Z"},
			}
		},
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{
				Rows: [][]interface{}{
					{"dev_001", "pending"},
				},
			}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/deployments/dep_001", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var dep Deployment
	if err := json.Unmarshal(w.Body.Bytes(), &dep); err != nil {
		t.Fatal(err)
	}
	if dep.ID != "dep_001" {
		t.Fatalf("expected dep_001, got %s", dep.ID)
	}
	if len(dep.DeviceStates) != 1 {
		t.Fatalf("expected 1 device state, got %d", len(dep.DeviceStates))
	}
}

func TestOTA_DeploymentStatus_NotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
			return &sparkdbtest.MockRow{Err: http.ErrNoLocation}
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/deployments/nonexistent", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestOTA_DeviceStatus_Success(t *testing.T) {
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
	r := httptest.NewRequest("PUT", "/api/v1/deployments/dep_001/device-status", bytes.NewReader([]byte(
		`{"device_id":"dev_001","status":"completed"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOTA_DeviceStatus_BadJSON(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/deployments/dep_001/device-status", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestOTA_DeviceStatus_ExecError(t *testing.T) {
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
	r := httptest.NewRequest("PUT", "/api/v1/deployments/dep_001/device-status", bytes.NewReader([]byte(
		`{"device_id":"dev_001","status":"completed"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestOTA_UploadMultipart_Success(t *testing.T) {
	dir := t.TempDir()
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{}, nil
		},
	}, storeDir: dir}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	mp.WriteField("version", "2.0")
	mp.WriteField("target_device_type", "gps")
	mp.WriteField("changelog", "bugfix")
	fw, _ := mp.CreateFormFile("file", "fw.bin")
	fw.Write([]byte("firmware-data"))
	mp.Close()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/firmware", &buf)
	r.Header.Set("Content-Type", mp.FormDataContentType())
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var fwResult Firmware
	if err := json.Unmarshal(w.Body.Bytes(), &fwResult); err != nil {
		t.Fatal(err)
	}
	if fwResult.Version != "2.0" {
		t.Fatalf("expected 2.0, got %s", fwResult.Version)
	}
	if fwResult.Size == 0 {
		t.Fatal("expected non-zero size")
	}
	if _, err := os.Stat(dir + "/" + fwResult.ID); err != nil {
		t.Fatalf("firmware file not written: %v", err)
	}
}

func TestOTA_UploadMultipart_MissingFile(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}, storeDir: t.TempDir()}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	mp.Close()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/firmware", &buf)
	r.Header.Set("Content-Type", mp.FormDataContentType())
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
