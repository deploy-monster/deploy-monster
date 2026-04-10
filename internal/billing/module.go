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
	core    *core.Core
	store   core.Store
	meter   *Meter
	stripe  *StripeClient
	webhook *StripeEventHandler
	plans   []Plan
	logger  *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "billing" }
func (m *Module) Name() string                { return "Billing Engine" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

// StripeClient returns the initialized Stripe client, or nil when Stripe is
// not configured. External packages (api router, tests) use this to wire up
// the webhook handler and metered usage reporting.
func (m *Module) StripeClient() *StripeClient { return m.stripe }

// WebhookHandler returns the Stripe event handler exposed on the HTTP webhook
// endpoint. Returns nil when Stripe is not configured.
func (m *Module) WebhookHandler() *StripeEventHandler { return m.webhook }

// Plans returns the plan catalog the billing engine is using. This is the
// built-in catalog today but kept as an accessor so operators can later
// override or append plans via config.
func (m *Module) Plans() []Plan { return m.plans }

// Init sets up the billing module. Stripe wiring happens here (not in Start)
// so dependent modules — notably the API router — can read WebhookHandler()
// from their own Init phase once their Dependencies() list includes "billing".
func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())
	m.plans = BuiltinPlans

	if !c.Config.Billing.Enabled {
		return nil
	}

	// Stripe is optional — the billing engine also works with manual billing
	// or external integrations. Only initialize the Stripe client when both
	// secret and webhook keys are present.
	cfg := c.Config.Billing
	if cfg.StripeSecretKey != "" && cfg.StripeWebhookKey != "" {
		m.stripe = NewStripeClient(cfg.StripeSecretKey, cfg.StripeWebhookKey)
		m.webhook = NewStripeEventHandler(m.store, c.Events, m.stripe, m.plans, m.logger)
		m.logger.Info("stripe billing enabled")
	} else {
		m.logger.Info("stripe billing not configured — webhooks + usage reporting disabled")
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	if !m.core.Config.Billing.Enabled {
		m.logger.Info("billing disabled")
		return nil
	}

	// Start usage metering. The meter uses Stripe's metered-billing API to
	// push real usage records when Stripe is configured; otherwise it only
	// records usage locally for quota and dashboard purposes.
	m.meter = NewMeter(m.store, m.core.Services.Container, m.logger)
	m.meter.SetStripe(m.stripe, m.core.Events)
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
func (m *Module) Health() core.HealthStatus {
	if m.core != nil && m.core.Config.Billing.Enabled && m.meter == nil {
		return core.HealthDegraded
	}
	return core.HealthOK
}
