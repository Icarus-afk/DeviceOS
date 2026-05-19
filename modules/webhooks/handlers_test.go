package webhooks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func TestWebhooks_Create_Success(t *testing.T) {
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
	r := httptest.NewRequest("POST", "/api/v1/webhooks", bytes.NewReader([]byte(
		`{"name":"my-hook","url":"http://example.com/hook","events":["telemetry"]}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var wh Webhook
	if err := json.Unmarshal(w.Body.Bytes(), &wh); err != nil {
		t.Fatal(err)
	}
	if wh.Name != "my-hook" {
		t.Fatalf("expected my-hook, got %s", wh.Name)
	}
}

func TestWebhooks_Create_MissingFields(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	tests := []struct{ name, body string }{
		{"empty", `{}`},
		{"missing_url", `{"name":"test"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/v1/webhooks", bytes.NewReader([]byte(tt.body)))
			mux.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestWebhooks_Create_BadJSON(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/webhooks", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWebhooks_Create_ExecError(t *testing.T) {
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
	r := httptest.NewRequest("POST", "/api/v1/webhooks", bytes.NewReader([]byte(
		`{"name":"h","url":"http://example.com/h","events":["t"]}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestWebhooks_List_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{
				Rows: [][]interface{}{
					{"wh_1", "my-hook", "http://example.com/hook", "secret", `["telemetry"]`, true, "2026-01-01T00:00:00Z"},
				},
			}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/webhooks", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]Webhook
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["webhooks"]) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(result["webhooks"]))
	}
}

func TestWebhooks_List_QueryError(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/webhooks", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestWebhooks_List_Empty(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/webhooks", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestWebhooks_Update_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{Affected: 1}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/webhooks/wh_1", bytes.NewReader([]byte(
		`{"name":"updated","url":"http://example.com","events":["telemetry"]}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebhooks_Update_NotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{Affected: 0}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/webhooks/nonexistent", bytes.NewReader([]byte(
		`{"name":"x","url":"http://example.com","events":["t"]}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWebhooks_Update_BadJSON(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/webhooks/wh_1", bytes.NewReader([]byte(`{bad`)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWebhooks_Update_ExecError(t *testing.T) {
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
	r := httptest.NewRequest("PUT", "/api/v1/webhooks/wh_1", bytes.NewReader([]byte(
		`{"name":"x","url":"http://ex.com","events":["t"]}`,
	)))
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestWebhooks_Delete_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{Affected: 1}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/v1/webhooks/wh_1", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestWebhooks_Delete_NotFound(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
			return &sparkdbtest.MockResult{Affected: 0}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/v1/webhooks/nonexistent", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWebhooks_Delete_ExecError(t *testing.T) {
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
	r := httptest.NewRequest("DELETE", "/api/v1/webhooks/wh_1", nil)
	mux.ServeHTTP(w, r)

	// Handler returns 404 on any error (pre-existing behavior)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWebhooks_Deliveries_Success(t *testing.T) {
	m := &Module{db: &sparkdbtest.MockDB{
		OnQuery: func(sql string, args []interface{}) (sparkdb.RowsInterface, error) {
			return &sparkdbtest.MockRows{
				Rows: [][]interface{}{
					{"del_1", "wh_1", "telemetry", `{"temp":25}`, "success", 200, "2026-01-01T00:00:00Z"},
				},
			}, nil
		},
	}}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/webhooks/wh_1/deliveries", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string][]Delivery
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result["deliveries"]) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(result["deliveries"]))
	}
}

func TestWebhooks_Deliveries_QueryError(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/webhooks/wh_1/deliveries", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestWebhooks_Deliveries_Empty(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/api/v1/webhooks/wh_1/deliveries", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
