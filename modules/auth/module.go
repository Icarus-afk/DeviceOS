package auth

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/httperr"
)

type Module struct {
	db         db.DBClient
	jwtSecret  string
	adminToken string
	mu         sync.RWMutex
}

type Claims struct {
	Subject  string `json:"sub"`
	DeviceID string `json:"device_id,omitempty"`
	Role     string `json:"role"`
	OrgID    string `json:"org_id,omitempty"`
	jwt.RegisteredClaims
}

func New(db db.DBClient, jwtSecret, adminToken string) *Module {
	m := &Module{db: db}
	if jwtSecret == "" {
		jwtSecret = "dev-change-me-in-production"
	}
	if adminToken == "" {
		adminToken = generateAPIKey()
	}
	m.jwtSecret = jwtSecret
	m.adminToken = adminToken
	return m
}

func (m *Module) Name() string { return "auth" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("auth_v1", migration); err != nil {
		return fmt.Errorf("auth: migrate: %w", err)
	}

	m.db.Exec(`INSERT OR REPLACE INTO api_keys (key, role, created_at) VALUES (?, 'admin', ?)`,
		m.adminToken, time.Now())

	slog.Info("auth module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("auth: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/auth/login", m.handleLogin)
	r.HandleFunc("POST /api/v1/auth/token", m.handleDeviceToken)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

type LoginRequest struct {
	APIKey string `json:"api_key"`
}

type TokenRequest struct {
	DeviceID  string `json:"device_id"`
	SecretKey string `json:"secret_key"`
}

func (m *Module) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}

	var role string
	err := m.db.QueryRow(`SELECT role FROM api_keys WHERE key = ?`, req.APIKey).Scan(&role)
	if err != nil {
		httperr.Unauthorized(w, "invalid api key")
		return
	}

	claims := Claims{
		Subject: "admin",
		Role:    role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(m.jwtSecret))
	if err != nil {
		httperr.Internal(w, "token generation failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"token": signed, "role": role})
}

func (m *Module) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	var req TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid request")
		return
	}

	var storedKey, orgID string
	err := m.db.QueryRow(`SELECT secret_key, COALESCE(org_id, '') FROM devices WHERE id = ?`, req.DeviceID).Scan(&storedKey, &orgID)
	if err != nil {
		httperr.Unauthorized(w, "device not found")
		return
	}

	if storedKey != req.SecretKey {
		httperr.Unauthorized(w, "invalid secret key")
		return
	}

	claims := Claims{
		Subject:  req.DeviceID,
		DeviceID: req.DeviceID,
		Role:     "device",
		OrgID:    orgID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(m.jwtSecret))
	if err != nil {
		httperr.Internal(w, "token generation failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"token": signed, "role": "device"})
}

func (m *Module) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			httperr.Unauthorized(w, "authorization header required")
			return
		}

		if strings.HasPrefix(auth, "Bearer ") {
			tokenStr := strings.TrimPrefix(auth, "Bearer ")
			claims, err := m.validateJWT(tokenStr)
			if err != nil {
				httperr.Unauthorized(w, "invalid token")
				return
			}
			r.Header.Set("X-User-Role", claims.Role)
			r.Header.Set("X-User-Subject", claims.Subject)
			r.Header.Set("X-Org-ID", claims.OrgID)
			if claims.DeviceID != "" {
				r.Header.Set("X-Device-ID", claims.DeviceID)
			}
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(auth, "ApiKey ") {
			key := strings.TrimPrefix(auth, "ApiKey ")
			var role string
			err := m.db.QueryRow(`SELECT role FROM api_keys WHERE key = ?`, key).Scan(&role)
			if err != nil {
				httperr.Unauthorized(w, "invalid api key")
				return
			}
			r.Header.Set("X-User-Role", role)
			next.ServeHTTP(w, r)
			return
		}

		httperr.Unauthorized(w, "unsupported auth scheme")
	})
}

func (m *Module) validateJWT(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		return []byte(m.jwtSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
