package tenant

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
)

func TestTenant_CreateOrg_Success(t *testing.T) {
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
	r := httptest.NewRequest("POST", "/api/v1/orgs", bytes.NewReader([]byte(`{"name":"acme"}`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var org Org
	if err := json.Unmarshal(w.Body.Bytes(), &org); err != nil {
		t.Fatal(err)
	}
	if org.Name != "acme" {
		t.Fatalf("expected acme, got %s", org.Name)
	}
}

func TestTenant_CreateOrg_MissingName(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/orgs", bytes.NewReader([]byte(`{}`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTenant_CreateOrg_BadJSON(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/orgs", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTenant_CreateOrg_ExecError(t *testing.T) {
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
	r := httptest.NewRequest("POST", "/api/v1/orgs", bytes.NewReader([]byte(`{"name":"acme"}`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestTenant_ListOrgs_Success(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (db.RowsInterface, error) {
			return &dbtest.MockRows{
				Rows: [][]interface{}{
					{"org_1", "acme", "2026-01-01T00:00:00Z"},
				},
			}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/orgs", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]Org
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["orgs"]) != 1 {
		t.Fatalf("expected 1 org, got %d", len(result["orgs"]))
	}
}

func TestTenant_ListOrgs_QueryError(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/orgs", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestTenant_ListOrgs_Empty(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/orgs", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestTenant_InviteUser_Success(t *testing.T) {
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
	r := httptest.NewRequest("POST", "/api/v1/orgs/org_1/users", bytes.NewReader([]byte(
		`{"email":"user@acme.com","role":"admin"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var u User
	if err := json.Unmarshal(w.Body.Bytes(), &u); err != nil {
		t.Fatal(err)
	}
	if u.Email != "user@acme.com" {
		t.Fatalf("expected user@acme.com, got %s", u.Email)
	}
}

func TestTenant_InviteUser_MissingEmail(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/orgs/org_1/users", bytes.NewReader([]byte(`{}`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTenant_InviteUser_BadJSON(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/orgs/org_1/users", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTenant_InviteUser_ExecError(t *testing.T) {
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
	r := httptest.NewRequest("POST", "/api/v1/orgs/org_1/users", bytes.NewReader([]byte(
		`{"email":"user@acme.com"}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestTenant_ListUsers_Success(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (db.RowsInterface, error) {
			return &dbtest.MockRows{
				Rows: [][]interface{}{
					{"usr_1", "org_1", "user@acme.com", "admin", "2026-01-01T00:00:00Z"},
				},
			}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/orgs/org_1/users", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]User
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["users"]) != 1 {
		t.Fatalf("expected 1 user, got %d", len(result["users"]))
	}
}

func TestTenant_ListUsers_QueryError(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/orgs/org_1/users", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestTenant_ListUsers_Empty(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/orgs/org_1/users", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
