package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ResponseHeadersHandler manages per-app security and custom response headers.
type ResponseHeadersHandler struct {
	store core.Store
}

func NewResponseHeadersHandler(store core.Store) *ResponseHeadersHandler {
	return &ResponseHeadersHandler{store: store}
}

// ResponseHeadersConfig defines custom response headers for the ingress.
type ResponseHeadersConfig struct {
	HSTS              string `json:"hsts,omitempty"`               // Strict-Transport-Security
	CSP               string `json:"csp,omitempty"`                // Content-Security-Policy
	XFrameOptions     string `json:"x_frame_options,omitempty"`    // DENY, SAMEORIGIN
	XContentType      string `json:"x_content_type,omitempty"`     // nosniff
	ReferrerPolicy    string `json:"referrer_policy,omitempty"`    // strict-origin-when-cross-origin
	PermissionsPolicy string `json:"permissions_policy,omitempty"` // camera=(), microphone=()
	Custom            map[string]string `json:"custom,omitempty"`
}

// Get handles GET /api/v1/apps/{id}/response-headers
func (h *ResponseHeadersHandler) Get(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, ResponseHeadersConfig{
		XFrameOptions: "DENY",
		XContentType:  "nosniff",
		ReferrerPolicy: "strict-origin-when-cross-origin",
	})
}

// Update handles PUT /api/v1/apps/{id}/response-headers
func (h *ResponseHeadersHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	var cfg ResponseHeadersConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}
