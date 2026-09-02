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

	// RBAC: check member.invite permission
	member, err := h.store.GetUserMembership(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusForbidden, "not authorized to invite members")
		return
	}
	if member.TenantID != claims.TenantID {
		writeError(w, http.StatusForbidden, "not authorized to invite members")
		return
	}
	role, err := h.store.GetRole(r.Context(), member.RoleID)
	if err != nil {
		writeError(w, http.StatusForbidden, "not authorized to invite members")
		return
	}
	if !role.HasPermission(auth.PermMemberInvite) && !role.HasPermission(auth.PermAdminAll) {
		writeError(w, http.StatusForbidden, "missing member.invite permission")
		return
	}

	var req inviteRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}

	if req.Email == "" || req.RoleID == "" {
		writeError(w, http.StatusBadRequest, "email and role_id are required")
		return
	}

	targetRole, err := h.store.GetRole(r.Context(), req.RoleID)
	if err != nil || (targetRole.TenantID != "" && targetRole.TenantID != claims.TenantID) {
		writeError(w, http.StatusBadRequest, "role_id is invalid")
		return
	}

	// Role hierarchy check: inviter cannot assign a role higher than their own.
	if !canInviteWithRole(role, targetRole) {
		writeError(w, http.StatusForbidden, "cannot invite with a role higher than your own")
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

	publishEventAsync(r.Context(), h.events, core.NewTenantEvent(
		core.EventUserInvited, "api", claims.TenantID, claims.UserID,
		map[string]string{
			"email":   req.Email,
			"role_id": req.RoleID,
		},
	))

	// The plaintext token is the one-time invite code the inviter must
	// share with the invitee — it is the only value that can be redeemed
	// at /auth/register (invite_code). token_hash is returned as well for
	// verification but cannot be used to redeem.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         invite.ID,
		"email":      req.Email,
		"role_id":    req.RoleID,
		"token":      token,
		"token_hash": tokenHash,
		"expires_at": expiresAt,
	})
}

func canInviteWithRole(inviterRole, targetRole *core.Role) bool {
	if inviterRole == nil || targetRole == nil {
		return false
	}
	if !auth.CanAssignRole(inviterRole.ID, targetRole.ID) {
		return false
	}

	var targetPerms []string
	if err := json.Unmarshal([]byte(targetRole.PermissionsJSON), &targetPerms); err != nil {
		return false
	}
	for _, permission := range targetPerms {
		if !inviterRole.HasPermission(permission) {
			return false
		}
	}
	return true
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
	// SHA-256, deliberately NOT bcrypt: the invite code is 32 bytes of
	// crypto/rand entropy (256 bits), so a dictionary attack is infeasible
	// and adaptive hashing buys nothing. A deterministic digest is required
	// because redemption looks the invitation up by hash
	// (GetInviteByTokenHash → WHERE token_hash = ?); bcrypt's random salt
	// would make that lookup impossible.
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
