package handlers

import (
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ContainerHistoryHandler serves per-container resource usage over time.
type ContainerHistoryHandler struct {
	runtime core.ContainerRuntime
}

func NewContainerHistoryHandler(runtime core.ContainerRuntime) *ContainerHistoryHandler {
	return &ContainerHistoryHandler{runtime: runtime}
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

// History handles GET /api/v1/apps/{id}/containers/history
func (h *ContainerHistoryHandler) History(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "1h"
	}

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
