package resource

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// collector.go — CollectServer edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestCollector_CollectServer_NilHostEdge(t *testing.T) {
	c := &Collector{
		runtime: nil,
		logger:  slog.Default(),
		host:    nil, // nil host should trigger initialization
	}
	m := c.CollectServer(context.Background())
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.ServerID != "local" {
		t.Errorf("expected ServerID 'local', got %s", m.ServerID)
	}
}

func TestCollector_CollectServer_NilLogger(t *testing.T) {
	c := NewCollector(nil, nil)
	if c.logger == nil {
		t.Error("expected default logger when nil is provided")
	}
	m := c.CollectServer(context.Background())
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
}

func TestCollector_CollectContainers_NilRuntimeEdge(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	metrics := c.CollectContainers(context.Background())
	if metrics != nil {
		t.Errorf("expected nil for nil runtime, got %v", metrics)
	}
}

func TestCollector_CountContainers_NilRuntime(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	count := c.countContainers(context.Background())
	if count != 0 {
		t.Errorf("expected 0 for nil runtime, got %d", count)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// alerts.go — Send and Evaluate edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestAlertmanagerClient_Send_EmptyURLEdge(t *testing.T) {
	c := NewAlertmanagerClient("", slog.Default())
	err := c.Send(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil for empty url, got %v", err)
	}
}

func TestAlertmanagerClient_Send_NilLogger(t *testing.T) {
	c := NewAlertmanagerClient("http://alertmanager:9093/api/v1/alerts", nil)
	if c.logger == nil {
		t.Error("expected default logger when nil")
	}
}

func TestAlertEngine_New_NilLogger(t *testing.T) {
	ae := NewAlertEngine(nil, nil)
	if ae.logger == nil {
		t.Error("expected default logger when nil")
	}
	if len(ae.rules) == 0 {
		t.Error("expected default rules")
	}
}

func TestAlertEngine_Evaluate_NilStateEdge(t *testing.T) {
	ae := NewAlertEngine(nil, slog.Default())
	// Clear states to test nil state path
	ae.mu.Lock()
	ae.states = nil
	ae.mu.Unlock()

	metrics := &core.ServerMetrics{
		ServerID:   "test-server",
		CPUPercent: 95.0,
		RAMUsedMB:  8000,
		RAMTotalMB: 10000,
		DiskUsedMB: 90000,
		DiskTotalMB: 100000,
	}
	// Should not panic
	ae.Evaluate(context.Background(), metrics)
}

func TestAlertEngine_Evaluate_NilEvents(t *testing.T) {
	ae := NewAlertEngine(nil, slog.Default())
	ae.events = nil

	metrics := &core.ServerMetrics{
		ServerID:   "test-server",
		CPUPercent: 95.0,
	}
	// Should not panic — publish checks for nil events
	ae.Evaluate(context.Background(), metrics)
}

func TestAlertEngine_Evaluate_NilAMClient(t *testing.T) {
	ae := NewAlertEngine(nil, slog.Default())
	ae.amClient = nil

	metrics := &core.ServerMetrics{
		ServerID:   "test-server",
		CPUPercent: 95.0,
		RAMUsedMB:  8000,
		RAMTotalMB: 10000,
		DiskUsedMB: 90000,
		DiskTotalMB: 100000,
	}
	// Should not panic
	ae.Evaluate(context.Background(), metrics)
}

// ═══════════════════════════════════════════════════════════════════════════════
// alerts.go — extractMetricValue and evaluateCondition edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestExtractMetricValue_Unknown(t *testing.T) {
	v := extractMetricValue("unknown_metric", &core.ServerMetrics{})
	if v != 0 {
		t.Errorf("expected 0 for unknown metric, got %f", v)
	}
}

func TestExtractMetricValue_ZeroTotal(t *testing.T) {
	v := extractMetricValue("ram_percent", &core.ServerMetrics{RAMUsedMB: 500, RAMTotalMB: 0})
	if v != 0 {
		t.Errorf("expected 0 for zero total, got %f", v)
	}
}

func TestEvaluateCondition_Unknown(t *testing.T) {
	if evaluateCondition(50, "??", 30) {
		t.Error("expected false for unknown operator")
	}
}

func TestEvaluateCondition_LessThan(t *testing.T) {
	if !evaluateCondition(10, "<", 20) {
		t.Error("expected true for 10 < 20")
	}
	if evaluateCondition(30, "<", 20) {
		t.Error("expected false for 30 < 20")
	}
}

func TestEvaluateCondition_GreaterEqual(t *testing.T) {
	if !evaluateCondition(20, ">=", 20) {
		t.Error("expected true for 20 >= 20")
	}
}

func TestEvaluateCondition_LessEqual(t *testing.T) {
	if !evaluateCondition(10, "<=", 20) {
		t.Error("expected true for 10 <= 20")
	}
}

func TestEvaluateCondition_Equal(t *testing.T) {
	if !evaluateCondition(42, "==", 42) {
		t.Error("expected true for 42 == 42")
	}
}

// module.go — collectionLoop, Health, Init edge cases
// Health always returns OK for the resource module
// alerts.go — Evaluate triggers alert resolution
// ═══════════════════════════════════════════════════════════════════════════════

func TestAlertEngine_Evaluate_ResolveAlert(t *testing.T) {
	ae := NewAlertEngine(nil, slog.Default())
	ae.events = nil

	// First, trigger the high_cpu alert
	metrics := &core.ServerMetrics{
		ServerID:   "test",
		CPUPercent: 95.0,
	}
	ae.Evaluate(context.Background(), metrics)

	// Then, resolve it
	metrics.CPUPercent = 10.0
	ae.Evaluate(context.Background(), metrics)

	// State should be resolved
	ae.mu.RLock()
	state := ae.states["high_cpu"]
	ae.mu.RUnlock()
	if state == nil {
		t.Fatal("expected state for high_cpu")
	}
	if state.Firing {
		t.Error("expected alert to be resolved")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// alerts.go — SetAlertmanagerClient
// ═══════════════════════════════════════════════════════════════════════════════

func TestAlertEngine_SetAlertmanagerClientEdge(t *testing.T) {
	ae := NewAlertEngine(nil, slog.Default())
	client := NewAlertmanagerClient("http://am:9093", slog.Default())
	ae.SetAlertmanagerClient(client)
	if ae.amClient == nil {
		t.Error("expected amClient to be set")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// alerts.go — Send with retry (best effort)
// ═══════════════════════════════════════════════════════════════════════════════

func TestAlertmanagerClient_Send_WithAlerts(t *testing.T) {
	// Create a client with an unreachable URL — should fail but not panic
	c := NewAlertmanagerClient("http://127.0.0.1:1/api/v1/alerts", slog.Default())
	c.retries = 1
	alerts := []AlertmanagerAlert{
		{
			Labels: map[string]string{
				"alertname": "test",
				"severity":  "critical",
			},
			StartsAt: mustParseTime("2026-01-01T00:00:00Z"),
		},
	}
	err := c.Send(context.Background(), alerts)
	if err == nil {
		t.Log("expected error for unreachable URL, got nil (network may be different)")
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
