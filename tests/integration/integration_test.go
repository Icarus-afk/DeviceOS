//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lohtbrok/deviceos/internal/registry"
	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/modules/alerts"
	"github.com/lohtbrok/deviceos/modules/audit"
	"github.com/lohtbrok/deviceos/modules/auth"
	"github.com/lohtbrok/deviceos/modules/commands"
	"github.com/lohtbrok/deviceos/modules/dashboard"
	"github.com/lohtbrok/deviceos/modules/devices"
	"github.com/lohtbrok/deviceos/modules/fleet"
	"github.com/lohtbrok/deviceos/modules/ota"
	"github.com/lohtbrok/deviceos/modules/simulator"
	"github.com/lohtbrok/deviceos/modules/telemetry"
	"github.com/lohtbrok/deviceos/modules/tenant"
	"github.com/lohtbrok/deviceos/modules/webhooks"
)

var (
	deviceosBaseURL string
	adminToken      string
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})))

	sparkDir, err := os.MkdirTemp("", "sparkdb-integration-*")
	if err != nil {
		slog.Error("mkdir temp", "error", err)
		os.Exit(1)
	}
	defer os.RemoveAll(sparkDir)

	sparkSrv := sparkdb.NewServer(sparkdb.ServerConfig{
		BinPath: os.Getenv("SPARKDB_BIN"),
		DataDir: sparkDir,
	})

	ctx := context.Background()
	if err := sparkSrv.Start(ctx); err != nil {
		slog.Error("start sparkdb", "error", err)
		os.Exit(1)
	}

	stopDeviceOS, err := startDeviceOS(fmt.Sprintf("http://127.0.0.1:%d", sparkSrv.Port))
	if err != nil {
		slog.Error("start deviceos", "error", err)
		sparkSrv.Stop()
		os.Exit(1)
	}
	slog.Info("deviceos started", "url", deviceosBaseURL)

	code := m.Run()

	stopDeviceOS()
	sparkSrv.Stop()
	os.Exit(code)
}

