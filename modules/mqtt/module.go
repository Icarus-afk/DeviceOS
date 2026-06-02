package mqtt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/lohtbrok/deviceos/internal/db"
)

const DefaultPort = 1883

type TelemetryStore interface {
	Store(deviceID string, ts time.Time, metrics, metadata json.RawMessage, orgID string) (int64, error)
}

type Config struct {
	Port int
}

type deviceInfo struct {
	DeviceID string
	OrgID    string
}

type Module struct {
	server    *mqtt.Server
	db        db.DBClient
	telemetry TelemetryStore
	cfg       Config
	devices   sync.Map
}

func New(db db.DBClient, telemetry TelemetryStore, cfg Config) *Module {
	return &Module{
		db:        db,
		telemetry: telemetry,
		cfg:       cfg,
	}
}

func (m *Module) Name() string { return "mqtt" }

func (m *Module) Init(cfg any) error {
	opts := &mqtt.Options{
		Capabilities: mqtt.NewDefaultServerCapabilities(),
	}
	srv := mqtt.New(opts)

	hook := &deviceAuthHook{
		db:        m.db,
		telemetry: m.telemetry,
		devices:   &m.devices,
	}
	if err := srv.AddHook(hook, nil); err != nil {
		return fmt.Errorf("mqtt: add hook: %w", err)
	}

	m.server = srv
	slog.Info("mqtt module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("mqtt: unexpected mux type")
	}
	r.HandleFunc("GET /api/v1/mqtt/status", m.handleStatus)
	return nil
}

func (m *Module) Start() error {
	if m.server == nil {
		return fmt.Errorf("mqtt: server not initialized")
	}

	addr := fmt.Sprintf(":%d", m.cfg.Port)
	tcpListener := listeners.NewTCP(listeners.Config{
		ID:      "mqtt-tcp",
		Address: addr,
	})
	if err := m.server.AddListener(tcpListener); err != nil {
		return fmt.Errorf("mqtt: add tcp listener: %w", err)
	}

	slog.Info("mqtt broker starting", "addr", addr)
	go func() {
		if err := m.server.Serve(); err != nil {
			slog.Error("mqtt broker serve error", "error", err)
		}
	}()

	return nil
}

func (m *Module) Stop() error {
	if m.server != nil {
		if err := m.server.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) handleStatus(w http.ResponseWriter, r *http.Request) {
	var connected int
	m.devices.Range(func(_, _ any) bool {
		connected++
		return true
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":       "running",
		"port":         m.cfg.Port,
		"connected":    connected,
	})
}

type deviceAuthHook struct {
	mqtt.HookBase
	db        db.DBClient
	telemetry TelemetryStore
	devices   *sync.Map
}

func (h *deviceAuthHook) ID() string { return "deviceos-auth" }

func (h *deviceAuthHook) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnConnectAuthenticate,
		mqtt.OnACLCheck,
		mqtt.OnPublished,
		mqtt.OnDisconnect,
	}, []byte{b})
}

func (h *deviceAuthHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	username := string(pk.Connect.Username)
	password := string(pk.Connect.Password)

	if username == "" {
		slog.Warn("mqtt auth: empty username")
		return false
	}

	var storedKey, orgID string
	err := h.db.QueryRow(
		`SELECT secret_key, COALESCE(org_id, '') FROM devices WHERE id = ?`, username,
	).Scan(&storedKey, &orgID)
	if err != nil {
		slog.Warn("mqtt auth: device not found", "device_id", username)
		return false
	}

	if storedKey != password {
		slog.Warn("mqtt auth: invalid password", "device_id", username)
		return false
	}

	h.devices.Store(cl.ID, deviceInfo{DeviceID: username, OrgID: orgID})
	slog.Info("mqtt device connected", "device_id", username)
	return true
}

func (h *deviceAuthHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	if di, ok := h.devices.Load(cl.ID); ok {
		info := di.(deviceInfo)
		h.devices.Delete(cl.ID)
		slog.Info("mqtt device disconnected", "device_id", info.DeviceID)
	}
}

func (h *deviceAuthHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	if !write {
		return true
	}

	di, ok := h.devices.Load(cl.ID)
	if !ok {
		return false
	}
	info := di.(deviceInfo)
	expectedTopic := fmt.Sprintf("deviceos/%s/telemetry", info.DeviceID)

	return topic == expectedTopic
}

func (h *deviceAuthHook) OnPublished(cl *mqtt.Client, pk packets.Packet) {
	deviceID, ok := parseTelemetryTopic(pk.TopicName)
	if !ok {
		return
	}

	di, ok := h.devices.Load(cl.ID)
	if !ok {
		return
	}
	info := di.(deviceInfo)

	if info.DeviceID != deviceID {
		slog.Warn("mqtt: device topic mismatch",
			"client", info.DeviceID,
			"topic_device", deviceID,
		)
		return
	}

	var metrics json.RawMessage
	if err := json.Unmarshal(pk.Payload, &metrics); err != nil {
		slog.Warn("mqtt: invalid telemetry JSON",
			"device_id", deviceID,
			"error", err,
		)
		return
	}

	_, err := h.telemetry.Store(deviceID, time.Now(), metrics, nil, info.OrgID)
	if err != nil {
		slog.Error("mqtt: store telemetry",
			"device_id", deviceID,
			"error", err,
		)
	}
}

func parseTelemetryTopic(topic string) (string, bool) {
	parts := strings.Split(topic, "/")
	if len(parts) == 3 && parts[0] == "deviceos" && parts[2] == "telemetry" {
		return parts[1], true
	}
	return "", false
}
