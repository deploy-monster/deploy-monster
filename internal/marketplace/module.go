package marketplace

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the marketplace — one-click deploy of popular apps
// from a curated template registry.
type Module struct {
	core     *core.Core
	store    core.Store
	registry *TemplateRegistry
	logger   *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "marketplace" }
func (m *Module) Name() string                { return "Marketplace" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())

	m.registry = NewTemplateRegistry()
	m.registry.LoadBuiltins()

	// Load additional templates for 100+ total
	for _, t := range GetMoreTemplates100() {
		m.registry.Add(t)
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("marketplace started", "templates", m.registry.Count())
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }
func (m *Module) Health() core.HealthStatus    { return core.HealthOK }

// Registry returns the template registry for API handlers.
func (m *Module) Registry() *TemplateRegistry { return m.registry }
