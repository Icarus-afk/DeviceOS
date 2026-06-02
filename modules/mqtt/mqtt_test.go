package mqtt

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/dbtest"
)

type mockTelemetryStore struct {
	deviceID string
	orgID    string
	called   bool
}

func (m *mockTelemetryStore) Store(deviceID string, ts time.Time, metrics, metadata json.RawMessage, orgID string) (int64, error) {
	m.called = true
	m.deviceID = deviceID
	m.orgID = orgID
	return 1, nil
}

func TestParseTelemetryTopic_Valid(t *testing.T) {
	deviceID, ok := parseTelemetryTopic("deviceos/dev_abc123/telemetry")
	if !ok {
		t.Fatal("expected valid topic")
	}
	if deviceID != "dev_abc123" {
		t.Fatalf("expected dev_abc123, got %s", deviceID)
	}
}

func TestParseTelemetryTopic_InvalidPrefix(t *testing.T) {
	_, ok := parseTelemetryTopic("other/dev_abc123/telemetry")
	if ok {
		t.Fatal("expected invalid topic")
	}
}

func TestParseTelemetryTopic_InvalidSuffix(t *testing.T) {
	_, ok := parseTelemetryTopic("deviceos/dev_abc123/cmd")
	if ok {
		t.Fatal("expected invalid topic")
	}
}

func TestParseTelemetryTopic_WrongParts(t *testing.T) {
	_, ok := parseTelemetryTopic("deviceos/dev_abc123")
	if ok {
		t.Fatal("expected invalid topic")
	}
}

func TestOnConnectAuthenticate_Success(t *testing.T) {
	h := &deviceAuthHook{
		db: &dbtest.MockDB{
			OnQueryRow: func(sql string, args []interface{}) db.RowInterface {
				return &dbtest.MockRow{
					Row: []interface{}{"supersecret", "org_001"},
				}
			},
		},
		devices: &sync.Map{},
	}

	cl := &mqtt.Client{ID: "client_001"}
	pk := packets.Packet{
		Connect: packets.ConnectParams{
			Username: []byte("dev_001"),
			Password: []byte("supersecret"),
		},
	}

	ok := h.OnConnectAuthenticate(cl, pk)
	if !ok {
		t.Fatal("expected auth success")
	}

	di, loaded := h.devices.Load("client_001")
	if !loaded {
		t.Fatal("expected device info stored")
	}
	info := di.(deviceInfo)
	if info.DeviceID != "dev_001" || info.OrgID != "org_001" {
		t.Fatalf("unexpected device info: %+v", info)
	}
}

func TestOnConnectAuthenticate_WrongPassword(t *testing.T) {
	h := &deviceAuthHook{
		db: &dbtest.MockDB{
			OnQueryRow: func(sql string, args []interface{}) db.RowInterface {
				return &dbtest.MockRow{
					Row: []interface{}{"supersecret", ""},
				}
			},
		},
		devices: &sync.Map{},
	}

	cl := &mqtt.Client{ID: "client_001"}
	pk := packets.Packet{
		Connect: packets.ConnectParams{
			Username: []byte("dev_001"),
			Password: []byte("wrongpass"),
		},
	}

	ok := h.OnConnectAuthenticate(cl, pk)
	if ok {
		t.Fatal("expected auth failure")
	}
}

func TestOnConnectAuthenticate_DeviceNotFound(t *testing.T) {
	h := &deviceAuthHook{
		db: &dbtest.MockDB{
		OnQueryRow: func(sql string, args []interface{}) db.RowInterface {
			return &dbtest.MockRow{Err: fmt.Errorf("device not found")}
		},
		},
		devices: &sync.Map{},
	}

	cl := &mqtt.Client{ID: "client_001"}
	pk := packets.Packet{
		Connect: packets.ConnectParams{
			Username: []byte("nonexistent"),
			Password: []byte("any"),
		},
	}

	ok := h.OnConnectAuthenticate(cl, pk)
	if ok {
		t.Fatal("expected auth failure")
	}
}

func TestOnConnectAuthenticate_EmptyUsername(t *testing.T) {
	h := &deviceAuthHook{devices: &sync.Map{}}
	cl := &mqtt.Client{ID: "client_001"}
	pk := packets.Packet{
		Connect: packets.ConnectParams{
			Username: []byte(""),
			Password: []byte("any"),
		},
	}

	ok := h.OnConnectAuthenticate(cl, pk)
	if ok {
		t.Fatal("expected auth failure for empty username")
	}
}

func TestOnACLCheck_AllowTelemetryPublish(t *testing.T) {
	h := &deviceAuthHook{devices: &sync.Map{}}
	h.devices.Store("client_001", deviceInfo{DeviceID: "dev_001"})

	cl := &mqtt.Client{ID: "client_001"}
	ok := h.OnACLCheck(cl, "deviceos/dev_001/telemetry", true)
	if !ok {
		t.Fatal("expected ACL allow for telemetry publish")
	}
}

