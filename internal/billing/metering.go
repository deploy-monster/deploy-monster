package billing

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const (
	// meterCollectInterval is how often the meter samples Docker stats
	// and persists per-tenant usage records. One minute is frequent
	// enough for containers-as-billing-unit and lightweight enough to
	// run on the master node indefinitely.
	meterCollectInterval = 60 * time.Second

	// meterCollectTimeout bounds a single collect cycle. Before Tier 68
	// a stuck Docker daemon or slow database could let one collect tick
	// overlap the next — and because collect used context.Background()
	// there was no way to cancel the stuck call at all. Deadline is
	// 45 s so the worst case still fits well inside the 60 s tick.
	meterCollectTimeout = 45 * time.Second
)

// Meter collects resource usage per tenant for billing.
//
// Lifecycle notes for Tier 68:
//
//   - Stop is idempotent via stopOnce — the pre-Tier-68 code called
//     close(stopCh) directly and panicked with "close of closed
//     channel" on the second call.
//   - Stop blocks on wg.Wait so callers can rely on "after Stop
//     returns, no more metering collect calls will run". Before this
//     fix Stop only closed the channel and returned, leaving the loop
//     goroutine racing past the module shutdown.
//   - A cancellable stopCtx is derived from NewMeter and plumbed into
//     every collect/reportUsageToStripe call. Cancelling it (from Stop)
//     aborts any in-flight Docker list, database write, or Stripe API
//     call at the next I/O boundary.
//   - Start uses startOnce so a double-Start cannot spawn two goroutines
//     and silently double-count usage.
type Meter struct {
	store   core.Store
	runtime core.ContainerRuntime
	logger  *slog.Logger

	// stripe reports usage records to Stripe's metered billing API when
	// configured. Optional — the meter also functions in local-only mode.
	stripe *StripeClient
	events *core.EventBus

	// stopCh is the select-side signal for the loop goroutine. Kept as
	// a distinct channel (rather than folding onto stopCtx.Done()) so
	// existing tests that assert meter.stopCh != nil keep working
	// without churn.
	stopCh chan struct{}

	// Shutdown plumbing. stopCtx is canceled by Stop so long-running
	// Docker/DB/Stripe calls unblock promptly. wg tracks the loop
	// goroutine so Stop can wait for it to exit. stopOnce guards against
	// double-Stop panics; startOnce guards against double-Start leaking
	// goroutines.
	stopCtx    context.Context
	stopCancel context.CancelFunc
	stopOnce   sync.Once
	startOnce  sync.Once
	wg         sync.WaitGroup
}

// NewMeter creates a new usage meter. A nil logger is tolerated and
// replaced with slog.Default(); the pre-Tier-68 code would NPE in the
// collect loop's panic recovery branch if logger were nil.
func NewMeter(store core.Store, runtime core.ContainerRuntime, logger *slog.Logger) *Meter {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Meter{
		store:      store,
		runtime:    runtime,
		logger:     logger,
		stopCh:     make(chan struct{}),
		stopCtx:    ctx,
		stopCancel: cancel,
	}
}

// SetStripe attaches a Stripe client + event bus so the meter can push
// usage records to Stripe after each collection cycle. Passing a nil
// client explicitly disables Stripe reporting.
func (m *Meter) SetStripe(stripe *StripeClient, events *core.EventBus) {
	m.stripe = stripe
	m.events = events
}

// Start begins the metering collection loop. Subsequent calls are
// no-ops — starting the loop twice would spawn a duplicate goroutine,
// double-count usage on every tick, and deadlock Stop on wg.Wait.
func (m *Meter) Start() {
	m.startOnce.Do(func() {
		m.wg.Add(1)
		go m.loop()
	})
}

// Stop halts the metering loop. Safe to call multiple times; the second
// and subsequent calls are no-ops. Stop cancels the shared context
// (aborting any in-flight Docker, database, or Stripe calls) and waits
// for the loop goroutine to exit before returning.
func (m *Meter) Stop() {
	m.stopOnce.Do(func() {
		if m.stopCh != nil {
			close(m.stopCh)
		}
		if m.stopCancel != nil {
			m.stopCancel()
		}
	})
	m.wg.Wait()
}

func (m *Meter) loop() {
	defer m.wg.Done()
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
			// Guard against a tick that lands in the same scheduler
			// window as a concurrent Stop — bail instead of entering
			// collect with a cancelled context.
			if m.stopCtx != nil && m.stopCtx.Err() != nil {
				return
			}
			m.collect()
		case <-m.stopCh:
			return
		}
	}
}

