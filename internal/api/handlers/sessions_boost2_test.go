package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
)

func TestSessionHandler_LogoutAll_RevokeError(t *testing.T) {
	// Seed a session so revokeAllUserSessions has work to do,
	// but bolt.Delete will succeed, not fail. We need to simulate
	// an error in revokeAllUserSessions. Looking at the implementation,
	// it calls GetUserSessions then deletes each. GetUserSessions
	// returns nil error when bolt is present. So we need bolt.List
	// to return an error.
	bolt2 := &mockBoltStore{data: make(map[string]map[string][]byte)}
	bolt2.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a"}, 0)

	// Now make List return error by clearing the data after Set
	// Actually List returns error when bucket not found. Let's just
	// use the normal bolt but the error path is hard to trigger.
	// Instead, test LogoutAll with claims.ID revocation error.

	// Re-read the code: revokeAllUserSessions error path is when
	// GetUserSessions returns error. That happens when bolt.List fails.
	// Let's create a bolt that fails List.
	listErrBolt := &listErrorBolt{mockBoltStore: *newMockBoltStore()}
	listErrBolt.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a"}, 0)

	h := NewSessionHandler(newMockStore(), listErrBolt, testAuthModule(newMockStore()))

	req := httptest.NewRequest("POST", "/api/v1/auth/logout-all", nil)
	req = withClaims(req, "user-1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.LogoutAll(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// listErrorBolt is a mock bolt that fails List.
type listErrorBolt struct {
	mockBoltStore
}

func (l *listErrorBolt) List(bucket string) ([]string, error) {
	return nil, errors.New("bolt list error")
}

func TestSessionHandler_LogoutAll_RevokeAccessTokenError(t *testing.T) {
	bolt := newMockBoltStore()
	// Seed a session
	bolt.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a"}, 0)

	// Use a real auth module so claims can be validated
	store := newMockStore()
	authMod := testAuthModule(store)

	// Generate a valid token for the user so claims.ID is set
	tokens, _ := authMod.JWT().GenerateTokenPair("user-1", "t1", "role_admin", "admin@test.com")
	claims, _ := authMod.JWT().ValidateAccessToken(tokens.AccessToken)

	// Put the claims into the request context
	req := httptest.NewRequest("POST", "/api/v1/auth/logout-all", nil)
	ctx := auth.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h := NewSessionHandler(store, bolt, authMod)
	h.LogoutAll(rr, req)

	// Even if token revocation fails, the endpoint should return 200
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
