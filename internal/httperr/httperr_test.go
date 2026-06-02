package httperr_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lohtbrok/deviceos/internal/httperr"
)

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	httperr.BadRequest(w, "invalid request body")

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "bad_request" {
		t.Fatalf("expected code bad_request, got %s", body.Error.Code)
	}
	if body.Error.Message != "invalid request body" {
		t.Fatalf("expected message 'invalid request body', got %s", body.Error.Message)
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	httperr.NotFound(w, "device not found")

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Error.Code != "not_found" {
		t.Fatalf("expected code not_found, got %s", body.Error.Code)
	}
	if body.Error.Message != "device not found" {
		t.Fatalf("expected message 'device not found', got %s", body.Error.Message)
	}
}

func TestInternal(t *testing.T) {
	w := httptest.NewRecorder()
	httperr.Internal(w, "database error")

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Error.Code != "internal_error" {
		t.Fatalf("expected code internal_error, got %s", body.Error.Code)
	}
	if body.Error.Message != "database error" {
		t.Fatalf("expected message 'database error', got %s", body.Error.Message)
	}
}

func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	httperr.Unauthorized(w, "invalid api key")

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	httperr.Forbidden(w, "access denied")
	if w.Result().StatusCode != http.StatusForbidden {
		t.Fatal("expected 403")
	}
}

func TestConflict(t *testing.T) {
	w := httptest.NewRecorder()
	httperr.Conflict(w, "already exists")
	if w.Result().StatusCode != http.StatusConflict {
		t.Fatal("expected 409")
	}
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	httperr.Error(w, http.StatusTeapot, "short and stout")

	resp := w.Result()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("expected 418, got %d", resp.StatusCode)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Error.Code != http.StatusText(http.StatusTeapot) {
		t.Fatalf("expected %q, got %s", http.StatusText(http.StatusTeapot), body.Error.Code)
	}
	if body.Error.Message != "short and stout" {
		t.Fatalf("expected 'short and stout', got %s", body.Error.Message)
	}
}

func TestContentType(t *testing.T) {
	w := httptest.NewRecorder()
	httperr.NotFound(w, "x")
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
}
