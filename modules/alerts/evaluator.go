package alerts

import (
	"fmt"
	"log/slog"
	"time"
)

func (m *Module) Evaluate(deviceID string, metrics map[string]float64) []AlertEvent {
	rules, err := m.loadEnabledRules()
	if err != nil {
		slog.Error("load rules for evaluation", "error", err)
		return nil
	}

	var fired []AlertEvent
	for _, rule := range rules {
		val, exists := metrics[rule.Metric]
		if !exists {
			continue
		}

		if !rule.matches(val) {
			continue
		}

		severity := deriveSeverity(rule.Operator)
		evt := AlertEvent{
			ID:        fmt.Sprintf("alert_%d", time.Now().UnixNano()),
			RuleID:    rule.ID,
			RuleName:  rule.Name,
			DeviceID:  deviceID,
			Metric:    rule.Metric,
			Value:     val,
			Message:   fmt.Sprintf("%s = %v %s %v", rule.Metric, val, rule.Operator, rule.Threshold),
			Severity:  severity,
			CreatedAt: time.Now(),
		}

		m.db.Exec(
			`INSERT INTO alert_events (id, rule_id, rule_name, device_id, metric, value, message, severity, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			evt.ID, evt.RuleID, evt.RuleName, evt.DeviceID, evt.Metric, evt.Value, evt.Message, evt.Severity, evt.CreatedAt,
		)

		fired = append(fired, evt)
	}
	return fired
}

func (m *Module) loadEnabledRules() ([]Rule, error) {
	rows, err := m.db.Query(
		`SELECT id, name, metric, operator, threshold, duration, channel, channel_target, enabled, created_at
		 FROM alert_rules WHERE enabled = 1`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var rule Rule
		rows.Scan(&rule.ID, &rule.Name, &rule.Metric, &rule.Operator, &rule.Threshold,
			&rule.Duration, &rule.Channel, &rule.ChannelTarget, &rule.Enabled, &rule.CreatedAt)
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func deriveSeverity(op string) string {
	switch op {
	case ">", ">=":
		return "critical"
	case "<", "<=":
		return "warning"
	default:
		return "info"
	}
}

func (r *Rule) matches(val float64) bool {
	switch r.Operator {
	case ">":
		return val > r.Threshold
	case "<":
		return val < r.Threshold
	case ">=":
		return val >= r.Threshold
	case "<=":
		return val <= r.Threshold
	case "==":
		return val == r.Threshold
	default:
		return false
	}
}
