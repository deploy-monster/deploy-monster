package handlers

import (
	"encoding/json"
	"net/http"

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

// TransferApp handles POST /api/v1/apps/{id}/transfer. Moves an app to
// another tenant. Authorized by middleware.RequireSuperAdmin at the
// router — this is the one non-/admin/* route that requires super-admin.
func (h *TransferHandler) TransferApp(w http.ResponseWriter, r *http.Request) {
	appID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
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

	// Super-admin transfer: app must have valid tenant assignment
	if app.TenantID == "" {
		writeError(w, http.StatusBadRequest, "app has no tenant assigned")
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
