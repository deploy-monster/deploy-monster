package resource

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =====================================================
// HELPERS / MOCKS
// =====================================================

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockContainerRuntime implements core.ContainerRuntime for testing.
type mockContainerRuntime struct {
	containers []core.ContainerInfo
	listErr    error
	stats      *core.ContainerStats
	statsErr   error
}

func (m *mockContainerRuntime) Ping() error { return nil }
func (m *mockContainerRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (m *mockContainerRuntime) Stop(_ context.Context, _ string, _ int) error   { return nil }
func (m *mockContainerRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockContainerRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (m *mockContainerRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockContainerRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.containers, nil
}
func (m *mockContainerRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *mockContainerRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	return m.stats, nil
}
func (m *mockContainerRuntime) ImagePull(_ context.Context, _ string) error                 { return nil }
func (m *mockContainerRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error)       { return nil, nil }
func (m *mockContainerRuntime) ImageRemove(_ context.Context, _ string) error               { return nil }
func (m *mockContainerRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error)    { return nil, nil }
func (m *mockContainerRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error)      { return nil, nil }

// =====================================================
// MODULE TESTS
// =====================================================

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestModuleImplementsInterface(t *testing.T) {
	var _ core.Module = (*Module)(nil)
}

func TestModuleIdentity(t *testing.T) {
	m := New()

	tests := []struct {
		method string
		got    string
		want   string
	}{
		{"ID", m.ID(), "resource"},
		{"Name", m.Name(), "Resource Monitor"},
		{"Version", m.Version(), "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.method, tt.got, tt.want)
			}
		})
	}
}

func TestModuleDependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()

	expected := map[string]bool{"core.db": false, "deploy": false}
	if len(deps) != len(expected) {
		t.Fatalf("Dependencies() len = %d, want %d", len(deps), len(expected))
	}
	for _, d := range deps {
		if _, ok := expected[d]; !ok {
			t.Errorf("unexpected dependency %q", d)
		}
		expected[d] = true
	}
	for d, found := range expected {
		if !found {
			t.Errorf("missing dependency %q", d)
		}
	}
}

func TestModuleRoutes(t *testing.T) {
	m := New()
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
}

func TestModuleEvents(t *testing.T) {
	m := New()
	if events := m.Events(); events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

func TestModuleHealth(t *testing.T) {
	m := New()
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK", h)
	}
}

func TestModuleInit(t *testing.T) {
	c := &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if m.core != c {
		t.Error("Init() did not set core reference")
	}
	if m.collector == nil {
		t.Error("Init() did not create collector")
	}
	if m.alerter == nil {
		t.Error("Init() did not create alert engine")
	}
	if m.logger == nil {
		t.Error("Init() did not set logger")
	}
	if m.stopCh == nil {
		t.Error("Init() did not create stopCh")
	}
}

func TestModuleStart(t *testing.T) {
	c := &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give collection loop goroutine a moment to start
	time.Sleep(10 * time.Millisecond)

	// Stop to clean up goroutine
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestModuleStop(t *testing.T) {
	m := New()
	m.stopCh = make(chan struct{})
	err := m.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestModuleStartStop_FullLifecycle(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "app1"}},
		},
		stats: &core.ContainerStats{CPUPercent: 10},
	}

	c := &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}
	c.Services.Container = mock

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Let the collection loop run at least once (it ticks every 30s,
	// but we just verify the goroutine starts and stops cleanly)
	time.Sleep(10 * time.Millisecond)

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// TestCollectionLoop_DirectCall exercises the collection loop by directly
// invoking it through the module's collector and alerter, simulating what
// the loop does on each tick.
func TestCollectionLoop_DirectSimulation(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "app1"}},
		},
		stats: &core.ContainerStats{CPUPercent: 95, MemoryUsage: 512 * 1024 * 1024, MemoryLimit: 1024 * 1024 * 1024},
	}
	events := core.NewEventBus(testLogger())

	c := &core.Core{
		Logger:   testLogger(),
		Events:   events,
		Services: core.NewServices(),
	}
	c.Services.Container = mock

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Simulate what collectionLoop does on each tick
	ctx := context.Background()
	metrics := m.collector.CollectServer(ctx)
	if metrics == nil {
		t.Fatal("CollectServer returned nil")
	}
	m.alerter.Evaluate(ctx, metrics)

	containerMetrics := m.collector.CollectContainers(ctx)
	if len(containerMetrics) != 1 {
		t.Errorf("CollectContainers returned %d, want 1", len(containerMetrics))
	}
}

