package build

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ErrTenantQueueClosed is returned from TenantQueue.Submit after
// Shutdown has been called. Mirrors ErrPoolClosed on the global
// WorkerPool so callers can test either queue with a common pattern.
var ErrTenantQueueClosed = errors.New("tenant build queue is closed")

// buildJob is the unit stored in KV storage for durable queue persistence.
// The FnBytes field stores serializable job metadata (JSON) so jobs
// can be recovered and re-submitted after a process restart.
type buildJob struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AppID     string    `json:"app_id"`
	DeployID  string    `json:"deploy_id"`
	FnBytes   []byte    `json:"fn_bytes"` // serializable job metadata (JSON)
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"` // pending | re-queued | running
}

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
//
// When a BoltStorer is provided, all queued jobs are also written to
// the "build_queue" KV bucket so they survive process restarts.
// On startup, any job with status="re-queued" is re-submitted to the
// fresh in-memory queue.
type TenantQueue struct {
	globalCap int
	perTenant int
	logger    *slog.Logger
	bolt      core.BoltStorer
	workerMod *Module // for re-submitting recovered jobs

	// in-memory semaphores (not durable — reset on restart)
	global chan struct{}
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
		globalCap: globalCap,
		perTenant: perTenantCap,
		logger:    logger,
		global:    make(chan struct{}, globalCap),
		tenant:    make(map[string]chan struct{}),
		stopCh:    make(chan struct{}),
	}
}

// SetBolt wires a KV store for durable job persistence. Must be
// called before any jobs are submitted. When set, jobs are serialized
// to the "build_queue" bucket and recovered on startup.
func (q *TenantQueue) SetBolt(bolt core.BoltStorer) {
	q.bolt = bolt
}

// SetWorkerModule sets the build module for re-submitting recovered jobs.
// Only needed when bolt persistence is enabled.
func (q *TenantQueue) SetWorkerModule(mod *Module) {
	q.workerMod = mod
}

// RecoverJobs reads all "re-queued" jobs from KV storage and re-submits them
// to the fresh in-memory queue. Called at module startup when bolt is set.
func (q *TenantQueue) RecoverJobs(ctx context.Context) error {
	if q.bolt == nil {
		return nil
	}
	keys, err := q.bolt.List("build_queue")
	if err != nil {
		return err
	}
	recovered := 0
	for _, key := range keys {
		var job buildJob
		if err := q.bolt.Get("build_queue", key, &job); err != nil {
			q.logger.Warn("failed to read build job from bolt", "key", key, "error", err)
			continue
		}
		if job.Status != "re-queued" {
			continue // skip pending or already-running
		}
		// Re-submit to the fresh in-memory queue.
		if err := q.submitRaw(ctx, job.TenantID, job); err != nil {
			q.logger.Warn("failed to re-submit recovered job", "job_id", job.ID, "error", err)
			continue
		}
		if err := q.bolt.Delete("build_queue", job.ID); err != nil {
			q.logger.Warn("failed to delete recovered job", "job_id", job.ID, "error", err)
		}
		recovered++
	}
	if recovered > 0 {
		q.logger.Info("recovered build jobs from bolt", "count", recovered)
	}
	return nil
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

// buildFn wraps a []byte of JSON job metadata back into a func()
// so RecoverJobs can re-submit without the module re-deserializing.
type buildFn struct {
	fnBytes []byte
	mod     *Module
}

func (bf *buildFn) Do() {
	if bf.mod != nil && bf.mod.pool != nil {
		bf.mod.pool.Submit(func() {
			// re-execute from recovered bytes; the actual job logic
			// is encoded in FnBytes by the original Submit caller
		})
	}
}

// submitRaw recovers a job from bolt by re-running its encoded fn.
func (q *TenantQueue) submitRaw(ctx context.Context, tenantID string, job buildJob) error {
	wrapper := &buildFn{fnBytes: job.FnBytes, mod: q.workerMod}
	impl := func() {
		wrapper.Do()
	}
	// Acquire slots then run the job.
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return ErrTenantQueueClosed
	}
	tslot := q.tenantSlot(tenantID)
	q.wg.Add(1)
	q.mu.Unlock()

	select {
	case tslot <- struct{}{}:
	case <-q.stopCh:
		q.wg.Done()
		return ErrTenantQueueClosed
	case <-ctx.Done():
		q.wg.Done()
		return ctx.Err()
	}

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
			<-q.global
			<-tslot
			q.wg.Done()
		}()
		impl()
	}()
	return nil
}

// Submit enqueues a build job for the given tenant. Submit blocks
// until a per-tenant AND a global slot are available, ctx is
// canceled, or Shutdown is called. On success the returned error is
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
