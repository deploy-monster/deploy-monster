package billing

import (
	"context"
	"log/slog"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const meterCollectInterval = 60 * time.Second

// Meter collects resource usage per tenant for billing.
// Every 60 seconds it samples Docker stats and records usage.
type Meter struct {
	store   core.Store
	runtime core.ContainerRuntime
	logger  *slog.Logger
	stopCh  chan struct{}

	// stripe reports usage records to Stripe's metered billing API when
	// configured. Optional — the meter also functions in local-only mode.
	stripe *StripeClient
	events *core.EventBus
}

// NewMeter creates a new usage meter.
func NewMeter(store core.Store, runtime core.ContainerRuntime, logger *slog.Logger) *Meter {
	return &Meter{
		store:   store,
		runtime: runtime,
		logger:  logger,
		stopCh:  make(chan struct{}),
	}
}

// SetStripe attaches a Stripe client + event bus so the meter can push usage
// records to Stripe after each collection cycle. Passing a nil client
// explicitly disables Stripe reporting.
func (m *Meter) SetStripe(stripe *StripeClient, events *core.EventBus) {
	m.stripe = stripe
	m.events = events
}

// Start begins the metering collection loop.
func (m *Meter) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("panic in metering loop", "error", r)
			}
		}()
		ticker := time.NewTicker(meterCollectInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.collect()
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop halts the metering loop.
func (m *Meter) Stop() {
	close(m.stopCh)
}

func (m *Meter) collect() {
	if m.runtime == nil {
		return
	}

	ctx := context.Background()
	containers, err := m.runtime.ListByLabels(ctx, map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		m.logger.Debug("metering: failed to list containers", "error", err)
		return
	}

	// Group by tenant
	tenantUsage := make(map[string]*TenantUsage)
	for _, c := range containers {
		tenantID := c.Labels["monster.tenant"]
		if tenantID == "" {
			continue
		}

		usage, ok := tenantUsage[tenantID]
		if !ok {
			usage = &TenantUsage{}
			tenantUsage[tenantID] = usage
		}
		usage.Containers++
		usage.AppIDs = append(usage.AppIDs, c.Labels["monster.app.id"])
	}

	now := time.Now().UTC().Truncate(time.Hour)
	for tenantID, usage := range tenantUsage {
		record := &core.UsageRecord{
			TenantID:   tenantID,
			MetricType: "containers",
			Value:      float64(usage.Containers),
			HourBucket: now,
		}
		if err := m.store.CreateUsageRecord(ctx, record); err != nil {
			m.logger.Debug("metering: failed to save usage record", "tenant", tenantID, "error", err)
		}
	}

	// Push usage to Stripe's metered billing API when configured. This runs
	// after local recording so the dashboard remains authoritative even when
	// Stripe is down.
	if m.stripe != nil {
		m.reportUsageToStripe(ctx, tenantUsage, now)
	}

	m.logger.Debug("metering collected", "tenants", len(tenantUsage), "containers", len(containers))
}

// reportUsageToStripe pushes per-tenant container counts to Stripe as metered
// usage records. Tenants without a linked Stripe subscription item are
// silently skipped — that's the expected state for free-plan tenants.
func (m *Meter) reportUsageToStripe(ctx context.Context, tenantUsage map[string]*TenantUsage, bucket time.Time) {
	for tenantID, usage := range tenantUsage {
		tenant, err := m.store.GetTenant(ctx, tenantID)
		if err != nil {
			m.logger.Debug("metering: failed to load tenant for stripe reporting",
				"tenant", tenantID, "error", err)
			continue
		}
		md, err := GetStripeMetadata(tenant)
		if err != nil {
			m.logger.Debug("metering: failed to read stripe metadata",
				"tenant", tenantID, "error", err)
			continue
		}
		if md.SubscriptionItemID == "" {
			continue // tenant is not on a metered Stripe plan
		}

		qty := int64(usage.Containers)
		if err := m.stripe.ReportUsage(ctx, md.SubscriptionItemID, qty, bucket); err != nil {
			m.logger.Warn("metering: failed to report usage to stripe",
				"tenant", tenantID, "subscription_item_id", md.SubscriptionItemID, "error", err)
			continue
		}

		if m.events != nil {
			if err := m.events.EmitWithTenant(ctx, core.EventBillingUsageReported,
				"billing.meter", tenantID, "", map[string]any{
					"subscription_item_id": md.SubscriptionItemID,
					"quantity":             qty,
					"bucket":               bucket.Format(time.RFC3339),
				}); err != nil {
				m.logger.Debug("metering: failed to emit usage reported event",
					"tenant", tenantID, "error", err)
			}
		}
	}
}

// TenantUsage holds aggregated usage for a tenant.
type TenantUsage struct {
	Containers  int
	AppIDs      []string
	CPUSeconds  float64
	RAMMBHours  float64
	BandwidthMB float64
}

// QuotaCheck verifies if a tenant is within their plan limits.
func QuotaCheck(store core.Store, tenantID string, plan Plan) (*QuotaStatus, error) {
	ctx := context.Background()

	apps, total, err := store.ListAppsByTenant(ctx, tenantID, 1, 0)
	_ = apps
	if err != nil {
		return nil, err
	}

	status := &QuotaStatus{
		AppsUsed:  total,
		AppsLimit: plan.MaxApps,
		AppsOK:    plan.MaxApps < 0 || total < plan.MaxApps,
	}

	return status, nil
}

// QuotaStatus shows current usage vs limits.
type QuotaStatus struct {
	AppsUsed  int  `json:"apps_used"`
	AppsLimit int  `json:"apps_limit"`
	AppsOK    bool `json:"apps_ok"`
}
