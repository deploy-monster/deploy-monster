package handlers

import (
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// UsageHistoryHandler serves resource usage over time for billing and charts.
type UsageHistoryHandler struct {
	store core.Store
}

func NewUsageHistoryHandler(store core.Store) *UsageHistoryHandler {
	return &UsageHistoryHandler{store: store}
}

// UsageBucket represents aggregated usage for a time period.
type UsageBucket struct {
	Hour         string  `json:"hour"`
	CPUSeconds   float64 `json:"cpu_seconds"`
	RAMMBHours   float64 `json:"ram_mb_hours"`
	BandwidthMB  float64 `json:"bandwidth_mb"`
	BuildSeconds float64 `json:"build_seconds"`
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
