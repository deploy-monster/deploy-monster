package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/billing"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BillingHandler manages billing and subscription endpoints.
type BillingHandler struct {
	store core.Store
}

func NewBillingHandler(store core.Store) *BillingHandler {
	return &BillingHandler{store: store}
}

// ListPlans handles GET /api/v1/billing/plans
func (h *BillingHandler) ListPlans(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": billing.BuiltinPlans})
}

// GetUsage handles GET /api/v1/billing/usage
func (h *BillingHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Get current plan
	tenant, err := h.store.GetTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Find matching plan
	var currentPlan *billing.Plan
	for i := range billing.BuiltinPlans {
		if billing.BuiltinPlans[i].ID == tenant.PlanID {
			currentPlan = &billing.BuiltinPlans[i]
			break
		}
	}

	if currentPlan == nil {
		currentPlan = &billing.BuiltinPlans[0] // Default to free
	}

	// Get app count
	_, appCount, err := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 1, 0)
	if err != nil {
		slog.Warn("billing: failed to list apps", "error", err)
	}
	containersUsed := latestUsageMetric(r.Context(), h.store, claims.TenantID, "containers")
	ramUsedMB := latestUsageMetric(r.Context(), h.store, claims.TenantID, "ram_mb", "memory_mb", "ram")

	// Quota check — use the ctx-aware variant so a client disconnect
	// cancels the probe instead of letting it run against a dead
	// HTTP response writer.
	status, err := billing.QuotaCheckCtx(r.Context(), h.store, claims.TenantID, *currentPlan)
	if err != nil {
		slog.Warn("billing: failed quota check", "error", err)
	}
	appsOK := currentPlan.MaxApps < 0 || appCount < currentPlan.MaxApps
	if status != nil {
		appsOK = status.AppsOK
	}
	containersOK := currentPlan.MaxContainers < 0 || containersUsed < currentPlan.MaxContainers
	ramOK := currentPlan.MaxRAMMB < 0 || ramUsedMB <= currentPlan.MaxRAMMB

	writeJSON(w, http.StatusOK, map[string]any{
		"plan":             currentPlan,
		"apps_used":        appCount,
		"apps_limit":       currentPlan.MaxApps,
		"containers_used":  containersUsed,
		"containers_limit": currentPlan.MaxContainers,
		"ram_used_mb":      ramUsedMB,
		"ram_limit_mb":     currentPlan.MaxRAMMB,
		"quota": map[string]any{
			"apps_used":        appCount,
			"apps_limit":       currentPlan.MaxApps,
			"apps_ok":          appsOK,
			"containers_used":  containersUsed,
			"containers_limit": currentPlan.MaxContainers,
			"containers_ok":    containersOK,
			"ram_used_mb":      ramUsedMB,
			"ram_limit_mb":     currentPlan.MaxRAMMB,
			"ram_ok":           ramOK,
		},
	})
}

func latestUsageMetric(ctx context.Context, store core.Store, tenantID string, metricTypes ...string) int {
	records, _, err := store.ListUsageRecordsByTenant(ctx, tenantID, 100, 0)
	if err != nil {
		slog.Warn("billing: failed to list usage records", "tenant_id", tenantID, "error", err)
		return 0
	}
	metricSet := make(map[string]struct{}, len(metricTypes))
	for _, metricType := range metricTypes {
		metricSet[metricType] = struct{}{}
	}
	var latest *core.UsageRecord
	for _, record := range records {
		if _, ok := metricSet[record.MetricType]; !ok {
			continue
		}
		if latest == nil || usageRecordTime(record).After(usageRecordTime(*latest)) {
			rec := record
			latest = &rec
		}
	}
	if latest != nil {
		return int(latest.Value)
	}
	return 0
}

func usageRecordTime(record core.UsageRecord) time.Time {
	if !record.HourBucket.IsZero() {
		return record.HourBucket
	}
	return record.CreatedAt
}
