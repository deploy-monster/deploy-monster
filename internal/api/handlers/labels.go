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
		if err := json.Unmarshal([]byte(app.LabelsJSON), &labels); err != nil {
			writeError(w, http.StatusInternalServerError, "stored labels are invalid")
			return
		}
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

	const maxLabels = 64
	const maxKeyLen = 63
	const maxValueLen = 253
	if len(labels) > maxLabels {
		writeError(w, http.StatusBadRequest, "too many labels (max 64)")
		return
	}
	for k, v := range labels {
		if k == "" {
			writeError(w, http.StatusBadRequest, "label key must not be empty")
			return
		}
		if len(k) > maxKeyLen {
			writeError(w, http.StatusBadRequest, "label key exceeds 63 characters")
			return
		}
		if len(v) > maxValueLen {
			writeError(w, http.StatusBadRequest, "label value exceeds 253 characters")
			return
		}
	}

	data, _ := json.Marshal(labels)
	app.LabelsJSON = string(data)

	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": labels})
}
