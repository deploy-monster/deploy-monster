package database

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module manages provisioning and lifecycle of managed databases
// (PostgreSQL, MySQL, MariaDB, Redis, MongoDB) as Docker containers.
type Module struct {
	core   *core.Core
	store  core.Store
	logger *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "database" }
func (m *Module) Name() string                { return "Database Manager" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())
	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("database manager started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }

func (m *Module) Health() core.HealthStatus {
	if m.core == nil || m.core.Services == nil || m.core.Services.Container == nil {
		return core.HealthDegraded
	}
	return core.HealthOK
}
