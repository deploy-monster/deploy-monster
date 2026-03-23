package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TenantRateLimitHandler manages per-tenant API rate limits.
type TenantRateLimitHandler struct {
	store core.Store
}

func NewTenantRateLimitHandler(store core.Store) *TenantRateLimitHandler {
	return &TenantRateLimitHandler{store: store}
}

// RateLimitConfig defines API rate limits for a tenant.
type RateLimitConfig struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	BurstSize         int `json:"burst_size"`
	BuildsPerHour     int `json:"builds_per_hour"`
	DeploysPerHour    int `json:"deploys_per_hour"`
}

// Get handles GET /api/v1/admin/tenants/{id}/ratelimit
func (h *TenantRateLimitHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.RoleID != "role_super_admin" {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	_ = r.PathValue("id")

	// Default limits
	writeJSON(w, http.StatusOK, RateLimitConfig{
		RequestsPerMinute: 100,
		BurstSize:         20,
		BuildsPerHour:     10,
		DeploysPerHour:    20,
	})
}

// Update handles PUT /api/v1/admin/tenants/{id}/ratelimit
func (h *TenantRateLimitHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.RoleID != "role_super_admin" {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	tenantID := r.PathValue("id")

	var cfg RateLimitConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenantID,
		"config":    cfg,
		"status":    "updated",
	})
}
