package handlers

import (
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// UsageHistoryHandler serves resource usage over time for billing and charts.
type UsageHistoryHandler struct {
	bolt core.BoltStorer
}

func NewUsageHistoryHandler(bolt core.BoltStorer) *UsageHistoryHandler {
	return &UsageHistoryHandler{bolt: bolt}
}

// UsageBucket represents aggregated usage for a time period.
type UsageBucket struct {
	Hour         string  `json:"hour"`
	CPUSeconds   float64 `json:"cpu_seconds"`
	RAMMBHours   float64 `json:"ram_mb_hours"`
	BandwidthMB  float64 `json:"bandwidth_mb"`
	BuildSeconds float64 `json:"build_seconds"`
}

// usageHistory is the persisted usage data for a tenant.
type usageHistory struct {
	Buckets []UsageBucket `json:"buckets"`
}

// Hourly handles GET /api/v1/billing/usage/history
func (h *UsageHistoryHandler) Hourly(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "24h"
	}

	var count int
	switch period {
	case "7d":
		count = 168
	case "30d":
		count = 720
	default:
		count = 24
	}

	// Try to load real usage data from BBolt
	bucketKey := claims.TenantID + ":" + period
	var stored usageHistory
	if err := h.bolt.Get("usage_history", bucketKey, &stored); err == nil && len(stored.Buckets) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant_id": claims.TenantID,
			"period":    period,
			"buckets":   stored.Buckets,
			"count":     len(stored.Buckets),
		})
		return
	}

	// No stored data — return empty time series
	now := time.Now()
	buckets := make([]UsageBucket, count)
	for i := range buckets {
		buckets[i] = UsageBucket{
			Hour: now.Add(-time.Duration(count-1-i) * time.Hour).Format("2006-01-02T15:00"),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": claims.TenantID,
		"period":    period,
		"buckets":   buckets,
		"count":     count,
	})
}
