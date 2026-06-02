package events

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestParseEventsParam_Empty(t *testing.T) {
	result := parseEventsParam("")
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestParseEventsParam_Single(t *testing.T) {
	result := parseEventsParam("telemetry")
	if len(result) != 1 || result[0] != "telemetry" {
		t.Fatalf("expected [telemetry], got %v", result)
	}
}

func TestParseEventsParam_Multiple(t *testing.T) {
	result := parseEventsParam("telemetry,alerts,devices")
	if len(result) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(result), result)
	}
	if result[0] != "telemetry" || result[1] != "alerts" || result[2] != "devices" {
		t.Fatalf("unexpected result: %v", result)
	}
}

func TestParseEventsParam_TrailingComma(t *testing.T) {
	result := parseEventsParam("telemetry,")
	if len(result) != 1 || result[0] != "telemetry" {
		t.Fatalf("expected [telemetry], got %v", result)
	}
}

func TestSubscriber_WantsAll(t *testing.T) {
	s := &subscriber{events: nil}
	if !s.wants("telemetry") {
		t.Fatal("expected subscriber to want all events")
	}
	if !s.wants("alerts") {
		t.Fatal("expected subscriber to want all events")
	}
}

func TestSubscriber_WantsSpecific(t *testing.T) {
	s := &subscriber{events: map[string]bool{"telemetry": true}}
	if !s.wants("telemetry") {
		t.Fatal("expected subscriber to want telemetry")
	}
	if s.wants("alerts") {
		t.Fatal("expected subscriber to not want alerts")
	}
}

func TestSubscriber_WantsEmptyMap(t *testing.T) {
	s := &subscriber{events: map[string]bool{}}
	if s.wants("telemetry") {
		t.Fatal("expected subscriber with empty map to want nothing")
	}
}

func TestHub_Subscribe_Unsubscribe_Len(t *testing.T) {
	hub := NewHub()
	if hub.Len() != 0 {
		t.Fatalf("expected 0, got %d", hub.Len())
	}

	client, srv := wsPair(t)
	defer client.Close()

	sub := hub.Subscribe(srv, []string{"telemetry"})
	if hub.Len() != 1 {
		t.Fatalf("expected 1, got %d", hub.Len())
	}
	if sub == nil {
		t.Fatal("expected non-nil subscriber")
	}
	if !sub.wants("telemetry") {
		t.Fatal("subscriber should want telemetry")
	}

	hub.Unsubscribe(srv)
	if hub.Len() != 0 {
		t.Fatalf("expected 0 after unsubscribe, got %d", hub.Len())
	}
}

func TestHub_Publish_DeliversToAllSubscribers(t *testing.T) {
	hub := NewHub()

	client1, srv1 := wsPair(t)
	defer client1.Close()

	client2, srv2 := wsPair(t)
	defer client2.Close()

	hub.Subscribe(srv1, nil)
	hub.Subscribe(srv2, nil)

	hub.Publish(Event{Type: "telemetry", Data: "hello"})

	assertWSMessage(t, client1, "telemetry")
	assertWSMessage(t, client2, "telemetry")
}

func TestHub_Publish_FiltersByType(t *testing.T) {
	hub := NewHub()

	client, srv := wsPair(t)
	defer client.Close()

	hub.Subscribe(srv, []string{"alerts"})

	hub.Publish(Event{Type: "telemetry", Data: "ignored"})

	client.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, _, err := client.ReadMessage()
	if err == nil {
		t.Fatal("client should NOT receive telemetry event")
	}
}

func TestHub_Publish_TypeFilteredDelivery(t *testing.T) {
	hub := NewHub()

	client, srv := wsPair(t)
	defer client.Close()

	hub.Subscribe(srv, []string{"alerts"})

	hub.Publish(Event{Type: "alerts", Data: "alert!"})

	assertWSMessage(t, client, "alerts")
}

func TestModule_RegisterRoutes_WS(t *testing.T) {
	m := New()
	mux := http.NewServeMux()
	if err := m.RegisterRoutes(mux); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws/events?events=telemetry"
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal("dial:", err)
	}
	defer client.Close()

	m.Hub().Publish(Event{Type: "telemetry", Data: "hi"})

	assertWSMessage(t, client, "telemetry")
}

func wsPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()

	srvConnCh := make(chan *websocket.Conn, 1)
	var mu sync.Mutex
	var srv *httptest.Server

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Log("upgrade error:", err)
			return
		}
		srvConnCh <- conn
		select {} // hang
	}))
	t.Cleanup(func() {
		mu.Lock()
		if srv != nil {
			srv.Close()
		}
		mu.Unlock()
	})

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal("dial:", err)
	}

	srvConn := <-srvConnCh

	return client, srvConn
}

func assertWSMessage(t *testing.T, conn *websocket.Conn, expectedType string) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var evt Event
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if evt.Type != expectedType {
		t.Fatalf("expected type %q, got %q", expectedType, evt.Type)
	}
}
