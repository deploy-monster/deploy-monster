package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/billing"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BillingHandler manages billing and subscription endpoints.
type BillingHandler struct {
	store core.Store
}

func NewBillingHandler(store core.Store) *BillingHandler {
	return &BillingHandler{store: store}
}

// ListPlans handles GET /api/v1/billing/plans
func (h *BillingHandler) ListPlans(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": billing.BuiltinPlans})
}

// GetUsage handles GET /api/v1/billing/usage
func (h *BillingHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Get current plan
	tenant, err := h.store.GetTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Find matching plan
	var currentPlan *billing.Plan
	for i := range billing.BuiltinPlans {
		if billing.BuiltinPlans[i].ID == tenant.PlanID {
			currentPlan = &billing.BuiltinPlans[i]
			break
		}
	}

	if currentPlan == nil {
		currentPlan = &billing.BuiltinPlans[0] // Default to free
	}

	// Get app count
	_, appCount, _ := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 1, 0)

	// Quota check
	status, _ := billing.QuotaCheck(h.store, claims.TenantID, *currentPlan)

	writeJSON(w, http.StatusOK, map[string]any{
		"plan":       currentPlan,
		"apps_used":  appCount,
		"apps_limit": currentPlan.MaxApps,
		"quota":      status,
	})
}
