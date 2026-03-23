package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// MetricsExportHandler exports metrics data as CSV or JSON.
type MetricsExportHandler struct{}

func NewMetricsExportHandler() *MetricsExportHandler {
	return &MetricsExportHandler{}
}

// ExportCSV handles GET /api/v1/apps/{id}/metrics/export?format=csv
func (h *MetricsExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	// Sample metrics data
	now := time.Now()
	points := make([]map[string]any, 24)
	for i := range points {
		points[i] = map[string]any{
			"timestamp":   now.Add(-time.Duration(23-i) * time.Hour).Format(time.RFC3339),
			"cpu_percent": 0.0,
			"memory_mb":   0,
			"requests":    0,
		}
	}

	switch format {
	case "csv":
		filename := fmt.Sprintf("%s-metrics-%s.csv", appID[:8], now.Format("20060102"))
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)

		writer := csv.NewWriter(w)
		writer.Write([]string{"timestamp", "cpu_percent", "memory_mb", "requests"})
		for _, p := range points {
			writer.Write([]string{
				fmt.Sprint(p["timestamp"]),
				fmt.Sprint(p["cpu_percent"]),
				fmt.Sprint(p["memory_mb"]),
				fmt.Sprint(p["requests"]),
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
