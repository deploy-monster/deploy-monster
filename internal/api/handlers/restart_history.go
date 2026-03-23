package handlers

import (
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RestartHistoryHandler tracks container restart events.
type RestartHistoryHandler struct {
	runtime core.ContainerRuntime
}

func NewRestartHistoryHandler(runtime core.ContainerRuntime) *RestartHistoryHandler {
	return &RestartHistoryHandler{runtime: runtime}
}

// RestartEvent records when and why a container restarted.
type RestartEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Reason    string    `json:"reason"` // manual, crash, oom, health_check
	ExitCode  int       `json:"exit_code"`
}

// List handles GET /api/v1/apps/{id}/restarts
func (h *RestartHistoryHandler) List(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	if h.runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "data": []any{}, "total": 0})
		return
	}

	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "data": []any{}, "total": 0})
		return
	}

	// Would parse container inspect for RestartCount and state history
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":       appID,
		"container_id": containers[0].ID[:12],
		"data":         []any{},
		"total":        0,
	})
}
