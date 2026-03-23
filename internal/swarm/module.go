package swarm

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module manages Docker Swarm cluster operations.
// Handles swarm init, join token management, node labels, and overlay networks.
type Module struct {
	core   *core.Core
	logger *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "swarm" }
func (m *Module) Name() string                { return "Swarm Orchestrator" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())
	return nil
}

func (m *Module) Start(_ context.Context) error {
	if !m.core.Config.Swarm.Enabled {
		m.logger.Info("swarm mode disabled")
		return nil
	}
	m.logger.Info("swarm orchestrator started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }
func (m *Module) Health() core.HealthStatus    { return core.HealthOK }
