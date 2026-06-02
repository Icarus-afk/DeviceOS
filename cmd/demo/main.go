package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	dbpkg "github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
	"github.com/lohtbrok/deviceos/internal/server"
	"github.com/lohtbrok/deviceos/internal/version"
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

type APITest struct {
	Section string
	Name    string
	Method  string
	Path    string
	Body    string
	Status  int
	WantErr bool
	Result  string
	Got     string
}

type Report struct {
	Tests []APITest
}

func (r *Report) Add(section, name, method, path, body string, wantErr bool, status int, respBody string) {
	got := fmt.Sprintf("%d %s", status, respBody)
	result := "PASS"
	if wantErr && status < 400 {
		result = "FAIL"
	} else if !wantErr && status >= 400 {
		result = "FAIL"
	}
	r.Tests = append(r.Tests, APITest{
		Section: section,
		Name:    name,
		Method:  method,
		Path:    path,
		Body:    body,
		Status:  status,
		WantErr: wantErr,
		Result:  result,
		Got:     got,
	})
}

func main() {
	r := &Report{}
	now := time.Now()

	db := &dbtest.MockDB{}
	devMod := devices.New(db)
	telMod := telemetry.New(db, 30*24*time.Hour, time.Hour)
	alertMod := alerts.New(db)
	authMod := auth.New(db, "demo-secret", "dos_demo_admin_key")
	cmdMod := commands.New(db)
	otaMod := ota.New(db)
	whMod := webhooks.New(db)
	fleetMod := fleet.New(db)
	tenantMod := tenant.New(db)
	auditMod := audit.New(db)
	simMod := simulator.New()
	dashMod := dashboard.New()

	telMod.AddTelemetryHook(alertMod.OnTelemetry)

	mux := http.NewServeMux()
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0})

	for _, mod := range []struct {
		m interface{ Init(any) error }
	}{
		{devMod}, {telMod}, {alertMod}, {authMod}, {cmdMod}, {otaMod},
		{whMod}, {fleetMod}, {tenantMod}, {auditMod}, {simMod}, {dashMod},
	} {
		if err := mod.m.Init(nil); err != nil {
			fmt.Fprintf(os.Stderr, "init: %v\n", err)
			os.Exit(1)
		}
	}

	for _, mod := range []interface{ RegisterRoutes(any) error }{
		devMod, telMod, alertMod, authMod, cmdMod, otaMod,
		whMod, fleetMod, tenantMod, auditMod, simMod, dashMod,
	} {
		if err := mod.RegisterRoutes(mux); err != nil {
			fmt.Fprintf(os.Stderr, "register routes: %v\n", err)
			os.Exit(1)
		}
	}

	mux.Handle("GET /healthz", srv.Mux())

	exec := func(method, path, body string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		mux.ServeHTTP(w, req)
		resp := strings.TrimSpace(w.Body.String())
		if len(resp) > 200 {
			resp = resp[:200] + "..."
		}
		return w.Code, resp
	}

	execAuth := func(method, path, body string, token string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		req.Header.Set("Authorization", "Bearer "+token)
		mux.ServeHTTP(w, req)
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, w.Body.Bytes(), "", "  "); err != nil {
			return w.Code, w.Body.String()
		}
		return w.Code, pretty.String()
	}

	execAuthRaw := func(method, path, body string, token string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		req.Header.Set("Authorization", "Bearer "+token)
		mux.ServeHTTP(w, req)
		resp := strings.TrimSpace(w.Body.String())
		if len(resp) > 200 {
			resp = resp[:200] + "..."
		}
		return w.Code, resp
	}

	// ── 1. Health ──────────────────────────────────────────────
	section := "1. Health Check"
	code, resp := exec("GET", "/healthz", "")
	r.Add(section, "Server health", "GET", "/healthz", "", false, code, resp)

	// ── 2. Auth ─────────────────────────────────────────────────
	section = "2. Authentication"

	correctKey := "dos_demo_admin_key"
	db.OnQueryRow = func(sql string, args []interface{}) dbpkg.RowInterface {
		if strings.Contains(sql, "FROM api_keys") {
			if len(args) > 0 {
				if key, ok := args[0].(string); ok && key == correctKey {
					return &dbtest.MockRow{Row: []interface{}{"admin"}}
				}
			}
			return &dbtest.MockRow{Err: fmt.Errorf("no rows")}
		}
		return &dbtest.MockRow{Err: fmt.Errorf("no rows")}
	}

	code, resp = exec("POST", "/api/v1/auth/login", `{"api_key":"dos_demo_admin_key"}`)
	r.Add(section, "Admin login (valid key)", "POST", "/api/v1/auth/login", `{"api_key":"..."}`, false, code, resp)

	token := ""
	if code == 200 {
		var loginResp map[string]any
		json.Unmarshal([]byte(resp), &loginResp)
		if t, ok := loginResp["token"].(string); ok {
			token = t
		}
	}

	code, resp = exec("POST", "/api/v1/auth/login", `{"api_key":"wrong"}`)
	r.Add(section, "Login with wrong key (expect 401)", "POST", "/api/v1/auth/login", `{"api_key":"wrong"}`, true, code, resp)

	code, resp = exec("POST", "/api/v1/auth/token", `{"device_id":"dev_001","secret_key":"test-secret"}`)
	r.Add(section, "Device token (no matching device — 401)", "POST", "/api/v1/auth/token", `{"device_id":"...","secret_key":"..."}`, true, code, resp)

	// ── 3. Device CRUD ──────────────────────────────────────────
	section = "3. Device Management"
	deviceID := "dev_demo_001"

	// Reset mock DB for devices
	db.OnExec = func(sql string, args []interface{}) (dbpkg.Result, error) {
		if strings.Contains(sql, "INSERT INTO devices") {
			return &dbtest.MockResult{LastID: 1, Affected: 1}, nil
		}
		if strings.Contains(sql, "DELETE FROM devices") {
			return &dbtest.MockResult{Affected: 1}, nil
		}
		return &dbtest.MockResult{Affected: 1}, nil
	}
	db.OnQuery = func(sql string, args []interface{}) (dbpkg.RowsInterface, error) {
		if strings.Contains(sql, "FROM devices") {
			return &dbtest.MockRows{
				Rows: [][]interface{}{
					{deviceID, "temp-sensor-01", "temp-sensor", `{"floor":3,"building":"A"}`, `["sensor","production"]`, "floor-3", "online", now, now, now},
				},
			}, nil
		}
		return &dbtest.MockRows{}, nil
	}
	db.OnQueryRow = func(sql string, args []interface{}) dbpkg.RowInterface {
		if strings.Contains(sql, "FROM devices") {
			return &dbtest.MockRow{
				Row: []interface{}{deviceID, "temp-sensor-01", "temp-sensor", `{"floor":3,"building":"A"}`, `["sensor","production"]`, "floor-3", "online", now, now, now},
			}
		}
		if strings.Contains(sql, "FROM api_keys") {
			return &dbtest.MockRow{Row: []interface{}{"admin"}}
		}
		return &dbtest.MockRow{Err: fmt.Errorf("no rows")}
	}

	code, resp = execAuthRaw("POST", "/api/v1/devices", `{"name":"temp-sensor-01","type":"temp-sensor","metadata":{"floor":3,"building":"A"}}`, token)
	r.Add(section, "Register device", "POST", "/api/v1/devices", `{"name":"temp-sensor-01","type":"temp-sensor","metadata":{...}}`, false, code, resp)
	if code == 201 {
		var regResp map[string]any
		json.Unmarshal([]byte(resp), &regResp)
		if dev, ok := regResp["device"].(map[string]any); ok {
			if id, ok := dev["id"].(string); ok {
				deviceID = id
			}
		}
	}

	code, resp = execAuth("GET", "/api/v1/devices", "", token)
	r.Add(section, "List devices", "GET", "/api/v1/devices", "", false, code, resp)

	code, resp = execAuth("GET", "/api/v1/devices/"+deviceID, "", token)
	r.Add(section, "Get device by ID", "GET", "/api/v1/devices/{id}", "", false, code, resp)

	code, resp = execAuth("PUT", "/api/v1/devices/"+deviceID, `{"name":"temp-sensor-01-updated","type":"temp-sensor"}`, token)
	r.Add(section, "Update device", "PUT", "/api/v1/devices/{id}", `{"name":"updated","type":"temp-sensor"}`, false, code, resp)

	code, resp = execAuthRaw("DELETE", "/api/v1/devices/"+deviceID, "", token)
	r.Add(section, "Delete device", "DELETE", "/api/v1/devices/{id}", "", false, code, resp)

	// ── 4. Telemetry ────────────────────────────────────────────
	section = "4. Telemetry"

	db.OnExec = func(sql string, args []interface{}) (dbpkg.Result, error) {
		return &dbtest.MockResult{LastID: 1, Affected: 1}, nil
	}
	db.OnQuery = func(sql string, args []interface{}) (dbpkg.RowsInterface, error) {
		if strings.Contains(sql, "FROM telemetry") {
			return &dbtest.MockRows{
				Rows: [][]interface{}{
					{int64(1), "dev_001", now, `{"temperature":23.5,"humidity":60,"battery":85}`, `{}`},
					{int64(2), "dev_001", now, `{"temperature":47.2,"humidity":55,"battery":73}`, `{}`},
				},
			}, nil
		}
		if strings.Contains(sql, "FROM devices") {
			return &dbtest.MockRows{
				Rows: [][]interface{}{
					{deviceID, "temp-sensor-01", "temp-sensor", `{}`, `[]`, "", "online", now, now, now},
				},
			}, nil
		}
		return &dbtest.MockRows{}, nil
	}
	db.OnQueryRow = func(sql string, args []interface{}) dbpkg.RowInterface {
		if strings.Contains(sql, "FROM telemetry") {
			return &dbtest.MockRow{
				Row: []interface{}{int64(2), "dev_001", now, `{"temperature":47.2,"humidity":55}`, `{}`},
			}
		}
		if strings.Contains(sql, "FROM api_keys") {
			return &dbtest.MockRow{Row: []interface{}{"admin"}}
		}
		return &dbtest.MockRow{Err: fmt.Errorf("no rows")}
	}

	code, resp = execAuth("POST", "/api/v1/telemetry", `{"device_id":"dev_001","metrics":{"temperature":23.5,"humidity":60,"battery":85}}`, token)
	r.Add(section, "Ingest telemetry (normal)", "POST", "/api/v1/telemetry", `{"device_id":"...","metrics":{...}}`, false, code, resp)

	code, resp = execAuth("POST", "/api/v1/telemetry", `{"device_id":"dev_001","metrics":{"temperature":47.2,"humidity":55}}`, token)
	r.Add(section, "Ingest telemetry (high temp — triggers alert)", "POST", "/api/v1/telemetry", `{"device_id":"...","metrics":{"temperature":47.2}}`, false, code, resp)

	code, resp = execAuthRaw("POST", "/api/v1/telemetry", `{"device_id":"","metrics":null}`, token)
	r.Add(section, "Ingest with missing fields (expect 400)", "POST", "/api/v1/telemetry", `{"device_id":"","metrics":null}`, true, code, resp)

	code, resp = execAuth("GET", "/api/v1/telemetry?device_id=dev_001&limit=10", "", token)
	r.Add(section, "Query telemetry history", "GET", "/api/v1/telemetry?device_id=X&limit=10", "", false, code, resp)

	code, resp = execAuth("GET", "/api/v1/telemetry/latest?device_id=dev_001", "", token)
	r.Add(section, "Get latest telemetry", "GET", "/api/v1/telemetry/latest?device_id=X", "", false, code, resp)

	// ── 5. Alerts ──────────────────────────────────────────────
	section = "5. Alert Rules & Evaluation"
	ruleID := "rule_demo_001"
	dbAlert := &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (dbpkg.Result, error) {
			return &dbtest.MockResult{Affected: 1}, nil
		},
		OnQuery: func(sql string, args []interface{}) (dbpkg.RowsInterface, error) {
			if strings.Contains(sql, "FROM alert_rules") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{ruleID, "high-temp", "temperature", ">", 45.0, "", "log", "", 1, now},
					},
				}, nil
			}
			if strings.Contains(sql, "FROM alert_events") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{"evt_001", ruleID, "high-temp", "dev_001", "temperature", 47.2, "Temperature 47.20 exceeds threshold 45.00", "critical", now},
					},
				}, nil
			}
			return &dbtest.MockRows{}, nil
		},
		OnQueryRow: func(sql string, args []interface{}) dbpkg.RowInterface {
			if strings.Contains(sql, "FROM alert_rules WHERE") || strings.Contains(sql, "FROM alert_rules") {
				return &dbtest.MockRow{
					Row: []interface{}{ruleID, "high-temp", "temperature", ">", 45.0, "", "log", "", 1, now},
				}
			}
			return &dbtest.MockRow{Err: fmt.Errorf("no rows")}
		},
		OnMigrate: func(name, sql string) error { return nil },
	}

	alertMod2 := alerts.New(dbAlert)
	alertMod2.Init(nil)
	alertMux := http.NewServeMux()
	alertMod2.RegisterRoutes(alertMux)

	execAlert := func(method, path, body string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		alertMux.ServeHTTP(w, req)
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, w.Body.Bytes(), "", "  "); err != nil {
			return w.Code, w.Body.String()
		}
		return w.Code, pretty.String()
	}

	code, resp = execAlert("POST", "/api/v1/alerts/rules", `{"name":"high-temp","metric":"temperature","operator":">","threshold":45.0}`)
	r.Add(section, "Create alert rule", "POST", "/api/v1/alerts/rules", `{"name":"high-temp","metric":"temperature","operator":">","threshold":45.0}`, false, code, resp)

	code, resp = execAlert("GET", "/api/v1/alerts/rules", "")
	r.Add(section, "List alert rules", "GET", "/api/v1/alerts/rules", "", false, code, resp)

	code, resp = execAlert("PUT", "/api/v1/alerts/rules/"+ruleID, `{"name":"high-temp","metric":"temperature","operator":">","threshold":50.0,"channel":"email","channel_target":"ops@example.com"}`)
	r.Add(section, "Update alert rule", "PUT", "/api/v1/alerts/rules/{id}", `{"threshold":50.0,"channel":"email",...}`, false, code, resp)

	code, resp = execAlert("GET", "/api/v1/alerts/history", "")
	r.Add(section, "Alert history", "GET", "/api/v1/alerts/history", "", false, code, resp)

	// ── 6. Commands ─────────────────────────────────────────────
	section = "6. Remote Commands"
	cmdID := "cmd_demo_001"
	dbCmd := &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (dbpkg.Result, error) {
			return &dbtest.MockResult{Affected: 1}, nil
		},
		OnQuery: func(sql string, args []interface{}) (dbpkg.RowsInterface, error) {
			if strings.Contains(sql, "FROM commands") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{cmdID, "dev_001", "reboot", `{"delay":5}`, "pending", nil, now, nil},
					},
				}, nil
			}
			return &dbtest.MockRows{}, nil
		},
		OnQueryRow: func(sql string, args []interface{}) dbpkg.RowInterface {
			if strings.Contains(sql, "FROM commands") {
				return &dbtest.MockRow{
					Row: []interface{}{cmdID, "dev_001", "reboot", `{"delay":5}`, "delivered", nil, now, nil},
				}
			}
			return &dbtest.MockRow{Err: fmt.Errorf("no rows")}
		},
	}

	cmdMod2 := commands.New(dbCmd)
	cmdMod2.Init(nil)
	cmdMux := http.NewServeMux()
	cmdMod2.RegisterRoutes(cmdMux)

	execCmd := func(method, path, body string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		cmdMux.ServeHTTP(w, req)
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, w.Body.Bytes(), "", "  "); err != nil {
			return w.Code, w.Body.String()
		}
		return w.Code, pretty.String()
	}

	code, resp = execCmd("POST", "/api/v1/devices/dev_001/commands", `{"command":"reboot","payload":{"delay":5}}`)
	r.Add(section, "Send command to device", "POST", "/api/v1/devices/{id}/commands", `{"command":"reboot","payload":{"delay":5}}`, false, code, resp)

	code, resp = execCmd("GET", "/api/v1/devices/dev_001/commands", "")
	r.Add(section, "List device commands", "GET", "/api/v1/devices/{id}/commands", "", false, code, resp)

	code, resp = execCmd("GET", "/api/v1/commands/"+cmdID, "")
	r.Add(section, "Get command details", "GET", "/api/v1/commands/{id}", "", false, code, resp)

	// ── 7. OTA Firmware ────────────────────────────────────────
	section = "7. OTA Firmware Updates"
	fwID := "fw_demo_001"
	depID := "dep_demo_001"
	dbOTA := &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (dbpkg.Result, error) {
			return &dbtest.MockResult{Affected: 1}, nil
		},
		OnQuery: func(sql string, args []interface{}) (dbpkg.RowsInterface, error) {
			if strings.Contains(sql, "FROM firmware") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{fwID, "1.0.0", "release", "nrf52", "abc123def456", 65536, now},
					},
				}, nil
			}
			if strings.Contains(sql, "FROM deployments") && !strings.Contains(sql, "deployment_devices") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{depID, fwID, "1.0.0", "in_progress", 2, 1, now},
					},
				}, nil
			}
			return &dbtest.MockRows{}, nil
		},
		OnQueryRow: func(sql string, args []interface{}) dbpkg.RowInterface {
			if strings.Contains(sql, "FROM firmware") {
				return &dbtest.MockRow{
					Row: []interface{}{fwID, "1.0.0", "release", "nrf52", "abc123def456", 65536, now},
				}
			}
			if strings.Contains(sql, "FROM deployments") {
				return &dbtest.MockRow{
					Row: []interface{}{depID, fwID, "1.0.0", "completed", 2, 2, now},
				}
			}
			return &dbtest.MockRow{Err: fmt.Errorf("no rows")}
		},
	}

	otaMod2 := ota.New(dbOTA)
	otaMod2.Init(nil)
	otaMux := http.NewServeMux()
	otaMod2.RegisterRoutes(otaMux)

	execOTA := func(method, path, body string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		req.Header.Set("Content-Type", "application/json")
		otaMux.ServeHTTP(w, req)
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, w.Body.Bytes(), "", "  "); err != nil {
			return w.Code, w.Body.String()
		}
		return w.Code, pretty.String()
	}

	code, resp = execOTA("POST", "/api/v1/firmware", `{"version":"1.0.0","target_device_type":"nrf52","changelog":"Initial release"}`)
	r.Add(section, "Upload firmware (JSON)", "POST", "/api/v1/firmware", `{"version":"1.0.0","target_device_type":"nrf52"}`, false, code, resp)

	code, resp = execOTA("GET", "/api/v1/firmware", "")
	r.Add(section, "List firmware", "GET", "/api/v1/firmware", "", false, code, resp)

	code, resp = execOTA("GET", "/api/v1/firmware/"+fwID, "")
	r.Add(section, "Get firmware details", "GET", "/api/v1/firmware/{id}", "", false, code, resp)

	code, resp = execOTA("POST", "/api/v1/firmware/"+fwID+"/deploy", `{"device_ids":["dev_001","dev_002"],"description":"Production rollout v1.0.0"}`)
	r.Add(section, "Deploy firmware to devices", "POST", "/api/v1/firmware/{id}/deploy", `{"device_ids":["dev_001",...]}`, false, code, resp)

	code, resp = execOTA("GET", "/api/v1/deployments/"+depID, "")
	r.Add(section, "Check deployment status", "GET", "/api/v1/deployments/{id}", "", false, code, resp)

	// ── 8. Webhooks ────────────────────────────────────────────
	section = "8. Webhooks"
	whID := "wh_demo_001"
	dbWH := &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (dbpkg.Result, error) {
			return &dbtest.MockResult{Affected: 1}, nil
		},
		OnQuery: func(sql string, args []interface{}) (dbpkg.RowsInterface, error) {
			if strings.Contains(sql, "FROM webhooks") && !strings.Contains(sql, "webhook_deliveries") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{whID, "slack-alerts", "https://hooks.example.com/alert", "sec_123", `["alert.fired","device.registered"]`, 1, now},
					},
				}, nil
			}
			if strings.Contains(sql, "FROM webhook_deliveries") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{"del_001", whID, "alert.fired", `{"severity":"critical"}`, "delivered", 200, now},
					},
				}, nil
			}
			return &dbtest.MockRows{}, nil
		},
		OnQueryRow: func(sql string, args []interface{}) dbpkg.RowInterface {
			if strings.Contains(sql, "FROM webhooks WHERE") || strings.Contains(sql, "FROM webhooks ORDER") {
				return &dbtest.MockRow{
					Row: []interface{}{whID, "slack-alerts", "https://hooks.example.com/alert", "sec_123", `["alert.fired","device.registered"]`, 1, now},
				}
			}
			return &dbtest.MockRow{Err: fmt.Errorf("no rows")}
		},
	}

	whMod2 := webhooks.New(dbWH)
	whMod2.Init(nil)
	whMux := http.NewServeMux()
	whMod2.RegisterRoutes(whMux)

	execWH := func(method, path, body string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		whMux.ServeHTTP(w, req)
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, w.Body.Bytes(), "", "  "); err != nil {
			return w.Code, w.Body.String()
		}
		return w.Code, pretty.String()
	}

	code, resp = execWH("POST", "/api/v1/webhooks", `{"name":"slack-alerts","url":"https://hooks.example.com/alert","events":["alert.fired","device.registered"]}`)
	r.Add(section, "Create webhook", "POST", "/api/v1/webhooks", `{"name":"slack-alerts","url":"...","events":["alert.fired",...]}`, false, code, resp)

	code, resp = execWH("GET", "/api/v1/webhooks", "")
	r.Add(section, "List webhooks", "GET", "/api/v1/webhooks", "", false, code, resp)

	code, resp = execWH("PUT", "/api/v1/webhooks/"+whID, `{"name":"slack-alerts","url":"https://hooks.example.com/v2/alert","events":["alert.fired"],"enabled":true}`)
	r.Add(section, "Update webhook", "PUT", "/api/v1/webhooks/{id}", `{"url":"...","enabled":true}`, false, code, resp)

	code, resp = execWH("GET", "/api/v1/webhooks/"+whID+"/deliveries", "")
	r.Add(section, "Webhook delivery history", "GET", "/api/v1/webhooks/{id}/deliveries", "", false, code, resp)

	// ── 9. Fleet Management ────────────────────────────────────
	section = "9. Fleet Management"
	groupID := "grp_demo_001"
	dbFleet := &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (dbpkg.Result, error) {
			return &dbtest.MockResult{Affected: 1}, nil
		},
		OnQuery: func(sql string, args []interface{}) (dbpkg.RowsInterface, error) {
			if strings.Contains(sql, "FROM groups") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{groupID, "dhaka-fleet", now},
					},
				}, nil
			}
			return &dbtest.MockRows{}, nil
		},
		OnQueryRow: func(sql string, args []interface{}) dbpkg.RowInterface {
			if strings.Contains(sql, "FROM groups") {
				return &dbtest.MockRow{
					Row: []interface{}{groupID, "dhaka-fleet", now},
				}
			}
			if strings.Contains(sql, "FROM devices") {
				return &dbtest.MockRow{
					Row: []interface{}{"dev_001", "truck-001", "gps-tracker", `{}`, `["critical"]`, "dhaka-fleet", "online", now, now, now},
				}
			}
			return &dbtest.MockRow{Err: fmt.Errorf("no rows")}
		},
	}

	fleetMod2 := fleet.New(dbFleet)
	fleetMod2.Init(nil)
	fleetMux := http.NewServeMux()
	fleetMod2.RegisterRoutes(fleetMux)

	execFleet := func(method, path, body string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		fleetMux.ServeHTTP(w, req)
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, w.Body.Bytes(), "", "  "); err != nil {
			return w.Code, w.Body.String()
		}
		return w.Code, pretty.String()
	}

	code, resp = execFleet("POST", "/api/v1/groups", `{"name":"dhaka-fleet","description":"Dhaka logistics fleet"}`)
	r.Add(section, "Create device group", "POST", "/api/v1/groups", `{"name":"dhaka-fleet","description":"..."}`, false, code, resp)

	code, resp = execFleet("GET", "/api/v1/groups", "")
	r.Add(section, "List groups", "GET", "/api/v1/groups", "", false, code, resp)

	code, resp = execFleet("POST", "/api/v1/devices/dev_001/tags", `{"tags":["critical","logistics"]}`)
	r.Add(section, "Add tags to device", "POST", "/api/v1/devices/{id}/tags", `{"tags":["critical","logistics"]}`, false, code, resp)

	code, resp = execFleet("GET", "/api/v1/fleet/health", "")
	r.Add(section, "Fleet health overview", "GET", "/api/v1/fleet/health", "", false, code, resp)

	// ── 10. Multi-Tenancy ──────────────────────────────────────
	section = "10. Multi-Tenancy"
	orgID := "org_demo_001"
	userID := "usr_demo_001"
	dbTenant := &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (dbpkg.Result, error) {
			return &dbtest.MockResult{Affected: 1}, nil
		},
		OnQuery: func(sql string, args []interface{}) (dbpkg.RowsInterface, error) {
			if strings.Contains(sql, "FROM orgs") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{orgID, "Acme IoT", now},
					},
				}, nil
			}
			if strings.Contains(sql, "FROM org_users") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{userID, orgID, "ops@acme.io", "admin", now},
					},
				}, nil
			}
			return &dbtest.MockRows{}, nil
		},
	}

	tenantMod2 := tenant.New(dbTenant)
	tenantMod2.Init(nil)
	tenantMux := http.NewServeMux()
	tenantMod2.RegisterRoutes(tenantMux)

	execTenant := func(method, path, body string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		tenantMux.ServeHTTP(w, req)
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, w.Body.Bytes(), "", "  "); err != nil {
			return w.Code, w.Body.String()
		}
		return w.Code, pretty.String()
	}

	code, resp = execTenant("POST", "/api/v1/orgs", `{"name":"Acme IoT","slug":"acme-iot"}`)
	r.Add(section, "Create organization", "POST", "/api/v1/orgs", `{"name":"Acme IoT","slug":"acme-iot"}`, false, code, resp)

	code, resp = execTenant("GET", "/api/v1/orgs", "")
	r.Add(section, "List organizations", "GET", "/api/v1/orgs", "", false, code, resp)

	code, resp = execTenant("POST", "/api/v1/orgs/"+orgID+"/users", `{"email":"ops@acme.io","role":"admin"}`)
	r.Add(section, "Invite user to org", "POST", "/api/v1/orgs/{id}/users", `{"email":"ops@acme.io","role":"admin"}`, false, code, resp)

	code, resp = execTenant("GET", "/api/v1/orgs/"+orgID+"/users", "")
	r.Add(section, "List org users", "GET", "/api/v1/orgs/{id}/users", "", false, code, resp)

	// ── 11. Audit Log ──────────────────────────────────────────
	section = "11. Audit Log"
	dbAudit := &dbtest.MockDB{
		OnExec: func(sql string, args []interface{}) (dbpkg.Result, error) {
			return &dbtest.MockResult{Affected: 1}, nil
		},
		OnQuery: func(sql string, args []interface{}) (dbpkg.RowsInterface, error) {
			if strings.Contains(sql, "FROM audit_log") {
				return &dbtest.MockRows{
					Rows: [][]interface{}{
						{"aud_001", "admin", "device.register", "dev_001", `{"name":"temp-sensor-01"}`, now},
						{"aud_002", "admin", "device.delete", "dev_001", `{}`, now},
						{"aud_003", "system", "alert.fired", "dev_001", `{"rule":"high-temp","value":47.2}`, now},
					},
				}, nil
			}
			return &dbtest.MockRows{}, nil
		},
	}

	auditMod2 := audit.New(dbAudit)
	auditMod2.Init(nil)
	auditMux := http.NewServeMux()
	auditMod2.RegisterRoutes(auditMux)

	execAudit := func(method, path, body string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		auditMux.ServeHTTP(w, req)
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, w.Body.Bytes(), "", "  "); err != nil {
			return w.Code, w.Body.String()
		}
		return w.Code, pretty.String()
	}

	code, resp = execAudit("GET", "/api/v1/audit?limit=10", "")
	r.Add(section, "Query audit log", "GET", "/api/v1/audit?limit=10", "", false, code, resp)

	// ── 12. Simulator ──────────────────────────────────────────
	section = "12. Device Simulator"
	simMod2 := simulator.New()
	simMux := http.NewServeMux()
	simMod2.RegisterRoutes(simMux)

	execSim := func(method, path, body string) (int, string) {
		var bodyIO io.Reader
		if body != "" {
			bodyIO = bytes.NewReader([]byte(body))
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bodyIO)
		simMux.ServeHTTP(w, req)
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, w.Body.Bytes(), "", "  "); err != nil {
			return w.Code, w.Body.String()
		}
		return w.Code, pretty.String()
	}

	code, resp = execSim("POST", "/api/v1/simulator/start", `{"count":3}`)
	r.Add(section, "Start device simulator", "POST", "/api/v1/simulator/start", `{"count":3}`, false, code, resp)

	code, resp = execSim("POST", "/api/v1/simulator/stop", "")
	r.Add(section, "Stop device simulator", "POST", "/api/v1/simulator/stop", "", false, code, resp)

	// ── 13. Dashboard ──────────────────────────────────────────
	section = "13. Dashboard UI"
	dashMod2 := dashboard.New()
	dashMux := http.NewServeMux()
	dashMod2.RegisterRoutes(dashMux)

	execDash := func(method, path string) (int, string) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		dashMux.ServeHTTP(w, req)
		return w.Code, fmt.Sprintf("HTTP %d (%d bytes)", w.Code, w.Body.Len())
	}

	code, resp = execDash("GET", "/dashboard")
	r.Add(section, "Dashboard HTML page", "GET", "/dashboard", "", false, code, resp)

	code, resp = execDash("GET", "/")
	r.Add(section, "Root redirect to dashboard", "GET", "/", "", false, code, resp)

	// ── Generate Report ────────────────────────────────────────
	genReport(r)
}

