package strategies

import (
	"context"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/deploy/graceful"
)

// CanaryWeight is a single phase in a canary rollout. Percent is the
// share of traffic the new (canary) container should receive during
// the phase; Dwell is how long to hold at that weight before
// advancing. Phases are executed in order and a failure at any phase
// triggers rollback.
type CanaryWeight struct {
	Percent int
	Dwell   time.Duration
}

// DefaultCanaryPlan is the 10 → 50 → 100 schedule the spec calls for
// in §7.3 of the product spec ("Gradual traffic shift"). 30 seconds
// of dwell at each intermediate step gives operators a real window
// to watch metrics before the next step; the final 100% phase has
// no dwell because it just means "old is drained, only new serves".
var DefaultCanaryPlan = []CanaryWeight{
	{Percent: 10, Dwell: 30 * time.Second},
	{Percent: 50, Dwell: 30 * time.Second},
	{Percent: 100, Dwell: 0},
}

// CanaryController is the optional hook a caller plugs in when it
// wants real weighted traffic splitting during a canary rollout.
// Without a controller the Canary strategy still runs the phase
// timeline — starting the new container, health-checking it, dwelling
// between phases, removing the old container at 100% — but the
// per-phase weight is advisory because the label-driven route table
// can only express "one backend per (host, path)". The deployer is
// expected to wire a concrete implementation backed by the ingress
// route table + lb.Weighted when it constructs the DeployPlan.
//
// AdjustWeight is called at the start of each phase with the percent
// of traffic that should be routed to newContainerID (the canary).
// Implementations must return nil for a successful adjustment; any
// error is treated as a phase failure and triggers rollback.
//
// Finalize is called after the last phase and is where a controller
// typically removes the old backend from its weighted pool entirely
// so the route no longer tracks the soon-to-be-stopped container.
type CanaryController interface {
	AdjustWeight(ctx context.Context, app *core.Application, oldContainerID, newContainerID string, percent int) error
	Finalize(ctx context.Context, app *core.Application, oldContainerID, newContainerID string) error
}

// Canary rolls out a new container in phases, shifting a growing
// fraction of traffic to it at each phase. Phase transitions are
// controlled by CanaryWeight entries; dwell time between phases is
// the "soak" during which an operator can observe metrics and abort
// via ctx cancellation if anything looks wrong.
//
// Rollback semantics:
//
//   - If the new container fails to become healthy, remove it and
//     leave the old running. The route table is untouched.
//   - If the context is canceled between phases, remove the new
//     container, call Finalize with percent=0 (controller resets the
//     split), and return a cancellation error.
//   - If AdjustWeight returns an error, remove the new container and
//     return the error wrapped with the failing phase percent.
//
// At 100% the old container is drained and removed; the canary
// promotion is committed at that point.
type Canary struct{}

func (c *Canary) Name() string { return "canary" }

