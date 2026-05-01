package vps

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/vps/providers"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module manages VPS providers and remote server provisioning.
type Module struct {
	core   *core.Core
	store  core.Store
	logger *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "vps" }
func (m *Module) Name() string                { return "VPS Provider Manager" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())

	// Register built-in provider factories
	for name := range providers.Registry {
		m.logger.Debug("VPS provider available", "provider", name)
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("VPS provider manager started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }
func (m *Module) Health() core.HealthStatus    { return core.HealthOK }
