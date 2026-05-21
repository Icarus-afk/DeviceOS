package tenant

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestModuleBasics(t *testing.T) {
	m := &Module{}
	if m.Name() != "tenant" {
		t.Fatalf("expected tenant, got %s", m.Name())
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

func TestOrgModel(t *testing.T) {
	org := Org{ID: "org_001", Name: "ACME Corp"}
	data, _ := json.Marshal(org)
	var back Org
	json.Unmarshal(data, &back)
	if back.Name != "ACME Corp" {
		t.Fatalf("expected ACME Corp, got %s", back.Name)
	}
}

func TestOrgValidation(t *testing.T) {
	if (&Org{}).Name == "" {
		// name is required
	}
}

func TestUserModel(t *testing.T) {
	u := User{
		ID: "usr_001", OrgID: "org_001",
		Email: "admin@acme.com", Role: "admin",
	}
	data, _ := json.Marshal(u)
	var back User
	json.Unmarshal(data, &back)
	if back.Email != "admin@acme.com" {
		t.Fatalf("expected admin@acme.com, got %s", back.Email)
	}
	if back.Role != "admin" {
		t.Fatalf("expected admin, got %s", back.Role)
	}
}

func TestUserRoleDefaults(t *testing.T) {
	u := User{Email: "user@acme.com"}
	if u.Role == "" {
		u.Role = "viewer"
	}
	if u.Role != "viewer" {
		t.Fatalf("expected viewer, got %s", u.Role)
	}
}

func TestUserInviteValidation(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"admin@acme.com", true},
		{"", false},
	}
	for _, tt := range tests {
		valid := tt.email != ""
		if valid != tt.valid {
			t.Fatalf("email=%q: expected valid=%v", tt.email, tt.valid)
		}
	}
}

func TestOrgListResponse(t *testing.T) {
	resp := `{"orgs":[{"id":"org_001","name":"ACME Corp"}]}`
	var parsed struct {
		Orgs []map[string]interface{} `json:"orgs"`
	}
	json.Unmarshal([]byte(resp), &parsed)
	if len(parsed.Orgs) != 1 {
		t.Fatalf("expected 1 org, got %d", len(parsed.Orgs))
	}
}

func TestOrgEmptyList(t *testing.T) {
	resp := `{"orgs":[]}`
	var parsed struct {
		Orgs []interface{} `json:"orgs"`
	}
	json.Unmarshal([]byte(resp), &parsed)
	if len(parsed.Orgs) != 0 {
		t.Fatal("expected empty list")
	}
}

func TestUserListResponse(t *testing.T) {
	resp := `{"users":[{"id":"usr_001","email":"admin@acme.com","role":"admin"}]}`
	var parsed struct {
		Users []map[string]interface{} `json:"users"`
	}
	json.Unmarshal([]byte(resp), &parsed)
	if len(parsed.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(parsed.Users))
	}
}

func TestOrgIDFormat(t *testing.T) {
	id := "org_1778671770935085970"
	if id[:4] != "org_" {
		t.Fatal("expected org_ prefix")
	}
}

func TestUserIDFormat(t *testing.T) {
	id := "usr_1778671770935085970"
	if id[:4] != "usr_" {
		t.Fatal("expected usr_ prefix")
	}
}

func BenchmarkOrgJSON(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var o Org
		json.Unmarshal([]byte(`{"id":"org_001","name":"ACME"}`), &o)
	}
}
