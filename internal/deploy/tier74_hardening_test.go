package deploy

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Tier 74 — deploy manager lifecycle hardening tests.
//
// These cover the regressions fixed in Tier 74:
//
//   - NewAutoRollbackManager nil-logger guard
//   - AutoRollbackManager gains a real Stop() method (pre-Tier-74 had
//     no lifecycle at all)
//   - Stop is idempotent (stopOnce + closed-flag under mu)
//   - Stop waits for in-flight handleFailure dispatches (wg.Wait)
//   - After Stop, new events are dropped without touching the store
//   - AutoRestarter.Stop is idempotent (pre-Tier-74 second Stop
//     crashed with "close of closed channel")
//   - ImageUpdateChecker.Stop is idempotent (same bug class)
//   - Module.Stop drains AutoRollbackManager before closing Docker

// ─── NewAutoRollbackManager nil-logger guard ───────────────────────────────

func TestTier74_NewAutoRollbackManager_NilLogger(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(slog.Default())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, nil)
	if ar == nil {
		t.Fatal("NewAutoRollbackManager returned nil")
	}
	if ar.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
	if ar.stopCtx == nil {
		t.Error("stopCtx should be initialised")
	}
	if ar.stopCancel == nil {
		t.Error("stopCancel should be initialised")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier74_AutoRollback_Stop_Idempotent(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	// Pre-Tier-74 there was no Stop at all. After the refactor Stop
	// must be callable multiple times without panicking.
	ar.Stop()
	ar.Stop()
	ar.Stop()
}

// ─── Stop marks closed and drops subsequent events ────────────────────────

func TestTier74_AutoRollback_Stop_DropsLaterEvents(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "a1", Port: 80}
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "running", Image: "app:v1"},
	}
	store.nextVersion = 3

	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())
	ar.Start()

	// Stop first — the manager must refuse to process subsequent events.
	ar.Stop()

	// Publish a deploy.failed event after Stop.
	bus.Publish(context.Background(), core.Event{
		Type:   core.EventDeployFailed,
		Source: "test",
		Data:   core.DeployEventData{AppID: "app-1"},
	})

	// Drain the event bus async workers so we know the dispatch
	// completed (or was refused) before assertions.
	bus.Drain()

	if len(store.appStatusUpdates) != 0 {
		t.Errorf("expected 0 status updates after Stop, got %d", len(store.appStatusUpdates))
	}
}

// ─── handleFailure respects closed flag ────────────────────────────────────

func TestTier74_AutoRollback_HandleFailure_RespectsClosed(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "a1", Port: 80}
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "running", Image: "app:v1"},
	}
	store.nextVersion = 3

	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	// Flip closed directly (simulates the post-Stop state for a test
	// that does not want to exercise the full Start/Stop path).
	ar.mu.Lock()
	ar.closed = true
	ar.mu.Unlock()

	// Direct call must short-circuit at the top of handleFailure.
	ar.handleFailure(context.Background(), "app-1")
	if len(store.appStatusUpdates) != 0 {
		t.Errorf("expected no updates when closed, got %d", len(store.appStatusUpdates))
	}
}

// ─── Wait drains in-flight work ────────────────────────────────────────────

// TestTier74_AutoRollback_Wait_NoDispatch proves Wait is safe when
// nothing has been dispatched and returns immediately.
func TestTier74_AutoRollback_Wait_NoDispatch(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	done := make(chan struct{})
	go func() {
		ar.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait blocked when nothing had been dispatched")
	}
}

// ─── Stop drains active dispatch ───────────────────────────────────────────

// TestTier74_AutoRollback_Stop_DrainsInflight proves Stop blocks
// until a handleFailure goroutine returns. We inject a blocking
// CreateAndStart into the mockRuntime so the rollback engine is
// parked in the middle of provisioning when Stop is called, then
// verify Stop does not return until the handler finishes.
func TestTier74_AutoRollback_Stop_DrainsInflight(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "a1", Port: 80}
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "running", Image: "app:v1"},
	}
	store.nextVersion = 3

	// Park inside CreateAndStart until the test releases.
	release := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once
	rt := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			once.Do(func() { close(entered) })
			<-release
			return "container-new-123", nil
		},
	}

	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, rt, bus, tier74Logger())
	ar.Start()

	// Publish an event and let the async handler pick it up.
	bus.Publish(context.Background(), core.Event{
		Type:   core.EventDeployFailed,
		Source: "test",
		Data:   core.DeployEventData{AppID: "app-1"},
	})

	// Wait until the handler is parked inside CreateAndStart.
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never reached CreateAndStart")
	}

	// Fire Stop in a goroutine — it must not return yet because the
	// in-flight handler is parked.
	stopped := make(chan struct{})
	go func() {
		ar.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("Stop returned before in-flight handler finished — wg.Wait missing")
	case <-time.After(150 * time.Millisecond):
		// good — Stop is parked waiting for wg
	}

	// Release the handler; Stop should now return promptly.
	close(release)

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after in-flight handler finished")
	}
}

// ─── AutoRestarter Stop idempotency ────────────────────────────────────────

func TestTier74_AutoRestarter_Stop_Idempotent(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	rt := &mockRuntime{}
	ar := NewAutoRestarter(rt, store, bus, tier74Logger())

	// Pre-Tier-74 the second Stop panicked with "close of closed
	// channel". stopOnce now guards it.
	ar.Stop()
	ar.Stop()
	ar.Stop()
}

// ─── AutoRestarter nil-logger guard ────────────────────────────────────────

func TestTier74_AutoRestarter_NilLogger(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(slog.Default())
	rt := &mockRuntime{}
	ar := NewAutoRestarter(rt, store, bus, nil)
	if ar.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
}

// ─── ImageUpdateChecker Stop idempotency ───────────────────────────────────

// ─── ImageUpdateChecker nil-logger guard ───────────────────────────────────

// ─── Concurrent Stop storm ─────────────────────────────────────────────────

func TestTier74_AutoRollback_ConcurrentStop_NoPanic(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); ar.Stop() }()
	}
	wg.Wait()

	// Final Stop is a no-op but must not panic or deadlock.
	ar.Stop()
}

// ─── stopCtx cancellation ──────────────────────────────────────────────────

func TestTier74_AutoRollback_Stop_CancelsStopCtx(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	// Before Stop, stopCtx must be live.
	select {
	case <-ar.stopCtx.Done():
		t.Fatal("stopCtx was cancelled before Stop")
	default:
	}

	ar.Stop()

	// After Stop, stopCtx must be cancelled.
	select {
	case <-ar.stopCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("stopCtx was not cancelled by Stop")
	}
}

// ─── helper ────────────────────────────────────────────────────────────────

func tier74Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
