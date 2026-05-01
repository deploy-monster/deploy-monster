package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"golang.org/x/crypto/bcrypt"
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.RoleID == "" {
		writeError(w, http.StatusBadRequest, "email and role_id are required")
		return
	}

	// Role hierarchy check: inviter cannot assign a role higher than their own.
	if !auth.CanAssignRole(member.RoleID, req.RoleID) {
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

const inviteTokenBcryptCost = 10 // Lower than password hashing since tokens are high-entropy

func hashToken(token string) string {
	// Use bcrypt for adaptive hashing (not plain SHA-256).
	// Tokens are 32 bytes of crypto/rand entropy, so rainbow tables are impractical
	// but bcrypt adds defense-in-depth if the DB is compromised.
	hash, err := bcrypt.GenerateFromPassword([]byte(token), inviteTokenBcryptCost)
	if err != nil {
		// Fallback to SHA-256 if bcrypt fails (should never happen)
		h := sha256.Sum256([]byte(token))
		return hex.EncodeToString(h[:])
	}
	return string(hash)
}
