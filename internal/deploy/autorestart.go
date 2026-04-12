package deploy

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const autoRestartCheckInterval = 30 * time.Second

// AutoRestarter monitors containers and restarts crashed ones.
// Listens for container.died events and attempts restart with backoff.
//
// Lifecycle note for Tier 74: Stop used to call close(stopCh) with
// no sync.Once guard — a second Stop crashed the deploy module with
// "close of closed channel". stopOnce now serializes the close so
// Module.Stop is idempotent across retries.
type AutoRestarter struct {
	runtime    core.ContainerRuntime
	store      core.Store
	events     *core.EventBus
	logger     *slog.Logger
	maxRetries int
	stopCh     chan struct{}
	stopOnce   sync.Once
}

// NewAutoRestarter creates an auto-restart monitor. A nil logger is
// tolerated and replaced with slog.Default().
func NewAutoRestarter(runtime core.ContainerRuntime, store core.Store, events *core.EventBus, logger *slog.Logger) *AutoRestarter {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoRestarter{
		runtime:    runtime,
		store:      store,
		events:     events,
		logger:     logger,
		maxRetries: 5,
		stopCh:     make(chan struct{}),
	}
}

// Start subscribes to container death events and handles restarts.
func (ar *AutoRestarter) Start() {
	ar.events.SubscribeAsync(core.EventContainerDied, func(ctx context.Context, event core.Event) error {
		if data, ok := event.Data.(core.DeployEventData); ok {
			ar.handleCrash(ctx, data.AppID, data.ContainerID)
		}
		return nil
	})

	// Periodic check for crashed containers
	core.SafeGo(ar.logger, "autorestart-check", func() {
		ticker := time.NewTicker(autoRestartCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ar.checkCrashed()
			case <-ar.stopCh:
				return
			}
		}
	})

	ar.logger.Info("auto-restart monitor started", "max_retries", ar.maxRetries)
}

// Stop signals the auto-restart check goroutine to exit. Safe to
// call multiple times — the second and subsequent calls are no-ops.
func (ar *AutoRestarter) Stop() {
	ar.stopOnce.Do(func() {
		close(ar.stopCh)
	})
}

func (ar *AutoRestarter) handleCrash(ctx context.Context, appID, containerID string) {
	ar.logger.Warn("container crashed, attempting restart",
		"app_id", appID,
		"container_id", containerID,
	)

	ar.store.UpdateAppStatus(ctx, appID, "crashed")

	ar.events.Publish(ctx, core.NewEvent(core.EventAppCrashed, "deploy",
		core.AppEventData{AppID: appID, Status: "crashed"}))

	// Attempt restart with backoff
	for attempt := 1; attempt <= ar.maxRetries; attempt++ {
		time.Sleep(time.Duration(attempt*5) * time.Second)

		if ar.runtime == nil {
			break
		}

		if err := ar.runtime.Restart(ctx, containerID); err != nil {
			ar.logger.Error("restart attempt failed",
				"app_id", appID,
				"attempt", attempt,
				"error", err,
			)
			continue
		}

		ar.logger.Info("container restarted successfully",
			"app_id", appID,
			"attempt", attempt,
		)
		ar.store.UpdateAppStatus(ctx, appID, "running")
		return
	}

	ar.logger.Error("auto-restart failed after max retries",
		"app_id", appID,
		"max_retries", ar.maxRetries,
	)
	ar.store.UpdateAppStatus(ctx, appID, "failed")
}

func (ar *AutoRestarter) checkCrashed() {
	if ar.runtime == nil {
		return
	}

	ctx := context.Background()
	containers, err := ar.runtime.ListByLabels(ctx, map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		return
	}

	for _, c := range containers {
		if c.State == "exited" || c.State == "dead" {
			appID := c.Labels["monster.app.id"]
			if appID != "" {
				ar.handleCrash(ctx, appID, c.ID)
			}
		}
	}
}