// TestCollectionLoop_NilMetrics covers the case where CollectServer returns nil.
func TestCollectionLoop_NilCollectorMetrics(t *testing.T) {
	events := core.NewEventBus(testLogger())

	c := &core.Core{
		Logger:   testLogger(),
		Events:   events,
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// CollectServer never returns nil in practice, but CollectContainers
	// with nil runtime returns nil - verify no panic
	ctx := context.Background()
	metrics := m.collector.CollectServer(ctx)
	if metrics != nil {
		m.alerter.Evaluate(ctx, metrics)
	}
	containerMetrics := m.collector.CollectContainers(ctx)
	_ = containerMetrics // should be nil with no runtime
}

// =====================================================
// COLLECTOR TESTS
// =====================================================

func TestNewCollector(t *testing.T) {
	c := NewCollector(nil, testLogger())
	if c == nil {
		t.Fatal("NewCollector() returned nil")
	}
}

func TestCollector_CollectServer(t *testing.T) {
	c := NewCollector(nil, testLogger())
	metrics := c.CollectServer(context.Background())

	if metrics == nil {
		t.Fatal("CollectServer() returned nil")
	}
	if metrics.ServerID != "local" {
		t.Errorf("ServerID = %q, want %q", metrics.ServerID, "local")
	}
	if metrics.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	// With nil runtime, containers should be 0
	if metrics.Containers != 0 {
		t.Errorf("Containers = %d, want 0 with nil runtime", metrics.Containers)
	}
}

func TestCollector_CollectServer_WithRuntime(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running"},
			{ID: "c2", State: "running"},
		},
	}
	c := NewCollector(mock, testLogger())
	metrics := c.CollectServer(context.Background())

	if metrics == nil {
		t.Fatal("CollectServer() returned nil")
	}
	if metrics.Containers != 2 {
		t.Errorf("Containers = %d, want 2", metrics.Containers)
	}
}

func TestCollector_CollectContainers_NilRuntime(t *testing.T) {
	c := NewCollector(nil, testLogger())
	result := c.CollectContainers(context.Background())
	if result != nil {
		t.Errorf("CollectContainers() with nil runtime should return nil, got %v", result)
	}
}

func TestCollector_CollectContainers_ListError(t *testing.T) {
	mock := &mockContainerRuntime{
		listErr: fmt.Errorf("docker daemon not running"),
	}
	c := NewCollector(mock, testLogger())
	result := c.CollectContainers(context.Background())
	if result != nil {
		t.Errorf("CollectContainers() with list error should return nil, got %v", result)
	}
}

func TestCollector_CollectContainers_WithRunningContainers(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "c1",
				State:  "running",
				Labels: map[string]string{"monster.app.id": "app-1"},
			},
			{
				ID:     "c2",
				State:  "stopped", // should be skipped
				Labels: map[string]string{"monster.app.id": "app-2"},
			},
			{
				ID:     "c3",
				State:  "running",
				Labels: map[string]string{"monster.app.id": "app-3"},
			},
		},
		stats: &core.ContainerStats{
			CPUPercent:    25.5,
			MemoryUsage:   256 * 1024 * 1024, // 256 MB
			MemoryLimit:   1024 * 1024 * 1024, // 1 GB
			MemoryPercent: 25.0,
			NetworkRx:     10 * 1024 * 1024, // 10 MB
			NetworkTx:     5 * 1024 * 1024,  // 5 MB
			PIDs:          42,
		},
	}

	c := NewCollector(mock, testLogger())
	result := c.CollectContainers(context.Background())

	// Should only have 2 running containers (c2 is stopped)
	if len(result) != 2 {
		t.Fatalf("CollectContainers() returned %d entries, want 2", len(result))
	}

	// Verify first container metrics
	m := result[0]
	if m.ContainerID != "c1" {
		t.Errorf("ContainerID = %q, want %q", m.ContainerID, "c1")
	}
	if m.AppID != "app-1" {
		t.Errorf("AppID = %q, want %q", m.AppID, "app-1")
	}
	if m.CPUPercent != 25.5 {
		t.Errorf("CPUPercent = %f, want 25.5", m.CPUPercent)
	}
	if m.RAMUsedMB != 256 {
		t.Errorf("RAMUsedMB = %d, want 256", m.RAMUsedMB)
	}
	if m.RAMLimitMB != 1024 {
		t.Errorf("RAMLimitMB = %d, want 1024", m.RAMLimitMB)
	}
	if m.NetworkRxMB != 10 {
		t.Errorf("NetworkRxMB = %d, want 10", m.NetworkRxMB)
	}
	if m.NetworkTxMB != 5 {
		t.Errorf("NetworkTxMB = %d, want 5", m.NetworkTxMB)
	}
	if m.PIDs != 42 {
		t.Errorf("PIDs = %d, want 42", m.PIDs)
	}
}

