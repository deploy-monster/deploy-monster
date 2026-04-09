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

// AppStats handles GET /api/v1/apps/{id}/stats
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
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no container found for this app")
		return
	}

	// Return container info — full Docker stats would require
	// the Docker stats API streaming which is better served via SSE
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":     appID,
		"containers": containers,
		"count":      len(containers),
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
