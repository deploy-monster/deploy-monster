package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ErrorPageHandler manages custom error pages per app.
type ErrorPageHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewErrorPageHandler(store core.Store, bolt core.BoltStorer) *ErrorPageHandler {
	return &ErrorPageHandler{store: store, bolt: bolt}
}

// ErrorPageConfig holds custom error page HTML per status code.
type ErrorPageConfig struct {
	Page502         string `json:"page_502,omitempty"`         // Bad Gateway
	Page503         string `json:"page_503,omitempty"`         // Service Unavailable
	Page504         string `json:"page_504,omitempty"`         // Gateway Timeout
	PageMaintenance string `json:"page_maintenance,omitempty"` // Maintenance
}

// Get handles GET /api/v1/apps/{id}/error-pages
func (h *ErrorPageHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var cfg ErrorPageConfig
	if err := h.bolt.Get("error_pages", app.ID, &cfg); err != nil {
		// No custom pages — return empty config
		writeJSON(w, http.StatusOK, ErrorPageConfig{})
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/error-pages
func (h *ErrorPageHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var cfg ErrorPageConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	const maxPageBytes = 1 << 20 // 1 MB per page
	pages := map[string]string{
		"page_502":         cfg.Page502,
		"page_503":         cfg.Page503,
		"page_504":         cfg.Page504,
		"page_maintenance": cfg.PageMaintenance,
	}
	for name, body := range pages {
		if len(body) > maxPageBytes {
			writeError(w, http.StatusBadRequest, name+" exceeds 1 MB limit")
			return
		}
	}

	if err := h.bolt.Set("error_pages", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save error pages")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
