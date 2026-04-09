package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"

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
	authMod *internalAuth.Module
	store   core.Store
	bolt    core.BoltStorer
}

// setTokenCookies sets httpOnly cookies for both access and refresh tokens.
func setTokenCookies(w http.ResponseWriter, tokens *internalAuth.TokenPair) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieAccess,
		Value:    tokens.AccessToken,
		Path:     "/api",
		MaxAge:   tokens.ExpiresIn,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     cookieRefresh,
		Value:    tokens.RefreshToken,
		Path:     "/api/v1/auth",
		MaxAge:   internalAuth.RefreshTokenTTLSeconds,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearTokenCookies removes both token cookies.
func clearTokenCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieAccess,
		Value:    "",
		Path:     "/api",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     cookieRefresh,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(authMod *internalAuth.Module, store core.Store, bolt core.BoltStorer) *AuthHandler {
	return &AuthHandler{
		authMod: authMod,
		store:   store,
		bolt:    bolt,
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
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

	// Verify password
	if err := internalAuth.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
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

	setTokenCookies(w, tokens)
	middleware.SetCSRFCookie(w)
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
		} else if err := internalAuth.ValidatePasswordStrength(req.Password, 8); err != nil {
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

	setTokenCookies(w, tokens)
	middleware.SetCSRFCookie(w)
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

	// Generate new token pair
	tokens, err := h.authMod.JWT().GenerateTokenPair(user.ID, membership.TenantID, membership.RoleID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	setTokenCookies(w, tokens)
	middleware.SetCSRFCookie(w)
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

	// If we still have no token, just clear cookies and return OK
	if req.RefreshToken == "" {
		clearTokenCookies(w)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// Validate the refresh token to extract JTI
	rtClaims, err := h.authMod.JWT().ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		// Token is invalid/expired — effectively already "logged out"
		clearTokenCookies(w)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// Revoke the token
	if h.bolt != nil && rtClaims.JTI != "" {
		if err := h.bolt.Set("revoked_tokens", rtClaims.JTI, true, internalAuth.RefreshTokenTTLSeconds); err != nil {
			slog.Error("failed to revoke refresh token on logout", "jti", rtClaims.JTI, "error", err)
		}
	}

	clearTokenCookies(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Me handles GET /api/v1/auth/me — returns the current user from JWT claims.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := internalAuth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":        claims.UserID,
		"email":     claims.Email,
		"role":      claims.RoleID,
		"tenant_id": claims.TenantID,
	})
}

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
