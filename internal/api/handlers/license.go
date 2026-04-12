package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// LicenseHandler manages platform license validation.
type LicenseHandler struct {
	bolt core.BoltStorer
}

func NewLicenseHandler(bolt core.BoltStorer) *LicenseHandler {
	return &LicenseHandler{bolt: bolt}
}

// LicenseInfo represents the current license state.
type LicenseInfo struct {
	Type       string    `json:"type"` // community, pro, enterprise
	Key        string    `json:"key"`  // masked
	ValidUntil time.Time `json:"valid_until"`
	MaxNodes   int       `json:"max_nodes"`
	Features   []string  `json:"features"`
	Status     string    `json:"status"` // active, expired, invalid
}

// Get handles GET /api/v1/admin/license
func (h *LicenseHandler) Get(w http.ResponseWriter, _ *http.Request) {
	var info LicenseInfo
	if err := h.bolt.Get("license", "current", &info); err != nil {
		// No license stored — return community defaults
		writeJSON(w, http.StatusOK, LicenseInfo{
			Type:     "community",
			Key:      "",
			MaxNodes: 1,
			Features: []string{"core", "marketplace", "monitoring"},
			Status:   "active",
		})
		return
	}

	// Check expiration
	if !info.ValidUntil.IsZero() && time.Now().After(info.ValidUntil) {
		info.Status = "expired"
	}

	writeJSON(w, http.StatusOK, info)
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

	if len(req.Key) < 8 {
		writeError(w, http.StatusBadRequest, "license key too short")
		return
	}

	// Validate key format (simplified — production would verify signature)
	hash := sha256.Sum256([]byte(req.Key))
	fingerprint := hex.EncodeToString(hash[:8])

	masked := req.Key[:4] + "****" + req.Key[len(req.Key)-4:]

	info := LicenseInfo{
		Type:       "enterprise",
		Key:        masked,
		ValidUntil: time.Now().Add(365 * 24 * time.Hour),
		MaxNodes:   100,
		Features:   []string{"core", "marketplace", "monitoring", "whitelabel", "reseller", "whmcs", "ha", "priority_support"},
		Status:     "active",
	}

	if err := h.bolt.Set("license", "current", info, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save license")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"key":         masked,
		"fingerprint": fingerprint,
		"status":      "activated",
		"type":        info.Type,
		"features":    info.Features,
		"valid_until": info.ValidUntil,
	})
}
