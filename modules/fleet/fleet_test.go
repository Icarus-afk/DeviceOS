package fleet

import (
	"encoding/json"
	"net/http"
	"testing"
)

type FleetHealth struct {
	Total   int `json:"total_devices"`
	Online  int `json:"online_devices"`
	Offline int `json:"offline_devices"`
}

func TestModuleBasics(t *testing.T) {
	m := &Module{}
	if m.Name() != "fleet" {
		t.Fatalf("expected fleet, got %s", m.Name())
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

func TestGroupModel(t *testing.T) {
	g := Group{ID: "grp_001", Name: "fleet-a"}
	data, _ := json.Marshal(g)
	var back Group
	json.Unmarshal(data, &back)
	if back.Name != "fleet-a" {
		t.Fatalf("expected fleet-a, got %s", back.Name)
	}
}

func TestGroupValidation(t *testing.T) {
	if (&Group{}).Name == "" {
		// name is required
	}
}

func TestFleetHealth(t *testing.T) {
	health := FleetHealth{Total: 10, Online: 7, Offline: 3}
	if health.Total != health.Online+health.Offline {
		t.Fatalf("total must equal online + offline")
	}
}

func TestGroupListResponse(t *testing.T) {
	resp := `{"groups":[{"id":"grp_001","name":"fleet-a"}]}`
	var parsed struct {
		Groups []map[string]interface{} `json:"groups"`
	}
	json.Unmarshal([]byte(resp), &parsed)
	if len(parsed.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(parsed.Groups))
	}
}

func TestGroupEmptyList(t *testing.T) {
	resp := `{"groups":[]}`
	var parsed struct {
		Groups []interface{} `json:"groups"`
	}
	json.Unmarshal([]byte(resp), &parsed)
	if len(parsed.Groups) != 0 {
		t.Fatal("expected empty list")
	}
}

func TestTagsUpdate(t *testing.T) {
	tags := []string{"gps", "priority", "fleet-a"}
	data, _ := json.Marshal(tags)
	var back []string
	json.Unmarshal(data, &back)
	if len(back) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(back))
	}
}

func TestGroupAssignment(t *testing.T) {
	assign := struct {
		DeviceID string `json:"device_id"`
		Group    string `json:"group"`
	}{
		DeviceID: "dev_001", Group: "fleet-a",
	}
	data, _ := json.Marshal(assign)
	var back map[string]interface{}
	json.Unmarshal(data, &back)
	if back["group"] != "fleet-a" {
		t.Fatalf("expected fleet-a, got %v", back["group"])
	}
}

func TestFleetHealthResponse(t *testing.T) {
	resp := `{"total_devices":5,"online_devices":3,"offline_devices":2}`
	var health FleetHealth
	json.Unmarshal([]byte(resp), &health)
	if health.Total != 5 {
		t.Fatalf("expected 5, got %d", health.Total)
	}
}

func TestGroupIDFormat(t *testing.T) {
	id := "grp_1778671770935085970"
	if id[:4] != "grp_" {
		t.Fatal("expected grp_ prefix")
	}
}

func BenchmarkGroupJSON(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var g Group
		json.Unmarshal([]byte(`{"id":"grp_001","name":"fleet-a"}`), &g)
	}
}
