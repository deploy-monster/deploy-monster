package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	internalAuth "github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

const (
	cookieAccess  = "dm_access"
	cookieRefresh = "dm_refresh"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authMod       AuthServices
	store         core.Store
	bolt          core.BoltStorer
	totpValidator func(userID, code string) bool // TOTP validator function
}

// AuthServices is the narrow auth surface used by API handlers.
type AuthServices interface {
	JWT() *internalAuth.JWTService
	TOTP() *internalAuth.TOTPService
}

// isSecureRequest reports whether the request arrived over TLS, either
// directly (r.TLS != nil) or via a reverse proxy that set
// X-Forwarded-Proto: https. The returned value is used to gate the
// Cookie.Secure flag: a Secure cookie set over plain HTTP is silently
// dropped by Chromium, which breaks dev/test against a plain-HTTP
// listener — notably the E2E Playwright suite, which runs against
// http://localhost:8443 in CI.
func isSecureRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	return false
}

// setTokenCookies sets httpOnly cookies for both access and refresh tokens.
func setTokenCookies(w http.ResponseWriter, r *http.Request, tokens *internalAuth.TokenPair) {
	secure := isSecureRequest(r)
	// SameSite=Strict provides stronger CSRF protection than Lax.
	// All API calls go to the same origin, so this won't break legitimate usage.
	sameSite := http.SameSiteStrictMode
	http.SetCookie(w, &http.Cookie{
		Name:     cookieAccess,
		Value:    tokens.AccessToken,
		Path:     "/",
		MaxAge:   tokens.ExpiresIn,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     cookieRefresh,
		Value:    tokens.RefreshToken,
		Path:     "/",
		MaxAge:   internalAuth.RefreshTokenTTLSeconds,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}

// clearTokenCookies removes both token cookies (all known paths for migration).
func clearTokenCookies(w http.ResponseWriter, r *http.Request) {
	secure := isSecureRequest(r)
	sameSite := http.SameSiteStrictMode
	paths := []string{"/", "/api", "/api/v1/auth"}
	for _, p := range paths {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieAccess,
			Value:    "",
			Path:     p,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secure,
			SameSite: sameSite,
		})
		http.SetCookie(w, &http.Cookie{
			Name:     cookieRefresh,
			Value:    "",
			Path:     p,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secure,
			SameSite: sameSite,
		})
	}
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(authMod AuthServices, store core.Store, bolt core.BoltStorer) *AuthHandler {
	h := &AuthHandler{
		authMod: authMod,
		store:   store,
		bolt:    bolt,
	}
	// Set up TOTP validator if auth module provides one
	if authMod != nil && authMod.TOTP() != nil {
		h.totpValidator = authMod.TOTP().Validate
	}
	return h
}

// validateTOTP validates a TOTP code for a user.
// Uses the auth module's TOTP service if available.
func (h *AuthHandler) validateTOTP(userID, code string) bool {
	if h.totpValidator == nil {
		// TOTP not configured - fail closed
		return false
	}
	return h.totpValidator(userID, code)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"` // Required if TOTP is enabled for the user
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// SECURITY FIX: Session fixation prevention
	// Clear any existing authentication cookies before login
	// This ensures a new session is created even if an attacker provided a known session ID
	clearTokenCookies(w, r)

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	// Guard against excessively long passwords (bcrypt truncates at 72 bytes;
	// hashing very large inputs wastes CPU and can be used for DoS).
	if len(req.Password) > 256 {
		writeError(w, http.StatusBadRequest, "password must not exceed 256 characters")
		return
	}

	// Look up user by email
	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Per-account rate limit check before verifying password
	if h.loginRateLimitCheck(w, r, req.Email) != 0 {
		return
	}

	// Verify password
	if err := internalAuth.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		// Per-account rate limiting: track failed attempts per email to prevent
		// credential stuffing. A attacker cycling through many accounts from a
		// single IP is more dangerous than one trying many passwords on one account.
		h.incrementPerAccountRateLimit(r.Context(), req.Email)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// TOTP verification: if enabled, require a valid TOTP code
	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			// Signal that TOTP is required - client should prompt for code
			w.Header().Set("X-TOTP-Required", "true")
			writeError(w, http.StatusUnauthorized, "TOTP code required")
			return
		}
		// Validate TOTP code using the stored encrypted secret
		// The stored secret is bcrypt hashed, we need the raw secret for TOTP validation
		// Since we store bcrypt(secret), we pass the encrypted secret to validation
		// Note: in production, consider using a separate TOTP secret storage
		if !h.validateTOTP(user.ID, req.TOTPCode) {
			h.incrementPerAccountRateLimit(r.Context(), req.Email)
			writeError(w, http.StatusUnauthorized, "invalid TOTP code")
			return
		}
	}

	// Get user's membership (tenant + role)
	membership, err := h.store.GetUserMembership(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Generate token pair
	tokens, err := h.authMod.JWT().GenerateTokenPair(user.ID, membership.TenantID, membership.RoleID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	// Update last login
	if err := h.store.UpdateLastLogin(r.Context(), user.ID); err != nil {
		slog.Warn("failed to update last login", "user_id", user.ID, "error", err)
	}

	setTokenCookies(w, r, tokens)
	middleware.SetCSRFCookie(w, r)

	// SESS-003: Track this session for concurrent session limiting
	h.trackSession(r, user.ID, tokens.RefreshToken)

	writeJSON(w, http.StatusOK, tokens)
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Collect all validation errors so the client sees every issue at once.
	var fields []FieldError

	if req.Email == "" {
		fields = append(fields, FieldError{Field: "email", Message: "email is required"})
	} else {
		if len(req.Email) > 254 { // RFC 5321 max email length
			fields = append(fields, FieldError{Field: "email", Message: "must not exceed 254 characters"})
		} else if _, err := mail.ParseAddress(req.Email); err != nil {
			fields = append(fields, FieldError{Field: "email", Message: "invalid email format"})
		}
	}

	if req.Password == "" {
		fields = append(fields, FieldError{Field: "password", Message: "password is required"})
	} else {
		if len(req.Password) > 256 {
			fields = append(fields, FieldError{Field: "password", Message: "must not exceed 256 characters"})
		} else if err := internalAuth.ValidatePasswordStrength(req.Password, 0); err != nil {
			fields = append(fields, FieldError{Field: "password", Message: err.Error()})
		}
	}

	if len(req.Name) > 100 {
		fields = append(fields, FieldError{Field: "name", Message: "must not exceed 100 characters"})
	}

	if len(fields) > 0 {
		writeValidationErrors(w, "validation failed", fields)
		return
	}

	// Check if email already exists
	if _, err := h.store.GetUserByEmail(r.Context(), req.Email); err == nil {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	// Hash password
	hash, err := internalAuth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Create tenant + user
	name := req.Name
	if name == "" {
		name = req.Email
	}

	tenantID, err := h.store.CreateTenantWithDefaults(r.Context(), name+"'s Team", generateSlug(name))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	userID, err := h.store.CreateUserWithMembership(r.Context(), req.Email, hash, name, "active", tenantID, "role_owner")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Generate token pair
	tokens, err := h.authMod.JWT().GenerateTokenPair(userID, tenantID, "role_owner", req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	setTokenCookies(w, r, tokens)
	middleware.SetCSRFCookie(w, r)
	writeJSON(w, http.StatusCreated, tokens)
}

// Refresh handles POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	// Allow empty body — refresh token may come from httpOnly cookie
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Fall back to cookie if body didn't include refresh_token
	if req.RefreshToken == "" {
		if c, err := r.Cookie(cookieRefresh); err == nil {
			req.RefreshToken = c.Value
		}
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	// Validate refresh token
	rtClaims, err := h.authMod.JWT().ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	// Check if token has been revoked
	if h.bolt != nil && rtClaims.JTI != "" {
		var revoked bool
		if err := h.bolt.Get("revoked_tokens", rtClaims.JTI, &revoked); err == nil && revoked {
			writeError(w, http.StatusUnauthorized, "token has been revoked")
			return
		}
	}

	// Get user
	user, err := h.store.GetUser(r.Context(), rtClaims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	// Get membership
	membership, err := h.store.GetUserMembership(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Revoke the old refresh token (rotation)
	if h.bolt != nil && rtClaims.JTI != "" {
		if err := h.bolt.Set("revoked_tokens", rtClaims.JTI, true, internalAuth.RefreshTokenTTLSeconds); err != nil {
			slog.Error("failed to revoke refresh token", "jti", rtClaims.JTI, "error", err)
		}
	}

	// SESS-001: revoke the old access token paired with this refresh.
	// Without this, a stolen access token stays usable for up to 15 min
	// after the legitimate client rotated — defeating the rotation.
	h.revokeAccessTokenFromRequest(r)

	// Generate new token pair
	tokens, err := h.authMod.JWT().GenerateTokenPair(user.ID, membership.TenantID, membership.RoleID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	setTokenCookies(w, r, tokens)
	middleware.SetCSRFCookie(w, r)

	// SESS-003: Track the new session after rotation
	h.trackSession(r, user.ID, tokens.RefreshToken)

	writeJSON(w, http.StatusOK, tokens)
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Fall back to cookie if body didn't include refresh_token
	if req.RefreshToken == "" {
		if c, err := r.Cookie(cookieRefresh); err == nil {
			req.RefreshToken = c.Value
		}
	}

	// SESS-001: Revoke the access token on logout too. The endpoint is
	// unauthenticated (any pair of tokens presented ends the session),
	// so we read the access token from the Authorization header or the
	// dm_access cookie. An expired access token is a no-op inside
	// RevokeAccessToken (its TTL check short-circuits).
	h.revokeAccessTokenFromRequest(r)

	// If we still have no refresh token, clear cookies and return OK —
	// the access token has already been revoked above.
	if req.RefreshToken == "" {
		clearTokenCookies(w, r)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// Validate the refresh token to extract JTI
	rtClaims, err := h.authMod.JWT().ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		// Token is invalid/expired — effectively already "logged out"
		clearTokenCookies(w, r)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// Revoke the refresh token
	if h.bolt != nil && rtClaims.JTI != "" {
		if err := h.bolt.Set("revoked_tokens", rtClaims.JTI, true, internalAuth.RefreshTokenTTLSeconds); err != nil {
			slog.Error("failed to revoke refresh token on logout", "jti", rtClaims.JTI, "error", err)
		}
	}

	clearTokenCookies(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// revokeAccessTokenFromRequest parses the access token from Authorization
// header or dm_access cookie (in that priority order) and adds its JTI to
// the access-token denylist. No-op on any parse failure — a missing or
// malformed access token is not a reason to fail logout.
func (h *AuthHandler) revokeAccessTokenFromRequest(r *http.Request) {
	if h.bolt == nil {
		return
	}
	tokenStr := ""
	if hdr := r.Header.Get("Authorization"); strings.HasPrefix(hdr, "Bearer ") {
		tokenStr = strings.TrimPrefix(hdr, "Bearer ")
	} else if c, err := r.Cookie(cookieAccess); err == nil {
		tokenStr = c.Value
	}
	if tokenStr == "" {
		return
	}
	claims, err := h.authMod.JWT().ValidateAccessToken(tokenStr)
	if err != nil || claims == nil || claims.ID == "" || claims.ExpiresAt == nil {
		return
	}
	if err := h.authMod.JWT().RevokeAccessToken(h.bolt, claims.ID, claims.UserID, claims.ExpiresAt.Time); err != nil {
		slog.Warn("failed to revoke access token on logout", "jti", claims.ID, "error", err)
	}
}

// trackSession extracts the JTI from a refresh token and stores session info.
// Used for SESS-003 concurrent session limiting.
func (h *AuthHandler) trackSession(r *http.Request, userID, refreshToken string) {
	if h.bolt == nil {
		return
	}

	// Parse the refresh token to get JTI
	rtClaims, err := h.authMod.JWT().ValidateRefreshToken(refreshToken)
	if err != nil {
		return
	}

	// Get client IP and User-Agent
	ip := r.RemoteAddr
	if fwdFor := r.Header.Get("X-Forwarded-For"); fwdFor != "" {
		ip = fwdFor
	}
	userAgent := r.Header.Get("User-Agent")

	// Store session tracking info
	sessionKey := userID + ":" + rtClaims.JTI
	session := map[string]any{
		"user_id":    userID,
		"jti":        rtClaims.JTI,
		"ip":         ip,
		"user_agent": userAgent,
		"created_at": time.Now(),
	}

	if err := h.bolt.Set("user_sessions", sessionKey, session, internalAuth.RefreshTokenTTLSeconds); err != nil {
		slog.Warn("failed to track session", "user_id", userID, "error", err)
		return
	}

	// Enforce concurrent session limit
	h.enforceSessionLimit(userID)
}

// enforceSessionLimit revokes oldest sessions if user exceeds maxConcurrentSessions
func (h *AuthHandler) enforceSessionLimit(userID string) {
	if h.bolt == nil {
		return
	}

	keys, err := h.bolt.List("user_sessions")
	if err != nil {
		return
	}

	// Collect sessions for this user
	type sessionInfo struct {
		key       string
		jti       string
		createdAt time.Time
	}
	var sessions []sessionInfo
	prefix := userID + ":"

	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		var session struct {
			JTI       string    `json:"jti"`
			CreatedAt time.Time `json:"created_at"`
		}
		if err := h.bolt.Get("user_sessions", key, &session); err == nil {
			sessions = append(sessions, sessionInfo{
				key:       key,
				jti:       session.JTI,
				createdAt: session.CreatedAt,
			})
		}
	}

	// Sort by creation time (oldest first)
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].createdAt.Before(sessions[i].createdAt) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	// SESS-003: If over limit, revoke oldest sessions
	const maxConcurrentSessions = 10
	if len(sessions) > maxConcurrentSessions {
		toRevoke := len(sessions) - maxConcurrentSessions
		for i := 0; i < toRevoke && i < len(sessions); i++ {
			s := sessions[i]
			if err := h.bolt.Set("revoked_tokens", s.jti, true, internalAuth.RefreshTokenTTLSeconds); err != nil {
				slog.Warn("failed to revoke old session", "user_id", userID, "jti", s.jti, "error", err)
			}
			if err := h.bolt.Delete("user_sessions", s.key); err != nil {
				slog.Warn("failed to delete old session tracking", "user_id", userID, "key", s.key, "error", err)
			}
		}
	}
}

// generateSlug converts a name to a URL-friendly slug.
func generateSlug(name string) string {
	slug := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			slug += string(r)
		} else if r >= 'A' && r <= 'Z' {
			slug += string(r + 32)
		} else if r == ' ' || r == '_' {
			slug += "-"
		}
	}
	if slug == "" {
		slug = core.GenerateID()[:8]
	}
	return slug
}

// Per-account rate limiting: track failed login attempts per email address.
// After 5 failed attempts, the account is temporarily locked for 15 minutes.
// This is independent of per-IP limiting and prevents credential stuffing
// where an attacker tries many passwords against a single account.
const maxFailedAttempts = 5
const accountLockoutWindow = 15 * time.Minute

type accountRateLimitEntry struct {
	FailedCount int   `json:"f"`
	LockedUntil int64 `json:"l"` // 0 = not locked
}

func (h *AuthHandler) checkPerAccountRateLimit(email string) (bool, int64) {
	if h.bolt == nil {
		return false, 0
	}
	var entry accountRateLimitEntry
	err := h.bolt.Get("account_rl", email, &entry)
	if err != nil || entry.LockedUntil == 0 {
		return false, 0
	}
	now := time.Now().Unix()
	if now < entry.LockedUntil {
		return true, entry.LockedUntil
	}
	return false, 0
}

func (h *AuthHandler) incrementPerAccountRateLimit(ctx context.Context, email string) {
	if h.bolt == nil {
		return
	}
	var entry accountRateLimitEntry
	// A "key/bucket not found" Get is the expected path for the first
	// failed attempt against a fresh account — fall through with the
	// zero-value entry so the lockout counter actually starts. Any
	// other Get error (corrupted JSON, unexpected bolt failure) means
	// we cannot trust `entry`, so skip the write rather than reset a
	// possibly-already-counted state to FailedCount=1.
	err := h.bolt.Get("account_rl", email, &entry)
	if err != nil && !errors.Is(err, core.ErrBoltNotFound) {
		return
	}
	now := time.Now().Unix()

	if entry.LockedUntil > 0 {
		return // already locked — don't double-penalize
	}

	entry.FailedCount++
	resetAt := now + int64(accountLockoutWindow.Seconds())
	if entry.FailedCount >= maxFailedAttempts {
		entry.LockedUntil = resetAt
	}

	ttl := int64(accountLockoutWindow.Seconds())
	_ = h.bolt.Set("account_rl", email, entry, ttl)
}

// loginRateLimitCheck returns the locked-until timestamp if the account is locked,
// or 0 if not locked. Call this after email lookup but before password verification.
func (h *AuthHandler) loginRateLimitCheck(w http.ResponseWriter, r *http.Request, email string) int64 {
	if h.bolt == nil {
		return 0
	}
	locked, until := h.checkPerAccountRateLimit(email)
	if locked {
		retryAfter := until - time.Now().Unix()
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", maxFailedAttempts))
		w.Header().Set("X-RateLimit-Remaining", "0")
		writeError(w, http.StatusTooManyRequests, "account temporarily locked due to too many failed attempts")
		return until
	}
	return 0
}
