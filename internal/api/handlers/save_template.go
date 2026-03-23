package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SaveTemplateHandler saves a running app as a reusable marketplace template.
type SaveTemplateHandler struct {
	store core.Store
}

func NewSaveTemplateHandler(store core.Store) *SaveTemplateHandler {
	return &SaveTemplateHandler{store: store}
}

type saveTemplateRequest struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
}

// Save handles POST /api/v1/apps/{id}/save-template
func (h *SaveTemplateHandler) Save(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	appID := r.PathValue("id")
	app, err := h.store.GetApp(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	var req saveTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		req.Name = app.Name
	}
	if req.Slug == "" {
		req.Slug = app.Name
	}

	// Build template from app config
	template := map[string]any{
		"slug":        req.Slug,
		"name":        req.Name,
		"description": req.Description,
		"category":    req.Category,
		"tags":        req.Tags,
		"source": map[string]string{
			"type":       app.SourceType,
			"url":        app.SourceURL,
			"branch":     app.Branch,
			"dockerfile": app.Dockerfile,
		},
		"app_type": app.Type,
		"saved_from": appID,
	}

	writeJSON(w, http.StatusCreated, template)
}
