package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeploymentHandler handles deployment endpoints.
type DeploymentHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewDeploymentHandler(store core.Store, events *core.EventBus) *DeploymentHandler {
	return &DeploymentHandler{store: store, events: events}
}

// ListByApp handles GET /api/v1/apps/{id}/deployments
func (h *DeploymentHandler) ListByApp(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	deployments, err := h.store.ListDeploymentsByApp(r.Context(), app.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": deployments, "total": len(deployments)})
}

// GetLatest handles GET /api/v1/apps/{id}/deployments/latest
func (h *DeploymentHandler) GetLatest(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	dep, err := h.store.GetLatestDeployment(r.Context(), app.ID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "no deployments found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, dep)
}
