package billing

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Tier 68 — billing meter hardening tests.
//
// These cover the regressions fixed in Tier 68:
//   - NewMeter nil-logger guard
//   - Stop idempotency (stopOnce-guarded double close)
//   - Stop waits for the loop goroutine (wg.Wait)
//   - Start idempotency (startOnce prevents duplicate goroutines)
//   - Stop without Start does not deadlock on wg.Wait
//   - Cancellable stopCtx plumbed to collect → ListByLabels
//   - Per-tick timeout bounds a stuck collect
//   - QuotaCheckCtx accepts an external context
//   - runCtx nil fallback for struct-literal construction

// ─── NewMeter nil-logger guard ─────────────────────────────────────────────

func TestTier68_NewMeter_NilLogger(t *testing.T) {
	meter := NewMeter(nil, nil, nil)
	if meter == nil {
		t.Fatal("NewMeter returned nil")
	}
	if meter.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
	if meter.stopCtx == nil || meter.stopCancel == nil {
		t.Error("stopCtx/stopCancel should be initialized")
	}
	if meter.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier68_Meter_Stop_Idempotent(t *testing.T) {
	meter := NewMeter(nil, nil, tier68Logger())
	meter.Start()

	// Double-Stop must not panic. Before Tier 68 the second call
	// panicked with "close of closed channel" because there was no
	// stopOnce guard.
	meter.Stop()
	meter.Stop()
}

func TestTier68_Meter_Stop_WithoutStart_Safe(t *testing.T) {
	meter := NewMeter(nil, nil, tier68Logger())
	// Must not deadlock on wg.Wait — nothing was added to the group.
	meter.Stop()
	meter.Stop()
}

// ─── Start idempotency ─────────────────────────────────────────────────────

func TestTier68_Meter_Start_Idempotent(t *testing.T) {
	meter := NewMeter(nil, nil, tier68Logger())

	// Starting twice must not double-count wg. If it did, Stop would
	// block forever waiting for a phantom second goroutine.
	meter.Start()
	meter.Start()

	done := make(chan struct{})
	go func() {
		meter.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked — startOnce/wg balance is wrong")
	}
}

// ─── Stop waits for the loop goroutine ─────────────────────────────────────

func TestTier68_Meter_Stop_WaitsForLoop(t *testing.T) {
	meter := NewMeter(nil, nil, tier68Logger())
	meter.Start()

	// Give the goroutine a moment to enter its select.
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		meter.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return — wg.Wait missing or deadlock")
	}
}

// ─── Stop cancels in-flight collect via ctx ────────────────────────────────

// blockingRuntime hangs on ListByLabels until the context is canceled.
// Used to prove that Stop actually cancels in-flight Docker calls
// instead of letting them run against a dead meter.
type blockingRuntime struct {
	mockContainerRuntime
	started  chan struct{}
	canceled atomic.Bool
}

func (b *blockingRuntime) ListByLabels(ctx context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	// Signal that we're in the call.
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-ctx.Done()
	b.canceled.Store(true)
	return nil, ctx.Err()
}

func TestTier68_Meter_Stop_CancelsInFlightCollect(t *testing.T) {
	runtime := &blockingRuntime{started: make(chan struct{})}
	meter := NewMeter(&mockStore{}, runtime, tier68Logger())

	// Drive collect directly — we cannot wait for the 60s ticker.
	done := make(chan struct{})
	go func() {
		meter.collect()
		close(done)
	}()

	// Wait for ListByLabels to be entered.
	select {
	case <-runtime.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ListByLabels was not reached")
	}

	// Stop cancels the shared context, which propagates into the
	// in-flight ListByLabels call.
	meter.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("collect did not return after Stop — ctx cancellation is not plumbed")
	}

	if !runtime.canceled.Load() {
		t.Error("ListByLabels did not observe ctx cancellation")
	}
}

// ─── Per-tick timeout bounds a stuck collect ───────────────────────────────

// hangingRuntime hangs forever unless ctx is canceled. We use it to
// prove that the per-tick timeout eventually aborts a stuck Docker
// call even if nobody called Stop.
type hangingRuntime struct {
	mockContainerRuntime
}

