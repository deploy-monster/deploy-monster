package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ExecHandler handles container exec endpoints.
type ExecHandler struct {
	runtime core.ContainerRuntime
}

func NewExecHandler(runtime core.ContainerRuntime) *ExecHandler {
	return &ExecHandler{runtime: runtime}
}

type execRequest struct {
	Command string `json:"command"`
}

// Exec handles POST /api/v1/apps/{id}/exec
// Runs a command inside the application's container and returns output.
func (h *ExecHandler) Exec(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	// Find running container for this app
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no running container for this app")
		return
	}

	// For full exec support, we'd use Docker's exec API via ContainerRuntime.
	// This is a structural placeholder — production would create an exec instance,
	// attach stdin/stdout/stderr, and stream via WebSocket/SSE.
	writeJSON(w, http.StatusOK, map[string]any{
		"container_id": containers[0].ID,
		"command":      req.Command,
		"status":       "exec support requires WebSocket — use /logs/stream for now",
	})
}
