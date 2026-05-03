package handlers

import (
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// MonitoringHandler serves UI-level platform monitoring summaries.
type MonitoringHandler struct {
	core      *core.Core
	startedAt time.Time
	reader    core.SysMetricsReader
}

func NewMonitoringHandler(c *core.Core, startedAt time.Time) *MonitoringHandler {
	return &MonitoringHandler{
		core:      c,
		startedAt: startedAt,
		reader:    core.NewSysMetricsReader(),
	}
}

// Metrics handles GET /api/v1/metrics/server.
func (h *MonitoringHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	metrics, _ := h.reader.Read()

	totalContainers := 0
	runningContainers := 0
	if h.core.Services.Container != nil {
		containers, err := h.core.Services.Container.ListByLabels(r.Context(), map[string]string{"monster.enable": "true"})
		if err == nil {
			totalContainers = len(containers)
			for _, c := range containers {
				if c.State == "running" {
					runningContainers++
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cpu_percent":        metrics.CPUPercent,
		"memory_used":        metrics.RAMUsedMB * 1024 * 1024,
		"memory_total":       metrics.RAMTotalMB * 1024 * 1024,
		"disk_used":          metrics.DiskUsedMB * 1024 * 1024,
		"disk_total":         metrics.DiskTotalMB * 1024 * 1024,
		"network_rx":         0,
		"network_tx":         0,
		"uptime":             int64(time.Since(h.startedAt).Seconds()),
		"containers_running": runningContainers,
		"containers_total":   totalContainers,
		"load_avg":           []float64{metrics.LoadAvg[0], metrics.LoadAvg[1], metrics.LoadAvg[2]},
	})
}

// Alerts handles GET /api/v1/alerts.
func (h *MonitoringHandler) Alerts(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	writeJSON(w, http.StatusOK, map[string]any{
		"data": []map[string]any{
			{
				"id":           "cpu-high",
				"name":         "High CPU usage",
				"metric":       "cpu",
				"threshold":    90,
				"status":       "ok",
				"last_checked": now,
			},
			{
				"id":           "memory-high",
				"name":         "High memory usage",
				"metric":       "memory",
				"threshold":    90,
				"status":       "ok",
				"last_checked": now,
			},
			{
				"id":           "disk-high",
				"name":         "High disk usage",
				"metric":       "disk",
				"threshold":    90,
				"status":       "ok",
				"last_checked": now,
			},
		},
		"total": 3,
	})
}