// runCtx returns the cancellable context for a collect cycle. Falls
// back to context.Background() if the Meter was constructed via a bare
// struct literal (tests in other files may do this).
func (m *Meter) runCtx() context.Context {
	if m.stopCtx != nil {
		return m.stopCtx
	}
	return context.Background()
}

func (m *Meter) collect() {
	if m.runtime == nil {
		return
	}

	// Bound each tick so a slow docker daemon or database cannot overlap
	// the next tick. The parent is the shared stopCtx so Stop still
	// aborts a running collect instantly.
	ctx, cancel := context.WithTimeout(m.runCtx(), meterCollectTimeout)
	defer cancel()

	containers, err := m.runtime.ListByLabels(ctx, map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		// Pre-Tier 68 this was a Debug-level log which was effectively
		// invisible in production. A persistent metering failure would
		// silently compound over days until someone noticed empty
		// billing dashboards. Warn so it surfaces in the default log
		// level.
		m.logger.Warn("metering: failed to list containers", "error", err)
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
		if err := ctx.Err(); err != nil {
			m.logger.Debug("metering: collect aborted mid-write", "error", err)
			return
		}
		record := &core.UsageRecord{
			TenantID:   tenantID,
			MetricType: "containers",
			Value:      float64(usage.Containers),
			HourBucket: now,
		}
		if err := m.store.CreateUsageRecord(ctx, record); err != nil {
			// Elevated from Debug to Warn — silent billing write
			// failures are a real production incident.
			m.logger.Warn("metering: failed to save usage record",
				"tenant", tenantID, "error", err)
		}
	}

	// Push usage to Stripe's metered billing API when configured. This
	// runs after local recording so the dashboard remains authoritative
	// even when Stripe is down.
	if m.stripe != nil {
		m.reportUsageToStripe(ctx, tenantUsage, now)
	}

	m.logger.Debug("metering collected", "tenants", len(tenantUsage), "containers", len(containers))
}

// reportUsageToStripe pushes per-tenant container counts to Stripe as
// metered usage records. Tenants without a linked Stripe subscription
// item are silently skipped — that's the expected state for free-plan
// tenants. The ctx is the per-tick deadline derived in collect; if it
// is cancelled we abort the remaining tenants rather than pushing stale
// usage after a Stop.
func (m *Meter) reportUsageToStripe(ctx context.Context, tenantUsage map[string]*TenantUsage, bucket time.Time) {
	for tenantID, usage := range tenantUsage {
		if err := ctx.Err(); err != nil {
			m.logger.Debug("metering: stripe reporting aborted", "error", err)
			return
		}
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
//
// This is a backwards-compatibility wrapper around QuotaCheckCtx that
// passes a background context. New call sites should prefer
// QuotaCheckCtx so HTTP handlers can propagate their request context
// (and thus cancel a slow quota probe when the client disconnects).
func QuotaCheck(store core.Store, tenantID string, plan Plan) (*QuotaStatus, error) {
	return QuotaCheckCtx(context.Background(), store, tenantID, plan)
}

// QuotaCheckCtx is the context-aware quota check. Before Tier 68 the
// only entry point was QuotaCheck, which hardcoded context.Background()
// internally — an HTTP handler could not cancel a quota probe when its
// client disconnected, and the assigned-then-discarded `apps` slice
// variable wasted an allocation on every call.
func QuotaCheckCtx(ctx context.Context, store core.Store, tenantID string, plan Plan) (*QuotaStatus, error) {
	// We only need the count, not the slice. The pre-Tier-68 code read
	// `apps, total, err := ...` then discarded apps with `_ = apps`,
	// pointlessly decoding a row into the slice.
	_, total, err := store.ListAppsByTenant(ctx, tenantID, 1, 0)
	if err != nil {
		return nil, err
	}

	return &QuotaStatus{
		AppsUsed:  total,
		AppsLimit: plan.MaxApps,
		AppsOK:    plan.MaxApps < 0 || total < plan.MaxApps,
	}, nil
}

// QuotaStatus shows current usage vs limits.
type QuotaStatus struct {
	AppsUsed  int  `json:"apps_used"`
	AppsLimit int  `json:"apps_limit"`
	AppsOK    bool `json:"apps_ok"`
}
