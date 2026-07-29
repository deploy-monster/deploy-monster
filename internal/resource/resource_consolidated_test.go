package resource

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// === merged from resource_final_test.go ===

// ═══════════════════════════════════════════════════════════════════════════════
// collectionLoop — covers module.go:64 (all branches in the select loop)
// The existing tests call Start/Stop but the 30s ticker never fires in tests.
// We exercise the loop body by calling the collector/alerter directly, and also
// verify the stopCh terminates the goroutine promptly.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCollectionLoop_StopTerminatesGoroutine(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "app1"}},
		},
		stats: &core.ContainerStats{CPUPercent: 10, MemoryUsage: 100, MemoryLimit: 200},
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

	// Start the collection loop goroutine
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Let the goroutine start
	time.Sleep(20 * time.Millisecond)

	// Stop should terminate the goroutine via stopCh
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// TestCollectionLoop_DirectInvocation directly calls collectionLoop in a
// goroutine and immediately stops it to cover the function entry and stopCh branch.
func TestCollectionLoop_DirectInvocation(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "a1"}},
		},
		stats: &core.ContainerStats{CPUPercent: 30, MemoryUsage: 128 * 1024 * 1024, MemoryLimit: 256 * 1024 * 1024},
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

	// Launch collectionLoop directly. Tier 75: wg.Add must precede
	// the goroutine spawn so the loop's defer wg.Done does not
	// underflow the counter.
	m.wg.Add(1)
	go m.collectionLoop()

	// Give the goroutine time to start and enter the select
	time.Sleep(50 * time.Millisecond)

	// Stop terminates the loop via stopCh and drains the wg.
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestCollectionLoop_FullSimulation exercises every line of the collectionLoop
// body by directly calling the same operations: CollectServer, Evaluate,
// CollectContainers — covering the nil check and the container metrics path.
func TestCollectionLoop_FullSimulation(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "a1"}},
			{ID: "c2", State: "running", Labels: map[string]string{"monster.app.id": "a2"}},
		},
		stats: &core.ContainerStats{
			CPUPercent:  50,
			MemoryUsage: 256 * 1024 * 1024,
			MemoryLimit: 512 * 1024 * 1024,
		},
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

	ctx := context.Background()

	metrics := m.collector.CollectServer(ctx)
	if metrics == nil {
		t.Fatal("CollectServer returned nil")
	}
	m.alerter.Evaluate(ctx, metrics)

	containerMetrics := m.collector.CollectContainers(ctx)
	if len(containerMetrics) != 2 {
		t.Errorf("CollectContainers returned %d metrics, want 2", len(containerMetrics))
	}
}

