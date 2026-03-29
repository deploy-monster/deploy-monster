package strategies

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Strategy defines the deployment strategy interface.
type Strategy interface {
	Name() string
	Execute(ctx context.Context, plan *DeployPlan) error
}

// DeployPlan holds all information needed to execute a deployment.
type DeployPlan struct {
	App            *core.Application
	Deployment     *core.Deployment
	NewImage       string
	OldContainerID string
	Runtime        core.ContainerRuntime
	Store          core.Store
	Events         *core.EventBus
	Logger         *slog.Logger
	Graceful       *GracefulConfig
}

// GracefulConfig holds configuration for graceful deployment.
type GracefulConfig struct {
	DrainTimeout        time.Duration
	HealthCheckInterval time.Duration
	StartupTimeout      time.Duration
}

// DefaultGracefulConfig returns the default graceful configuration.
func DefaultGracefulConfig() GracefulConfig {
	return GracefulConfig{
		DrainTimeout:        30 * time.Second,
		HealthCheckInterval: 500 * time.Millisecond,
		StartupTimeout:      60 * time.Second,
	}
}

// New creates a strategy by name.
func New(name string) Strategy {
	switch name {
	case "rolling":
		return &Rolling{}
	default:
		return &Recreate{}
	}
}

// buildLabels creates container labels including HTTP routing labels from domains.
func buildLabels(ctx context.Context, plan *DeployPlan) map[string]string {
	labels := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         plan.App.ID,
		"monster.app.name":       plan.App.Name,
		"monster.tenant":         plan.App.TenantID,
		"monster.deploy.version": fmt.Sprintf("%d", plan.Deployment.Version),
	}

	// Fetch domains for this app and add HTTP routing labels
	if plan.Store != nil {
		domains, err := plan.Store.ListDomainsByApp(ctx, plan.App.ID)
		if err == nil && len(domains) > 0 {
			// Get port from app or default to 80
			port := plan.App.Port
			if port <= 0 {
				port = 80
			}

			// Add routing labels for each domain
			for i, domain := range domains {
				routerName := fmt.Sprintf("%s-%d", plan.App.Name, i)
				// Host rule for routing
				labels[fmt.Sprintf("monster.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain.FQDN)
				// Backend port
				labels[fmt.Sprintf("monster.http.services.%s.loadbalancer.server.port", routerName)] = fmt.Sprintf("%d", port)
			}
		}
	}

	return labels
}

// Recreate stops the old container, then starts the new one.
// Simple but causes brief downtime. Use for single-replica apps or when speed is priority.
type Recreate struct{}

func (r *Recreate) Name() string { return "recreate" }

func (r *Recreate) Execute(ctx context.Context, plan *DeployPlan) error {
	cfg := plan.Graceful
	if cfg == nil {
		cfg = &GracefulConfig{}
	}
	drainTimeout := cfg.DrainTimeout
	if drainTimeout == 0 {
		drainTimeout = 10 * time.Second
	}

	// 1. Stop old container with graceful drain
	if plan.OldContainerID != "" {
		if plan.Logger != nil {
			plan.Logger.Info("stopping old container", "container", plan.OldContainerID)
		}
		if err := plan.Runtime.Stop(ctx, plan.OldContainerID, int(drainTimeout.Seconds())); err != nil {
			// Non-fatal — old container might already be stopped
			if plan.Logger != nil {
				plan.Logger.Debug("stop returned error (may already be stopped)", "error", err)
			}
		}
		_ = plan.Runtime.Remove(ctx, plan.OldContainerID, true)
	}

	// 2. Start new container with routing labels
	labels := buildLabels(ctx, plan)

	containerName := fmt.Sprintf("monster-%s-%d", plan.App.Name, plan.Deployment.Version)
	containerID, err := plan.Runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         plan.NewImage,
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		return fmt.Errorf("start new container: %w", err)
	}

	plan.Deployment.ContainerID = containerID

	if plan.Logger != nil {
		plan.Logger.Info("container started", "container", containerID, "id", containerID)
	}

	return nil
}

// Rolling starts the new container first, waits for health, then stops the old one.
// Zero-downtime for multi-instance services.
type Rolling struct{}

func (r *Rolling) Name() string { return "rolling" }

func (r *Rolling) Execute(ctx context.Context, plan *DeployPlan) error {
	cfg := plan.Graceful
	if cfg == nil {
		defaultCfg := DefaultGracefulConfig()
		cfg = &defaultCfg
	}

	// 1. Start new container (alongside old) with routing labels
	labels := buildLabels(ctx, plan)

	containerName := fmt.Sprintf("monster-%s-%d", plan.App.Name, plan.Deployment.Version)
	containerID, err := plan.Runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         plan.NewImage,
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		return fmt.Errorf("start new container: %w", err)
	}

	plan.Deployment.ContainerID = containerID

	if plan.Logger != nil {
		plan.Logger.Info("new container started, waiting for health",
			"container", containerName,
			"id", containerID,
		)
	}

	// 2. Wait for new container to be healthy
	// Poll health endpoint with proper timeout
	healthyCtx, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()

	healthy := false
	ticker := time.NewTicker(cfg.HealthCheckInterval)
	defer ticker.Stop()

healthLoop:
	for {
		select {
		case <-ticker.C:
			// Check if container is still running
			stats, err := plan.Runtime.Stats(healthyCtx, containerID)
			if err != nil {
				if plan.Logger != nil {
					plan.Logger.Debug("health check failed", "error", err)
				}
				continue
			}

			// Check health status if available
			if stats != nil {
				if stats.Health == "healthy" {
					healthy = true
					break healthLoop
				}
				// If container has no health check defined, check if it's running
				if stats.Health == "" && stats.Running {
					// Give it a moment to stabilize
					time.Sleep(2 * time.Second)
					healthy = true
					break healthLoop
				}
				// If health check is failing, continue waiting
				if stats.Health == "unhealthy" {
					if plan.Logger != nil {
						plan.Logger.Warn("container reported unhealthy, continuing to wait",
							"container", containerID,
						)
					}
				}
			}

		case <-healthyCtx.Done():
			// Timeout waiting for health
			if plan.Logger != nil {
				plan.Logger.Error("timeout waiting for container to become healthy",
					"container", containerID,
					"timeout", cfg.StartupTimeout,
				)
			}
			// Clean up the new container
			_ = plan.Runtime.Stop(ctx, containerID, 5)
			_ = plan.Runtime.Remove(ctx, containerID, true)
			return fmt.Errorf("container did not become healthy within %v", cfg.StartupTimeout)
		}
	}

	if plan.Logger != nil {
		plan.Logger.Info("container is healthy, updating routing",
			"container", containerID,
			"healthy", healthy,
		)
	}

	// 3. Drain and stop old container with graceful shutdown
	if plan.OldContainerID != "" {
		if plan.Logger != nil {
			plan.Logger.Info("draining old container",
				"container", plan.OldContainerID,
				"drain_timeout", cfg.DrainTimeout,
			)
		}

		// Stop with graceful drain timeout
		if err := plan.Runtime.Stop(ctx, plan.OldContainerID, int(cfg.DrainTimeout.Seconds())); err != nil {
			if plan.Logger != nil {
				plan.Logger.Debug("stop returned error", "error", err)
			}
		}
		_ = plan.Runtime.Remove(ctx, plan.OldContainerID, true)

		if plan.Logger != nil {
			plan.Logger.Info("old container removed", "container", plan.OldContainerID)
		}
	}

	return nil
}