func TestOnACLCheck_DenyOtherTopic(t *testing.T) {
	h := &deviceAuthHook{devices: &sync.Map{}}
	h.devices.Store("client_001", deviceInfo{DeviceID: "dev_001"})

	cl := &mqtt.Client{ID: "client_001"}
	ok := h.OnACLCheck(cl, "deviceos/dev_002/telemetry", true)
	if ok {
		t.Fatal("expected ACL deny for other device topic")
	}
}

func TestOnACLCheck_AllowSubscribe(t *testing.T) {
	h := &deviceAuthHook{devices: &sync.Map{}}
	h.devices.Store("client_001", deviceInfo{DeviceID: "dev_001"})

	cl := &mqtt.Client{ID: "client_001"}
	ok := h.OnACLCheck(cl, "deviceos/dev_001/cmd", false)
	if !ok {
		t.Fatal("expected ACL allow for subscribe")
	}
}

func TestOnACLCheck_DenyUnauthenticated(t *testing.T) {
	h := &deviceAuthHook{devices: &sync.Map{}}

	cl := &mqtt.Client{ID: "client_001"}
	ok := h.OnACLCheck(cl, "deviceos/dev_001/telemetry", true)
	if ok {
		t.Fatal("expected ACL deny for unauthenticated client")
	}
}

func TestOnPublished_StoresTelemetry(t *testing.T) {
	store := &mockTelemetryStore{}
	h := &deviceAuthHook{
		telemetry: store,
		devices:   &sync.Map{},
	}
	h.devices.Store("client_001", deviceInfo{DeviceID: "dev_001", OrgID: "org_001"})

	cl := &mqtt.Client{ID: "client_001"}
	pk := packets.Packet{
		TopicName: "deviceos/dev_001/telemetry",
		Payload:   []byte(`{"temperature": 31.5}`),
	}

	h.OnPublished(cl, pk)

	if !store.called {
		t.Fatal("expected telemetry store to be called")
	}
	if store.deviceID != "dev_001" {
		t.Fatalf("expected dev_001, got %s", store.deviceID)
	}
	if store.orgID != "org_001" {
		t.Fatalf("expected org_001, got %s", store.orgID)
	}
}

func TestOnPublished_WrongDevice(t *testing.T) {
	store := &mockTelemetryStore{}
	h := &deviceAuthHook{
		telemetry: store,
		devices:   &sync.Map{},
	}
	h.devices.Store("client_001", deviceInfo{DeviceID: "dev_001"})

	cl := &mqtt.Client{ID: "client_001"}
	pk := packets.Packet{
		TopicName: "deviceos/dev_002/telemetry",
		Payload:   []byte(`{"temperature": 31.5}`),
	}

	h.OnPublished(cl, pk)

	if store.called {
		t.Fatal("expected no telemetry store for wrong device")
	}
}

func TestOnPublished_InvalidJSON(t *testing.T) {
	store := &mockTelemetryStore{}
	h := &deviceAuthHook{
		telemetry: store,
		devices:   &sync.Map{},
	}
	h.devices.Store("client_001", deviceInfo{DeviceID: "dev_001"})

	cl := &mqtt.Client{ID: "client_001"}
	pk := packets.Packet{
		TopicName: "deviceos/dev_001/telemetry",
		Payload:   []byte(`not json`),
	}

	h.OnPublished(cl, pk)

	if store.called {
		t.Fatal("expected no telemetry store for invalid JSON")
	}
}

func TestOnPublished_UnrecognizedTopic(t *testing.T) {
	store := &mockTelemetryStore{}
	h := &deviceAuthHook{
		telemetry: store,
		devices:   &sync.Map{},
	}
	h.devices.Store("client_001", deviceInfo{DeviceID: "dev_001"})

	cl := &mqtt.Client{ID: "client_001"}
	pk := packets.Packet{
		TopicName: "deviceos/dev_001/cmd",
		Payload:   []byte(`{"command": "reboot"}`),
	}

	h.OnPublished(cl, pk)

	if store.called {
		t.Fatal("expected no telemetry store for non-telemetry topic")
	}
}

func TestOnDisconnect_CleansDevice(t *testing.T) {
	h := &deviceAuthHook{devices: &sync.Map{}}
	h.devices.Store("client_001", deviceInfo{DeviceID: "dev_001"})

	h.OnDisconnect(&mqtt.Client{ID: "client_001"}, nil, false)

	_, loaded := h.devices.Load("client_001")
	if loaded {
		t.Fatal("expected device info to be cleaned up")
	}
}
