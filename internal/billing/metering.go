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

	m.logger.Debug("metering collected", "tenants", len(tenantUsage), "containers", len(containers))
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
