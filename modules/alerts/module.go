package alerts

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
)

type Module struct {
	db sparkdb.DBClient
}

func New(db sparkdb.DBClient) *Module {
	return &Module{db: db}
}

func (m *Module) Name() string { return "alerts" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("alerts_v1", migrations); err != nil {
		return fmt.Errorf("alerts: migrate: %w", err)
	}
	slog.Info("alerts module initialized")
	return nil
}

func (m *Module) RegisterRoutes(mux any) error {
	r, ok := mux.(*http.ServeMux)
	if !ok {
		return fmt.Errorf("alerts: unexpected mux type")
	}
	r.HandleFunc("POST /api/v1/alerts/rules", m.handleCreateRule)
	r.HandleFunc("GET /api/v1/alerts/rules", m.handleListRules)
	r.HandleFunc("PUT /api/v1/alerts/rules/{id}", m.handleUpdateRule)
	r.HandleFunc("DELETE /api/v1/alerts/rules/{id}", m.handleDeleteRule)
	r.HandleFunc("GET /api/v1/alerts/history", m.handleHistory)
	return nil
}

func (m *Module) Start() error { return nil }
func (m *Module) Stop() error  { return nil }

type Rule struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Metric            string    `json:"metric"`
	Operator          string    `json:"operator"`
	Threshold         float64   `json:"threshold"`
	Duration          string    `json:"duration,omitempty"`
	Channel           string    `json:"channel"`
	ChannelTarget     string    `json:"channel_target"`
	Enabled           bool      `json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
}

type AlertEvent struct {
	ID        string    `json:"id"`
	RuleID    string    `json:"rule_id"`
	RuleName  string    `json:"rule_name"`
	DeviceID  string    `json:"device_id"`
	Metric    string    `json:"metric"`
	Value     float64   `json:"value"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`
	CreatedAt time.Time `json:"created_at"`
}

func (m *Module) OnTelemetry(deviceID string, metrics json.RawMessage, metadata json.RawMessage) {
	var parsed map[string]float64
	if err := json.Unmarshal(metrics, &parsed); err != nil {
		return
	}
	fired := m.Evaluate(deviceID, parsed)
	for _, evt := range fired {
		slog.Warn("alert fired", "rule", evt.RuleName, "device", evt.DeviceID, "message", evt.Message)
	}
}

func (m *Module) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var rule Rule
	rule.Enabled = true
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if rule.Name == "" || rule.Metric == "" || rule.Operator == "" {
		http.Error(w, `{"error":"name, metric, and operator are required"}`, http.StatusBadRequest)
		return
	}
	if rule.Operator != ">" && rule.Operator != "<" && rule.Operator != ">=" && rule.Operator != "<=" && rule.Operator != "==" {
		http.Error(w, `{"error":"operator must be one of: >, <, >=, <=, =="}`, http.StatusBadRequest)
		return
	}
	if rule.Channel == "" {
		rule.Channel = "log"
	}
	rule.ID = fmt.Sprintf("rule_%d", time.Now().UnixNano())
	rule.CreatedAt = time.Now()

	_, err := m.db.Exec(
		`INSERT INTO alert_rules (id, name, metric, operator, threshold, duration, channel, channel_target, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.Name, rule.Metric, rule.Operator, rule.Threshold, rule.Duration,
		rule.Channel, rule.ChannelTarget, rule.Enabled, rule.CreatedAt,
	)
	if err != nil {
		slog.Error("create rule", "error", err)
		http.Error(w, `{"error":"failed to create rule"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (m *Module) handleListRules(w http.ResponseWriter, r *http.Request) {
	rows, err := m.db.Query(
		`SELECT id, name, metric, operator, threshold, duration, channel, channel_target, enabled, created_at
		 FROM alert_rules ORDER BY created_at DESC`,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var rule Rule
		rows.Scan(&rule.ID, &rule.Name, &rule.Metric, &rule.Operator, &rule.Threshold,
			&rule.Duration, &rule.Channel, &rule.ChannelTarget, &rule.Enabled, &rule.CreatedAt)
		rules = append(rules, rule)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (m *Module) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var rule Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	res, err := m.db.Exec(
		`UPDATE alert_rules SET name=?, metric=?, operator=?, threshold=?, duration=?, channel=?, channel_target=?, enabled=?
		 WHERE id=?`,
		rule.Name, rule.Metric, rule.Operator, rule.Threshold, rule.Duration,
		rule.Channel, rule.ChannelTarget, rule.Enabled, id,
	)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		http.Error(w, `{"error":"rule not found"}`, http.StatusNotFound)
		return
	}
	rule.ID = id
	writeJSON(w, http.StatusOK, rule)
}

func (m *Module) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := m.db.Exec(`DELETE FROM alert_rules WHERE id = ?`, id)
	if err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		http.Error(w, `{"error":"rule not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) handleHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := m.db.Query(
		`SELECT id, rule_id, rule_name, device_id, metric, value, message, severity, created_at
		 FROM alert_events ORDER BY created_at DESC LIMIT 100`,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []AlertEvent
	for rows.Next() {
		var e AlertEvent
		rows.Scan(&e.ID, &e.RuleID, &e.RuleName, &e.DeviceID, &e.Metric, &e.Value, &e.Message, &e.Severity, &e.CreatedAt)
		events = append(events, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
