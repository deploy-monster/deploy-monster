package strategies

import (
	"context"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/deploy/graceful"
)

// DefaultBlueGreenHoldback is the window during which the old
// ("blue") container stays alive after the new ("green") container
// has been promoted and is serving traffic. A non-zero holdback is
// the entire point of blue-green over rolling — it gives operators a
// cheap instant rollback target: traffic flipping back to blue is a
// single container restart away, no rebuild required.
const DefaultBlueGreenHoldback = 60 * time.Second

// BlueGreen runs two full container replicas in parallel: "blue"
// (the currently-serving version) stays live while "green" (the new
// version) is built, started, health-checked, and promoted. After
// promotion both containers continue running for a configurable
// holdback window, then blue is drained and removed.
//
// Semantics on this codebase:
//
//   - The route table is driven by Docker labels scanned by
//     internal/discovery/watcher. When green becomes running, the
//     watcher's next Upsert replaces the (host, path) route entry
//     with green's backend because both containers share the same
//     router labels. The holdback window guarantees blue is still
//     available during and immediately after that swap, so a bad
//     green deployment can be rolled back by stopping green and
//     letting the watcher's next sync re-point the route at blue.
//
//   - If the context is canceled during the holdback, the strategy
//     interprets that as "rollback requested" — green is removed and
//     blue is left running. This lets the auto-rollback subsystem
//     abort a blue-green promotion without the strategy needing a
//     separate rollback RPC.
//
//   - Health checking mirrors Rolling's polling loop so the two
//     strategies behave consistently when the new container fails to
//     become ready. BlueGreen does NOT stop blue on health failure —
//     it removes green and returns an error so the caller can decide
//     what to do.
type BlueGreen struct{}

func (b *BlueGreen) Name() string { return "blue-green" }

// Execute performs a blue-green deployment. The plan's OldContainerID
// names "blue"; the new image becomes "green". Errors returned from
// Execute are safe to treat as "deployment failed, blue still live"
// unless they occur AFTER the holdback — in which case green is
// fully promoted and blue has already been stopped, so the error is
// about cleanup rather than availability.
func (b *BlueGreen) Execute(ctx context.Context, plan *DeployPlan) error {
	cfg := plan.Graceful
	if cfg == nil {
		defaultCfg := DefaultGracefulConfig()
		cfg = &defaultCfg
	}

	// 1. Start green alongside blue. Both carry the same router labels
	// so the discovery watcher will eventually point the route table
	// at green once it notices the new container.
	labels := buildLabels(ctx, plan)

	containerName := fmt.Sprintf("monster-%s-%d-green", plan.App.Name, plan.Deployment.Version)
	greenID, err := plan.Runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         plan.NewImage,
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		return fmt.Errorf("blue-green: start green container: %w", err)
	}
	plan.Deployment.ContainerID = greenID

	if plan.Logger != nil {
		plan.Logger.Info("blue-green: green container started, waiting for health",
			"green", greenID,
			"blue", plan.OldContainerID,
		)
	}

	// 2. Poll for health using the same model Rolling uses. On any
	// failure we tear down green (leaving blue untouched) so the
	// caller always returns to a clean "blue is authoritative" state.
	if err := b.waitHealthy(ctx, plan, greenID, cfg); err != nil {
		b.cleanupGreen(ctx, plan, greenID)
		return err
	}

	if plan.Logger != nil {
		plan.Logger.Info("blue-green: green is healthy, holdback window begins",
			"green", greenID,
			"holdback", blueGreenHoldbackFor(cfg),
		)
	}

	// 3. Holdback window. Both containers run; traffic has (or is
	// about to be) swapped by the route watcher. A canceled ctx
	// during this window is interpreted as a rollback signal.
	holdback := blueGreenHoldbackFor(cfg)
	if holdback > 0 {
		select {
		case <-time.After(holdback):
		case <-ctx.Done():
			if plan.Logger != nil {
				plan.Logger.Warn("blue-green: holdback canceled, rolling back to blue",
					"green", greenID,
					"blue", plan.OldContainerID,
				)
			}
			b.cleanupGreen(context.Background(), plan, greenID)
			return fmt.Errorf("blue-green: promotion canceled during holdback: %w", ctx.Err())
		}
	}

	// 4. Drain and remove blue. Past the holdback, green owns traffic
	// and blue is just holding resources. Failures here don't affect
	// availability — log and keep going so the deployment marks as
	// completed even if the old container's shutdown glitches.
	if plan.OldContainerID != "" {
		graceSec := stopGraceFor(cfg)
		if plan.Logger != nil {
			plan.Logger.Info("blue-green: draining blue",
				"blue", plan.OldContainerID,
				"grace_seconds", graceSec,
			)
		}
		if err := graceful.Shutdown(ctx, plan.Runtime, plan.OldContainerID, graceSec); err != nil {
			if plan.Logger != nil {
				plan.Logger.Warn("blue-green: blue drain returned error, ignoring",
					"blue", plan.OldContainerID,
					"error", err,
				)
			}
		}
	}

	if plan.Logger != nil {
		plan.Logger.Info("blue-green: promotion complete", "green", greenID)
	}
	return nil
}

// waitHealthy is the same polling loop Rolling uses: poll Runtime
// stats until the container either reports healthy or the startup
// timeout expires. An "unhealthy" intermediate state is logged but
// doesn't immediately fail — the container may still recover within
// the timeout window.
func (b *BlueGreen) waitHealthy(ctx context.Context, plan *DeployPlan, containerID string, cfg *GracefulConfig) error {
	healthyCtx, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()

	ticker := time.NewTicker(cfg.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats, err := plan.Runtime.Stats(healthyCtx, containerID)
			if err != nil {
				if plan.Logger != nil {
					plan.Logger.Debug("blue-green: health check error", "error", err)
				}
				continue
			}
			if stats == nil {
				continue
			}
			if stats.Health == "healthy" {
				return nil
			}
			if stats.Health == "" && stats.Running {
				// Container has no Docker healthcheck defined — trust
				// "running" after a short grace so we don't hang forever
				// on images that don't ship a HEALTHCHECK.
				time.Sleep(2 * time.Second)
				return nil
			}
			if stats.Health == "unhealthy" && plan.Logger != nil {
				plan.Logger.Warn("blue-green: container reported unhealthy, still waiting",
					"container", containerID)
			}
		case <-healthyCtx.Done():
			return fmt.Errorf("blue-green: green container did not become healthy within %v", cfg.StartupTimeout)
		}
	}
}

// cleanupGreen tears down the green container on a failed promotion.
// Runs under a fresh context when the outer ctx has been canceled
// so a cancellation can't leave a dangling container on the host.
func (b *BlueGreen) cleanupGreen(ctx context.Context, plan *DeployPlan, greenID string) {
	if greenID == "" {
		return
	}
	if plan.Logger != nil {
		plan.Logger.Info("blue-green: removing failed green", "green", greenID)
	}
	_ = plan.Runtime.Stop(ctx, greenID, 5)
	_ = plan.Runtime.Remove(ctx, greenID, true)
}

// blueGreenHoldbackFor resolves the effective holdback duration.
// GracefulConfig carries a BlueGreenHoldback override; when zero or
// unset we use the package default so operators who flip strategies
// via config get sensible behavior without touching timing knobs.
func blueGreenHoldbackFor(cfg *GracefulConfig) time.Duration {
	if cfg != nil && cfg.BlueGreenHoldback > 0 {
		return cfg.BlueGreenHoldback
	}
	return DefaultBlueGreenHoldback
}
