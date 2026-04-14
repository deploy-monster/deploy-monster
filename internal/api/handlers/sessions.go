package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SessionHandler handles session management endpoints.
type SessionHandler struct {
	store   core.Store
	bolt    core.BoltStorer
	authMod *auth.Module
}

func NewSessionHandler(store core.Store, bolt core.BoltStorer, authMod *auth.Module) *SessionHandler {
	return &SessionHandler{
		store:   store,
		bolt:    bolt,
		authMod: authMod,
	}
}

// SessionInfo represents an active user session.
type SessionInfo struct {
	ID        string    `json:"id"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	Current   bool      `json:"current"`
}

// GetCurrentUser handles GET /api/v1/auth/me
func (h *SessionHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.store.GetUser(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	membership, _ := h.store.GetUserMembership(r.Context(), claims.UserID)

	writeJSON(w, http.StatusOK, map[string]any{
		"user":       user,
		"tenant_id":  claims.TenantID,
		"role_id":    claims.RoleID,
		"membership": membership,
	})
}

// UpdateProfile handles PATCH /api/v1/auth/me
func (h *SessionHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var fields []FieldError
	if len(req.Name) > 100 {
		fields = append(fields, FieldError{Field: "name", Message: "must not exceed 100 characters"})
	}
	if len(req.AvatarURL) > 2048 {
		fields = append(fields, FieldError{Field: "avatar_url", Message: "must not exceed 2048 characters"})
	}
	if len(fields) > 0 {
		writeValidationErrors(w, "validation failed", fields)
		return
	}

	user, err := h.store.GetUser(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.AvatarURL != "" {
		user.AvatarURL = req.AvatarURL
	}

	if err := h.store.UpdateUser(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// ChangePassword handles POST /api/v1/auth/change-password
func (h *SessionHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.NewPassword) > 256 {
		writeError(w, http.StatusBadRequest, "password must not exceed 256 characters")
		return
	}

	// Verify current password
	user, err := h.store.GetUser(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if err := auth.VerifyPassword(user.PasswordHash, req.CurrentPassword); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	// Validate new password
	if err := auth.ValidatePasswordStrength(req.NewPassword, 8); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.store.UpdatePassword(r.Context(), claims.UserID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "password update failed")
		return
	}

	// SECURITY FIX (SESS-004): Invalidate all refresh tokens for this user
	// This ensures that after a password change, all existing sessions are terminated
	// and the user must re-authenticate with the new password
	if h.bolt != nil {
		if err := h.revokeAllUserSessions(r.Context(), claims.UserID); err != nil {
			// Log the error but don't fail the password change
			slog.Warn("failed to revoke user sessions after password change",
				"user_id", claims.UserID,
				"error", err)
		}

		// Also revoke the current access token
		if claims.ID != "" {
			if err := h.authMod.JWT().RevokeAccessToken(h.bolt, claims.ID, claims.UserID, claims.ExpiresAt.Time); err != nil {
				slog.Warn("failed to revoke access token after password change",
					"user_id", claims.UserID,
					"jti", claims.ID,
					"error", err)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}

// SessionTrackingInfo stores information about an active user session
type SessionTrackingInfo struct {
	UserID    string    `json:"user_id"`
	JTI       string    `json:"jti"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

const userSessionsBucket = "user_sessions"
const maxConcurrentSessions = 10 // SESS-003: Limit concurrent sessions per user

// TrackUserSession records a new session for a user (called during login/refresh)
func (h *SessionHandler) TrackUserSession(userID, jti, ip, userAgent string) error {
	if h.bolt == nil {
		return nil
	}

	sessionKey := fmt.Sprintf("%s:%s", userID, jti)
	session := SessionTrackingInfo{
		UserID:    userID,
		JTI:       jti,
		IP:        ip,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
	}

	// Store with 7-day TTL (matching refresh token lifetime)
	return h.bolt.Set(userSessionsBucket, sessionKey, session, auth.RefreshTokenTTLSeconds)
}

// GetUserSessions returns all active sessions for a user
func (h *SessionHandler) GetUserSessions(userID string) ([]SessionTrackingInfo, error) {
	if h.bolt == nil {
		return nil, nil
	}

	keys, err := h.bolt.List(userSessionsBucket)
	if err != nil {
		return nil, err
	}

	var sessions []SessionTrackingInfo
	prefix := userID + ":"

	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		var session SessionTrackingInfo
		if err := h.bolt.Get(userSessionsBucket, key, &session); err == nil {
			sessions = append(sessions, session)
		}
	}

	// Sort by creation time (oldest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.Before(sessions[j].CreatedAt)
	})

	return sessions, nil
}

