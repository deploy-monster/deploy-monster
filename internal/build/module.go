package build

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the build engine — detects project types,
// generates Dockerfiles, and builds container images.
type Module struct {
	core   *core.Core
	store  core.Store
	pool   *WorkerPool
	queue  *TenantQueue
	logger *slog.Logger
}

func New() *Module {
	return &Module{}
}

func (m *Module) ID() string                  { return "build" }
func (m *Module) Name() string                { return "Build Engine" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())

	maxConcurrent := c.Config.Limits.MaxConcurrentBuilds
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	m.pool = NewWorkerPoolWithLogger(maxConcurrent, m.logger)

	// Phase 3.3.7: per-tenant build queue layered on top of the
	// global pool so a single noisy tenant cannot starve the
	// platform. NewTenantQueue clamps non-positive caps to 1, so a
	// zero-valued MaxConcurrentBuildsPerTenant falls back to a safe
	// minimum rather than deadlocking on a zero-capacity channel.
	perTenant := c.Config.Limits.MaxConcurrentBuildsPerTenant
	if perTenant <= 0 {
		perTenant = 2
	}
	m.queue = NewTenantQueue(maxConcurrent, perTenant, m.logger)

	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("build engine started", "max_concurrent", m.pool.maxWorkers)
	return nil
}

// Stop shuts the build pool down and drains any in-flight jobs,
// honoring ctx for a shutdown deadline. Before Tier 69 Stop called
// pool.Wait() which accepted no context and could hang indefinitely
// on a stuck build. Phase 3.3.7 adds the tenant queue to the drain —
// it is stopped first so new tenant-scoped submissions cease, then
// the global pool is stopped so any jobs the queue handed off are
// allowed to complete.
func (m *Module) Stop(ctx context.Context) error {
	if m.queue != nil {
		if err := m.queue.Shutdown(ctx); err != nil {
			m.logger.Warn("tenant build queue shutdown did not drain cleanly", "error", err)
		}
	}
	if m.pool == nil {
		return nil
	}
	if err := m.pool.Shutdown(ctx); err != nil {
		m.logger.Warn("build pool shutdown did not drain cleanly", "error", err)
		return err
	}
	return nil
}

func (m *Module) Health() core.HealthStatus {
	return core.HealthOK
}

// ErrPoolClosed is returned by SubmitCtx when the pool has been
// Shutdown. The plain Submit method silently drops submissions on a
// closed pool to preserve backward compatibility.
var ErrPoolClosed = errors.New("build worker pool is closed")

// WorkerPool limits concurrent builds.
//
// Lifecycle notes for Tier 69:
//
//   - Submit + Wait used to race on wg.Add. Go's sync.WaitGroup
//     documentation explicitly forbids a wg.Add with positive delta
//     that races with a concurrent Wait; under load the race detector
//     flagged this every time. Tier 69 introduces a mutex-guarded
//     closed-flag so Submit either observes the pool is still open
//     (and wg.Add happens-before Wait) or observes that it is shutting
//     down and drops the submission cleanly.
//   - Submit used to block forever on a full sem. If the process was
//     shutting down while a call was pending, it would wedge the
//     caller indefinitely. Submit now selects between the sem and the
//     stopCh so Shutdown unblocks every pending slot acquire.
//   - Module.Stop used to call pool.Wait() with no ctx, so a stuck
//     build could pin shutdown forever. Stop now calls Shutdown which
//     honors the ctx deadline.
//   - Panic recovery used slog.Error on the default logger; the pool
//     now carries its own logger so panic logs are tagged with the
//     build module.
type WorkerPool struct {
	maxWorkers int
	sem        chan struct{}
	wg         sync.WaitGroup
	logger     *slog.Logger

	// mu guards closed and serializes closed-check+wg.Add against
	// Shutdown's close(stopCh) so Go's happens-before rule on
	// WaitGroup is preserved.
	mu     sync.Mutex
	closed bool
	stopCh chan struct{}
}

// NewWorkerPool creates a pool with the default logger. Prefer
// NewWorkerPoolWithLogger from production code so panic logs are
// scoped to the calling module.
func NewWorkerPool(max int) *WorkerPool {
	return NewWorkerPoolWithLogger(max, slog.Default())
}

// NewWorkerPoolWithLogger creates a pool bound to a structured logger.
// A negative max is clamped to 0 because make(chan struct{}, -1)
// panics at runtime — the pre-Tier-69 code would crash the process on
// a misconfigured limits.MaxConcurrentBuilds.
func NewWorkerPoolWithLogger(max int, logger *slog.Logger) *WorkerPool {
	if logger == nil {
		logger = slog.Default()
	}
	semSize := max
	if semSize < 0 {
		semSize = 0
	}
	return &WorkerPool{
		maxWorkers: max,
		sem:        make(chan struct{}, semSize),
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

// Submit adds a build job to the pool. Blocks until a worker slot is
// available OR the pool is shut down. A submission to a shut-down
// pool is silently dropped to preserve the pre-Tier-69 signature
// (which returned no error). New callers should use SubmitCtx.
func (wp *WorkerPool) Submit(fn func()) {
	_ = wp.SubmitCtx(context.Background(), fn)
}

// SubmitCtx is the context-aware Submit variant. It returns an error
// when the pool is closed or when ctx is canceled before a worker
// slot becomes available. Execution of fn itself is not wrapped in
// ctx — callers that need per-job cancellation should close over
// their own ctx inside fn.
func (wp *WorkerPool) SubmitCtx(ctx context.Context, fn func()) error {
	// Serialize the closed-check with wg.Add so Shutdown can rely on
	// a happens-before relationship when it calls wg.Wait. Without
	// this the race detector flags a real violation of the WaitGroup
	// contract.
	wp.mu.Lock()
	if wp.closed {
		wp.mu.Unlock()
		return ErrPoolClosed
	}
	wp.wg.Add(1)
	wp.mu.Unlock()

	// Slot acquire — respect ctx and stopCh so a pending Submit can
	// be unblocked by either a caller cancel or a pool shutdown.
	select {
	case wp.sem <- struct{}{}:
		// got a slot
	case <-wp.stopCh:
		wp.wg.Done()
		return ErrPoolClosed
	case <-ctx.Done():
		wp.wg.Done()
		return ctx.Err()
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				wp.logger.Error("panic in build worker", "error", r)
			}
			<-wp.sem
			wp.wg.Done()
		}()
		fn()
	}()

	return nil
}

// Wait blocks until all currently-submitted jobs complete. Wait does
// NOT stop the pool from accepting new submissions — use Shutdown for
// that. Wait is retained as a backwards-compatible drain entry point
// for existing tests; production callers should prefer Shutdown.
func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

// Shutdown marks the pool as closed so Submit rejects new jobs and
// blocks until every in-flight job drains, honoring ctx for a
// deadline. Shutdown is idempotent — the closed flag + mutex mean a
// second Shutdown call waits for the same drain as the first without
// double-closing stopCh.
func (wp *WorkerPool) Shutdown(ctx context.Context) error {
	wp.mu.Lock()
	if !wp.closed {
		wp.closed = true
		close(wp.stopCh)
	}
	wp.mu.Unlock()

	// Wait for in-flight jobs, with ctx honored for a deadline. The
	// goroutine spawned here will eventually exit because every
	// Submit path decrements wg.
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
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
// and for higher-level schedulers that want to short-circuit before
// calling Submit.
func (wp *WorkerPool) Closed() bool {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return wp.closed
}
