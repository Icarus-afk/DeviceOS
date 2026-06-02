package alerts

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/httperr"
)

type Module struct {
	db db.DBClient
}

func New(db db.DBClient) *Module {
	return &Module{db: db}
}

func (m *Module) Name() string { return "alerts" }

func (m *Module) Init(cfg any) error {
	if err := m.db.Migrate("alerts_v1", migrations); err != nil {
		return fmt.Errorf("alerts: migrate: %w", err)
	}
	if err := m.db.Migrate("alerts_v2_org", orgMigration); err != nil {
		return fmt.Errorf("alerts: migrate org: %w", err)
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
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Metric        string    `json:"metric"`
	Operator      string    `json:"operator"`
	Threshold     float64   `json:"threshold"`
	Duration      string    `json:"duration,omitempty"`
	Channel       string    `json:"channel"`
	ChannelTarget string    `json:"channel_target"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
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
		httperr.BadRequest(w, "invalid request body")
		return
	}
	if rule.Name == "" || rule.Metric == "" || rule.Operator == "" {
		httperr.BadRequest(w, "name, metric, and operator are required")
		return
	}
	if rule.Operator != ">" && rule.Operator != "<" && rule.Operator != ">=" && rule.Operator != "<=" && rule.Operator != "==" {
		httperr.BadRequest(w, "operator must be one of: >, <, >=, <=, ==")
		return
	}
	if rule.Channel == "" {
		rule.Channel = "log"
	}
	oid := orgID(r)
	rule.ID = fmt.Sprintf("rule_%d", time.Now().UnixNano())
	rule.CreatedAt = time.Now()

	_, err := m.db.Exec(
		`INSERT INTO alert_rules (id, name, metric, operator, threshold, duration, channel, channel_target, enabled, created_at, org_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.Name, rule.Metric, rule.Operator, rule.Threshold, rule.Duration,
		rule.Channel, rule.ChannelTarget, rule.Enabled, rule.CreatedAt, oid,
	)
	if err != nil {
		slog.Error("create rule", "error", err)
		httperr.Internal(w, "failed to create rule")
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (m *Module) handleListRules(w http.ResponseWriter, r *http.Request) {
	oid := orgID(r)
	q := `SELECT id, name, metric, operator, threshold, duration, channel, channel_target, enabled, created_at
		 FROM alert_rules`
	var args []any
	if oid != "" {
		q += ` WHERE org_id = ?`
		args = append(args, oid)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := m.db.Query(q, args...)
	if err != nil {
		httperr.Internal(w, "query failed")
		return
	}
	defer rows.Close()

	rules := make([]Rule, 0)
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
		httperr.BadRequest(w, "invalid request")
		return
	}

	q := `UPDATE alert_rules SET name=?, metric=?, operator=?, threshold=?, duration=?, channel=?, channel_target=?, enabled=?`
	args := []any{rule.Name, rule.Metric, rule.Operator, rule.Threshold, rule.Duration,
		rule.Channel, rule.ChannelTarget, rule.Enabled}
	oid := orgID(r)
	if oid != "" {
		q += ` WHERE id=? AND org_id=?`
		args = append(args, id, oid)
	} else {
		q += ` WHERE id=?`
		args = append(args, id)
	}
	res, err := m.db.Exec(q, args...)
	if err != nil {
		httperr.Internal(w, "update failed")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		httperr.NotFound(w, "rule not found")
		return
	}
	rule.ID = id
	writeJSON(w, http.StatusOK, rule)
}

func (m *Module) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	oid := orgID(r)
	q := `DELETE FROM alert_rules WHERE id = ?`
	args := []any{id}
	if oid != "" {
		q += ` AND org_id = ?`
		args = append(args, oid)
	}
	res, err := m.db.Exec(q, args...)
	if err != nil {
		httperr.Internal(w, "delete failed")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		httperr.NotFound(w, "rule not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) handleHistory(w http.ResponseWriter, r *http.Request) {
	oid := orgID(r)
	q := `SELECT id, rule_id, rule_name, device_id, metric, value, message, severity, created_at
		 FROM alert_events`
	var args []any
	if oid != "" {
		q += ` WHERE org_id = ?`
		args = append(args, oid)
	}
	q += ` ORDER BY created_at DESC LIMIT 100`
	rows, err := m.db.Query(q, args...)
	if err != nil {
		httperr.Internal(w, "query failed")
		return
	}
	defer rows.Close()

	events := make([]AlertEvent, 0)
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

func orgID(r *http.Request) string {
	return r.Header.Get("X-Org-ID")
}
