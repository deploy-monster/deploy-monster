package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Login ───────────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(loginRequest{Email: "user@example.com", Password: "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var tokenPair auth.TokenPair
	if err := json.Unmarshal(rr.Body.Bytes(), &tokenPair); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if tokenPair.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if tokenPair.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if tokenPair.TokenType != "Bearer" {
		t.Errorf("expected token_type Bearer, got %q", tokenPair.TokenType)
	}
	if tokenPair.ExpiresIn <= 0 {
		t.Errorf("expected positive expires_in, got %d", tokenPair.ExpiresIn)
	}

	// Verify UpdateLastLogin was called.
	if store.lastLoginUserID != "user1" {
		t.Errorf("expected UpdateLastLogin for user1, got %q", store.lastLoginUserID)
	}
}

func TestLogin_InvalidJSON(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestLogin_MissingFields(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	tests := []struct {
		name string
		body loginRequest
	}{
		{"missing email", loginRequest{Password: "pass"}},
		{"missing password", loginRequest{Email: "a@b.com"}},
		{"both empty", loginRequest{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.Login(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
			assertErrorMessage(t, rr, "email and password required")
		})
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(loginRequest{Email: "nobody@example.com", Password: "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid credentials")
}

func TestLogin_WrongPassword(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(loginRequest{Email: "user@example.com", Password: "WrongPass1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid credentials")
}

func TestLogin_StoreError(t *testing.T) {
	store := newMockStore()
	store.errGetUserByEmail = errors.New("db connection lost")

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(loginRequest{Email: "user@example.com", Password: "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "internal error")
}

func TestLogin_MembershipError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	store.errGetUserMembership = errors.New("membership lookup failed")

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(loginRequest{Email: "user@example.com", Password: "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "internal error")
}

// ─── Register ────────────────────────────────────────────────────────────────

func TestRegister_Success(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(registerRequest{
		Email:    "new@example.com",
		Password: "StrongPass1",
		Name:     "New User",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var tokenPair auth.TokenPair
	if err := json.Unmarshal(rr.Body.Bytes(), &tokenPair); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if tokenPair.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if tokenPair.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
}

func TestRegister_DefaultsNameToEmail(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(registerRequest{
		Email:    "noname@example.com",
		Password: "StrongPass1",
		// Name intentionally omitted
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRegister_InvalidJSON(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader([]byte("{{{")))
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestRegister_MissingFields(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	tests := []struct {
		name string
		body registerRequest
	}{
		{"missing email", registerRequest{Password: "StrongPass1"}},
		{"missing password", registerRequest{Email: "a@b.com"}},
		{"both empty", registerRequest{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.Register(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
		})
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	tests := []struct {
		name     string
		password string
	}{
		{"too short", "Ab1"},
		{"no uppercase", "lowercase1"},
		{"no lowercase", "UPPERCASE1"},
		{"no digit", "NoDigitsHere"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(registerRequest{Email: "a@b.com", Password: tt.password})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.Register(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "taken@example.com", "Password1", "t1", "role_owner")

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(registerRequest{Email: "taken@example.com", Password: "StrongPass1", Name: "Dup"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "email already registered")
}

func TestRegister_TenantCreationError(t *testing.T) {
	store := newMockStore()
	store.errCreateTenantWithDefaults = errors.New("db error")

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(registerRequest{Email: "new@example.com", Password: "StrongPass1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestRegister_UserCreationError(t *testing.T) {
	store := newMockStore()
	store.errCreateUserWithMembership = errors.New("constraint violation")

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(registerRequest{Email: "new@example.com", Password: "StrongPass1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Refresh ─────────────────────────────────────────────────────────────────

func TestRefresh_Success(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	// Generate a valid refresh token.
	refreshToken := generateTestRefreshToken("user1", "tenant1", "role_owner", "user@example.com")

	body, _ := json.Marshal(refreshRequest{RefreshToken: refreshToken})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var tokenPair auth.TokenPair
	if err := json.Unmarshal(rr.Body.Bytes(), &tokenPair); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if tokenPair.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
}

func TestRefresh_InvalidJSON(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader([]byte("bad")))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestRefresh_MissingToken(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(refreshRequest{RefreshToken: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "refresh_token required")
}

func TestRefresh_InvalidToken(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(refreshRequest{RefreshToken: "invalid.token.here"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid refresh token")
}

func TestRefresh_UserNotFound(t *testing.T) {
	store := newMockStore()
	// User not seeded — the refresh token refers to a non-existent user.
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	refreshToken := generateTestRefreshToken("ghost", "t1", "role_owner", "ghost@example.com")

	body, _ := json.Marshal(refreshRequest{RefreshToken: refreshToken})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "user not found")
}

func TestRefresh_MembershipError(t *testing.T) {
	store := newMockStore()
	// Seed user but make membership lookup fail.
	u := &core.User{ID: "user1", Email: "user@example.com", Name: "Test"}
	store.addUser(u, nil) // no membership
	store.errGetUserMembership = errors.New("db error")

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	refreshToken := generateTestRefreshToken("user1", "t1", "role_owner", "user@example.com")

	body, _ := json.Marshal(refreshRequest{RefreshToken: refreshToken})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Session / GetCurrentUser (GET /me) ──────────────────────────────────────

func TestGetCurrentUser_Success(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")

	handler := NewSessionHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetCurrentUser(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	userMap, ok := resp["user"].(map[string]any)
	if !ok {
		t.Fatal("expected user object in response")
	}
	if userMap["email"] != "user@example.com" {
		t.Errorf("expected email user@example.com, got %v", userMap["email"])
	}
	if resp["tenant_id"] != "tenant1" {
		t.Errorf("expected tenant_id tenant1, got %v", resp["tenant_id"])
	}
	if resp["role_id"] != "role_owner" {
		t.Errorf("expected role_id role_owner, got %v", resp["role_id"])
	}
}

func TestGetCurrentUser_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewSessionHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	// No claims in context.
	rr := httptest.NewRecorder()

	handler.GetCurrentUser(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestGetCurrentUser_UserNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewSessionHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req = withClaims(req, "ghost", "tenant1", "role_owner", "ghost@example.com")
	rr := httptest.NewRecorder()

	handler.GetCurrentUser(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// ─── UpdateProfile (PATCH /me) ───────────────────────────────────────────────

func TestUpdateProfile_Success(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")

	handler := NewSessionHandler(store)

	body, _ := json.Marshal(map[string]string{"name": "Updated Name"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/me", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.UpdateProfile(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if store.updatedUser == nil {
		t.Fatal("expected user to be updated")
	}
	if store.updatedUser.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %q", store.updatedUser.Name)
	}
}

func TestUpdateProfile_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewSessionHandler(store)

	body, _ := json.Marshal(map[string]string{"name": "X"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/me", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.UpdateProfile(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestUpdateProfile_InvalidJSON(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	handler := NewSessionHandler(store)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/me", bytes.NewReader([]byte("not json")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.UpdateProfile(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestUpdateProfile_StoreError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	store.errUpdateUser = errors.New("db error")

	handler := NewSessionHandler(store)

	body, _ := json.Marshal(map[string]string{"name": "Fail"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/me", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.UpdateProfile(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── ChangePassword ──────────────────────────────────────────────────────────

func TestChangePassword_Success(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")

	handler := NewSessionHandler(store)

	body, _ := json.Marshal(map[string]string{
		"current_password": "Password1",
		"new_password":     "NewStrongPass2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ChangePassword(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if store.updatedPassword == "" {
		t.Error("expected password to be updated")
	}

	// Verify the new password hash works.
	if err := auth.VerifyPassword(store.updatedPassword, "NewStrongPass2"); err != nil {
		t.Errorf("new password hash should verify: %v", err)
	}
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")

	handler := NewSessionHandler(store)

	body, _ := json.Marshal(map[string]string{
		"current_password": "WrongPass1",
		"new_password":     "NewStrongPass2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ChangePassword(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "current password is incorrect")
}

func TestChangePassword_WeakNewPassword(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")

	handler := NewSessionHandler(store)

	body, _ := json.Marshal(map[string]string{
		"current_password": "Password1",
		"new_password":     "weak",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ChangePassword(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestChangePassword_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewSessionHandler(store)

	body, _ := json.Marshal(map[string]string{
		"current_password": "X",
		"new_password":     "Y",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ChangePassword(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestChangePassword_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewSessionHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewReader([]byte("bad")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ChangePassword(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestChangePassword_StoreError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	store.errUpdatePassword = errors.New("db failure")

	handler := NewSessionHandler(store)

	body, _ := json.Marshal(map[string]string{
		"current_password": "Password1",
		"new_password":     "NewStrongPass2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ChangePassword(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// assertErrorMessage checks the JSON response has the expected error message.
func assertErrorMessage(t *testing.T, rr *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v (body: %s)", err, rr.Body.String())
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured error object, got %T: %v", resp["error"], resp["error"])
	}
	msg, _ := errObj["message"].(string)
	if msg != expected {
		t.Errorf("expected error %q, got %q", expected, msg)
	}
}
