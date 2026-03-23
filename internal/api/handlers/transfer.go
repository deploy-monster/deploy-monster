package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TransferHandler moves resources between tenants.
type TransferHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewTransferHandler(store core.Store, events *core.EventBus) *TransferHandler {
	return &TransferHandler{store: store, events: events}
}

type transferRequest struct {
	TargetTenantID string `json:"target_tenant_id"`
}

// TransferApp handles POST /api/v1/apps/{id}/transfer
// Moves an app to another tenant (super admin only).
func (h *TransferHandler) TransferApp(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.RoleID != "role_super_admin" {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	appID := r.PathValue("id")
	var req transferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TargetTenantID == "" {
		writeError(w, http.StatusBadRequest, "target_tenant_id required")
		return
	}

	app, err := h.store.GetApp(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	// Verify target tenant exists
	_, err = h.store.GetTenant(r.Context(), req.TargetTenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target tenant not found")
		return
	}

	oldTenant := app.TenantID
	app.TenantID = req.TargetTenantID
	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "transfer failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":      appID,
		"from_tenant": oldTenant,
		"to_tenant":   req.TargetTenantID,
		"status":      "transferred",
	})
}
