package billing

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the billing engine — plans, usage metering,
// quota enforcement, and Stripe integration.
type Module struct {
	core   *core.Core
	store  core.Store
	meter  *Meter
	logger *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "billing" }
func (m *Module) Name() string                { return "Billing Engine" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())
	return nil
}

func (m *Module) Start(_ context.Context) error {
	if !m.core.Config.Billing.Enabled {
		m.logger.Info("billing disabled")
		return nil
	}
	// Start usage metering
	m.meter = NewMeter(m.store, m.core.Services.Container, m.logger)
	m.meter.Start()

	m.logger.Info("billing engine started")
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	if m.meter != nil {
		m.meter.Stop()
	}
	return nil
}
func (m *Module) Health() core.HealthStatus    { return core.HealthOK }
