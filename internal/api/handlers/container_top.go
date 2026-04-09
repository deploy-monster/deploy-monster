package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ContainerTopHandler lists running processes inside a container.
type ContainerTopHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewContainerTopHandler(store core.Store, runtime core.ContainerRuntime) *ContainerTopHandler {
	return &ContainerTopHandler{store: store, runtime: runtime}
}

// Top handles GET /api/v1/apps/{id}/processes
func (h *ContainerTopHandler) Top(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime not available")
		return
	}

	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no container found")
		return
	}

	// Docker top would list processes — structural response
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":       appID,
		"container_id": containers[0].ID[:12],
		"processes":    []any{},
		"titles":       []string{"PID", "USER", "TIME", "COMMAND"},
	})
}
