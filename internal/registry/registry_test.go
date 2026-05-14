package registry_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/lohtbrok/deviceos/internal/registry"
)

type mockModule struct {
	name         string
	initErr      error
	routesErr    error
	startErr     error
	stopCalled   bool
	startCalled  bool
	routesCalled bool
}

func (m *mockModule) Name() string              { return m.name }
func (m *mockModule) Init(any) error             { return m.initErr }
func (m *mockModule) RegisterRoutes(any) error   { m.routesCalled = true; return m.routesErr }
func (m *mockModule) Start() error                { m.startCalled = true; return m.startErr }
func (m *mockModule) Stop() error                 { m.stopCalled = true; return nil }

func TestRegisterAndGet(t *testing.T) {
	r := registry.New()
	m := &mockModule{name: "test"}
	r.Register(m)

	if got := r.Get("test"); got != m {
		t.Fatal("expected to get registered module")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate register")
		}
	}()
	r := registry.New()
	r.Register(&mockModule{name: "dup"})
	r.Register(&mockModule{name: "dup"})
}

func TestInitAll(t *testing.T) {
	r := registry.New()
	r.Register(&mockModule{name: "a"})
	r.Register(&mockModule{name: "b"})
	if err := r.InitAll(nil); err != nil {
		t.Fatal(err)
	}
}

func TestInitAllError(t *testing.T) {
	r := registry.New()
	r.Register(&mockModule{name: "a"})
	r.Register(&mockModule{name: "bad", initErr: errors.New("fail")})
	if err := r.InitAll(nil); err == nil {
		t.Fatal("expected init error")
	}
}

func TestRegisterAllRoutes(t *testing.T) {
	r := registry.New()
	m := &mockModule{name: "test"}
	r.Register(m)
	if err := r.RegisterAllRoutes(nil); err != nil {
		t.Fatal(err)
	}
	if !m.routesCalled {
		t.Fatal("routes should have been called")
	}
}

func TestStartAll(t *testing.T) {
	r := registry.New()
	m := &mockModule{name: "test"}
	r.Register(m)
	if err := r.StartAll(); err != nil {
		t.Fatal(err)
	}
	if !m.startCalled {
		t.Fatal("start should have been called")
	}
}

func TestStopAll(t *testing.T) {
	r := registry.New()
	m := &mockModule{name: "test"}
	r.Register(m)
	r.StopAll()
	if !m.stopCalled {
		t.Fatal("stop should have been called")
	}
}

func TestTelemetryHook(t *testing.T) {
	r := registry.New()
	var hookCalled bool
	r.OnTelemetry(func(deviceID string, metrics, metadata json.RawMessage) {
		hookCalled = true
	})
	r.TriggerTelemetry("dev-1", json.RawMessage(`{"temp":25}`), json.RawMessage(`{}`))
	if !hookCalled {
		t.Fatal("telemetry hook should have been called")
	}
}

func TestGetNil(t *testing.T) {
	r := registry.New()
	if m := r.Get("nonexistent"); m != nil {
		t.Fatal("expected nil for nonexistent module")
	}
}

func TestStopAllReverseOrder(t *testing.T) {
	r := registry.New()
	order := []string{"a", "b", "c"}
	for _, name := range order {
		r.Register(&mockModule{name: name})
	}
	r.StopAll()
}

func TestRegisterAllRoutesError(t *testing.T) {
	r := registry.New()
	r.Register(&mockModule{name: "bad", routesErr: errors.New("route fail")})
	if err := r.RegisterAllRoutes(nil); err == nil {
		t.Fatal("expected route error")
	}
}

func TestStartAllError(t *testing.T) {
	r := registry.New()
	r.Register(&mockModule{name: "bad", startErr: errors.New("start fail")})
	if err := r.StartAll(); err == nil {
		t.Fatal("expected start error")
	}
}
