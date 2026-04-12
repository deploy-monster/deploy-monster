package gitsources

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/gitsources/providers"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module manages Git source provider connections.
type Module struct {
	core   *core.Core
	store  core.Store
	logger *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "gitsources" }
func (m *Module) Name() string                { return "Git Source Manager" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())

	// Register available Git providers in Services
	for name, factory := range providers.Registry {
		provider := factory("")
		c.Services.RegisterGitProvider(name, provider)
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("git source manager started", "providers", m.core.Services.GitProviders())
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }

func (m *Module) Health() core.HealthStatus {
	// If git source credentials are configured but no providers registered, something is wrong
	if m.core != nil && m.core.Config != nil && m.core.Services != nil {
		cfg := m.core.Config.GitSources
		hasConfig := cfg.GitHubClientID != "" || cfg.GitLabClientID != ""
		if hasConfig && len(m.core.Services.GitProviders()) == 0 {
			return core.HealthDegraded
		}
	}
	return core.HealthOK
}
