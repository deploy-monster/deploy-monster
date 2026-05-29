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
	if c.Services == nil {
		c.Services = core.NewServices()
	}

	for name, factory := range providers.Registry {
		token := m.providerToken(name)
		if name == "custom" || token != "" {
			c.Services.RegisterVPSProvisioner(name, factory(token))
			m.logger.Debug("VPS provider registered", "provider", name)
			continue
		}
		m.logger.Debug("VPS provider available but not configured", "provider", name)
	}

	return nil
}

func (m *Module) providerToken(name string) string {
	if m.core == nil || m.core.Config == nil {
		return ""
	}
	switch name {
	case "hetzner":
		return m.core.Config.VPSProviders.HetznerToken
	case "digitalocean":
		return m.core.Config.VPSProviders.DigitalOceanToken
	case "vultr":
		return m.core.Config.VPSProviders.VultrToken
	case "linode":
		return m.core.Config.VPSProviders.LinodeToken
	default:
		return ""
	}
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("VPS provider manager started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }
func (m *Module) Health() core.HealthStatus    { return core.HealthOK }
