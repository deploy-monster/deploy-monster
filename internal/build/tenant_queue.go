package build

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// ErrTenantQueueClosed is returned from TenantQueue.Submit after
// Shutdown has been called. Mirrors ErrPoolClosed on the global
// WorkerPool so callers can test either queue with a common pattern.
var ErrTenantQueueClosed = errors.New("tenant build queue is closed")

// TenantQueue is the Phase 3.3.7 per-tenant build gate. It wraps the
// existing global-concurrency WorkerPool pattern with a second layer
// of bookkeeping that caps the number of in-flight builds for any
// single tenant. Without this layer, one noisy tenant pushing ten
// webhook-triggered builds in a row would occupy every slot in the
// global semaphore and starve every other tenant on the platform.
//
// The fairness contract is:
//
//  1. Per-tenant cap is acquired first. A tenant at their cap waits
//     inside the queue even when global slots are free — this is the
//     only way to guarantee tenant B can queue jobs while tenant A is
//     saturated, because Submit returns control to the caller (which
//     is typically a web request) only after BOTH slots are held.
//
//  2. Global cap is acquired second. Holding a tenant slot while
//     waiting for a global slot is safe because the tenant slot is
//     tiny in absolute terms (default 2) and cannot deadlock against
//     the global slot (default 5). A global slot will always free
//     before every tenant slot is permanently held.
//
//  3. Release order is reverse: global slot first, tenant slot second.
//     Releasing the global slot first lets queued cross-tenant work
//     wake up immediately; releasing the tenant slot last keeps the
//     tenant's own queue moving without fighting over the global.
//
// The queue is NOT aware of the underlying WorkerPool — it owns its
// own semaphores. This keeps the tenant gate testable in isolation
// and avoids a second round of lifecycle plumbing when the module
// shuts down: WorkerPool and TenantQueue each drain themselves.
type TenantQueue struct {
	global    chan struct{}
	perTenant int
	logger    *slog.Logger

	mu     sync.Mutex
	tenant map[string]chan struct{}
	closed bool
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewTenantQueue constructs a queue with the given global cap and
// per-tenant cap. Non-positive caps are clamped to a safe default:
// globalCap < 1 becomes 1 (never a zero-capacity chan, which would
// deadlock every Submit), perTenantCap < 1 becomes 1. A nil logger
// is tolerated and replaced with slog.Default().
func NewTenantQueue(globalCap, perTenantCap int, logger *slog.Logger) *TenantQueue {
	if globalCap < 1 {
		globalCap = 1
	}
	if perTenantCap < 1 {
		perTenantCap = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TenantQueue{
		global:    make(chan struct{}, globalCap),
		perTenant: perTenantCap,
		logger:    logger,
		tenant:    make(map[string]chan struct{}),
		stopCh:    make(chan struct{}),
	}
}

// tenantSlot returns the semaphore channel for a tenant, creating it
// on first use. Caller must hold q.mu.
func (q *TenantQueue) tenantSlot(tenantID string) chan struct{} {
	ch, ok := q.tenant[tenantID]
	if !ok {
		ch = make(chan struct{}, q.perTenant)
		q.tenant[tenantID] = ch
	}
	return ch
}

// Submit enqueues a build job for the given tenant. Submit blocks
// until a per-tenant AND a global slot are available, ctx is
// cancelled, or Shutdown is called. On success the returned error is
// nil and fn has been SCHEDULED (not awaited) — fn runs on a
// goroutine owned by the queue.
//
// The two-phase acquire holds the tenant slot while waiting for the
// global slot. If Shutdown fires during either wait, Submit returns
// ErrTenantQueueClosed and any slots it did hold are released. If
// ctx fires, it returns ctx.Err() and the same cleanup happens.
func (q *TenantQueue) Submit(ctx context.Context, tenantID string, fn func()) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return ErrTenantQueueClosed
	}
	tslot := q.tenantSlot(tenantID)
	// Register the job with the drain WaitGroup BEFORE releasing mu.
	// This mirrors the same happens-before guard WorkerPool uses in
	// SubmitCtx (see module.go:164) and prevents a racing Shutdown
	// from returning before this job runs.
	q.wg.Add(1)
	q.mu.Unlock()

	// Phase 1: acquire per-tenant slot.
	select {
	case tslot <- struct{}{}:
	case <-q.stopCh:
		q.wg.Done()
		return ErrTenantQueueClosed
	case <-ctx.Done():
		q.wg.Done()
		return ctx.Err()
	}

	// Phase 2: acquire global slot. Holding the tenant slot during
	// this wait is intentional — see type-level doc for why.
	select {
	case q.global <- struct{}{}:
	case <-q.stopCh:
		<-tslot
		q.wg.Done()
		return ErrTenantQueueClosed
	case <-ctx.Done():
		<-tslot
		q.wg.Done()
		return ctx.Err()
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				q.logger.Error("panic in tenant build job",
					"tenant", tenantID, "error", r)
			}
			// Release global slot first so cross-tenant work can wake,
			// then the tenant slot so this tenant's own queue moves.
			<-q.global
			<-tslot
			q.wg.Done()
		}()
		fn()
	}()
	return nil
}

// Shutdown blocks new submissions and waits for every in-flight job
// to drain, honoring ctx for a deadline. Idempotent — a second call
// is a no-op that still waits on the same drain.
func (q *TenantQueue) Shutdown(ctx context.Context) error {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.stopCh)
	}
	q.mu.Unlock()

	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Closed reports whether Shutdown has been called. Useful for tests
// and for higher-level orchestration that wants to bail out before
// calling Submit.
func (q *TenantQueue) Closed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closed
}

// TenantInFlight reports the current number of in-flight jobs for a
// tenant. Primarily a test-observable hook — production code does
// not need to inspect this.
func (q *TenantQueue) TenantInFlight(tenantID string) int {
	q.mu.Lock()
	ch, ok := q.tenant[tenantID]
	q.mu.Unlock()
	if !ok {
		return 0
	}
	return len(ch)
}

// GlobalInFlight reports the current global in-flight job count.
func (q *TenantQueue) GlobalInFlight() int {
	return len(q.global)
}
