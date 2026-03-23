package deploy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the deployment module.
type Module struct {
	core    *core.Core
	docker  *DockerManager
	store   core.Store
	logger  *slog.Logger
}

// New creates a new deploy module.
func New() *Module {
	return &Module{}
}

func (m *Module) ID() string                    { return "deploy" }
func (m *Module) Name() string                  { return "Deploy Engine" }
func (m *Module) Version() string               { return "1.0.0" }
func (m *Module) Dependencies() []string        { return []string{"core.db"} }
func (m *Module) Routes() []core.Route          { return nil }
func (m *Module) Events() []core.EventHandler   { return nil }

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

	// Register container runtime in service registry
	c.Services.Container = docker

	m.logger.Info("docker connected", "host", c.Config.Docker.Host)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if m.docker != nil {
		// Ensure monster-network exists
		if err := m.docker.EnsureNetwork(ctx, "monster-network"); err != nil {
			m.logger.Warn("failed to create monster-network", "error", err)
		}

		// Start auto-restart monitor
		autoRestart := NewAutoRestarter(m.docker, m.store, m.core.Events, m.logger)
		autoRestart.Start()

		// Start image update checker
		imageChecker := NewImageUpdateChecker(m.store, m.core.Events, m.logger)
		imageChecker.Start()
	}

	m.logger.Info("deploy module started")
	return nil
}

func (m *Module) Stop(_ context.Context) error {
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
