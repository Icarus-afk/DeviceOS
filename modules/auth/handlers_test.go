package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func TestAuth_Login_Success(t *testing.T) {
	m := &Module{
		db: &sparkdbtest.MockDB{
			OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
				return &sparkdbtest.MockRow{
					Row: []interface{}{"admin"},
				}
			},
		},
		jwtSecret: "test-secret",
	}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader([]byte(
		`{"api_key":"test-key"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["token"] == "" {
		t.Fatal("expected non-empty token")
	}
	if resp["role"] != "admin" {
		t.Fatalf("expected admin role, got %v", resp["role"])
	}
}

func TestAuth_Login_InvalidKey(t *testing.T) {
	m := &Module{
		db: &sparkdbtest.MockDB{
			OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
				return &sparkdbtest.MockRow{Err: http.ErrNoLocation}
			},
		},
	}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader([]byte(
		`{"api_key":"bad-key"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_Login_BadJSON(t *testing.T) {
	m := &Module{jwtSecret: "test"}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAuth_DeviceToken_Success(t *testing.T) {
	m := &Module{
		db: &sparkdbtest.MockDB{
			OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
				return &sparkdbtest.MockRow{
					Row: []interface{}{"correct-secret"},
				}
			},
		},
		jwtSecret: "test-secret",
	}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/token", bytes.NewReader([]byte(
		`{"device_id":"dev_001","secret_key":"correct-secret"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["token"] == "" {
		t.Fatal("expected non-empty token")
	}
	if resp["role"] != "device" {
		t.Fatalf("expected device role, got %v", resp["role"])
	}
}

func TestAuth_DeviceToken_WrongSecret(t *testing.T) {
	m := &Module{
		db: &sparkdbtest.MockDB{
			OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
				return &sparkdbtest.MockRow{
					Row: []interface{}{"correct-secret"},
				}
			},
		},
		jwtSecret: "test-secret",
	}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/token", bytes.NewReader([]byte(
		`{"device_id":"dev_001","secret_key":"wrong-secret"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_DeviceToken_DeviceNotFound(t *testing.T) {
	m := &Module{
		db: &sparkdbtest.MockDB{
			OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
				return &sparkdbtest.MockRow{Err: http.ErrNoLocation}
			},
		},
	}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/token", bytes.NewReader([]byte(
		`{"device_id":"nonexistent","secret_key":"key"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_DeviceToken_BadJSON(t *testing.T) {
	m := &Module{jwtSecret: "test"}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/token", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAuth_Middleware_BearerValid(t *testing.T) {
	m := &Module{jwtSecret: "test-secret"}
	claims := Claims{Subject: "admin", Role: "admin"}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := jwtToken.SignedString([]byte("test-secret"))

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+tokenStr)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuth_Middleware_NoAuth(t *testing.T) {
	m := &Module{jwtSecret: "test-secret"}
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_Middleware_BadToken(t *testing.T) {
	m := &Module{jwtSecret: "test-secret"}
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer bad-token")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_Middleware_APIKeyValid(t *testing.T) {
	m := &Module{
		db: &sparkdbtest.MockDB{
			OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
				return &sparkdbtest.MockRow{
					Row: []interface{}{"admin"},
				}
			},
		},
	}
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User-Role") != "admin" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "ApiKey valid-key")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuth_Middleware_APIKeyInvalid(t *testing.T) {
	m := &Module{
		db: &sparkdbtest.MockDB{
			OnQueryRow: func(sql string, args []interface{}) sparkdb.RowInterface {
				return &sparkdbtest.MockRow{Err: http.ErrNoLocation}
			},
		},
	}
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "ApiKey bad-key")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_Middleware_UnsupportedScheme(t *testing.T) {
	m := &Module{jwtSecret: "test"}
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic dGVzdDp0ZXN0")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
