package telemetry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/sparkdbtest"
)

func TestTelemetry_WebSocket_ConnectAndBroadcast(t *testing.T) {
	m := &Module{
		db: &sparkdbtest.MockDB{
			OnExec: func(sql string, args []interface{}) (sparkdb.Result, error) {
				return &sparkdbtest.MockResult{}, nil
			},
		},
		hub: NewHub(),
	}
	// Start WS server
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.handleWS(w, r)
	}))
	defer wsSrv.Close()

	url := "ws" + strings.TrimPrefix(wsSrv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Wait for connection to register
	time.Sleep(10 * time.Millisecond)

	// Ingest telemetry — this triggers broadcast to WS clients
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/telemetry", bytes.NewReader([]byte(
		`{"device_id":"dev_001","metrics":{"temp":25.5}}`,
	)))
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("ingest: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Should receive broadcast on WS
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var event map[string]any
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("json: %v (msg: %s)", err, string(msg))
	}
	if event["event"] != "telemetry" {
		t.Fatalf("expected telemetry event, got %v", event["event"])
	}
}

func TestTelemetry_WebSocket_CloseOnDisconnect(t *testing.T) {
	m := &Module{hub: NewHub()}
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.handleWS(w, r)
	}))
	defer wsSrv.Close()

	url := "ws" + strings.TrimPrefix(wsSrv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Close from client side
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	conn.Close()

	// Give handler time to process
	time.Sleep(20 * time.Millisecond)
}

func TestHub_BroadcastToMultiple(t *testing.T) {
	h := NewHub()
	msg := []byte(`{"test":true}`)
	h.Broadcast(msg) // no clients, shouldn't panic
}
