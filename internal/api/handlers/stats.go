package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// StatsHandler serves container and server resource metrics.
type StatsHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
}

func NewStatsHandler(runtime core.ContainerRuntime, store core.Store) *StatsHandler {
	return &StatsHandler{runtime: runtime, store: store}
}

// AppStats handles GET /api/v1/apps/{id}/stats.
// Returns aggregated CPU/memory/network usage from all containers attached
// to the app via the monster.app.id label. Empty list returns zeros instead
// of an error so the UI can show a calm "not running" state.
func (h *StatsHandler) AppStats(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}

	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list containers")
		return
	}

	type containerStats struct {
		ID            string  `json:"id"`
		Name          string  `json:"name"`
		State         string  `json:"state"`
		CPUPercent    float64 `json:"cpu_percent"`
		MemoryUsage   int64   `json:"memory_usage"`
		MemoryLimit   int64   `json:"memory_limit"`
		MemoryPercent float64 `json:"memory_percent"`
		NetworkRx     int64   `json:"network_rx"`
		NetworkTx     int64   `json:"network_tx"`
		Health        string  `json:"health"`
		Running       bool    `json:"running"`
	}

	perContainer := make([]containerStats, 0, len(containers))
	var aggCPU, aggMemPct float64
	var aggMemUsage, aggMemLimit, aggNetRx, aggNetTx int64
	runningCount := 0

	for _, c := range containers {
		entry := containerStats{
			ID:    c.ID,
			Name:  c.Name,
			State: c.State,
		}
		s, statsErr := h.runtime.Stats(r.Context(), c.ID)
		if statsErr == nil && s != nil {
			entry.CPUPercent = s.CPUPercent
			entry.MemoryUsage = s.MemoryUsage
			entry.MemoryLimit = s.MemoryLimit
			entry.MemoryPercent = s.MemoryPercent
			entry.NetworkRx = s.NetworkRx
			entry.NetworkTx = s.NetworkTx
			entry.Health = s.Health
			entry.Running = s.Running

			aggCPU += s.CPUPercent
			aggMemUsage += s.MemoryUsage
			aggMemLimit += s.MemoryLimit
			aggMemPct += s.MemoryPercent
			aggNetRx += s.NetworkRx
			aggNetTx += s.NetworkTx
			if s.Running {
				runningCount++
			}
		}
		perContainer = append(perContainer, entry)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":         appID,
		"containers":     perContainer,
		"count":          len(perContainer),
		"running":        runningCount,
		"cpu_percent":    aggCPU,
		"memory_usage":   aggMemUsage,
		"memory_limit":   aggMemLimit,
		"memory_percent": aggMemPct,
		"network_rx":     aggNetRx,
		"network_tx":     aggNetTx,
	})
}

// ServerStats handles GET /api/v1/servers/stats
func (h *StatsHandler) ServerStats(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}

	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	running := 0
	stopped := 0
	for _, c := range containers {
		if c.State == "running" {
			running++
		} else {
			stopped++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(containers),
		"running": running,
		"stopped": stopped,
	})
}
