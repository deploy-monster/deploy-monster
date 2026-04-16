package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
)

// SESS-001: regression tests proving both /auth/logout and /auth/refresh
// revoke the current access token's JTI by adding it to the
// "revoked_access_tokens" BBolt bucket. Middleware checks this bucket on
// every authenticated request, so without these revocations a stolen
// access token would remain usable for up to the full 15-minute TTL
// after the legitimate client signed out or rotated.

func sess001Pair(t *testing.T, store *mockStore) *auth.TokenPair {
	t.Helper()
	jwt := testJWT()
	pair, err := jwt.GenerateTokenPair("user1", "tenant1", "role_owner", "user@example.com")
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}
	_ = store
	return pair
}

func sess001JTI(t *testing.T, accessToken string) string {
	t.Helper()
	claims, err := testJWT().ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate access token: %v", err)
	}
	if claims.ID == "" {
		t.Fatalf("access token has empty JTI")
	}
	return claims.ID
}

func TestAuthHandler_Logout_RevokesAccessToken_BearerHeader(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	bolt := newMockBoltStore()

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, bolt)

	pair := sess001Pair(t, store)
	jti := sess001JTI(t, pair.AccessToken)

	body, _ := json.Marshal(logoutRequest{RefreshToken: pair.RefreshToken})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()

	handler.Logout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if !testJWT().IsAccessTokenRevoked(bolt, jti) {
		t.Fatalf("expected access token JTI %q to be revoked, but lookup returned false", jti)
	}
}

func TestAuthHandler_Logout_RevokesAccessToken_Cookie(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	bolt := newMockBoltStore()

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, bolt)

	pair := sess001Pair(t, store)
	jti := sess001JTI(t, pair.AccessToken)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: cookieAccess, Value: pair.AccessToken})
	req.AddCookie(&http.Cookie{Name: cookieRefresh, Value: pair.RefreshToken})
	rr := httptest.NewRecorder()

	handler.Logout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if !testJWT().IsAccessTokenRevoked(bolt, jti) {
		t.Fatalf("expected access token JTI %q from cookie to be revoked", jti)
	}
}

func TestAuthHandler_Logout_NoAccessToken_NoOp(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, bolt)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rr := httptest.NewRecorder()

	handler.Logout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// No access token presented, so the denylist must stay empty — a
	// regression that began writing empty-JTI entries would show up as a
	// populated bucket here.
	bolt.mu.Lock()
	defer bolt.mu.Unlock()
	if bkt := bolt.data["revoked_access_tokens"]; len(bkt) != 0 {
		t.Fatalf("expected empty revoked_access_tokens bucket, got %d entries", len(bkt))
	}
}

func TestAuthHandler_Logout_NilBolt_DoesNotPanic(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")

	authMod := testAuthModule(store)
	// Intentionally pass nil bolt — handler must degrade gracefully.
	handler := NewAuthHandler(authMod, store, nil)

	pair := sess001Pair(t, store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()

	handler.Logout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthHandler_Refresh_RevokesOldAccessToken(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	bolt := newMockBoltStore()

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, bolt)

	pair := sess001Pair(t, store)
	oldJTI := sess001JTI(t, pair.AccessToken)

	body, _ := json.Marshal(refreshRequest{RefreshToken: pair.RefreshToken})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// The *old* access token paired with the rotated refresh must be on
	// the denylist — otherwise a token theft right before rotation would
	// keep working silently.
	if !testJWT().IsAccessTokenRevoked(bolt, oldJTI) {
		t.Fatalf("expected old access token JTI %q to be revoked after refresh", oldJTI)
	}

	// Sanity: the new access token minted by Refresh must NOT be revoked.
	var newPair auth.TokenPair
	if err := json.Unmarshal(rr.Body.Bytes(), &newPair); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	newJTI := sess001JTI(t, newPair.AccessToken)
	if newJTI == oldJTI {
		t.Fatalf("new access token JTI must differ from old (got %q for both)", newJTI)
	}
	if testJWT().IsAccessTokenRevoked(bolt, newJTI) {
		t.Fatalf("new access token JTI %q must not be on the denylist", newJTI)
	}
}
