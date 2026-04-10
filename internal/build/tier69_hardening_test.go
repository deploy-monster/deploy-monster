package build

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tier 69 — build worker pool hardening tests.
//
// These cover the regressions fixed in Tier 69:
//   - Submit+Wait race on wg.Add (Go WaitGroup contract violation)
//   - Submit wedges indefinitely on a full sem after pool shutdown
//   - Module.Stop ignores its ctx parameter
//   - NewWorkerPool(negative) panics at runtime in make(chan)
//   - Submit after Shutdown silently drops (backward compat) /
//     SubmitCtx returns ErrPoolClosed
//   - Shutdown is idempotent and honors ctx deadline
//   - Panic recovery uses the pool's structured logger

// ─── Shutdown idempotency ─────────────────────────────────────────────────

func TestTier69_WorkerPool_Shutdown_Idempotent(t *testing.T) {
	pool := NewWorkerPoolWithLogger(3, tier69Logger())

	// Two Shutdown calls must not panic (no double-close of stopCh).
	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
	if !pool.Closed() {
		t.Error("Closed() should report true after Shutdown")
	}
}

// ─── Submit after Shutdown is a no-op ──────────────────────────────────────

func TestTier69_WorkerPool_SubmitAfterShutdown_Dropped(t *testing.T) {
	pool := NewWorkerPoolWithLogger(3, tier69Logger())
	_ = pool.Shutdown(context.Background())

	// The legacy Submit contract is "no error" — we drop silently.
	var called atomic.Bool
	pool.Submit(func() { called.Store(true) })

	// Give any goroutine a moment to run (there shouldn't be one).
	time.Sleep(20 * time.Millisecond)
	if called.Load() {
		t.Error("Submit after Shutdown executed the job — should have been dropped")
	}

	// SubmitCtx returns ErrPoolClosed so callers can distinguish.
	err := pool.SubmitCtx(context.Background(), func() {})
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

// ─── Shutdown unblocks a pending Submit waiting for a full sem ────────────

// TestTier69_WorkerPool_Shutdown_UnblocksPendingSubmit exercises the
// "Submit wedges when the semaphore is full and nothing else will
// drain it" bug. Before Tier 69 a Submit on a saturated pool had no
// escape hatch; Shutdown could not rescue it.
func TestTier69_WorkerPool_Shutdown_UnblocksPendingSubmit(t *testing.T) {
	pool := NewWorkerPoolWithLogger(1, tier69Logger())

	// Occupy the single worker slot with a job that blocks until we
	// release it.
	release := make(chan struct{})
	first := make(chan struct{})
	pool.Submit(func() {
		close(first)
		<-release
	})

	<-first // make sure the blocker is holding the slot

	// Now try to submit a second job — it should block on the full
	// semaphore. We wrap it in SubmitCtx so we can observe the
	// rejection.
	submitReturned := make(chan error, 1)
	go func() {
		submitReturned <- pool.SubmitCtx(context.Background(), func() {})
	}()

	// Give the second submit a moment to park on sem.
	time.Sleep(50 * time.Millisecond)

	// Shutdown must rescue the pending submit by signalling stopCh.
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- pool.Shutdown(context.Background()) }()

	// The pending SubmitCtx should return ErrPoolClosed quickly.
	select {
	case err := <-submitReturned:
		if !errors.Is(err, ErrPoolClosed) {
			t.Errorf("pending SubmitCtx should return ErrPoolClosed after Shutdown, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("pending SubmitCtx never returned — Shutdown did not unblock the slot acquire")
	}

	// Release the in-flight job so Shutdown can drain.
	close(release)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Errorf("Shutdown returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Shutdown never returned after in-flight job completed")
	}
}

// ─── Shutdown honors ctx deadline when jobs don't drain ────────────────────

func TestTier69_WorkerPool_Shutdown_HonorsCtxDeadline(t *testing.T) {
	pool := NewWorkerPoolWithLogger(1, tier69Logger())

	// A job that blocks forever (until the test ends).
	never := make(chan struct{})
	defer close(never)
	pool.Submit(func() { <-never })

	// Give the job a tick to enter fn.
	time.Sleep(20 * time.Millisecond)

	// Shutdown with a tight deadline should return ctx.DeadlineExceeded
	// rather than wait forever.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := pool.Shutdown(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown took %v — did not honor ctx deadline", elapsed)
	}
}

// ─── NewWorkerPool guards against negative max ─────────────────────────────

