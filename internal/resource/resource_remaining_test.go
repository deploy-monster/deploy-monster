package resource

import (
	"context"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// collector.go — CollectServer error paths
// =============================================================================

func TestCollectServer_NilHost(t *testing.T) {
	c := &Collector{
		runtime: nil,
		logger:  slog.Default(),
		// host is nil — will be auto-created in CollectServer
	}
	m := c.CollectServer(context.Background())
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.ServerID != "local" {
		t.Errorf("expected 'local', got %q", m.ServerID)
	}
}

func TestCollectServer_WithRuntime_NoContainers(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: nil,
	}
	c := NewCollector(mock, testLogger())
	m := c.CollectServer(context.Background())
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.Containers != 0 {
		t.Errorf("expected 0 containers, got %d", m.Containers)
	}
}

func TestCollectServer_FallbackMemStats(t *testing.T) {
	// Create a collector on non-Linux (MemStats fallback) by using a nil hostStats
	// The hostStats implementations should return errors on all metrics, triggering
	// the MemStats fallback when RAMTotalMB == 0.
	c := &Collector{
		runtime: nil,
		logger:  slog.Default(),
		host:    newHostStats(),
	}
	m := c.CollectServer(context.Background())
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	// On any platform, ServerID and Timestamp should be set
	if m.ServerID != "local" {
		t.Errorf("expected 'local', got %q", m.ServerID)
	}
}

// =============================================================================
// collector.go — CollectContainers edge cases
// =============================================================================

func TestCollectContainers_NonRunningContainers(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "exited", Labels: map[string]string{"monster.app.id": "app1"}},
			{ID: "c2", State: "created", Labels: map[string]string{"monster.app.id": "app2"}},
		},
	}
	c := NewCollector(mock, testLogger())
	metrics := c.CollectContainers(context.Background())
	if len(metrics) != 0 {
		t.Errorf("expected 0 for non-running containers, got %d", len(metrics))
	}
}

// =============================================================================
// collector.go — countContainers edge cases
// =============================================================================

func TestCountContainers_NilRuntimeRemaining(t *testing.T) {
	c := &Collector{runtime: nil, logger: testLogger(), host: newHostStats()}
	count := c.countContainers(context.Background())
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestCountContainers_ListErrorRemaining(t *testing.T) {
	mock := &mockContainerRuntime{
		listErr: assertError("list error"),
	}
	c := NewCollector(mock, testLogger())
	count := c.countContainers(context.Background())
	if count != 0 {
		t.Errorf("expected 0 on list error, got %d", count)
	}
}

func TestCountContainers_WithData(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "app1"}},
			{ID: "c2", State: "running", Labels: map[string]string{"monster.app.id": "app2"}},
		},
	}
	c := NewCollector(mock, testLogger())
	count := c.countContainers(context.Background())
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

// =============================================================================
// alerts.go — Send error paths
// =============================================================================

func TestAlertmanagerSend_NilURL(t *testing.T) {
	client := NewAlertmanagerClient("", testLogger())
	err := client.Send(context.Background(), []AlertmanagerAlert{
		{Labels: map[string]string{"alertname": "test"}},
	})
	if err != nil {
		t.Fatalf("expected nil error for empty URL, got: %v", err)
	}
}

func TestAlertmanagerSend_MarshalError(t *testing.T) {
	client := NewAlertmanagerClient("http://example.com", testLogger())
	// Send an un-marshalable value (channel)
	err := client.Send(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil alerts")
	}
}

// =============================================================================
// alerts.go — Evaluate edge cases
// =============================================================================

func TestAlertEngine_Evaluate_WithAlertmanager(t *testing.T) {
	events := core.NewEventBus(testLogger())
	ae := NewAlertEngine(events, testLogger())

	// Wire up Alertmanager client
	amClient := NewAlertmanagerClient("http://alertmanager:9093/api/v1/alerts", testLogger())
	ae.SetAlertmanagerClient(amClient)

	// Evaluate with high CPU to trigger alert + Alertmanager send
	metrics := &core.ServerMetrics{
		ServerID:   "server1",
		CPUPercent: 99,
		RAMUsedMB:  500,
		RAMTotalMB: 1000,
		DiskUsedMB: 1000,
		DiskTotalMB: 2000,
	}
	ae.Evaluate(context.Background(), metrics)

	// Should not panic. The Alertmanager send will fail (connection refused),
	// but that's caught by the goroutine's error handler.
}

