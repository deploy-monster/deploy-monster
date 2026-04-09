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
type AutoRollbackManager struct {
	store    core.Store
	runtime  core.ContainerRuntime
	events   *core.EventBus
	logger   *slog.Logger
	cooldown time.Duration

	mu          sync.Mutex
	lastAttempt map[string]time.Time // appID -> last rollback attempt time
}

// NewAutoRollbackManager creates an auto-rollback manager.
func NewAutoRollbackManager(store core.Store, runtime core.ContainerRuntime, events *core.EventBus, logger *slog.Logger) *AutoRollbackManager {
	return &AutoRollbackManager{
		store:       store,
		runtime:     runtime,
		events:      events,
		logger:      logger,
		cooldown:    5 * time.Minute,
		lastAttempt: make(map[string]time.Time),
	}
}

// Start subscribes to deploy failure events.
func (ar *AutoRollbackManager) Start() {
	ar.events.SubscribeAsync(core.EventDeployFailed, func(ctx context.Context, event core.Event) error {
		data, ok := event.Data.(core.DeployEventData)
		if !ok {
			return nil
		}
		ar.handleFailure(ctx, data.AppID)
		return nil
	})

	ar.logger.Info("auto-rollback manager started", "cooldown", ar.cooldown)
}

func (ar *AutoRollbackManager) handleFailure(ctx context.Context, appID string) {
	// Check cooldown to prevent cascading rollbacks
	ar.mu.Lock()
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
