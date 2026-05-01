package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployApprovalHandler manages deployment approval workflows.
// When enabled, deploys require admin approval before executing.
type DeployApprovalHandler struct {
	store   core.Store
	events  *core.EventBus
	mu      sync.RWMutex
	pending map[string]*ApprovalRequest
}

func NewDeployApprovalHandler(store core.Store, events *core.EventBus) *DeployApprovalHandler {
	return &DeployApprovalHandler{
		store:   store,
		events:  events,
		pending: make(map[string]*ApprovalRequest),
	}
}

// ApprovalRequest represents a pending deployment approval.
type ApprovalRequest struct {
	ID          string     `json:"id"`
	AppID       string     `json:"app_id"`
	TenantID    string     `json:"tenant_id"` // required for tenant isolation
	RequestedBy string     `json:"requested_by"`
	Image       string     `json:"image"`
	Branch      string     `json:"branch"`
	Status      string     `json:"status"` // pending, approved, rejected
	Reason      string     `json:"reason,omitempty"`
	ReviewedBy  string     `json:"reviewed_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
}

// CreatePending adds a new pending approval request. Call this when a deploy needs approval.
func (h *DeployApprovalHandler) CreatePending(req *ApprovalRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending[req.ID] = req
}

// ListPending handles GET /api/v1/deploy/approvals
func (h *DeployApprovalHandler) ListPending(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	items := make([]*ApprovalRequest, 0)
	for _, req := range h.pending {
		if req.Status == "pending" {
			items = append(items, req)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

// Approve handles POST /api/v1/deploy/approvals/{id}/approve
func (h *DeployApprovalHandler) Approve(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	approvalID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	h.mu.Lock()
	req, exists := h.pending[approvalID]
	h.mu.Unlock()

	if !exists {
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}
	if req.Status != "pending" {
		writeError(w, http.StatusConflict, "approval already processed")
		return
	}

	// Tenant isolation: only the owning tenant can approve its own deploys
	if req.TenantID != claims.TenantID {
		writeError(w, http.StatusForbidden, "not authorized to approve this request")
		return
	}

	h.mu.Lock()
	now := time.Now()
	req.Status = "approved"
	req.ReviewedBy = claims.UserID
	req.ReviewedAt = &now
	h.mu.Unlock()

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
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	approvalID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // Reason is optional

	h.mu.Lock()
	req, exists := h.pending[approvalID]
	h.mu.Unlock()

	if !exists {
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}
	if req.Status != "pending" {
		writeError(w, http.StatusConflict, "approval already processed")
		return
	}

	// Tenant isolation: only the owning tenant can reject its own deploys
	if req.TenantID != claims.TenantID {
		writeError(w, http.StatusForbidden, "not authorized to reject this request")
		return
	}

	h.mu.Lock()
	now := time.Now()
	req.Status = "rejected"
	req.Reason = body.Reason
	req.ReviewedBy = claims.UserID
	req.ReviewedAt = &now
	h.mu.Unlock()

	h.events.PublishAsync(r.Context(), core.NewEvent("deploy.rejected", "api",
		map[string]string{"approval_id": approvalID, "reason": body.Reason}))

	writeJSON(w, http.StatusOK, map[string]string{
		"approval_id": approvalID,
		"status":      "rejected",
		"reason":      body.Reason,
	})
}
