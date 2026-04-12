package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ScaleHandler manages application replica scaling.
type ScaleHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewScaleHandler(store core.Store, events *core.EventBus) *ScaleHandler {
	return &ScaleHandler{store: store, events: events}
}

type scaleRequest struct {
	Replicas int `json:"replicas"`
}

// Scale handles POST /api/v1/apps/{id}/scale
func (h *ScaleHandler) Scale(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var req scaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Replicas < 0 || req.Replicas > 100 {
		writeError(w, http.StatusBadRequest, "replicas must be between 0 and 100")
		return
	}

	oldReplicas := app.Replicas
	app.Replicas = req.Replicas

	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "scale failed")
		return
	}

	h.events.Publish(r.Context(), core.NewEvent(core.EventAppScaled, "api",
		core.AppEventData{
			AppID:   appID,
			AppName: app.Name,
			Status:  app.Status,
		},
	))

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":       appID,
		"old_replicas": oldReplicas,
		"new_replicas": req.Replicas,
	})
}