// revokeAllUserSessions invalidates all refresh tokens for a user
func (h *SessionHandler) revokeAllUserSessions(ctx context.Context, userID string) error {
	sessions, err := h.GetUserSessions(userID)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		// Revoke the refresh token
		if err := h.bolt.Set("revoked_tokens", session.JTI, true, auth.RefreshTokenTTLSeconds); err != nil {
			slog.Warn("failed to revoke refresh token during logout all",
				"user_id", userID,
				"jti", session.JTI,
				"error", err)
		}

		// Remove from user sessions tracking
		sessionKey := fmt.Sprintf("%s:%s", userID, session.JTI)
		if err := h.bolt.Delete(userSessionsBucket, sessionKey); err != nil {
			slog.Warn("failed to delete session tracking entry",
				"user_id", userID,
				"jti", session.JTI,
				"error", err)
		}
	}

	return nil
}

// enforceSessionLimit ensures a user doesn't exceed max concurrent sessions
func (h *SessionHandler) enforceSessionLimit(userID string) error {
	sessions, err := h.GetUserSessions(userID)
	if err != nil {
		return err
	}

	// SESS-003: If over limit, revoke oldest sessions
	if len(sessions) > maxConcurrentSessions {
		toRevoke := len(sessions) - maxConcurrentSessions
		for i := 0; i < toRevoke && i < len(sessions); i++ {
			session := sessions[i]
			if err := h.bolt.Set("revoked_tokens", session.JTI, true, auth.RefreshTokenTTLSeconds); err != nil {
				slog.Warn("failed to revoke old session",
					"user_id", userID,
					"jti", session.JTI,
					"error", err)
			}
			// Remove from tracking
			sessionKey := fmt.Sprintf("%s:%s", userID, session.JTI)
			h.bolt.Delete(userSessionsBucket, sessionKey)
		}
	}

	return nil
}

// LogoutAll handles POST /api/v1/auth/logout-all
// SESS-005: Invalidate all sessions for the current user
func (h *SessionHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Revoke all sessions
	if err := h.revokeAllUserSessions(r.Context(), claims.UserID); err != nil {
		slog.Error("failed to revoke all sessions",
			"user_id", claims.UserID,
			"error", err)
		writeError(w, http.StatusInternalServerError, "failed to logout all sessions")
		return
	}

	// Also revoke current access token
	if h.bolt != nil && claims.ID != "" {
		if err := h.authMod.JWT().RevokeAccessToken(h.bolt, claims.ID, claims.UserID, claims.ExpiresAt.Time); err != nil {
			slog.Warn("failed to revoke current access token",
				"user_id", claims.UserID,
				"jti", claims.ID,
				"error", err)
		}
	}

	// Clear cookies
	clearTokenCookies(w, r)

	writeJSON(w, http.StatusOK, map[string]string{"status": "all sessions logged out"})
}

// ListSessions handles GET /api/v1/auth/sessions
// SESS-005: List all active sessions for the current user
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sessions, err := h.GetUserSessions(claims.UserID)
	if err != nil {
		slog.Error("failed to get user sessions",
			"user_id", claims.UserID,
			"error", err)
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	// Get current request info for marking current session
	currentIP := r.RemoteAddr
	if fwdFor := r.Header.Get("X-Forwarded-For"); fwdFor != "" {
		currentIP = strings.Split(fwdFor, ",")[0]
	}
	currentUA := r.Header.Get("User-Agent")

	// Convert to response format
	var sessionInfos []SessionInfo
	for _, s := range sessions {
		// Try to determine if this is the current session
		isCurrent := s.IP == currentIP && s.UserAgent == currentUA

		sessionInfos = append(sessionInfos, SessionInfo{
			ID:        s.JTI[:8] + "...", // Truncated for security
			IPAddress: s.IP,
			UserAgent: s.UserAgent,
			CreatedAt: s.CreatedAt,
			Current:   isCurrent,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": sessionInfos,
		"count":    len(sessionInfos),
		"limit":    maxConcurrentSessions,
	})
}