func TestCollector_CollectContainers_StatsError(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "c1",
				State:  "running",
				Labels: map[string]string{"monster.app.id": "app-1"},
			},
		},
		statsErr: fmt.Errorf("stats unavailable"),
	}

	c := NewCollector(mock, testLogger())
	result := c.CollectContainers(context.Background())

	if len(result) != 1 {
		t.Fatalf("CollectContainers() returned %d entries, want 1", len(result))
	}

	// Stats error should result in zero values but still include the entry
	m := result[0]
	if m.ContainerID != "c1" {
		t.Errorf("ContainerID = %q, want %q", m.ContainerID, "c1")
	}
	if m.CPUPercent != 0 {
		t.Errorf("CPUPercent should be 0 on stats error, got %f", m.CPUPercent)
	}
}

// =====================================================
// countContainers TESTS
// =====================================================

func TestCountContainers_NilRuntime(t *testing.T) {
	c := &Collector{runtime: nil, logger: testLogger()}
	count := c.countContainers(context.Background())
	if count != 0 {
		t.Errorf("countContainers() with nil runtime = %d, want 0", count)
	}
}

func TestCountContainers_ListError(t *testing.T) {
	mock := &mockContainerRuntime{
		listErr: fmt.Errorf("connection refused"),
	}
	c := &Collector{runtime: mock, logger: testLogger()}
	count := c.countContainers(context.Background())
	if count != 0 {
		t.Errorf("countContainers() with error = %d, want 0", count)
	}
}

func TestCountContainers_WithContainers(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1"}, {ID: "c2"}, {ID: "c3"},
		},
	}
	c := &Collector{runtime: mock, logger: testLogger()}
	count := c.countContainers(context.Background())
	if count != 3 {
		t.Errorf("countContainers() = %d, want 3", count)
	}
}

// =====================================================
// ALERT ENGINE TESTS
// =====================================================

func TestNewAlertEngine(t *testing.T) {
	events := core.NewEventBus(testLogger())
	ae := NewAlertEngine(events, testLogger())

	if ae == nil {
		t.Fatal("NewAlertEngine() returned nil")
	}

	// Should have 3 default rules
	ae.mu.RLock()
	ruleCount := len(ae.rules)
	ae.mu.RUnlock()

	if ruleCount != 3 {
		t.Errorf("expected 3 default rules, got %d", ruleCount)
	}
}

func TestNewAlertEngine_DefaultRules(t *testing.T) {
	events := core.NewEventBus(testLogger())
	ae := NewAlertEngine(events, testLogger())

	expectedRules := []struct {
		name      string
		metric    string
		threshold float64
		severity  string
	}{
		{"high_cpu", "cpu_percent", 90, "warning"},
		{"disk_full", "disk_percent", 95, "critical"},
		{"high_memory", "ram_percent", 90, "warning"},
	}

	ae.mu.RLock()
	defer ae.mu.RUnlock()

	for _, expected := range expectedRules {
		state, ok := ae.states[expected.name]
		if !ok {
			t.Errorf("missing default rule %q", expected.name)
			continue
		}
		if state.Rule.Metric != expected.metric {
			t.Errorf("rule %q metric = %q, want %q", expected.name, state.Rule.Metric, expected.metric)
		}
		if state.Rule.Threshold != expected.threshold {
			t.Errorf("rule %q threshold = %f, want %f", expected.name, state.Rule.Threshold, expected.threshold)
		}
		if state.Rule.Severity != expected.severity {
			t.Errorf("rule %q severity = %q, want %q", expected.name, state.Rule.Severity, expected.severity)
		}
	}
}

