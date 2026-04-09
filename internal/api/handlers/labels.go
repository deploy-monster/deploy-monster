package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// LabelsHandler manages app labels/tags for organization.
type LabelsHandler struct {
	store core.Store
}

func NewLabelsHandler(store core.Store) *LabelsHandler {
	return &LabelsHandler{store: store}
}

// Get handles GET /api/v1/apps/{id}/labels
func (h *LabelsHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var labels map[string]string
	if app.LabelsJSON != "" && app.LabelsJSON != "{}" {
		json.Unmarshal([]byte(app.LabelsJSON), &labels)
	}
	if labels == nil {
		labels = map[string]string{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": labels})
}

// Update handles PUT /api/v1/apps/{id}/labels
func (h *LabelsHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var labels map[string]string
	if err := json.NewDecoder(r.Body).Decode(&labels); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body — expected JSON object")
		return
	}

	data, _ := json.Marshal(labels)
	app.LabelsJSON = string(data)

	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": labels})
}
