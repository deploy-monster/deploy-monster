package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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

// AlertmanagerClient sends alerts to Prometheus Alertmanager via its v2 API.
type AlertmanagerClient struct {
	url       string
	client    *http.Client
	retries   int
	logger    *slog.Logger
	transport http.RoundTripper // for testing
}

// NewAlertmanagerClient creates an Alertmanager webhook client.
// The url should be the full Alertmanager webhook endpoint, e.g.
// http://alertmanager:9093/api/v1/alerts. A nil logger is tolerated.
func NewAlertmanagerClient(url string, logger *slog.Logger) *AlertmanagerClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &AlertmanagerClient{
		url:     url,
		client:  &http.Client{Timeout: 10 * time.Second},
		retries: 3,
		logger:  logger,
	}
}

// Send sends a list of alerts to Alertmanager. It implements the
// Alertmanager webhook API (v2 POST /api/v1/alerts).
// The context is used for the HTTP request timeout and cancellation.
func (c *AlertmanagerClient) Send(ctx context.Context, alerts []AlertmanagerAlert) error {
	if c.url == "" {
		return nil
	}

	body, err := json.Marshal(alerts)
	if err != nil {
		return fmt.Errorf("encode alertmanager payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < c.retries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create alertmanager request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn("alertmanager delivery failed, retrying",
				"attempt", attempt+1, "url", c.url, "error", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("alertmanager HTTP %d", resp.StatusCode)
		c.logger.Warn("alertmanager delivery failed, retrying",
			"attempt", attempt+1, "status", resp.StatusCode)
	}

	return fmt.Errorf("alertmanager send failed after %d retries: %w", c.retries, lastErr)
}

// AlertmanagerAlert represents a single alert in the Alertmanager webhook API.
type AlertmanagerAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations,omitempty"`
	StartsAt    time.Time        `json:"startsAt"`
	EndsAt      time.Time        `json:"endsAt,omitempty"`
	GeneratorURL string          `json:"generatorURL,omitempty"`
}

// AlertEngine evaluates metrics against alert rules and fires events.
type AlertEngine struct {
	mu       sync.RWMutex
	rules    []*AlertRule
	states   map[string]*AlertState // rule.Name -> state
	events   *core.EventBus
	logger   *slog.Logger
	amClient *AlertmanagerClient // optional Alertmanager webhook
}

// NewAlertEngine creates a new alert engine. A nil logger is
// tolerated and replaced with slog.Default() so the Tier 75 module
// panic-recovery branch cannot NPE on a struct-literal engine.
func NewAlertEngine(events *core.EventBus, logger *slog.Logger) *AlertEngine {
	if logger == nil {
		logger = slog.Default()
	}
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

// SetAlertmanagerClient wires an Alertmanager webhook client into the engine.
func (ae *AlertEngine) SetAlertmanagerClient(client *AlertmanagerClient) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	ae.amClient = client
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

			// Send to Alertmanager if configured.
			if ae.amClient != nil {
				alert := AlertmanagerAlert{
					Labels: map[string]string{
						"alertname": rule.Name,
						"severity":  rule.Severity,
						"instance":  metrics.ServerID,
					},
					Annotations: map[string]string{
						"summary": rule.Name + " threshold exceeded",
						"value":  fmt.Sprintf("%.2f", value),
					},
					StartsAt:     state.FiredAt,
					GeneratorURL: "deploymonster://server/" + metrics.ServerID,
				}
				go func() {
					if err := ae.amClient.Send(context.Background(), []AlertmanagerAlert{alert}); err != nil {
						ae.logger.Warn("failed to send alert to Alertmanager", "rule", rule.Name, "error", err)
					}
				}()
			}

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