func TestAlertEngine_AddRule(t *testing.T) {
	events := core.NewEventBus(testLogger())
	ae := NewAlertEngine(events, testLogger())

	ae.AddRule(&AlertRule{
		Name:      "custom_alert",
		Metric:    "cpu_percent",
		Operator:  ">",
		Threshold: 50,
		Severity:  "info",
	})

	ae.mu.RLock()
	defer ae.mu.RUnlock()

	if _, ok := ae.states["custom_alert"]; !ok {
		t.Error("AddRule() did not register 'custom_alert'")
	}
	// 3 default + 1 custom
	if len(ae.rules) != 4 {
		t.Errorf("expected 4 rules, got %d", len(ae.rules))
	}
}

func TestAlertEngine_Evaluate_AlertFires(t *testing.T) {
	events := core.NewEventBus(testLogger())

	// Track published events
	var firedEvents []core.Event
	events.SubscribeAsync(core.EventAlertTriggered, func(_ context.Context, e core.Event) error {
		firedEvents = append(firedEvents, e)
		return nil
	})

	ae := NewAlertEngine(events, testLogger())

	// High CPU metrics should trigger high_cpu alert
	metrics := &core.ServerMetrics{
		ServerID:   "srv-1",
		Timestamp:  time.Now(),
		CPUPercent: 95, // > 90 threshold
		RAMUsedMB:  100,
		RAMTotalMB: 1000,
	}

	ae.Evaluate(context.Background(), metrics)

	// Check state was updated
	ae.mu.RLock()
	state := ae.states["high_cpu"]
	ae.mu.RUnlock()

	if state == nil {
		t.Fatal("high_cpu state should exist")
	}
	if !state.Firing {
		t.Error("high_cpu alert should be firing")
	}
	if state.Value != 95 {
		t.Errorf("high_cpu value = %f, want 95", state.Value)
	}

	// Give async event handler time to run
	time.Sleep(50 * time.Millisecond)
}

func TestAlertEngine_Evaluate_AlertResolves(t *testing.T) {
	events := core.NewEventBus(testLogger())
	ae := NewAlertEngine(events, testLogger())

	// First, trigger the alert
	highMetrics := &core.ServerMetrics{
		ServerID:   "srv-1",
		Timestamp:  time.Now(),
		CPUPercent: 95,
		RAMUsedMB:  100,
		RAMTotalMB: 1000,
	}
	ae.Evaluate(context.Background(), highMetrics)

	ae.mu.RLock()
	firing := ae.states["high_cpu"].Firing
	ae.mu.RUnlock()
	if !firing {
		t.Fatal("high_cpu should be firing after high CPU")
	}

	// Now send normal metrics to resolve
	normalMetrics := &core.ServerMetrics{
		ServerID:   "srv-1",
		Timestamp:  time.Now(),
		CPUPercent: 50, // below threshold
		RAMUsedMB:  100,
		RAMTotalMB: 1000,
	}
	ae.Evaluate(context.Background(), normalMetrics)

	ae.mu.RLock()
	state := ae.states["high_cpu"]
	ae.mu.RUnlock()

	if state.Firing {
		t.Error("high_cpu should not be firing after normal CPU")
	}
	if state.ResolvedAt.IsZero() {
		t.Error("ResolvedAt should be set")
	}

	// Give async handlers time
	time.Sleep(50 * time.Millisecond)
}

func TestAlertEngine_Evaluate_AlreadyFiring_StaysFiring(t *testing.T) {
	events := core.NewEventBus(testLogger())
	ae := NewAlertEngine(events, testLogger())

	highMetrics := &core.ServerMetrics{
		ServerID:   "srv-1",
		Timestamp:  time.Now(),
		CPUPercent: 95,
		RAMUsedMB:  100,
		RAMTotalMB: 1000,
	}

	// First evaluation: triggers alert
	ae.Evaluate(context.Background(), highMetrics)

	ae.mu.RLock()
	firedAt := ae.states["high_cpu"].FiredAt
	ae.mu.RUnlock()

	// Second evaluation: still high, should stay firing (no re-trigger)
	ae.Evaluate(context.Background(), highMetrics)

	ae.mu.RLock()
	state := ae.states["high_cpu"]
	ae.mu.RUnlock()

	if !state.Firing {
		t.Error("high_cpu should still be firing")
	}
	if state.FiredAt != firedAt {
		t.Error("FiredAt should not change on subsequent evaluations")
	}
}

