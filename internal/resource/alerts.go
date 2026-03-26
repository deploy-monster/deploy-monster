package resource

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AlertRule defines a threshold-based alert.
type AlertRule struct {
	Name      string        `json:"name"`
	Metric    string        `json:"metric"`   // cpu_percent, ram_percent, disk_percent
	Operator  string        `json:"operator"` // >, <, >=, <=, ==
	Threshold float64       `json:"threshold"`
	Duration  time.Duration `json:"duration"` // How long condition must hold
	Severity  string        `json:"severity"` // info, warning, critical
	Channels  []string      `json:"channels"` // notification channels
}

// AlertState tracks the current state of an alert.
type AlertState struct {
	Rule       *AlertRule
	Firing     bool
	FiredAt    time.Time
	ResolvedAt time.Time
	Value      float64
}

// AlertEngine evaluates metrics against alert rules and fires events.
type AlertEngine struct {
	mu     sync.RWMutex
	rules  []*AlertRule
	states map[string]*AlertState // rule.Name -> state
	events *core.EventBus
	logger *slog.Logger
}

// NewAlertEngine creates a new alert engine.
func NewAlertEngine(events *core.EventBus, logger *slog.Logger) *AlertEngine {
	ae := &AlertEngine{
		states: make(map[string]*AlertState),
		events: events,
		logger: logger,
	}

	// Default alert rules
	ae.AddRule(&AlertRule{
		Name: "high_cpu", Metric: "cpu_percent",
		Operator: ">", Threshold: 90, Duration: 5 * time.Minute,
		Severity: "warning", Channels: []string{"email", "slack"},
	})
	ae.AddRule(&AlertRule{
		Name: "disk_full", Metric: "disk_percent",
		Operator: ">", Threshold: 95, Duration: time.Minute,
		Severity: "critical", Channels: []string{"email", "slack", "telegram"},
	})
	ae.AddRule(&AlertRule{
		Name: "high_memory", Metric: "ram_percent",
		Operator: ">", Threshold: 90, Duration: 5 * time.Minute,
		Severity: "warning", Channels: []string{"email", "slack"},
	})

	return ae
}

// AddRule registers a new alert rule.
func (ae *AlertEngine) AddRule(rule *AlertRule) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	ae.rules = append(ae.rules, rule)
	ae.states[rule.Name] = &AlertState{Rule: rule}
}

// Evaluate checks all rules against current metrics.
func (ae *AlertEngine) Evaluate(ctx context.Context, metrics *core.ServerMetrics) {
	ae.mu.RLock()
	defer ae.mu.RUnlock()

	for _, rule := range ae.rules {
		value := extractMetricValue(rule.Metric, metrics)
		triggered := evaluateCondition(value, rule.Operator, rule.Threshold)

		state := ae.states[rule.Name]
		if state == nil {
			continue
		}

		if triggered && !state.Firing {
			// Alert fired
			state.Firing = true
			state.FiredAt = time.Now()
			state.Value = value

			ae.logger.Warn("alert triggered",
				"rule", rule.Name,
				"severity", rule.Severity,
				"value", value,
				"threshold", rule.Threshold,
			)

			ae.events.PublishAsync(ctx, core.NewEvent(
				core.EventAlertTriggered, "resource",
				core.AlertEventData{
					Name:     rule.Name,
					Severity: rule.Severity,
					Message:  rule.Name + " threshold exceeded",
					Resource: metrics.ServerID,
				},
			))
		} else if !triggered && state.Firing {
			// Alert resolved
			state.Firing = false
			state.ResolvedAt = time.Now()

			ae.logger.Info("alert resolved", "rule", rule.Name)

			ae.events.PublishAsync(ctx, core.NewEvent(
				core.EventAlertResolved, "resource",
				core.AlertEventData{
					Name:     rule.Name,
					Severity: "info",
					Message:  rule.Name + " returned to normal",
					Resource: metrics.ServerID,
				},
			))
		}
	}
}

// extractMetricValue reads a metric value from server metrics.
func extractMetricValue(metric string, m *core.ServerMetrics) float64 {
	switch metric {
	case "cpu_percent":
		return m.CPUPercent
	case "ram_percent":
		if m.RAMTotalMB > 0 {
			return float64(m.RAMUsedMB) / float64(m.RAMTotalMB) * 100
		}
		return 0
	case "disk_percent":
		if m.DiskTotalMB > 0 {
			return float64(m.DiskUsedMB) / float64(m.DiskTotalMB) * 100
		}
		return 0
	default:
		return 0
	}
}

// evaluateCondition checks if a value meets the threshold condition.
func evaluateCondition(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	default:
		return false
	}
}
