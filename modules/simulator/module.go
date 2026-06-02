package simulator

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lohtbrok/deviceos/internal/httperr"
)

type Module struct {
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	devices []SimDevice
}

type SimDevice struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	TempBase  float64 `json:"temp_base"`
	HumidBase float64 `json:"humid_base"`
	Battery   float64 `json:"battery"`
	Connected bool    `json:"connected"`
}

func New() *Module {
	return &Module{}
}

func (m *Module) Name() string { return "simulator" }

func (m *Module) Init(cfg any) error {
	slog.Info("simulator module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("simulator: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/simulator/start", m.handleStart)
	r.HandleFunc("POST /api/v1/simulator/stop", m.handleStop)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		close(m.stopCh)
		m.running = false
	}
	return nil
}

type StartRequest struct {
	Count int `json:"count"`
}

func (m *Module) handleStart(w http.ResponseWriter, r *http.Request) {
	var req StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Count = 3
	}
	if req.Count <= 0 {
		req.Count = 3
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		httperr.Conflict(w, "simulator already running")
		return
	}

	m.devices = nil
	for i := 0; i < req.Count; i++ {
		dev := SimDevice{
			ID:        fmt.Sprintf("sim_%s_%d", randomName(), i),
			Name:      fmt.Sprintf("sim-%s-%d", randomName(), i),
			Type:      []string{"temp-sensor", "gps-tracker", "multi-sensor"}[i%3],
			TempBase:  20 + rand.Float64()*15,
			HumidBase: 40 + rand.Float64()*30,
			Battery:   80 + rand.Float64()*20,
			Connected: true,
		}
		m.devices = append(m.devices, dev)

		// Register each simulated device
		regReq := fmt.Sprintf(`{"name":"%s","type":"%s","metadata":{"simulated":true}}`, dev.Name, dev.Type)
		resp, err := http.Post("http://localhost:8080/api/v1/devices",
			"application/json", strings.NewReader(regReq))
		if err == nil {
			var result map[string]any
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()
			if d, ok := result["device"].(map[string]any); ok {
				if id, ok := d["id"].(string); ok {
					m.devices[i].ID = id
				}
			}
		}
	}

	m.stopCh = make(chan struct{})
	m.running = true
	go m.run()

	slog.Info("simulator started", "count", req.Count)
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "started", "count": req.Count, "devices": m.devices,
	})
}

func (m *Module) handleStop(w http.ResponseWriter, r *http.Request) {
	m.Stop()
	writeJSON(w, http.StatusOK, map[string]any{"status": "stopped"})
}

var simTickInterval = 5 * time.Second

func (m *Module) run() {
	ticker := time.NewTicker(simTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.mu.Lock()
			for i := range m.devices {
				d := &m.devices[i]
				if !d.Connected {
					if rand.Float64() < 0.1 {
						d.Connected = true
					}
					continue
				}

				// Simulate network flakiness
				if rand.Float64() < 0.05 {
					d.Connected = false
					continue
				}

				temp := d.TempBase + math.Sin(float64(time.Now().Unix()%60)/10)*5 + rand.Float64()*2
				humid := d.HumidBase + rand.Float64()*5 - 2.5
				d.Battery -= rand.Float64() * 0.5
				if d.Battery < 0 {
					d.Battery = 0
				}

				metrics := map[string]float64{
					"temperature": math.Round(temp*10) / 10,
					"humidity":    math.Round(humid*10) / 10,
					"battery":     math.Round(d.Battery*10) / 10,
				}
				payload, _ := json.Marshal(map[string]any{
					"device_id": d.ID,
					"metrics":   metrics,
				})

				http.Post("http://localhost:8080/api/v1/telemetry",
					"application/json", strings.NewReader(string(payload)))
			}
			m.mu.Unlock()
		}
	}
}

func randomName() string {
	names := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel",
		"india", "juliett", "kilo", "lima", "mike", "november", "oscar", "papa"}
	return names[rand.Intn(len(names))]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
