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
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var req updateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate field lengths
	var fieldErrs []FieldError
	if req.Name != "" {
		if err := validateAppName(req.Name); err != nil {
			fieldErrs = append(fieldErrs, FieldError{Field: "name", Message: err.Error()})
		}
	}
	if len(req.SourceURL) > 2048 {
		fieldErrs = append(fieldErrs, FieldError{Field: "source_url", Message: "must be 2048 characters or fewer"})
	}
	if len(req.Branch) > 100 {
		fieldErrs = append(fieldErrs, FieldError{Field: "branch", Message: "must be 100 characters or fewer"})
	}
	if len(req.Dockerfile) > 500 {
		fieldErrs = append(fieldErrs, FieldError{Field: "dockerfile", Message: "must be 500 characters or fewer"})
	}
	if req.Replicas != nil && (*req.Replicas < 0 || *req.Replicas > 100) {
		fieldErrs = append(fieldErrs, FieldError{Field: "replicas", Message: "must be between 0 and 100"})
	}
	if len(fieldErrs) > 0 {
		writeValidationErrors(w, "field validation failed", fieldErrs)
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
