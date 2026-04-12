package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TenantRateLimitHandler manages per-tenant API rate limits.
type TenantRateLimitHandler struct {
	bolt core.BoltStorer
}

func NewTenantRateLimitHandler(bolt core.BoltStorer) *TenantRateLimitHandler {
	return &TenantRateLimitHandler{bolt: bolt}
}

// RateLimitConfig defines API rate limits for a tenant.
type RateLimitConfig struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	BurstSize         int `json:"burst_size"`
	BuildsPerHour     int `json:"builds_per_hour"`
	DeploysPerHour    int `json:"deploys_per_hour"`
}

// defaultRateLimits returns sensible defaults.
func defaultRateLimits() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute: 100,
		BurstSize:         20,
		BuildsPerHour:     10,
		DeploysPerHour:    20,
	}
}

// Get handles GET /api/v1/admin/tenants/{id}/ratelimit. Authorized by
// middleware.RequireSuperAdmin at the router.
func (h *TenantRateLimitHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	var cfg RateLimitConfig
	if err := h.bolt.Get("tenant_ratelimit", tenantID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, defaultRateLimits())
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/admin/tenants/{id}/ratelimit. Authorized by
// middleware.RequireSuperAdmin at the router.
func (h *TenantRateLimitHandler) Update(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	var cfg RateLimitConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.RequestsPerMinute <= 0 {
		cfg.RequestsPerMinute = 100
	}
	if cfg.BurstSize <= 0 {
		cfg.BurstSize = 20
	}

	if err := h.bolt.Set("tenant_ratelimit", tenantID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save rate limit config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenantID,
		"config":    cfg,
		"status":    "updated",
	})
}
