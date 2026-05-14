package registry

import "encoding/json"

type TelemetryHook func(deviceID string, metrics json.RawMessage, metadata json.RawMessage)

type Module interface {
	Name() string
	Init(cfg any) error
	RegisterRoutes(mux any) error
	Start() error
	Stop() error
}

type Registry struct {
	modules        map[string]Module
	order          []string
	telemetryHooks []TelemetryHook
}

func New() *Registry {
	return &Registry{modules: make(map[string]Module)}
}

func (r *Registry) Register(m Module) {
	name := m.Name()
	if _, exists := r.modules[name]; exists {
		panic("module already registered: " + name)
	}
	r.modules[name] = m
	r.order = append(r.order, name)
}

func (r *Registry) Get(name string) Module {
	return r.modules[name]
}

func (r *Registry) InitAll(cfg any) error {
	for _, name := range r.order {
		m := r.modules[name]
		if err := m.Init(cfg); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) RegisterAllRoutes(mux any) error {
	for _, name := range r.order {
		m := r.modules[name]
		if err := m.RegisterRoutes(mux); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) StartAll() error {
	for _, name := range r.order {
		m := r.modules[name]
		if err := m.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) OnTelemetry(fn TelemetryHook) {
	r.telemetryHooks = append(r.telemetryHooks, fn)
}

func (r *Registry) TriggerTelemetry(deviceID string, metrics, metadata json.RawMessage) {
	for _, fn := range r.telemetryHooks {
		fn(deviceID, metrics, metadata)
	}
}

func (r *Registry) Names() []string {
	names := make([]string, len(r.order))
	copy(names, r.order)
	return names
}

func (r *Registry) StopAll() {
	for i := len(r.order) - 1; i >= 0; i-- {
		r.modules[r.order[i]].Stop()
	}
}
