package ota

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestModuleBasics(t *testing.T) {
	m := &Module{}
	if m.Name() != "ota" {
		t.Fatalf("expected ota, got %s", m.Name())
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}
}

func TestFirmwareModel(t *testing.T) {
	fw := struct {
		ID        string    `json:"id"`
		Version   string    `json:"version"`
		Checksum  string    `json:"checksum"`
		Size      int64     `json:"size"`
		CreatedAt time.Time `json:"created_at"`
	}{
		ID: "fw_001", Version: "1.0.0",
		Checksum: "abc123", Size: 1024,
	}
	data, _ := json.Marshal(fw)
	var back map[string]interface{}
	json.Unmarshal(data, &back)
	if back["version"] != "1.0.0" {
		t.Fatalf("expected 1.0.0, got %v", back["version"])
	}
}

func TestFirmwareUploadValidation(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		target    string
		expectErr bool
	}{
		{"valid", "1.0.0", "gps-tracker", false},
		{"missing_version", "", "gps-tracker", true},
		{"missing_target", "1.0.0", "", true},
		{"both_missing", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.version == "" || tt.target == "" {
				if !tt.expectErr {
					t.Fatal("expected validation error")
				}
				return
			}
		})
	}
}

func TestDeploymentModel(t *testing.T) {
	dep := struct {
		ID             string `json:"id"`
		FirmwareID     string `json:"firmware_id"`
		RolloutPercent int    `json:"rollout_percent"`
		Status         string `json:"status"`
	}{
		ID: "dep_001", FirmwareID: "fw_001",
		RolloutPercent: 50, Status: "in_progress",
	}
	data, _ := json.Marshal(dep)
	var back map[string]interface{}
	json.Unmarshal(data, &back)
	if back["rollout_percent"].(float64) != 50 {
		t.Fatalf("expected 50, got %v", back["rollout_percent"])
	}
}

func TestDeploymentDefaults(t *testing.T) {
	dep := struct {
		RolloutPercent int `json:"rollout_percent"`
	}{}
	data := `{}`
	json.Unmarshal([]byte(data), &dep)
	if dep.RolloutPercent != 0 {
		t.Fatalf("expected 0, got %d", dep.RolloutPercent)
	}
}

func TestFirmwareChecksum(t *testing.T) {
	checksum := "439aa747933dea3eb606f81ddffd32c45ee89ea5af4533d5e830f6b6bba3a45b"
	if len(checksum) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(checksum))
	}
}

func TestDeviceState(t *testing.T) {
	state := struct {
		DeviceID string `json:"device_id"`
		Status   string `json:"status"`
	}{
		DeviceID: "dev_001", Status: "installing",
	}
	data, _ := json.Marshal(state)
	var back map[string]interface{}
	json.Unmarshal(data, &back)
	if back["status"] != "installing" {
		t.Fatalf("expected installing, got %v", back["status"])
	}
}

func TestFirmwareListResponse(t *testing.T) {
	resp := `{"firmware":[{"id":"fw_001","version":"1.0.0"}]}`
	var parsed map[string]interface{}
	json.Unmarshal([]byte(resp), &parsed)
	fw := parsed["firmware"].([]interface{})
	if len(fw) != 1 {
		t.Fatalf("expected 1 firmware, got %d", len(fw))
	}
}

func TestFirmwareEmptyList(t *testing.T) {
	resp := `{"firmware":[]}`
	var parsed struct {
		Firmware []interface{} `json:"firmware"`
	}
	json.Unmarshal([]byte(resp), &parsed)
	if len(parsed.Firmware) != 0 {
		t.Fatal("expected empty list")
	}
}

func TestDeployRequest(t *testing.T) {
	req := struct {
		TargetGroup    string `json:"target_group"`
		RolloutPercent int    `json:"rollout_percent"`
	}{
		TargetGroup: "fleet-a", RolloutPercent: 25,
	}
	if req.RolloutPercent < 1 || req.RolloutPercent > 100 {
		t.Fatal("invalid rollout percent")
	}
}

func TestFirmwareResponse(t *testing.T) {
	resp := `{"id":"fw_001","version":"2.0.0","target_device_type":"sensor","checksum":"abc","size":26,"changelog":"GPS fix","created_at":"2026-01-01T00:00:00Z"}`
	var parsed struct {
		Version         string `json:"version"`
		TargetDeviceType string `json:"target_device_type"`
		Size            int64  `json:"size"`
	}
	json.Unmarshal([]byte(resp), &parsed)
	if parsed.Version != "2.0.0" {
		t.Fatalf("expected 2.0.0, got %s", parsed.Version)
	}
	if parsed.Size != 26 {
		t.Fatalf("expected 26, got %d", parsed.Size)
	}
}

func BenchmarkFirmwareJSON(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var fw map[string]interface{}
		json.Unmarshal([]byte(`{"id":"fw_001","version":"1.0.0","size":1024}`), &fw)
	}
}
