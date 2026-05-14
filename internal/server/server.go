package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/lohtbrok/deviceos/internal/version"
)

type ModuleStatusFunc func() map[string]string

type Server struct {
	cfg       Config
	mux       *http.ServeMux
	srv       *http.Server
	startTime time.Time
	modStatus ModuleStatusFunc
	mu        sync.RWMutex
}

type Config struct {
	Host        string
	Port        int
	ModuleStats ModuleStatusFunc
}

func New(cfg Config) *Server {
	s := &Server{
		cfg:       cfg,
		mux:       http.NewServeMux(),
		startTime: time.Now(),
		modStatus: cfg.ModuleStats,
	}
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	return s
}

func (s *Server) SetModuleStatusFn(fn ModuleStatusFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modStatus = fn
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	bi := version.ReadBuildInfo()
	uptime := time.Since(s.startTime).String()

	resp := map[string]any{
		"status":  "ok",
		"version": bi.Version,
		"uptime":  uptime,
	}

	s.mu.RLock()
	if s.modStatus != nil {
		resp["modules"] = s.modStatus()
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	s.srv = &http.Server{
		Addr:         addr,
		Handler:      s.withMiddleware(s.mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	slog.Info("server starting", "addr", addr)
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	slog.Info("server shutting down")
	return s.srv.Shutdown(ctx)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		bi := version.ReadBuildInfo()
		w.Header().Set("X-DeviceOS-Version", bi.Version)
		next.ServeHTTP(w, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}
