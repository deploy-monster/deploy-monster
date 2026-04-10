package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AutoRollbackManager subscribes to deploy.failed events and automatically
// rolls back to the last stable version when a deployment fails.
// Respects a cooldown to avoid cascading rollbacks.
//
// Lifecycle notes for Tier 74:
//
//   - Pre-Tier-74 this manager had NO Stop() method at all. It was
//     instantiated as a local variable in deploy/module.go Start(),
//     its SubscribeAsync closure captured the manager and survived
//     for the lifetime of the process, and Module.Stop had no way to
//     drain in-flight rollbacks. That meant a deploy.failed event
//     fired mid-shutdown could kick off a container rollback while
//     the Docker manager was being torn down.
//   - Pre-Tier-74 handleFailure used the event's context for the
//     rollback engine. During shutdown the publisher's context is
//     often already cancelled, but if a caller used Background the
//     rollback would run to completion regardless of module Stop.
//   - Pre-Tier-74 the struct had no panic recovery. A panic inside
//     the rollback engine (e.g. nil pointer through a half-initialised
//     store) would take down the async worker pool.
//   - Pre-Tier-74 NewAutoRollbackManager had no nil-logger guard.
//
// Post-Tier-74 Stop() is idempotent (stopOnce), cancels stopCtx so
// in-flight handleFailure calls can observe the shutdown, and
// Wait()s for the wg so Module.Stop can rely on "after Stop returns,
// no rollback goroutine is still touching Docker".
type AutoRollbackManager struct {
	store    core.Store
	runtime  core.ContainerRuntime
	events   *core.EventBus
	logger   *slog.Logger
	cooldown time.Duration

	mu          sync.Mutex
	lastAttempt map[string]time.Time // appID -> last rollback attempt time
	closed      bool                 // true once Stop has been called

	// Shutdown plumbing. stopCtx is cancelled by Stop so in-flight
	// rollback work can unblock cleanly. wg tracks the async handler
	// goroutines spawned by SubscribeAsync so Wait() can drain them.
	// stopOnce guards stopCancel + the closed flag against double Stop.
	stopCtx    context.Context
	stopCancel context.CancelFunc
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

// NewAutoRollbackManager creates an auto-rollback manager. A nil
// logger is tolerated and replaced with slog.Default().
func NewAutoRollbackManager(store core.Store, runtime core.ContainerRuntime, events *core.EventBus, logger *slog.Logger) *AutoRollbackManager {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &AutoRollbackManager{
		store:       store,
		runtime:     runtime,
		events:      events,
		logger:      logger,
		cooldown:    5 * time.Minute,
		lastAttempt: make(map[string]time.Time),
		stopCtx:     ctx,
		stopCancel:  cancel,
	}
}

// Start subscribes to deploy failure events. Safe to call multiple
// times; the subscription is added each call (the event bus does not
// expose an unsubscribe API) but the closed-flag guard inside
// handleFailure short-circuits after Stop.
func (ar *AutoRollbackManager) Start() {
	ar.events.SubscribeAsync(core.EventDeployFailed, func(ctx context.Context, event core.Event) error {
		// Fast path: after Stop the whole handler is a no-op. This
		// prevents a deploy.failed event published during shutdown
		// drain from racing with Docker teardown.
		if ar.isClosed() {
			return nil
		}

		data, ok := event.Data.(core.DeployEventData)
		if !ok {
			return nil
		}

		// Track this dispatch so Stop().Wait() can drain it. We take
		// the lock briefly to avoid a race with Stop clearing the wg
		// contract — if closed flipped between the check above and
		// here, bail out before Add'ing to the wg.
		ar.mu.Lock()
		if ar.closed {
			ar.mu.Unlock()
			return nil
		}
		ar.wg.Add(1)
		ar.mu.Unlock()
		defer ar.wg.Done()

		defer func() {
			// Recover from panics inside the rollback engine so a
			// bug there cannot take down the event bus async worker.
			if r := recover(); r != nil {
				ar.logger.Error("panic in auto-rollback handler",
					"app_id", data.AppID,
					"error", r,
				)
			}
		}()

		ar.handleFailure(ar.runCtx(ctx), data.AppID)
		return nil
	})

	ar.logger.Info("auto-rollback manager started", "cooldown", ar.cooldown)
}

// Stop halts the manager. Safe to call multiple times — the second
// and subsequent calls are no-ops. Stop blocks until every in-flight
// handleFailure dispatch returns, so after Stop returns the caller
// can rely on "no rollback goroutine is still touching Docker".
func (ar *AutoRollbackManager) Stop() {
	ar.stopOnce.Do(func() {
		ar.mu.Lock()
		ar.closed = true
		ar.mu.Unlock()
		if ar.stopCancel != nil {
			ar.stopCancel()
		}
	})
	ar.wg.Wait()
}

// Wait blocks until every in-flight handleFailure returns without
// initiating the Stop sequence. Exposed for tests and for callers
// that want a drain without tearing down the subscription (although
// in practice Stop is preferred).
func (ar *AutoRollbackManager) Wait() {
	ar.wg.Wait()
}

// isClosed returns whether Stop has been called.
func (ar *AutoRollbackManager) isClosed() bool {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	return ar.closed
}

// runCtx returns the manager's stopCtx, or the incoming event ctx
// as a fallback if the manager was constructed without stopCtx
// (e.g. via a struct literal in a test). If stopCtx has already
// been cancelled we propagate that so downstream rollback work
// observes the shutdown immediately.
func (ar *AutoRollbackManager) runCtx(eventCtx context.Context) context.Context {
	if ar.stopCtx != nil {
		return ar.stopCtx
	}
	if eventCtx != nil {
		return eventCtx
	}
	return context.Background()
}

func (ar *AutoRollbackManager) handleFailure(ctx context.Context, appID string) {
	// Check cooldown to prevent cascading rollbacks
	ar.mu.Lock()
	if ar.closed {
		ar.mu.Unlock()
		return
	}
	if last, ok := ar.lastAttempt[appID]; ok && time.Since(last) < ar.cooldown {
		ar.mu.Unlock()
		ar.logger.Info("auto-rollback skipped (cooldown active)",
			"app_id", appID,
			"retry_after", ar.cooldown-time.Since(last),
		)
		return
	}
	ar.lastAttempt[appID] = time.Now()
	ar.mu.Unlock()

	// Find the last stable version
	stableVersion, err := ar.findLastStable(ctx, appID)
	if err != nil {
		ar.logger.Warn("auto-rollback: no stable version found",
			"app_id", appID,
			"error", err,
		)
		return
	}

	ar.logger.Info("auto-rollback triggered",
		"app_id", appID,
		"target_version", stableVersion,
	)

	// Perform rollback using the existing engine
	engine := NewRollbackEngine(ar.store, ar.runtime, ar.events)
	deployment, err := engine.Rollback(ctx, appID, stableVersion)
	if err != nil {
		ar.logger.Error("auto-rollback failed",
			"app_id", appID,
			"target_version", stableVersion,
			"error", err,
		)
		return
	}

	ar.logger.Info("auto-rollback succeeded",
		"app_id", appID,
		"rolled_back_to", stableVersion,
		"new_deployment", deployment.ID,
	)
}

// findLastStable finds the most recent deployment with status "running"
// that is older than the current (failed) deployment.
func (ar *AutoRollbackManager) findLastStable(ctx context.Context, appID string) (int, error) {
	deployments, err := ar.store.ListDeploymentsByApp(ctx, appID, 20)
	if err != nil {
		return 0, err
	}

	// Skip the first (most recent, which is the failed one)
	for i := 1; i < len(deployments); i++ {
		d := deployments[i]
		if d.Status == "running" && d.Image != "" {
			return d.Version, nil
		}
	}

	return 0, fmt.Errorf("no previous stable deployment found for app %s", appID)
}
