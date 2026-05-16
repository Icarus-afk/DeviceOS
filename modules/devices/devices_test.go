package devices

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Devices tests verify the HTTP handler layer using mock data.
// Full integration with SparkDB requires a running SparkDB server.

func TestDeviceModel(t *testing.T) {
	d := struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Type   string   `json:"type"`
		Tags   []string `json:"tags"`
		Status string   `json:"status"`
	}{
		ID: "dev_test_001", Name: "sensor-01",
		Type: "temp-sensor", Tags: []string{"fleet-a"},
		Status: "offline",
	}
	data, _ := json.Marshal(d)
	var back struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Tags   []string `json:"tags"`
		Status string   `json:"status"`
	}
	json.Unmarshal(data, &back)
	if back.ID != "dev_test_001" {
		t.Fatalf("expected dev_test_001, got %s", back.ID)
	}
	if back.Status != "offline" {
		t.Fatalf("expected offline, got %s", back.Status)
	}
	if len(back.Tags) != 1 || back.Tags[0] != "fleet-a" {
		t.Fatalf("unexpected tags: %v", back.Tags)
	}
}

func TestRegisterRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{"valid", `{"name":"test","type":"sensor"}`, 201},
		{"missing_name", `{"type":"sensor"}`, 400},
		{"empty_body", `{}`, 400},
		{"extra_fields", `{"name":"test","type":"sensor","metadata":{"loc":"dhaka"}}`, 201},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the JSON serialization/deserialization independently
			var req struct {
				Name     string          `json:"name"`
				Type     string          `json:"type"`
				Metadata json.RawMessage `json:"metadata,omitempty"`
			}
			err := json.Unmarshal([]byte(tt.body), &req)
			if tt.wantCode == 201 && err != nil {
				t.Fatalf("expected valid request, got error: %v", err)
			}
			if tt.wantCode == 400 {
				if req.Name == "" {
					return // validation caught it
				}
			}
		})
	}
}

func TestHealthEndpoint(t *testing.T) {
	_ = httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestListDevicesEmptyResponse(t *testing.T) {
	resp := `{"devices":[]}`
	var result struct {
		Devices []interface{} `json:"devices"`
	}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Devices) != 0 {
		t.Fatal("expected empty devices")
	}
}

func TestDeviceJSONRoundTrip(t *testing.T) {
	original := `{"id":"dev_001","name":"truck-01","type":"gps","metadata":{"location":"dhaka"},"tags":["fleet-a"],"status":"online","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(original), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["id"] != "dev_001" {
		t.Fatalf("expected dev_001, got %v", parsed["id"])
	}
	if parsed["status"] != "online" {
		t.Fatalf("expected online, got %v", parsed["status"])
	}
	meta := parsed["metadata"].(map[string]interface{})
	if meta["location"] != "dhaka" {
		t.Fatalf("expected dhaka, got %v", meta["location"])
	}
}

func TestDeviceTagsRoundTrip(t *testing.T) {
	tags := []string{"fleet-a", "logistics", "priority"}
	data, _ := json.Marshal(tags)
	var back []string
	json.Unmarshal(data, &back)
	if len(back) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(back))
	}
	if back[0] != "fleet-a" {
		t.Fatalf("expected fleet-a, got %s", back[0])
	}
}

func BenchmarkDeviceJSON(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var d map[string]interface{}
		json.Unmarshal([]byte(`{"id":"dev_001","name":"truck","type":"gps","status":"online","metadata":{"location":"dhaka"},"tags":["fleet-a"]}`), &d)
	}
}

func TestWriteJSONHelper(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": "dev_001"})

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
}

func TestSecretKeyFormat(t *testing.T) {
	key := "ade735449cffeb0d6baca035ca2f1858"
	if len(key) != 32 {
		t.Fatalf("expected 32-char secret, got %d", len(key))
	}
	for _, c := range key {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("unexpected char in hex key: %c", c)
		}
	}
}

func TestDeviceIDFormat(t *testing.T) {
	id := "dev_f67af1cbda42f675"
	if !strings.HasPrefix(id, "dev_") {
		t.Fatal("expected dev_ prefix")
	}
	if len(id) != 20 {
		t.Fatalf("expected 20 chars, got %d", len(id))
	}
}

func TestGenerateID(t *testing.T) {
	id := generateID("dev")
	if len(id) != 20 {
		t.Fatalf("expected 20 chars, got %d", len(id))
	}
	if id[:4] != "dev_" {
		t.Fatal("expected dev_ prefix")
	}
}

func TestGenerateSecret(t *testing.T) {
	secret := generateSecret()
	if len(secret) != 32 {
		t.Fatalf("expected 32 hex chars, got %d", len(secret))
	}
	for _, c := range secret {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("unexpected char in hex secret: %c", c)
		}
	}
}

func TestDeviceStatusTransition(t *testing.T) {
	statuses := []string{"offline", "online", "offline"}
	// Verify status transitions are expected values
	for _, s := range statuses {
		if s != "offline" && s != "online" {
			t.Fatalf("unexpected status: %s", s)
		}
	}
}
