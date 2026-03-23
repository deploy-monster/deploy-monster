package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	internalAuth "github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authMod *internalAuth.Module
	store   core.Store
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(authMod *internalAuth.Module, store core.Store) *AuthHandler {
	return &AuthHandler{
		authMod: authMod,
		store:   store,
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
	h.store.UpdateLastLogin(r.Context(), user.ID)

	writeJSON(w, http.StatusOK, tokens)
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	// Validate password strength
	if err := internalAuth.ValidatePasswordStrength(req.Password, 8); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

	writeJSON(w, http.StatusCreated, tokens)
}

// Refresh handles POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	// Validate refresh token
	userID, err := h.authMod.JWT().ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	// Get user
	user, err := h.store.GetUser(r.Context(), userID)
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

	// Generate new token pair
	tokens, err := h.authMod.JWT().GenerateTokenPair(user.ID, membership.TenantID, membership.RoleID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	writeJSON(w, http.StatusOK, tokens)
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