// Execute runs the canary timeline. The plan's Graceful config
// controls the phase schedule (Graceful.CanaryPlan), health polling
// interval, and the final blue drain window. The optional
// Graceful.CanaryController gets called at each phase.
func (c *Canary) Execute(ctx context.Context, plan *DeployPlan) error {
	cfg := plan.Graceful
	if cfg == nil {
		defaultCfg := DefaultGracefulConfig()
		cfg = &defaultCfg
	}
	phases := cfg.CanaryPlan
	if len(phases) == 0 {
		phases = DefaultCanaryPlan
	}

	// 1. Start the canary container alongside the stable one.
	labels := buildLabels(ctx, plan)
	containerName := fmt.Sprintf("monster-%s-%d-canary", plan.App.Name, plan.Deployment.Version)
	newID, err := plan.Runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         plan.NewImage,
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		return fmt.Errorf("canary: start canary container: %w", err)
	}
	plan.Deployment.ContainerID = newID

	if plan.Logger != nil {
		plan.Logger.Info("canary: container started, waiting for health",
			"canary", newID,
			"stable", plan.OldContainerID,
			"phases", len(phases),
		)
	}

	// 2. Health check before any traffic is shifted. A failure here
	// leaves the stable container untouched — rollback is implicit.
	if err := c.waitHealthy(ctx, plan, newID, cfg); err != nil {
		c.rollback(context.Background(), plan, cfg, newID)
		return err
	}

	// 3. Walk the phase schedule. Each phase: adjust weight → dwell →
	// check ctx → advance. Any failure unwinds to the stable version.
	for i, phase := range phases {
		if plan.Logger != nil {
			plan.Logger.Info("canary: advancing phase",
				"phase", i+1,
				"total", len(phases),
				"percent", phase.Percent,
				"dwell", phase.Dwell,
			)
		}
		if cfg.CanaryController != nil {
			if err := cfg.CanaryController.AdjustWeight(ctx, plan.App, plan.OldContainerID, newID, phase.Percent); err != nil {
				if plan.Logger != nil {
					plan.Logger.Error("canary: AdjustWeight failed, rolling back",
						"phase", i+1, "percent", phase.Percent, "error", err,
					)
				}
				c.rollback(context.Background(), plan, cfg, newID)
				return fmt.Errorf("canary: phase %d (%d%%) weight adjust: %w", i+1, phase.Percent, err)
			}
		}

		if phase.Dwell > 0 {
			select {
			case <-time.After(phase.Dwell):
			case <-ctx.Done():
				if plan.Logger != nil {
					plan.Logger.Warn("canary: canceled during dwell, rolling back",
						"phase", i+1, "percent", phase.Percent,
					)
				}
				c.rollback(context.Background(), plan, cfg, newID)
				return fmt.Errorf("canary: canceled during phase %d (%d%%): %w",
					i+1, phase.Percent, ctx.Err())
			}
		}
	}

	// 4. Finalize — at 100% the controller typically drops the old
	// backend from the weighted pool. Non-fatal if it errors; at
	// this point traffic is already fully on the new container.
	if cfg.CanaryController != nil {
		if err := cfg.CanaryController.Finalize(ctx, plan.App, plan.OldContainerID, newID); err != nil && plan.Logger != nil {
			plan.Logger.Warn("canary: Finalize returned error, continuing with drain",
				"error", err,
			)
		}
	}

	// 5. Drain and remove the stable container. Failures here don't
	// affect availability — log and press on so the deployment is
	// marked successful.
	if plan.OldContainerID != "" {
		graceSec := stopGraceFor(cfg)
		if plan.Logger != nil {
			plan.Logger.Info("canary: draining stable container",
				"stable", plan.OldContainerID,
				"grace_seconds", graceSec,
			)
		}
		if err := graceful.Shutdown(ctx, plan.Runtime, plan.OldContainerID, graceSec); err != nil && plan.Logger != nil {
			plan.Logger.Warn("canary: stable drain returned error, ignoring",
				"stable", plan.OldContainerID,
				"error", err,
			)
		}
	}

	if plan.Logger != nil {
		plan.Logger.Info("canary: rollout complete", "canary", newID)
	}
	return nil
}

// waitHealthy mirrors the rolling/blue-green poll loop. An
// "unhealthy" intermediate state is logged but doesn't fail until
// the startup timeout expires, so containers with slow warm-up
// paths still have a chance to stabilize.
func (c *Canary) waitHealthy(ctx context.Context, plan *DeployPlan, containerID string, cfg *GracefulConfig) error {
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
					plan.Logger.Debug("canary: health check error", "error", err)
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
				time.Sleep(2 * time.Second)
				return nil
			}
		case <-healthyCtx.Done():
			return fmt.Errorf("canary: new container did not become healthy within %v", cfg.StartupTimeout)
		}
	}
}

// rollback tears down the canary container. When a CanaryController
// is present it's also asked to reset the traffic split to 0% so
// partial weight state doesn't leak into the next deployment.
func (c *Canary) rollback(ctx context.Context, plan *DeployPlan, cfg *GracefulConfig, newID string) {
	if cfg != nil && cfg.CanaryController != nil {
		// Best effort — a failing controller during rollback is
		// logged but doesn't block the container cleanup.
		if err := cfg.CanaryController.AdjustWeight(ctx, plan.App, plan.OldContainerID, newID, 0); err != nil && plan.Logger != nil {
			plan.Logger.Warn("canary: rollback AdjustWeight(0) failed", "error", err)
		}
	}
	if newID == "" {
		return
	}
	if plan.Logger != nil {
		plan.Logger.Info("canary: removing failed container", "canary", newID)
	}
	_ = plan.Runtime.Stop(ctx, newID, 5)
	_ = plan.Runtime.Remove(ctx, newID, true)
}
