package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

// LicenseHandler manages platform license validation.
type LicenseHandler struct{}

func NewLicenseHandler() *LicenseHandler {
	return &LicenseHandler{}
}

// LicenseInfo represents the current license state.
type LicenseInfo struct {
	Type       string    `json:"type"`       // community, pro, enterprise
	Key        string    `json:"key"`        // masked
	ValidUntil time.Time `json:"valid_until"`
	MaxNodes   int       `json:"max_nodes"`
	Features   []string  `json:"features"`
	Status     string    `json:"status"`     // active, expired, invalid
}

// Get handles GET /api/v1/admin/license
func (h *LicenseHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, LicenseInfo{
		Type:       "community",
		Key:        "",
		MaxNodes:   1,
		Features:   []string{"core", "marketplace", "monitoring"},
		Status:     "active",
	})
}

// Activate handles POST /api/v1/admin/license
func (h *LicenseHandler) Activate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeError(w, http.StatusBadRequest, "license key required")
		return
	}

	// Validate key format (simplified — production would verify signature)
	hash := sha256.Sum256([]byte(req.Key))
	fingerprint := hex.EncodeToString(hash[:8])

	masked := req.Key[:4] + "****" + req.Key[len(req.Key)-4:]

	writeJSON(w, http.StatusOK, map[string]any{
		"key":         masked,
		"fingerprint": fingerprint,
		"status":      "activated",
		"type":        "enterprise",
		"features":    []string{"core", "marketplace", "monitoring", "whitelabel", "reseller", "whmcs", "ha", "priority_support"},
	})
}
