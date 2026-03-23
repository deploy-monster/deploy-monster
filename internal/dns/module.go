package dns

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/dns/providers"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module manages DNS record synchronization.
// When domains are added/removed, it creates/deletes DNS records
// via the configured provider (Cloudflare, Route53, etc.).
type Module struct {
	core   *core.Core
	store  core.Store
	logger *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "dns.sync" }
func (m *Module) Name() string                { return "DNS Synchronizer" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())

	// Register Cloudflare if token configured
	if c.Config.DNS.CloudflareToken != "" {
		cf := providers.NewCloudflare(c.Config.DNS.CloudflareToken)
		c.Services.RegisterDNSProvider("cloudflare", cf)
		m.logger.Info("DNS provider registered", "provider", "cloudflare")
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	// Subscribe to domain events for auto-sync
	m.core.Events.SubscribeAsync(core.EventDomainAdded, func(ctx context.Context, event core.Event) error {
		if data, ok := event.Data.(core.DomainEventData); ok {
			m.logger.Info("domain added, syncing DNS", "fqdn", data.FQDN)
			// DNS sync logic will query the configured provider
		}
		return nil
	})

	m.core.Events.SubscribeAsync(core.EventDomainRemoved, func(ctx context.Context, event core.Event) error {
		m.logger.Info("domain removed, cleaning DNS")
		return nil
	})

	m.logger.Info("DNS sync started", "providers", m.core.Services.DNSProviders())
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }
func (m *Module) Health() core.HealthStatus    { return core.HealthOK }
