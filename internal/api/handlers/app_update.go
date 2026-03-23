package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

type updateAppRequest struct {
	Name       string `json:"name,omitempty"`
	SourceURL  string `json:"source_url,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Dockerfile string `json:"dockerfile,omitempty"`
	Replicas   *int   `json:"replicas,omitempty"`
}

// Update handles PATCH /api/v1/apps/{id}
func (h *AppHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	app, err := h.store.GetApp(r.Context(), id)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req updateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		app.Name = req.Name
	}
	if req.SourceURL != "" {
		app.SourceURL = req.SourceURL
	}
	if req.Branch != "" {
		app.Branch = req.Branch
	}
	if req.Dockerfile != "" {
		app.Dockerfile = req.Dockerfile
	}
	if req.Replicas != nil {
		app.Replicas = *req.Replicas
	}

	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	h.core.Events.Publish(r.Context(), core.NewEvent(core.EventAppUpdated, "api",
		core.AppEventData{AppID: app.ID, AppName: app.Name}))

	writeJSON(w, http.StatusOK, app)
}
