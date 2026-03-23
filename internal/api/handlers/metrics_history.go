package handlers

import (
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// MetricsHistoryHandler serves historical metrics data for charts.
type MetricsHistoryHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewMetricsHistoryHandler(store core.Store, runtime core.ContainerRuntime) *MetricsHistoryHandler {
	return &MetricsHistoryHandler{store: store, runtime: runtime}
}

// MetricsPoint represents a single data point in a time series.
type MetricsPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	CPUPercent float64   `json:"cpu_percent"`
	MemoryMB   int64     `json:"memory_mb"`
	NetworkRx  int64     `json:"network_rx_mb"`
	NetworkTx  int64     `json:"network_tx_mb"`
}

// AppMetrics handles GET /api/v1/apps/{id}/metrics
func (h *MetricsHistoryHandler) AppMetrics(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	period := r.URL.Query().Get("period") // 1h, 24h, 7d, 30d
	if period == "" {
		period = "24h"
	}

	// Generate sample data points — in production would query metrics rollup tables
	now := time.Now()
	var points []MetricsPoint
	var interval time.Duration
	var count int

	switch period {
	case "1h":
		interval = time.Minute
		count = 60
	case "7d":
		interval = time.Hour
		count = 168
	case "30d":
		interval = 24 * time.Hour
		count = 30
	default: // 24h
		interval = 15 * time.Minute
		count = 96
	}

	for i := count - 1; i >= 0; i-- {
		points = append(points, MetricsPoint{
			Timestamp: now.Add(-time.Duration(i) * interval),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"period": period,
		"points": points,
		"count":  len(points),
	})
}

// ServerMetrics handles GET /api/v1/servers/{id}/metrics
func (h *MetricsHistoryHandler) ServerMetrics(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "24h"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"server_id": serverID,
		"period":    period,
		"points":    []MetricsPoint{},
	})
}
