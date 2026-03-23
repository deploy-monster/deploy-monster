package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployApprovalHandler manages deployment approval workflows.
// When enabled, deploys require admin approval before executing.
type DeployApprovalHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewDeployApprovalHandler(store core.Store, events *core.EventBus) *DeployApprovalHandler {
	return &DeployApprovalHandler{store: store, events: events}
}

// ApprovalRequest represents a pending deployment approval.
type ApprovalRequest struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	RequestedBy string    `json:"requested_by"`
	Image       string    `json:"image"`
	Branch      string    `json:"branch"`
	Status      string    `json:"status"` // pending, approved, rejected
	CreatedAt   time.Time `json:"created_at"`
}

// ListPending handles GET /api/v1/deploy/approvals
func (h *DeployApprovalHandler) ListPending(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}

// Approve handles POST /api/v1/deploy/approvals/{id}/approve
func (h *DeployApprovalHandler) Approve(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	approvalID := r.PathValue("id")

	h.events.PublishAsync(r.Context(), core.NewEvent("deploy.approved", "api",
		map[string]string{"approval_id": approvalID, "approved_by": claims.UserID}))

	writeJSON(w, http.StatusOK, map[string]string{
		"approval_id": approvalID,
		"status":      "approved",
		"approved_by": claims.UserID,
	})
}

// Reject handles POST /api/v1/deploy/approvals/{id}/reject
func (h *DeployApprovalHandler) Reject(w http.ResponseWriter, r *http.Request) {
	approvalID := r.PathValue("id")

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	writeJSON(w, http.StatusOK, map[string]string{
		"approval_id": approvalID,
		"status":      "rejected",
		"reason":      req.Reason,
	})
}
