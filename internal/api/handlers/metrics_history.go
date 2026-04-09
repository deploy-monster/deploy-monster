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
	bolt    core.BoltStorer
}

func NewMetricsHistoryHandler(store core.Store, runtime core.ContainerRuntime, bolt core.BoltStorer) *MetricsHistoryHandler {
	return &MetricsHistoryHandler{store: store, runtime: runtime, bolt: bolt}
}

// MetricsPoint represents a single data point in a time series.
type MetricsPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	CPUPercent float64   `json:"cpu_percent"`
	MemoryMB   int64     `json:"memory_mb"`
	NetworkRx  int64     `json:"network_rx_mb"`
	NetworkTx  int64     `json:"network_tx_mb"`
}

// metricsRing wraps persisted metrics history for an app.
type metricsRing struct {
	Points []MetricsPoint `json:"points"`
}

// AppMetrics handles GET /api/v1/apps/{id}/metrics
func (h *MetricsHistoryHandler) AppMetrics(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	period := r.URL.Query().Get("period") // 1h, 24h, 7d, 30d
	if period == "" {
		period = "24h"
	}

	// Try to read stored metrics from BBolt
	bucketKey := appID + ":" + period
	var ring metricsRing
	if err := h.bolt.Get("metrics_ring", bucketKey, &ring); err == nil && len(ring.Points) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"app_id": appID,
			"period": period,
			"points": ring.Points,
			"count":  len(ring.Points),
		})
		return
	}

	// If no stored metrics, try to get current stats from runtime
	var points []MetricsPoint
	if h.runtime != nil {
		containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{"app_id": appID})
		if err == nil && len(containers) > 0 {
			stats, err := h.runtime.Stats(r.Context(), containers[0].ID)
			if err == nil {
				points = append(points, MetricsPoint{
					Timestamp:  time.Now(),
					CPUPercent: stats.CPUPercent,
					MemoryMB:   stats.MemoryUsage / (1024 * 1024),
					NetworkRx:  stats.NetworkRx / (1024 * 1024),
					NetworkTx:  stats.NetworkTx / (1024 * 1024),
				})
			}
		}
	}

	if points == nil {
		points = []MetricsPoint{}
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

	// Read stored server metrics from BBolt
	bucketKey := "server:" + serverID + ":" + period
	var ring metricsRing
	if err := h.bolt.Get("metrics_ring", bucketKey, &ring); err == nil && len(ring.Points) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"server_id": serverID,
			"period":    period,
			"points":    ring.Points,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"server_id": serverID,
		"period":    period,
		"points":    []MetricsPoint{},
	})
}