// TestTier69_NewWorkerPool_NegativeMax_NoPanic catches a real panic:
// make(chan struct{}, -1) panics at runtime with "makechan: size out
// of range". Before Tier 69 a misconfigured Limits.MaxConcurrentBuilds
// would crash the process at Init time.
func TestTier69_NewWorkerPool_NegativeMax_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewWorkerPool(-1) panicked: %v", r)
		}
	}()

	pool := NewWorkerPoolWithLogger(-1, tier69Logger())
	if pool == nil {
		t.Fatal("NewWorkerPool returned nil")
	}
	// The raw input field is preserved for observability.
	if pool.maxWorkers != -1 {
		t.Errorf("expected maxWorkers=-1 (raw), got %d", pool.maxWorkers)
	}
	// Submit is expected to be unusable on a negative-max pool (the
	// sem is unbuffered), but Shutdown must still succeed cleanly.
	if err := pool.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on negative-max pool: %v", err)
	}
}

// ─── NewWorkerPoolWithLogger nil-logger guard ──────────────────────────────

func TestTier69_NewWorkerPoolWithLogger_NilLogger(t *testing.T) {
	pool := NewWorkerPoolWithLogger(3, nil)
	if pool == nil {
		t.Fatal("NewWorkerPoolWithLogger returned nil")
	}
	if pool.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
}

// ─── Panic in a worker does not tear the pool down ─────────────────────────

func TestTier69_WorkerPool_PanicRecovery(t *testing.T) {
	pool := NewWorkerPoolWithLogger(2, tier69Logger())

	var okCount atomic.Int32
	var done sync.WaitGroup

	// Panic job.
	done.Add(1)
	pool.Submit(func() {
		defer done.Done()
		panic("kaboom")
	})

	// Normal job submitted after the panic must still run.
	done.Add(1)
	pool.Submit(func() {
		defer done.Done()
		okCount.Add(1)
	})

	done.Wait()

	if okCount.Load() != 1 {
		t.Errorf("normal job should still run after panic, got %d", okCount.Load())
	}

	if err := pool.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown after panic: %v", err)
	}
}

// ─── SubmitCtx cancellation during slot acquire ────────────────────────────

func TestTier69_WorkerPool_SubmitCtx_CancelPendingSlot(t *testing.T) {
	pool := NewWorkerPoolWithLogger(1, tier69Logger())
	defer func() { _ = pool.Shutdown(context.Background()) }()

	// Occupy the single slot.
	release := make(chan struct{})
	defer close(release)
	pool.Submit(func() { <-release })

	// Give the first job a moment to claim the slot.
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := pool.SubmitCtx(ctx, func() { t.Error("job should not run") })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

// ─── Concurrent Submit + Shutdown does not race wg ─────────────────────────

// TestTier69_WorkerPool_ConcurrentSubmitShutdown stresses the
// mutex-guarded closed-check. Before Tier 69 the race detector
// flagged this under load: Submit's wg.Add(1) could happen after
// Wait() had already observed zero, which is undefined behavior per
// the WaitGroup contract.
func TestTier69_WorkerPool_ConcurrentSubmitShutdown(t *testing.T) {
	pool := NewWorkerPoolWithLogger(10, tier69Logger())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Submit(func() { time.Sleep(1 * time.Millisecond) })
		}()
	}

	// Race Shutdown against the submits.
	shutdownDone := make(chan error, 1)
	go func() {
		time.Sleep(5 * time.Millisecond)
		shutdownDone <- pool.Shutdown(context.Background())
	}()

	wg.Wait()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Errorf("Shutdown returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown deadlocked under concurrent submit")
	}
}

// ─── Module.Stop honors its ctx ────────────────────────────────────────────

// TestTier69_Module_Stop_HonorsCtx proves that Module.Stop now
// propagates its context parameter to pool.Shutdown. Before Tier 69
// Stop was `Stop(_ context.Context)` and called pool.Wait() with no
// deadline, so a stuck build could hold up the entire module graph
// indefinitely.
func TestTier69_Module_Stop_HonorsCtx(t *testing.T) {
	m := &Module{
		logger: tier69Logger(),
		pool:   NewWorkerPoolWithLogger(1, tier69Logger()),
	}

	never := make(chan struct{})
	defer close(never)
	m.pool.Submit(func() { <-never })

	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := m.Stop(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded from Module.Stop, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Module.Stop took %v — ctx deadline not honored", elapsed)
	}
}

// ─── Closed() status tracking ──────────────────────────────────────────────

func TestTier69_WorkerPool_ClosedStatus(t *testing.T) {
	pool := NewWorkerPoolWithLogger(1, tier69Logger())
	if pool.Closed() {
		t.Error("new pool should not be Closed")
	}
	_ = pool.Shutdown(context.Background())
	if !pool.Closed() {
		t.Error("pool should be Closed after Shutdown")
	}
}

// ─── helper ────────────────────────────────────────────────────────────────

func tier69Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
