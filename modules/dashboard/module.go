package dashboard

import (
	"fmt"
	"log/slog"
	"net/http"
)

type Module struct{}

func New() *Module { return &Module{} }

func (m *Module) Name() string { return "dashboard" }

func (m *Module) Init(cfg any) error {
	slog.Info("dashboard module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("dashboard: unexpected mux type")
	}
	r.HandleFunc("GET /dashboard", m.handleDashboard)
	r.HandleFunc("GET /", m.handleRoot)
	return nil
}

func (m *Module) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(indexHTML))
}

func (m *Module) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }
