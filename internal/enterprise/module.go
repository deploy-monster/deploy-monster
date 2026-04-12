package enterprise

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements enterprise features — white-label branding,
// reseller management, WHMCS integration, GDPR compliance, and HA.
type Module struct {
	core   *core.Core
	logger *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "enterprise" }
func (m *Module) Name() string                { return "Enterprise Engine" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "billing"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())
	return nil
}

func (m *Module) Start(_ context.Context) error {
	if !m.core.Config.Enterprise.Enabled {
		m.logger.Info("enterprise features disabled")
		return nil
	}
	m.logger.Info("enterprise engine started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }
func (m *Module) Health() core.HealthStatus    { return core.HealthOK }
