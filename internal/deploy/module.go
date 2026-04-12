package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the deployment module.
//
// Lifecycle note for Tier 74: pre-Tier-74 autoRollback was created
// as a local variable inside Start() and never retained, so its
// SubscribeAsync closure kept the manager alive while Module.Stop
// had no handle to drain it. During shutdown a deploy.failed event
// could trigger a rollback that raced with Docker.Close. autoRollback
// is now a Module field and Module.Stop drains it before closing
// Docker.
type Module struct {
	core         *core.Core
	docker       *DockerManager
	store        core.Store
	logger       *slog.Logger
	autoRestart  *AutoRestarter
	autoRollback *AutoRollbackManager
}

// New creates a new deploy module.
func New() *Module {
	return &Module{}
}

func (m *Module) ID() string                  { return "deploy" }
func (m *Module) Name() string                { return "Deploy Engine" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())

	// Get store reference
	if c.Store == nil {
		return fmt.Errorf("database store not available")
	}
	m.store = c.Store

	// Create Docker manager
	docker, err := NewDockerManager(c.Config.Docker.Host)
	if err != nil {
		m.logger.Warn("Docker not available — container operations will fail", "error", err)
		return nil // Non-fatal: allow startup without Docker for development
	}
	m.docker = docker

	// Apply container resource defaults from config
	if c.Config.Docker.DefaultCPUQuota > 0 || c.Config.Docker.DefaultMemoryMB > 0 {
		docker.SetResourceDefaults(c.Config.Docker.DefaultCPUQuota, c.Config.Docker.DefaultMemoryMB)
		m.logger.Info("container resource defaults set",
			"cpu_quota", c.Config.Docker.DefaultCPUQuota,
			"memory_mb", c.Config.Docker.DefaultMemoryMB)
	}

	// Register container runtime in service registry
	c.Services.Container = docker

	m.logger.Info("docker connected", "host", c.Config.Docker.Host)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	// Tier 100 (Phase 3.1.2 restart storm): reclaim any deployment rows
	// left in "deploying" or "building" state by a crashed master. This
	// runs regardless of Docker availability — the sweep is a pure
	// database operation and cleaning up stale rows is more important
	// than having a healthy Docker connection. Guarded on m.store so
	// tests that bypass Init (no store wired) still exercise Start.
	if m.store != nil {
		m.reclaimStaleDeployments(ctx)
	}

	if m.docker != nil {
		// Ensure monster-network exists
		if err := m.docker.EnsureNetwork(ctx, "monster-network"); err != nil {
			m.logger.Warn("failed to create monster-network", "error", err)
		}

		// Clean up orphan containers from prior crash
		m.cleanOrphanContainers(ctx)

		// Start auto-restart monitor
		m.autoRestart = NewAutoRestarter(m.docker, m.store, m.core.Events, m.logger)
		m.autoRestart.Start()

		// Start auto-rollback on failed deployments. Tier 74: keep a
		// handle on the Module so Stop can drain in-flight rollbacks.
		m.autoRollback = NewAutoRollbackManager(m.store, m.docker, m.core.Events, m.logger)
		m.autoRollback.Start()
	}

	m.logger.Info("deploy module started")
	return nil
}

// reclaimStaleDeployments sweeps deployment rows left in "deploying" or
// "building" status by a crashed master and marks them "failed". This is
// the Phase 3.1.2 (restart storm) fix: before Tier 100 a master crash
// mid-deploy left the deployment row eternally in-flight, so the UI,
// auto-rollback listener, and "next deploy" version counter all saw a
// phantom in-progress record that never completed.
//
// The sweep also reconciles the app's status field: any app with a
// latest deployment in the just-reclaimed set is marked "failed" so
// operators see the crash reflected in the app list without having to
// drill into the deployment history.
func (m *Module) reclaimStaleDeployments(ctx context.Context) {
	reclaimed := 0
	for _, staleStatus := range []string{"deploying", "building"} {
		deps, err := m.store.ListDeploymentsByStatus(ctx, staleStatus)
		if err != nil {
			m.logger.Warn("reclaim sweep: list failed",
				"status", staleStatus, "error", err)
			continue
		}
		for i := range deps {
			d := &deps[i]
			finished := time.Now()
			d.Status = "failed"
			d.FinishedAt = &finished
			// BuildLog is a free-text field; append a reclaim marker so
			// operators reading the deployment log see exactly why this
			// row was transitioned to failed on startup.
			reason := "[reclaimed on startup after crash — previous status: " + staleStatus + "]"
			if d.BuildLog == "" {
				d.BuildLog = reason
			} else {
				d.BuildLog = d.BuildLog + "\n" + reason
			}
			if err := m.store.UpdateDeployment(ctx, d); err != nil {
				m.logger.Warn("reclaim sweep: update failed",
					"deployment_id", d.ID, "error", err)
				continue
			}
			// Reflect the reclaim on the app's own status so the app list
			// doesn't show a stale "deploying" badge after a crash.
			if err := m.store.UpdateAppStatus(ctx, d.AppID, "failed"); err != nil {
				m.logger.Warn("reclaim sweep: app status update failed",
					"app_id", d.AppID, "error", err)
			}
			reclaimed++
		}
	}
	if reclaimed > 0 {
		m.logger.Info("reclaim sweep complete", "deployments", reclaimed)
	}
}

// cleanOrphanContainers removes containers with monster.managed=true whose
// app no longer exists in the store. This handles containers left behind by
// a crash or unclean shutdown.
func (m *Module) cleanOrphanContainers(ctx context.Context) {
	containers, err := m.docker.ListByLabels(ctx, map[string]string{"monster.managed": "true"})
	if err != nil {
		m.logger.Warn("orphan cleanup: failed to list containers", "error", err)
		return
	}

	removed := 0
	for _, c := range containers {
		appID := c.Labels["monster.app.id"]
		if appID == "" {
			continue
		}

		_, err := m.store.GetApp(ctx, appID)
		if err == nil {
			continue // App exists, container is valid
		}

		// App not found — this container is an orphan
		m.logger.Info("removing orphan container",
			"container_id", c.ID[:12],
			"container_name", c.Name,
			"app_id", appID,
		)
		if err := m.docker.Remove(ctx, c.ID, true); err != nil {
			m.logger.Warn("orphan cleanup: failed to remove container",
				"container_id", c.ID[:12],
				"error", err,
			)
			continue
		}
		removed++
	}

	if removed > 0 {
		m.logger.Info("orphan cleanup complete", "removed", removed)
	}
}

func (m *Module) Stop(_ context.Context) error {
	// Tier 74: drain autoRollback BEFORE closing Docker. Pre-Tier-74
	// autoRollback had no Stop at all, so a deploy.failed event
	// published during shutdown could race with docker.Close and
	// dereference a closed client.
	if m.autoRollback != nil {
		m.autoRollback.Stop()
	}
	if m.autoRestart != nil {
		m.autoRestart.Stop()
	}
	if m.docker != nil {
		return m.docker.Close()
	}
	return nil
}

func (m *Module) Health() core.HealthStatus {
	if m.docker == nil {
		return core.HealthDegraded
	}
	if err := m.docker.Ping(); err != nil {
		return core.HealthDown
	}
	return core.HealthOK
}

// Docker returns the Docker manager for use by other modules.
func (m *Module) Docker() *DockerManager {
	return m.docker
}
