package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/lohtbrok/deviceos/internal/httperr"
	"github.com/lohtbrok/deviceos/internal/version"
)

type ModuleStatusFunc func() map[string]string

type Middleware func(http.Handler) http.Handler

type Config struct {
	Host           string
	Port           int
	TLSKey         string
	TLSCert        string
	AllowedOrigins []string
	RateLimitRPM   int
	ModuleStats    ModuleStatusFunc
	AuthMiddleware Middleware
}

type Server struct {
	cfg       Config
	mux       *http.ServeMux
	srv       *http.Server
	startTime time.Time
	modStatus ModuleStatusFunc
	mu        sync.RWMutex
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
	handler := s.buildHandler()

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	s.srv = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if s.cfg.TLSCert != "" && s.cfg.TLSKey != "" {
		slog.Info("server starting with TLS", "addr", addr)
		return s.srv.ListenAndServeTLS(s.cfg.TLSCert, s.cfg.TLSKey)
	}

	slog.Info("server starting", "addr", addr)
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	slog.Info("server shutting down")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.srv.Shutdown(shutdownCtx)
}

func (s *Server) buildHandler() http.Handler {
	var h http.Handler = s.mux
	h = s.withRequestID(h)
	h = s.withLogging(h)
	h = s.withVersion(h)
	if s.cfg.AuthMiddleware != nil {
		h = s.wrapAuth(h)
	}
	if s.cfg.RateLimitRPM > 0 {
		h = s.withRateLimit(h)
	}
	if len(s.cfg.AllowedOrigins) > 0 {
		h = s.withCORS(h, s.cfg.AllowedOrigins)
	}
	return h
}

func (s *Server) withVersion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bi := version.ReadBuildInfo()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-DeviceOS-Version", bi.Version)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	var idCounter uint64
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		idCounter++
		id := fmt.Sprintf("%x-%d", time.Now().UnixNano(), idCounter)
		mu.Unlock()
		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", lrw.status,
			"duration", time.Since(start),
			"request_id", r.Header.Get("X-Request-ID"),
		)
	})
}

func (s *Server) wrapAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if isPublicRoute(path) {
			s.cfg.AuthMiddleware(next).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPublicRoute(path string) bool {
	return path == "/healthz" ||
		path == "/" ||
		path == "/dashboard" ||
		path == "/api/v1/auth/login" ||
		path == "/api/v1/auth/token" ||
		path == "/api/v1/ws/telemetry" ||
		path == "/api/v1/ws/commands"
}

func (s *Server) withCORS(next http.Handler, origins []string) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[origin] || origins[0] == "*" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type rateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*clientBucket
	rate     int
	interval time.Duration
}

type clientBucket struct {
	tokens   int
	lastDrop time.Time
}

func newRateLimiter(rpm int) *rateLimiter {
	return &rateLimiter{
		clients:  make(map[string]*clientBucket),
		rate:     rpm,
		interval: time.Minute,
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cb, ok := rl.clients[ip]
	if !ok {
		cb = &clientBucket{tokens: rl.rate, lastDrop: time.Now()}
		rl.clients[ip] = cb
	}

	now := time.Now()
	elapsed := now.Sub(cb.lastDrop)
	cb.lastDrop = now

	refill := int(elapsed / rl.interval * time.Duration(rl.rate))
	if refill > 0 {
		cb.tokens = min(cb.tokens+refill, rl.rate)
	}

	if cb.tokens <= 0 {
		return false
	}
	cb.tokens--
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Server) withRateLimit(next http.Handler) http.Handler {
	rl := newRateLimiter(s.cfg.RateLimitRPM)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "60")
			httperr.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

var defaultTLSConfig = &tls.Config{
	MinVersion:               tls.VersionTLS12,
	PreferServerCipherSuites: true,
	CurvePreferences:         []tls.CurveID{tls.X25519, tls.CurveP256},
}
