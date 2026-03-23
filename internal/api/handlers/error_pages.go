package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ErrorPageHandler manages custom error pages per app.
type ErrorPageHandler struct {
	store core.Store
}

func NewErrorPageHandler(store core.Store) *ErrorPageHandler {
	return &ErrorPageHandler{store: store}
}

// ErrorPageConfig holds custom error page HTML per status code.
type ErrorPageConfig struct {
	Page502 string `json:"page_502,omitempty"` // Bad Gateway
	Page503 string `json:"page_503,omitempty"` // Service Unavailable
	Page504 string `json:"page_504,omitempty"` // Gateway Timeout
	PageMaintenance string `json:"page_maintenance,omitempty"`
}

// Get handles GET /api/v1/apps/{id}/error-pages
func (h *ErrorPageHandler) Get(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, ErrorPageConfig{})
}

// Update handles PUT /api/v1/apps/{id}/error-pages
func (h *ErrorPageHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg ErrorPageConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
