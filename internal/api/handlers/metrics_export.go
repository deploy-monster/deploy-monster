package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// MetricsExportHandler exports metrics data as CSV or JSON.
type MetricsExportHandler struct {
	store   core.Store
	bolt    core.BoltStorer
	runtime core.ContainerRuntime
}

func NewMetricsExportHandler(store core.Store, bolt core.BoltStorer, runtime core.ContainerRuntime) *MetricsExportHandler {
	return &MetricsExportHandler{store: store, bolt: bolt, runtime: runtime}
}

// metricsPoint is a single metrics data point stored in BBolt.
type metricsPoint struct {
	Timestamp  string  `json:"timestamp"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   int64   `json:"memory_mb"`
	Requests   int64   `json:"requests"`
}

// Export handles GET /api/v1/apps/{id}/metrics/export?format=csv
func (h *MetricsExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	// Try to load real metrics from BBolt
	var storedPoints []metricsPoint
	_ = h.bolt.Get("metrics_export", appID, &storedPoints)

	// If no stored metrics, get current stats from runtime and generate points
	now := time.Now()
	points := storedPoints
	if len(points) == 0 {
		// Attempt to read live stats for the app
		var currentCPU float64
		var currentMemMB int64
		if h.runtime != nil {
			containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{"app": appID})
			if err == nil && len(containers) > 0 {
				stats, err := h.runtime.Stats(r.Context(), containers[0].ID)
				if err == nil {
					currentCPU = stats.CPUPercent
					currentMemMB = stats.MemoryUsage / (1024 * 1024)
				}
			}
		}

		// Generate 24 points, last one with current data
		points = make([]metricsPoint, 24)
		for i := range points {
			points[i] = metricsPoint{
				Timestamp: now.Add(-time.Duration(23-i) * time.Hour).Format(time.RFC3339),
			}
		}
		// Fill in the latest point with real data
		if currentCPU > 0 || currentMemMB > 0 {
			points[23].CPUPercent = currentCPU
			points[23].MemoryMB = currentMemMB
		}
	}

	switch format {
	case "csv":
		if len(appID) < 8 {
			appID = appID + "________"
		}
		filename := fmt.Sprintf("%s-metrics-%s.csv", appID[:8], now.Format("20060102"))
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename="+safeFilename(filename))

		writer := csv.NewWriter(w)
		writer.Write([]string{"timestamp", "cpu_percent", "memory_mb", "requests"})
		for _, p := range points {
			writer.Write([]string{
				p.Timestamp,
				fmt.Sprintf("%.2f", p.CPUPercent),
				fmt.Sprint(p.MemoryMB),
				fmt.Sprint(p.Requests),
			})
		}
		writer.Flush()

	default:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"app_id": appID,
			"period": "24h",
			"points": points,
		})
	}
}
