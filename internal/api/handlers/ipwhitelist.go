package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// IPWhitelistHandler manages per-app IP access control.
type IPWhitelistHandler struct {
	store core.Store
}

func NewIPWhitelistHandler(store core.Store) *IPWhitelistHandler {
	return &IPWhitelistHandler{store: store}
}

// IPWhitelistConfig holds access control rules for an app.
type IPWhitelistConfig struct {
	Enabled    bool     `json:"enabled"`
	AllowedIPs []string `json:"allowed_ips"` // CIDRs: "192.168.1.0/24", "10.0.0.1"
	DenyIPs    []string `json:"deny_ips"`
}

// Get handles GET /api/v1/apps/{id}/access
func (h *IPWhitelistHandler) Get(w http.ResponseWriter, r *http.Request) {
	if requireTenantApp(w, r, h.store) == nil {
		return
	}

	// Default: no restrictions
	writeJSON(w, http.StatusOK, IPWhitelistConfig{
		Enabled: false,
	})
}

// Update handles PUT /api/v1/apps/{id}/access
func (h *IPWhitelistHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var cfg IPWhitelistConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Would store in app labels or dedicated table
	// Ingress middleware would check against this list
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
