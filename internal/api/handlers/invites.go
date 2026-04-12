package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// InviteHandler handles team invitation endpoints.
type InviteHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewInviteHandler(store core.Store, events *core.EventBus) *InviteHandler {
	return &InviteHandler{store: store, events: events}
}

type inviteRequest struct {
	Email  string `json:"email"`
	RoleID string `json:"role_id"`
}

// Create handles POST /api/v1/team/invites
func (h *InviteHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req inviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.RoleID == "" {
		writeError(w, http.StatusBadRequest, "email and role_id are required")
		return
	}

	// Generate invite token
	token := core.GenerateSecret(32)
	tokenHash := hashToken(token)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// Store invite in database
	invite := &core.Invitation{
		TenantID:  claims.TenantID,
		Email:     req.Email,
		RoleID:    req.RoleID,
		InvitedBy: claims.UserID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		Status:    "pending",
	}
	if err := h.store.CreateInvite(r.Context(), invite); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create invitation")
		return
	}

	h.events.PublishAsync(r.Context(), core.NewTenantEvent(
		core.EventUserInvited, "api", claims.TenantID, claims.UserID,
		map[string]string{
			"email":   req.Email,
			"role_id": req.RoleID,
		},
	))

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         invite.ID,
		"email":      req.Email,
		"role_id":    req.RoleID,
		"token":      token, // Only returned once
		"token_hash": tokenHash,
		"expires_at": expiresAt,
	})
}

// List handles GET /api/v1/team/invites
func (h *InviteHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	invites, err := h.store.ListInvitesByTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list invitations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  invites,
		"total": len(invites),
	})
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