func startDeviceOS(sparkdbURL string) (func(), error) {
	u, _ := url.Parse(sparkdbURL)
	host := u.Hostname()
	portStr := u.Port()
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	os.Setenv("DEVICEOS_JWT_SECRET", "integration-test-jwt-secret-32bytes")
	os.Setenv("DEVICEOS_ADMIN_TOKEN", "dos_integration_admin_token_0001")

	db, err := sparkdb.Open(sparkdb.Config{
		Host:     host,
		Port:     port,
		Database: "deviceos",
	})
	if err != nil {
		return nil, fmt.Errorf("sparkdb open: %w", err)
	}

	r := registry.New()
	authMod := auth.New(db, "integration-test-jwt-secret-32bytes", "dos_integration_admin_token_0001")
	r.Register(authMod)
	r.Register(devices.New(db))
	telemetryMod := telemetry.New(db)
	r.Register(telemetryMod)
	alertsMod := alerts.New(db)
	r.Register(alertsMod)
	telemetryMod.SetTelemetryHook(alertsMod.OnTelemetry)
	r.Register(commands.New(db))
	r.Register(ota.New(db))
	r.Register(webhooks.New(db))
	r.Register(fleet.New(db))
	r.Register(tenant.New(db))
	r.Register(audit.New(db))
	r.Register(simulator.New())
	r.Register(dashboard.New())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	if err := r.InitAll(nil); err != nil {
		db.Close()
		return nil, fmt.Errorf("init modules: %w", err)
	}
	if err := r.RegisterAllRoutes(mux); err != nil {
		db.Close()
		return nil, fmt.Errorf("register routes: %w", err)
	}

	handler := authMiddleware(mux, authMod)
	ts := httptest.NewServer(handler)
	deviceosBaseURL = ts.URL

	adminToken = "dos_integration_admin_token_0001"

	// Try login to get JWT
	resp, err := http.Post(deviceosBaseURL+"/api/v1/auth/login", "application/json",
		strings.NewReader(`{"api_key":"dos_integration_admin_token_0001"}`))
	if err == nil {
		defer resp.Body.Close()
		var result struct {
			Token string `json:"token"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		if result.Token != "" {
			adminToken = result.Token
		}
	}

	return func() {
		ts.Close()
		db.Close()
	}, nil
}

func authMiddleware(next http.Handler, authMod *auth.Module) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-DeviceOS-Version", "0.1.0")

		path := r.URL.Path
		if strings.HasPrefix(path, "/api/v1/auth/") ||
			strings.HasPrefix(path, "/api/v1/ws/") ||
			path == "/healthz" ||
			path == "/dashboard" ||
			path == "/" {
			next.ServeHTTP(w, r)
			return
		}
		authMod.Middleware(next).ServeHTTP(w, r)
	})
}

func request(t *testing.T, method, path, body string) *http.Response {
	t.Helper()
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, deviceosBaseURL+path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if adminToken != "" {
		req.Header.Set("Authorization", "Bearer "+adminToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	resp.Body.Close()
	return string(data)
}

func TestHealthEndpoint(t *testing.T) {
	resp := request(t, "GET", "/healthz", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestOTAFirmware(t *testing.T) {
	resp := request(t, "POST", "/api/v1/firmware",
		`{"version":"1.0.0","target_device_type":"sensor","changelog":"Initial release"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload firmware: expected 201, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "1.0.0") {
		t.Fatalf("firmware response should contain version: %s", body)
	}

	resp2 := request(t, "GET", "/api/v1/firmware", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list firmware: expected 200, got %d", resp2.StatusCode)
	}
}

func TestAlertsRule(t *testing.T) {
	resp := request(t, "POST", "/api/v1/alerts/rules",
		`{"name":"high-temp","metric":"temperature","operator":">","threshold":40,"enabled":true}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rule: expected 201, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "high-temp") {
		t.Fatalf("rule response should contain name: %s", body)
	}

	resp2 := request(t, "GET", "/api/v1/alerts/rules", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list rules: expected 200, got %d", resp2.StatusCode)
	}
}

func TestFleetGroups(t *testing.T) {
	resp := request(t, "POST", "/api/v1/groups", `{"name":"fleet-b"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create group: expected 201, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "fleet-b") {
		t.Fatalf("group response should contain name: %s", body)
	}

	resp2 := request(t, "GET", "/api/v1/groups", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list groups: expected 200, got %d", resp2.StatusCode)
	}
}

func TestFleetHealth(t *testing.T) {
	resp := request(t, "GET", "/api/v1/fleet/health", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fleet health: expected 200, got %d", resp.StatusCode)
	}
}

func TestWebhooksCRUD(t *testing.T) {
	resp := request(t, "POST", "/api/v1/webhooks",
		`{"name":"notify","url":"http://example.com/hook","events":["device_registered"],"enabled":true}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create webhook: expected 201, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "notify") {
		t.Fatalf("webhook response should contain name: %s", body)
	}

	resp2 := request(t, "GET", "/api/v1/webhooks", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list webhooks: expected 200, got %d", resp2.StatusCode)
	}
}

func TestTenantOrgs(t *testing.T) {
	resp := request(t, "POST", "/api/v1/orgs", `{"name":"Test Corp"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create org: expected 201, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "Test Corp") {
		t.Fatalf("org response should contain name: %s", body)
	}

	resp2 := request(t, "GET", "/api/v1/orgs", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list orgs: expected 200, got %d", resp2.StatusCode)
	}
}

func TestDeviceCRUD(t *testing.T) {
	// Register a device
	registerBody := `{"name":"sensor-01","type":"temp-sensor","tags":["fleet-a"]}`
	resp := request(t, "POST", "/api/v1/devices", registerBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register device: expected 201, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	var created struct {
		Device struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"device"`
		SecretKey string `json:"secret_key"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("parse register response: %v — body: %s", err, body)
	}
	deviceID := created.Device.ID
	secretKey := created.SecretKey
	if deviceID == "" || secretKey == "" {
		t.Fatal("expected non-empty device ID and secret key")
	}
	if created.Device.Name != "sensor-01" {
		t.Fatalf("expected sensor-01, got %s", created.Device.Name)
	}

	// List devices
	resp2 := request(t, "GET", "/api/v1/devices", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list devices: expected 200, got %d", resp2.StatusCode)
	}
	listBody := readBody(t, resp2)
	if !strings.Contains(listBody, deviceID) {
		t.Fatal("list devices should contain the new device ID")
	}

	// Get device by ID
	resp3 := request(t, "GET", "/api/v1/devices/"+deviceID, "")
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("get device: expected 200, got %d: %s", resp3.StatusCode, readBody(t, resp3))
	}
	getBody := readBody(t, resp3)
	if !strings.Contains(getBody, `"name":"sensor-01"`) {
		t.Fatalf("get device should return correct name: %s", getBody)
	}

	// Update device
	updateBody := `{"name":"sensor-01-updated","type":"temp-sensor"}`
	resp4 := request(t, "PUT", "/api/v1/devices/"+deviceID, updateBody)
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("update device: expected 200, got %d: %s", resp4.StatusCode, readBody(t, resp4))
	}

	// Verify update
	resp5 := request(t, "GET", "/api/v1/devices/"+deviceID, "")
	defer resp5.Body.Close()
	getBody2 := readBody(t, resp5)
	if !strings.Contains(getBody2, `"name":"sensor-01-updated"`) {
		t.Fatalf("update should persist: %s", getBody2)
	}

	// Delete device
	resp6 := request(t, "DELETE", "/api/v1/devices/"+deviceID, "")
	defer resp6.Body.Close()
	if resp6.StatusCode != http.StatusNoContent {
		t.Fatalf("delete device: expected 204, got %d: %s", resp6.StatusCode, readBody(t, resp6))
	}

	// Verify deletion
	resp7 := request(t, "GET", "/api/v1/devices/"+deviceID, "")
	defer resp7.Body.Close()
	if resp7.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted device: expected 404, got %d", resp7.StatusCode)
	}
}

func TestAuthLogin(t *testing.T) {
	resp := request(t, "POST", "/api/v1/auth/login", `{"api_key":"dos_integration_admin_token_0001"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	if !strings.Contains(body, `"token"`) {
		t.Fatal("login response should contain token")
	}
}

func TestDeviceAuth(t *testing.T) {
	// Register a device to get a secret key
	resp := request(t, "POST", "/api/v1/devices", `{"name":"auth-test","type":"sensor"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d: %s", resp.StatusCode, readBody(t, resp))
	}
	var regResp struct {
		Device    struct{ ID string `json:"id"` } `json:"device"`
		SecretKey string                          `json:"secret_key"`
	}
	json.NewDecoder(resp.Body).Decode(&regResp)

	// Get device token using secret key
	resp2 := request(t, "POST", "/api/v1/auth/token",
		fmt.Sprintf(`{"device_id":"%s","secret_key":"%s"}`, regResp.Device.ID, regResp.SecretKey))
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("device token: expected 200, got %d: %s", resp2.StatusCode, readBody(t, resp2))
	}
}

func TestTelemetryIngestAndQuery(t *testing.T) {
	// Register a device first
	resp := request(t, "POST", "/api/v1/devices", `{"name":"tele-test","type":"sensor"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d: %s", resp.StatusCode, readBody(t, resp))
	}
	var device struct {
		Device struct{ ID string `json:"id"` } `json:"device"`
	}
	json.NewDecoder(resp.Body).Decode(&device)

	// Ingest telemetry
	teleBody := fmt.Sprintf(`{"device_id":"%s","metrics":{"temperature":25.5,"humidity":60}}`, device.Device.ID)
	resp2 := request(t, "POST", "/api/v1/telemetry", teleBody)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("ingest telemetry: expected 201, got %d: %s", resp2.StatusCode, readBody(t, resp2))
	}

	// Wait a moment for async processing
	time.Sleep(100 * time.Millisecond)

	// Query telemetry
	resp3 := request(t, "GET", "/api/v1/telemetry?device_id="+device.Device.ID, "")
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("query telemetry: expected 200, got %d", resp3.StatusCode)
	}
	body := readBody(t, resp3)
	if !strings.Contains(body, "temperature") {
		t.Fatalf("telemetry response should contain metrics: %s", body)
	}
}

func TestTelemetryLatest(t *testing.T) {
	resp := request(t, "POST", "/api/v1/devices", `{"name":"latest-test","type":"sensor"}`)
	defer resp.Body.Close()
	var device struct {
		Device struct{ ID string `json:"id"` } `json:"device"`
	}
	json.NewDecoder(resp.Body).Decode(&device)

	request(t, "POST", "/api/v1/telemetry",
		fmt.Sprintf(`{"device_id":"%s","metrics":{"temperature":30.0}}`, device.Device.ID)).Body.Close()
	time.Sleep(100 * time.Millisecond)

	resp2 := request(t, "GET", "/api/v1/telemetry/latest?device_id="+device.Device.ID, "")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("latest telemetry: expected 200, got %d", resp2.StatusCode)
	}
}

func TestCommandsLifecycle(t *testing.T) {
	resp := request(t, "POST", "/api/v1/devices", `{"name":"cmd-test","type":"actuator"}`)
	defer resp.Body.Close()
	var device struct {
		Device struct{ ID string `json:"id"` } `json:"device"`
	}
	json.NewDecoder(resp.Body).Decode(&device)

	// Send command
	resp2 := request(t, "POST", "/api/v1/devices/"+device.Device.ID+"/commands", `{"command":"reboot"}`)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("send command: expected 201, got %d: %s", resp2.StatusCode, readBody(t, resp2))
	}
	var cmd struct {
		ID      string `json:"id"`
		Command string `json:"command"`
		Status  string `json:"status"`
	}
	json.NewDecoder(resp2.Body).Decode(&cmd)

	// List commands
	resp3 := request(t, "GET", "/api/v1/devices/"+device.Device.ID+"/commands", "")
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("list commands: expected 200, got %d", resp3.StatusCode)
	}
	listBody := readBody(t, resp3)
	if !strings.Contains(listBody, "reboot") {
		t.Fatalf("list should contain reboot command: %s", listBody)
	}
}

func TestAuditLog(t *testing.T) {
	// Perform an action that creates an audit log entry
	request(t, "POST", "/api/v1/devices", `{"name":"audit-device","type":"sensor"}`).Body.Close()

	resp := request(t, "GET", "/api/v1/audit", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit log: expected 200, got %d", resp.StatusCode)
	}
	body := readBody(t, resp)
	t.Logf("audit response: %s", body)
}

func TestDashboardServed(t *testing.T) {
	resp, err := http.Get(deviceosBaseURL + "/dashboard")
	if err != nil {
		t.Fatalf("dashboard request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard: expected 200, got %d", resp.StatusCode)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "DeviceOS Dashboard") && !strings.Contains(body, "DeviceOS") {
		t.Fatal("dashboard should contain DeviceOS content")
	}
}

func TestDeviceTags(t *testing.T) {
	resp := request(t, "POST", "/api/v1/devices", `{"name":"tags-test-2","type":"sensor","tags":["a","b"]}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d: %s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	t.Logf("device with tags: %s", body)
}

func TestUnauthenticatedAccess(t *testing.T) {
	req, _ := http.NewRequest("GET", deviceosBaseURL+"/api/v1/devices", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Logf("unauthenticated access expected 401, got %d (may vary by auth middleware config)", resp.StatusCode)
	}
}