func TestAlertEngine_Evaluate_NotTriggered_StaysNotFiring(t *testing.T) {
	events := core.NewEventBus(testLogger())
	ae := NewAlertEngine(events, testLogger())

	normalMetrics := &core.ServerMetrics{
		ServerID:   "srv-1",
		Timestamp:  time.Now(),
		CPUPercent: 50,
		RAMUsedMB:  100,
		RAMTotalMB: 1000,
	}

	ae.Evaluate(context.Background(), normalMetrics)

	ae.mu.RLock()
	state := ae.states["high_cpu"]
	ae.mu.RUnlock()

	if state.Firing {
		t.Error("high_cpu should not be firing with normal metrics")
	}
}

func TestAlertEngine_Evaluate_NilState(t *testing.T) {
	events := core.NewEventBus(testLogger())
	ae := NewAlertEngine(events, testLogger())

	// Manually add a rule without a corresponding state entry
	ae.mu.Lock()
	ae.rules = append(ae.rules, &AlertRule{
		Name: "orphan_rule", Metric: "cpu_percent",
		Operator: ">", Threshold: 50, Severity: "info",
	})
	// Intentionally do NOT set ae.states["orphan_rule"]
	ae.mu.Unlock()

	// Should not panic even with nil state
	metrics := &core.ServerMetrics{
		ServerID:   "srv-1",
		CPUPercent: 95,
		RAMUsedMB:  100,
		RAMTotalMB: 1000,
	}
	ae.Evaluate(context.Background(), metrics)
}

// =====================================================
// extractMetricValue TESTS
// =====================================================

func TestExtractMetricValue(t *testing.T) {
	tests := []struct {
		name    string
		metric  string
		metrics *core.ServerMetrics
		want    float64
	}{
		{
			name:    "cpu_percent",
			metric:  "cpu_percent",
			metrics: &core.ServerMetrics{CPUPercent: 85.5},
			want:    85.5,
		},
		{
			name:    "ram_percent_with_total",
			metric:  "ram_percent",
			metrics: &core.ServerMetrics{RAMUsedMB: 512, RAMTotalMB: 1024},
			want:    50.0,
		},
		{
			name:    "ram_percent_zero_total",
			metric:  "ram_percent",
			metrics: &core.ServerMetrics{RAMUsedMB: 512, RAMTotalMB: 0},
			want:    0,
		},
		{
			name:    "disk_percent_with_total",
			metric:  "disk_percent",
			metrics: &core.ServerMetrics{DiskUsedMB: 250, DiskTotalMB: 1000},
			want:    25.0,
		},
		{
			name:    "disk_percent_zero_total",
			metric:  "disk_percent",
			metrics: &core.ServerMetrics{DiskUsedMB: 250, DiskTotalMB: 0},
			want:    0,
		},
		{
			name:    "unknown_metric",
			metric:  "network_io",
			metrics: &core.ServerMetrics{CPUPercent: 50},
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMetricValue(tt.metric, tt.metrics)
			if got != tt.want {
				t.Errorf("extractMetricValue(%q) = %f, want %f", tt.metric, got, tt.want)
			}
		})
	}
}

// =====================================================
// evaluateCondition TESTS
// =====================================================

func TestEvaluateCondition(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		operator  string
		threshold float64
		want      bool
	}{
		{"greater_true", 95, ">", 90, true},
		{"greater_false", 90, ">", 90, false},
		{"greater_false_below", 85, ">", 90, false},
		{"greater_eq_true_equal", 90, ">=", 90, true},
		{"greater_eq_true_above", 91, ">=", 90, true},
		{"greater_eq_false", 89, ">=", 90, false},
		{"less_true", 5, "<", 10, true},
		{"less_false", 10, "<", 10, false},
		{"less_false_above", 15, "<", 10, false},
		{"less_eq_true_equal", 10, "<=", 10, true},
		{"less_eq_true_below", 9, "<=", 10, true},
		{"less_eq_false", 11, "<=", 10, false},
		{"equal_true", 50, "==", 50, true},
		{"equal_false", 50.1, "==", 50, false},
		{"unknown_operator", 50, "!=", 50, false},
		{"empty_operator", 50, "", 50, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateCondition(tt.value, tt.operator, tt.threshold)
			if got != tt.want {
				t.Errorf("evaluateCondition(%f, %q, %f) = %v, want %v",
					tt.value, tt.operator, tt.threshold, got, tt.want)
			}
		})
	}
}
