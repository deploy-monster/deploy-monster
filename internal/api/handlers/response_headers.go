package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ResponseHeadersHandler manages per-app security and custom response headers.
type ResponseHeadersHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewResponseHeadersHandler(store core.Store, bolt core.BoltStorer) *ResponseHeadersHandler {
	return &ResponseHeadersHandler{store: store, bolt: bolt}
}

// ResponseHeadersConfig defines custom response headers for the ingress.
type ResponseHeadersConfig struct {
	HSTS              string            `json:"hsts,omitempty"`               // Strict-Transport-Security
	CSP               string            `json:"csp,omitempty"`                // Content-Security-Policy
	XFrameOptions     string            `json:"x_frame_options,omitempty"`    // DENY, SAMEORIGIN
	XContentType      string            `json:"x_content_type,omitempty"`     // nosniff
	ReferrerPolicy    string            `json:"referrer_policy,omitempty"`    // strict-origin-when-cross-origin
	PermissionsPolicy string            `json:"permissions_policy,omitempty"` // camera=(), microphone=()
	Custom            map[string]string `json:"custom,omitempty"`
}

// defaultResponseHeaders returns secure defaults.
func defaultResponseHeaders() ResponseHeadersConfig {
	return ResponseHeadersConfig{
		XFrameOptions:  "DENY",
		XContentType:   "nosniff",
		ReferrerPolicy: "strict-origin-when-cross-origin",
	}
}

// Get handles GET /api/v1/apps/{id}/response-headers
func (h *ResponseHeadersHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var cfg ResponseHeadersConfig
	if err := h.bolt.Get("response_headers", app.ID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, defaultResponseHeaders())
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/response-headers
func (h *ResponseHeadersHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var cfg ResponseHeadersConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.bolt.Set("response_headers", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save response headers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}
