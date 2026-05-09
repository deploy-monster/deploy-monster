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
		"network_rx":         metrics.NetRxBytes,
		"network_tx":         metrics.NetTxBytes,
		"uptime":             int64(time.Since(h.startedAt).Seconds()),
		"containers_running": runningContainers,
		"containers_total":   totalContainers,
		"load_avg":           []float64{metrics.LoadAvg[0], metrics.LoadAvg[1], metrics.LoadAvg[2]},
	})
}

// Alerts handles GET /api/v1/alerts.
// Returns the current evaluation of each built-in alert rule against live
// host metrics. status="alert" means the threshold is breached now; "ok"
// means current value is below threshold; "unknown" means we can't read
// the metric on this host.
func (h *MonitoringHandler) Alerts(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	metrics, err := h.reader.Read()
	cpuPct := metrics.CPUPercent
	var memPct, diskPct float64
	if metrics.RAMTotalMB > 0 {
		memPct = float64(metrics.RAMUsedMB) / float64(metrics.RAMTotalMB) * 100
	}
	if metrics.DiskTotalMB > 0 {
		diskPct = float64(metrics.DiskUsedMB) / float64(metrics.DiskTotalMB) * 100
	}

	rule := func(id, name, metric string, threshold float64, value float64, available bool) map[string]any {
		status := "ok"
		switch {
		case !available:
			status = "unknown"
		case value >= threshold:
			status = "alert"
		}
		return map[string]any{
			"id":           id,
			"name":         name,
			"metric":       metric,
			"threshold":    threshold,
			"value":        value,
			"status":       status,
			"last_checked": now,
		}
	}

	rules := []map[string]any{
		rule("cpu-high", "High CPU usage", "cpu", 90, cpuPct, err == nil),
		rule("memory-high", "High memory usage", "memory", 90, memPct, err == nil && metrics.RAMTotalMB > 0),
		rule("disk-high", "High disk usage", "disk", 90, diskPct, err == nil && metrics.DiskTotalMB > 0),
	}

	// Container-down alert: any monster-managed container in a non-running state.
	if h.core.Services.Container != nil {
		containers, lerr := h.core.Services.Container.ListByLabels(r.Context(), map[string]string{"monster.enable": "true"})
		stopped := 0
		if lerr == nil {
			for _, c := range containers {
				if c.State != "running" {
					stopped++
				}
			}
			rules = append(rules, rule("containers-down", "Containers not running", "containers_stopped", 1, float64(stopped), true))
		} else {
			rules = append(rules, rule("containers-down", "Containers not running", "containers_stopped", 1, 0, false))
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  rules,
		"total": len(rules),
	})
}
