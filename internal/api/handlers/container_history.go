package handlers

import (
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ContainerHistoryHandler serves per-container resource usage over time.
type ContainerHistoryHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	bolt    core.BoltStorer
}

func NewContainerHistoryHandler(store core.Store, runtime core.ContainerRuntime, bolt core.BoltStorer) *ContainerHistoryHandler {
	return &ContainerHistoryHandler{store: store, runtime: runtime, bolt: bolt}
}

// ContainerResourcePoint represents a data point in container history.
type ContainerResourcePoint struct {
	Timestamp  time.Time `json:"timestamp"`
	CPUPercent float64   `json:"cpu_percent"`
	MemoryMB   int64     `json:"memory_mb"`
	MemoryMax  int64     `json:"memory_max_mb"`
	NetRxKB    int64     `json:"net_rx_kb"`
	NetTxKB    int64     `json:"net_tx_kb"`
	PIDs       int       `json:"pids"`
}

// metricsRingData is what we store in the metrics_ring bucket per app.
type metricsRingData struct {
	Points []ContainerResourcePoint `json:"points"`
}

// History handles GET /api/v1/apps/{id}/containers/history
func (h *ContainerHistoryHandler) History(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "1h"
	}

	// Try to load real metrics from BBolt
	var ring metricsRingData
	if h.bolt != nil {
		_ = h.bolt.Get("metrics_ring", appID, &ring)
	}

	if len(ring.Points) > 0 {
		// Filter points by requested period
		var cutoff time.Time
		now := time.Now()
		switch period {
		case "24h":
			cutoff = now.Add(-24 * time.Hour)
		case "7d":
			cutoff = now.Add(-7 * 24 * time.Hour)
		default:
			cutoff = now.Add(-1 * time.Hour)
		}

		filtered := make([]ContainerResourcePoint, 0, len(ring.Points))
		for _, p := range ring.Points {
			if p.Timestamp.After(cutoff) {
				filtered = append(filtered, p)
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"app_id": appID,
			"period": period,
			"points": filtered,
			"count":  len(filtered),
		})
		return
	}

	// No stored metrics — return empty timeline
	var count int
	switch period {
	case "24h":
		count = 96
	case "7d":
		count = 168
	default:
		count = 60
	}

	now := time.Now()
	points := make([]ContainerResourcePoint, count)
	for i := range points {
		points[i] = ContainerResourcePoint{
			Timestamp: now.Add(-time.Duration(count-1-i) * time.Minute),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"period": period,
		"points": points,
		"count":  count,
	})
}
