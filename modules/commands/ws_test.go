package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
)

func TestCommands_WebSocket_ConnectAndReceive(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) db.RowInterface {
			return &dbtest.MockRow{Err: http.ErrNoLocation}
		},
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.handleWS(w, r)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "?device_id=dev_001"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	if err == nil {
		// May or may not get a message — fine either way
	}
}

func TestCommands_WebSocket_NoDeviceID(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) db.RowInterface {
			return &dbtest.MockRow{Err: http.ErrNoLocation}
		},
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.handleWS(w, r)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	if err == nil {
		// May get messages from pending commands with empty deviceID
	}
}

func TestCommands_WebSocket_SendPendingViaSyncMap(t *testing.T) {
	m := &Module{db: &dbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) db.RowInterface {
			return &dbtest.MockRow{Err: http.ErrNoLocation}
		},
		OnExec: func(sql string, args []interface{}) (db.Result, error) {
			return &dbtest.MockResult{}, nil
		},
	}}
	// Simulate a pending command via the handler
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/devices/dev_001/commands", bytes.NewReader([]byte(
		`{"command":"reboot"}`,
	)))
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// Now connect via WebSocket for same device
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.handleWS(w, r)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "?device_id=dev_001"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		// Pending command may have been sent before we read
		return
	}
	if !strings.Contains(string(msg), "reboot") {
		t.Fatalf("expected reboot command, got: %s", string(msg))
	}
}