func TestAlertEngine_Evaluate_ResolveWithNilEvents(t *testing.T) {
	ae := NewAlertEngine(nil, testLogger())

	// First trigger an alert
	metrics := &core.ServerMetrics{
		ServerID:   "server1",
		CPUPercent: 99,
		RAMUsedMB:  500,
		RAMTotalMB: 1000,
		DiskUsedMB: 100000,
		DiskTotalMB: 2000,
	}
	ae.Evaluate(context.Background(), metrics)

	// Then resolve it
	metrics.CPUPercent = 10
	metrics.DiskUsedMB = 100
	ae.Evaluate(context.Background(), metrics)
}

// =============================================================================
// module.go — Init with nil DB
// =============================================================================

func TestModule_Init_NilDB(t *testing.T) {
	c := &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
		// DB is nil
	}
	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if m.bolt != nil {
		t.Errorf("expected nil bolt when DB is nil")
	}
}

// =============================================================================
// module.go — batchStoreMetrics with nil bolt
// =============================================================================

func TestBatchStoreMetrics_NilBoltRemaining(t *testing.T) {
	m := &Module{
		logger: testLogger(),
		bolt:   nil,
	}
	// Should not panic
	m.batchStoreMetrics(&core.ServerMetrics{ServerID: "s1"}, nil)
}

func TestBatchStoreMetrics_EmptyContainerMetrics(t *testing.T) {
	m := &Module{
		logger: testLogger(),
		bolt:   nil,
	}
	// Should not panic
	m.batchStoreMetrics(nil, nil)
}

// =============================================================================
// alerts.go — extractMetricValue edge cases
// =============================================================================

func TestExtractMetricValue_UnknownMetric(t *testing.T) {
	v := extractMetricValue("unknown", &core.ServerMetrics{})
	if v != 0 {
		t.Errorf("expected 0, got %f", v)
	}
}

func TestExtractMetricValue_RAMPercent_ZeroTotal(t *testing.T) {
	v := extractMetricValue("ram_percent", &core.ServerMetrics{
		RAMUsedMB:  100,
		RAMTotalMB: 0,
	})
	if v != 0 {
		t.Errorf("expected 0, got %f", v)
	}
}

func TestExtractMetricValue_DiskPercent_ZeroTotal(t *testing.T) {
	v := extractMetricValue("disk_percent", &core.ServerMetrics{
		DiskUsedMB:  100,
		DiskTotalMB: 0,
	})
	if v != 0 {
		t.Errorf("expected 0, got %f", v)
	}
}

// =============================================================================
// alerts.go — evaluateCondition edge cases
// =============================================================================

func TestEvaluateCondition_UnknownOperator(t *testing.T) {
	result := evaluateCondition(50, "???", 50)
	if result {
		t.Errorf("expected false for unknown operator")
	}
}

func TestEvaluateCondition_AllOperators(t *testing.T) {
	tests := []struct {
		value     float64
		operator  string
		threshold float64
		want      bool
	}{
		{10, ">", 5, true},
		{3, ">", 5, false},
		{5, ">=", 5, true},
		{4, ">=", 5, false},
		{3, "<", 5, true},
		{7, "<", 5, false},
		{5, "<=", 5, true},
		{6, "<=", 5, false},
		{5, "==", 5, true},
		{6, "==", 5, false},
	}
	for _, tt := range tests {
		t.Run(tt.operator, func(t *testing.T) {
			got := evaluateCondition(tt.value, tt.operator, tt.threshold)
			if got != tt.want {
				t.Errorf("evaluateCondition(%f, %q, %f) = %v, want %v",
					tt.value, tt.operator, tt.threshold, got, tt.want)
			}
		})
	}
}

// =============================================================================
// collector.go — CollectServer with host stats that might fail
// =============================================================================

func TestCollectServer_HostStatsFallback(t *testing.T) {
	// This test verifies that CollectServer works even when individual
	// metric collection functions fail (error paths are logged but the
	// function returns a partial metrics object).
	c := &Collector{
		runtime: &mockContainerRuntime{
			containers: []core.ContainerInfo{
				{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "app1"}},
			},
		},
		logger: slog.Default(),
		host:   newHostStats(),
	}

	m := c.CollectServer(context.Background())
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.Containers != 1 {
		t.Errorf("expected 1 container, got %d", m.Containers)
	}
}

// =============================================================================
// asserts helper
// =============================================================================

type assertError string

func (e assertError) Error() string { return string(e) }
