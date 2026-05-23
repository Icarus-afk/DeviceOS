package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboard_ModuleBasics(t *testing.T) {
	m := &Module{}
	if m.Name() != "dashboard" {
		t.Fatalf("expected dashboard, got %s", m.Name())
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestDashboard_Init(t *testing.T) {
	m := &Module{}
	if err := m.Init(nil); err != nil {
		t.Fatal(err)
	}
}

func TestDashboard_HandleDashboard(t *testing.T) {
	m := &Module{}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/dashboard", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html, got %s", ct)
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected non-empty body")
	}
}

func TestDashboard_HandleRoot(t *testing.T) {
	m := &Module{}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/dashboard" {
		t.Fatalf("expected redirect to /dashboard, got %s", loc)
	}
}

func TestDashboard_HandleRoot_NotFound(t *testing.T) {
	m := &Module{}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/other", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDashboard_RegisterRoutes_InvalidMux(t *testing.T) {
	m := &Module{}
	if err := m.RegisterRoutes("not-a-mux"); err == nil {
		t.Fatal("expected error")
	}
}
