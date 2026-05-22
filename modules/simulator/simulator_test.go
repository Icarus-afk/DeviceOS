package simulator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSimulator_ModuleBasics(t *testing.T) {
	m := &Module{}
	if m.Name() != "simulator" {
		t.Fatalf("expected simulator, got %s", m.Name())
	}
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
}

func TestSimulator_RegisterRoutes_InvalidMux(t *testing.T) {
	m := &Module{}
	if err := m.RegisterRoutes("not-a-mux"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSimulator_Start_DefaultCount(t *testing.T) {
	m := &Module{}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/simulator/start", bytes.NewReader([]byte(`{}`)))
	mux.ServeHTTP(w, r)
	m.Stop()

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "started" {
		t.Fatalf("expected started, got %v", resp["status"])
	}
}

func TestSimulator_Start_AlreadyRunning(t *testing.T) {
	m := &Module{}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest("POST", "/api/v1/simulator/start", bytes.NewReader([]byte(`{"count":1}`)))
	mux.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first start: expected 200, got %d", w1.Code)
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("POST", "/api/v1/simulator/start", bytes.NewReader([]byte(`{"count":1}`)))
	mux.ServeHTTP(w2, r2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("second start: expected 409, got %d: %s", w2.Code, w2.Body.String())
	}

	m.Stop()
}

func TestSimulator_Start_WithCount(t *testing.T) {
	m := &Module{}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/simulator/start", bytes.NewReader([]byte(`{"count":5}`)))
	mux.ServeHTTP(w, r)
	m.Stop()

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSimulator_Stop(t *testing.T) {
	m := &Module{}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/simulator/stop", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "stopped" {
		t.Fatalf("expected stopped, got %v", resp["status"])
	}
}

func TestSimulator_Start_BadJSON(t *testing.T) {
	m := &Module{}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/simulator/start", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)
	m.Stop()

	// Should handle gracefully with default count = 3
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSimulator_Stop_WhileRunning(t *testing.T) {
	m := &Module{}
	m.mu.Lock()
	m.running = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	m.Stop()

	m.mu.Lock()
	if m.running {
		t.Fatal("expected running to be false after Stop")
	}
	m.mu.Unlock()
}