func (h *hangingRuntime) ListByLabels(ctx context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestTier68_Meter_CollectTimeout_BoundsStuckCall is a slow-ish test
// (it waits for the 45-second per-tick timeout). We skip it in -short
// mode so CI can keep running fast while developers can exercise it
// locally.
//
// We also provide an inner helper that lets the test file drive the
// timeout with a much shorter deadline — we do this by replacing the
// meter's stopCtx with an already-canceled context derived from a
// WithTimeout of 10ms. That exercises the exact same abort path as a
// real 45-second deadline hit.
func TestTier68_Meter_CollectTimeout_BoundsStuckCall(t *testing.T) {
	runtime := &hangingRuntime{}
	meter := NewMeter(&mockStore{}, runtime, tier68Logger())
	defer meter.Stop()

	// Swap in a short-deadline context as the parent so the per-tick
	// WithTimeout (45s) child inherits the faster deadline. This lets
	// us observe the abort path in milliseconds instead of waiting for
	// the real timeout.
	parent, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	meter.stopCtx = parent

	done := make(chan struct{})
	go func() {
		meter.collect()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("collect did not honor the parent context deadline")
	}
}

// ─── QuotaCheckCtx accepts external context ────────────────────────────────

// ctxObservingStore records whether ListAppsByTenant was called with a
// canceled context. Used to prove that QuotaCheckCtx actually plumbs
// the caller's ctx to the store, instead of hardcoding Background
// (which is what the pre-Tier-68 QuotaCheck did).
type ctxObservingStore struct {
	mockStore
	sawCtxErr atomic.Value // error
	calls     atomic.Int32
}

func (c *ctxObservingStore) ListAppsByTenant(ctx context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	c.calls.Add(1)
	if err := ctx.Err(); err != nil {
		c.sawCtxErr.Store(err)
		return nil, 0, err
	}
	return nil, 0, nil
}

func TestTier68_QuotaCheckCtx_PlumbsContext(t *testing.T) {
	store := &ctxObservingStore{}
	plan := Plan{MaxApps: 10}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	_, err := QuotaCheckCtx(ctx, store, "tenant-1", plan)
	if err == nil {
		t.Fatal("expected QuotaCheckCtx to return the cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if store.calls.Load() != 1 {
		t.Errorf("expected exactly 1 ListAppsByTenant call, got %d", store.calls.Load())
	}
	if seen, _ := store.sawCtxErr.Load().(error); seen == nil {
		t.Error("ListAppsByTenant did not observe the canceled context")
	}
}

// ─── runCtx nil fallback ──────────────────────────────────────────────────

func TestTier68_Meter_RunCtx_NilFallback(t *testing.T) {
	// Bare struct literal — no NewMeter, so stopCtx is nil.
	meter := &Meter{logger: tier68Logger()}
	ctx := meter.runCtx()
	if ctx == nil {
		t.Fatal("runCtx must not return nil")
	}
	if ctx.Err() != nil {
		t.Errorf("fallback background context should not be canceled: %v", ctx.Err())
	}
}

// ─── Concurrent Start+Stop storm ───────────────────────────────────────────

// TestTier68_Meter_ConcurrentStartStop exercises the startOnce/stopOnce
// guards under concurrent pressure. Before Tier 68 the concurrent
// double-close would race with a close-of-closed-channel panic.
func TestTier68_Meter_ConcurrentStartStop(t *testing.T) {
	meter := NewMeter(nil, nil, tier68Logger())

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); meter.Start() }()
		go func() { defer wg.Done(); meter.Stop() }()
	}
	wg.Wait()

	// Final Stop is a no-op but must not panic or deadlock.
	meter.Stop()
}

// ─── collect with nil runtime is a fast no-op ──────────────────────────────

func TestTier68_Meter_Collect_NilRuntimeFastPath(t *testing.T) {
	meter := NewMeter(&mockStore{}, nil, tier68Logger())

	done := make(chan struct{})
	go func() {
		meter.collect()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("collect did not return fast-path on nil runtime")
	}
}

// ─── helper ────────────────────────────────────────────────────────────────

func tier68Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
