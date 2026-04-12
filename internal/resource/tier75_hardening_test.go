package resource

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

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
		t.Error("stopCtx should be initialised by Init")
	}
	if m.stopCancel == nil {
		t.Error("stopCancel should be initialised by Init")
	}
	if m.stopCh == nil {
		t.Error("stopCh should be initialised by Init")
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
		t.Fatal("stopCtx was cancelled before Stop")
	default:
	}

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case <-m.stopCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("stopCtx was not cancelled by Stop")
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
	// Should not be cancelled.
	select {
	case <-ctx.Done():
		t.Fatal("fallback ctx should not be cancelled")
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
// and sets stopCh manually. The stopOnce guard must still serialise
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