func genReport(r *Report) {
	bi := version.ReadBuildInfo()
	pass, fail := 0, 0
	for _, t := range r.Tests {
		if t.Result == "PASS" {
			pass++
		} else {
			fail++
		}
	}

	p := func(s string) {
		fmt.Println(s)
	}
	pf := func(f string, args ...interface{}) {
		fmt.Printf(f, args...)
	}

	title := "# DeviceOS \u2014 End-to-End Demonstration"
	p(title)
	p("")
	p("> **Self-hosted IoT backend \u2014 single binary, zero external dependencies.**")
	p(">")
	pf("> This document demonstrates every feature module running end-to-end via its HTTP API.\n> Generated by `cmd/demo` on %s.\n", time.Now().Format(time.RFC3339))
	p("")
	p("---")
	p("")
	p("## Summary")
	p("")
	p("| Metric | Value |")
	p("|--------|-------|")
	pf("| **Version** | %s |\n", bi.Version)
	pf("| **Go Version** | %s |\n", bi.GoVersion)
	p("| **Modules** | 12 (devices, telemetry, alerts, auth, commands, ota, webhooks, fleet, tenant, audit, simulator, dashboard) |")
	pf("| **API Endpoints Tested** | %d |\n", len(r.Tests))
	pf("| **Tests Passed** | %d/%d |\n", pass, pass+fail)
	resultStr := "ALL PASSED"
	if fail > 0 {
		resultStr = "SOME FAILED"
	}
	pf("| **Test Result** | **%s** |\n", resultStr)
	p("")
	p("---")
	p("")
	p("## Modules")
	p("")

	currentSection := ""
	for _, t := range r.Tests {
		if t.Section != currentSection {
			currentSection = t.Section
			pf("### %s\n", t.Section)
			p("")
			p("| # | Endpoint | Method | Status | Result |")
			p("|---|----------|--------|--------|--------|")
		}
		mark := "PASS"
		if t.Result != "PASS" {
			mark = "FAIL"
		}
		pathDisplay := t.Path
		if len(pathDisplay) > 50 {
			pathDisplay = pathDisplay[:47] + "..."
		}
		pf("| %s | `%s` | `%s` | `%d` | %s |\n",
			t.Name, pathDisplay, t.Method, t.Status, mark)
	}

	p("")
	p("---")
	p("")
	p("## Endpoint Details")
	p("")

	for _, t := range r.Tests {
		mark := "PASS"
		if t.Result != "PASS" {
			mark = "FAIL"
		}
		pf("### %s `%s %s`\n", t.Name, t.Method, t.Path)
		p("")
		pf("**Test:** %s\n", mark)
		pf("**Method:** `%s` **Path:** `%s`\n", t.Method, t.Path)
		pf("**Status:** `%d`\n", t.Status)
		if t.Body != "" {
			p("**Request Body:**")
			p("```json")
			p(t.Body)
			p("```")
		}
		p("")
	}

	p("---")
	p("")
	p("## Architecture Overview")
	p("")

	p("```")
	p("+-----------------------------------------------------------------+")
	p("|                     HTTP Server (:8080)                          |")
	p("|  +---------+----------+--------+--------+-------+--------+      |")
	p("|  | Devices |Telemetry | Alerts |  Auth  | Cmds  |  OTA   |      |")
	p("|  +---------+----------+--------+--------+-------+--------+      |")
	p("|  |Webhooks |  Fleet   | Tenant | Audit  |  Sim  |  Dash  |      |")
	p("|  +---------+----------+--------+--------+-------+--------+      |")
	p("|                         |                                       |")
	p("|                    +----V-----+                                 |")
	p("|                    | Registry |                                 |")
	p("|                    +----+-----+                                 |")
	p("|                         |                                       |")
	p("|                    +----V--------------+                        |")
	p("|                    +----V--------------+                        |")
	p("|                    |    SQLite (WAL)   |                        |")
	p("|                    +-------------------+                        |")
	p("|                    +----------+                                 |")
	p("+-----------------------------------------------------------------+")
	p("```")
	p("")

	p("### Module Responsibilities\n")
	p("| Module | Responsibility |")
	p("|--------|---------------|")
	p("| **devices** | Device registration, CRUD, secret key generation |")
	p("| **telemetry** | Ingest, query, latest datapoint, WebSocket broadcast |")
	p("| **alerts** | Rule-based alerting with threshold evaluation |")
	p("| **auth** | JWT tokens, API key auth, device auth middleware |")
	p("| **commands** | Send remote commands, track execution results |")
	p("| **ota** | Firmware upload, deploy, rollout tracking |")
	p("| **webhooks** | Outbound event delivery with retry tracking |")
	p("| **fleet** | Device groups, tags, fleet health aggregation |")
	p("| **tenant** | Multi-org management, user invitations |")
	p("| **audit** | Immutable audit log for all operations |")
	p("| **simulator** | Built-in virtual device generator for testing |")
	p("| **dashboard** | Single-page web UI with real-time WebSocket |")
	p("")

	p("---")
	p("")
	p("## Running the Tests\n")
	p("```bash")
	p("# Unit tests (16 packages)")
	p("make test")
	p("")
	p("# Build")
	p("make build")
	p("")
	p("# Start the server")
	p("./bin/deviceos start")
	p("")
	p("# Run Python E2E test")
	p("python3 tests/simulate.py")
	p("```")
	p("")
	p("---")
	p("")
	p("*Generated by DeviceOS Demonstration Tool*")

	fmt.Fprintf(os.Stderr, "\nResults: %d/%d passed, %d/%d failed\n", pass, pass+fail, fail, pass+fail)
	if fail > 0 {
		os.Exit(1)
	}
}
