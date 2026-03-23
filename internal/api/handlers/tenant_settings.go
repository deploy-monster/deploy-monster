package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TenantSettingsHandler manages per-tenant configuration.
type TenantSettingsHandler struct {
	store core.Store
}

func NewTenantSettingsHandler(store core.Store) *TenantSettingsHandler {
	return &TenantSettingsHandler{store: store}
}

// Get handles GET /api/v1/tenant/settings
func (h *TenantSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenant, err := h.store.GetTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenant.ID,
		"name":      tenant.Name,
		"slug":      tenant.Slug,
		"plan_id":   tenant.PlanID,
		"status":    tenant.Status,
		"limits":    tenant.LimitsJSON,
		"metadata":  tenant.MetadataJSON,
	})
}

// Update handles PATCH /api/v1/tenant/settings
func (h *TenantSettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name     string `json:"name,omitempty"`
		Metadata string `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tenant, err := h.store.GetTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if req.Name != "" {
		tenant.Name = req.Name
	}
	if req.Metadata != "" {
		tenant.MetadataJSON = req.Metadata
	}

	if err := h.store.UpdateTenant(r.Context(), tenant); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