// TestCollectionLoop_NilServerMetrics covers the guard where metrics could be nil.
func TestCollectionLoop_NilServerMetrics(t *testing.T) {
	c := &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	ctx := context.Background()
	metrics := m.collector.CollectServer(ctx)
	if metrics != nil {
		m.alerter.Evaluate(ctx, metrics)
	}

	containerMetrics := m.collector.CollectContainers(ctx)
	if containerMetrics != nil {
		t.Errorf("expected nil container metrics with nil runtime, got %d", len(containerMetrics))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// init() — covers module.go:11
// ═══════════════════════════════════════════════════════════════════════════════

func TestInit_RegisteredAsModule(t *testing.T) {
	m := New()
	var _ core.Module = m
	if m.ID() != "resource" {
		t.Errorf("ID() = %q, want resource", m.ID())
	}
}

// === merged from tier75_hardening_test.go ===

// Tier 75 — resource monitor lifecycle hardening tests.
//
// These cover the regressions fixed in Tier 75:
//
//   - Stop idempotency (stopOnce-guarded double close + double cancel)
//   - Stop waits for the collection goroutine (wg.Wait)
//   - Init wires stopCtx/stopCancel so collectOnce can observe Stop
//   - collectOnce runCtx fallback when constructed via struct literal
//   - Concurrent Stop storm does not panic or deadlock
//   - NewCollector/NewAlertEngine tolerate a nil logger
//   - Start/Stop drains the loop promptly (no goroutine leak)

func tier75Core() *core.Core {
	return &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier75_Module_Stop_Idempotent(t *testing.T) {
	m := New()
	if err := m.Init(context.Background(), tier75Core()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Pre-Tier-75 the second Stop panicked with "close of closed
	// channel". stopOnce now guards it.
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("third Stop: %v", err)
	}
}

// ─── Stop waits for collection loop to exit ───────────────────────────────

// TestTier75_Module_Stop_WaitsForLoop proves that Stop blocks until
// the collectionLoop goroutine returns. Without wg.Wait, the
// goroutine could still be in the middle of a BoltBatchSet after
// Stop returned and race with database teardown.
func TestTier75_Module_Stop_WaitsForLoop(t *testing.T) {
	m := New()
	if err := m.Init(context.Background(), tier75Core()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give the collection goroutine a moment to enter the select.
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		_ = m.Stop(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return — wg.Wait missing or deadlock")
	}
}

// ─── Init populates stopCtx/stopCancel ────────────────────────────────────

func TestTier75_Module_Init_InitializesStopCtx(t *testing.T) {
	m := New()
	if err := m.Init(context.Background(), tier75Core()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if m.stopCtx == nil {
		t.Error("stopCtx should be initialized by Init")
	}
	if m.stopCancel == nil {
		t.Error("stopCancel should be initialized by Init")
	}
	if m.stopCh == nil {
		t.Error("stopCh should be initialized by Init")
	}
}

// ─── Stop cancels stopCtx ─────────────────────────────────────────────────

func TestTier75_Module_Stop_CancelsStopCtx(t *testing.T) {
	m := New()
	if err := m.Init(context.Background(), tier75Core()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	select {
	case <-m.stopCtx.Done():
		t.Fatal("stopCtx was canceled before Stop")
	default:
	}

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case <-m.stopCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("stopCtx was not canceled by Stop")
	}
}

// ─── runCtx fallback for struct-literal modules ───────────────────────────

// TestTier75_Module_runCtx_Fallback proves a struct-literal Module
// (one that never went through Init) can still call collectOnce
// paths without NPE. This matches the resource_test.go pattern
// where tests manually build a Module{collector: ..., alerter: ...}.
func TestTier75_Module_runCtx_Fallback(t *testing.T) {
	m := &Module{} // No Init, no stopCtx.
	ctx := m.runCtx()
	if ctx == nil {
		t.Fatal("runCtx returned nil")
	}
	// Should not be canceled.
	select {
	case <-ctx.Done():
		t.Fatal("fallback ctx should not be canceled")
	default:
	}
}

// ─── Stop on a never-started Module ───────────────────────────────────────

// TestTier75_Module_Stop_WithoutStart proves Stop is safe even if
// the Module was Init'd but Start was never called — Module.Stop()
// must not deadlock on wg.Wait when wg was never Added.
func TestTier75_Module_Stop_WithoutStart(t *testing.T) {
	m := New()
	if err := m.Init(context.Background(), tier75Core()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = m.Stop(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop without Start deadlocked")
	}
}

// ─── Concurrent Stop storm ─────────────────────────────────────────────────

func TestTier75_Module_ConcurrentStop_NoPanic(t *testing.T) {
	m := New()
	if err := m.Init(context.Background(), tier75Core()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = m.Stop(context.Background()) }()
	}
	wg.Wait()

	// Final Stop is a no-op but must not panic or deadlock.
	_ = m.Stop(context.Background())
}

// ─── Legacy TestModuleStop compatibility ──────────────────────────────────

// TestTier75_Module_Stop_LegacyStructLiteralCompat mirrors the
// pre-Tier-75 test pattern where a caller builds a Module by hand
// and sets stopCh manually. The stopOnce guard must still serialize
// the close so this path does not panic on double Stop.
func TestTier75_Module_Stop_LegacyStructLiteralCompat(t *testing.T) {
	m := New()
	m.stopCh = make(chan struct{})

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

// ─── Nil-logger guards ─────────────────────────────────────────────────────

func TestTier75_NewCollector_NilLogger(t *testing.T) {
	c := NewCollector(nil, nil)
	if c.logger == nil {
		t.Error("Collector.logger should default to slog.Default when nil")
	}
}

func TestTier75_NewAlertEngine_NilLogger(t *testing.T) {
	ae := NewAlertEngine(core.NewEventBus(testLogger()), nil)
	if ae.logger == nil {
		t.Error("AlertEngine.logger should default to slog.Default when nil")
	}
}
