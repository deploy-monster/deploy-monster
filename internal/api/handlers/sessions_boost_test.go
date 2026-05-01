package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionHandler_GetUserSessions(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewSessionHandler(newMockStore(), bolt, nil)

	// Seed two sessions for user-1 and one for user-2
	bolt.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a", IP: "1.1.1.1", CreatedAt: time.Now().Add(-2 * time.Hour)}, 0)
	bolt.Set("user_sessions", "user-1:jti-b", SessionTrackingInfo{UserID: "user-1", JTI: "jti-b", IP: "2.2.2.2", CreatedAt: time.Now().Add(-1 * time.Hour)}, 0)
	bolt.Set("user_sessions", "user-2:jti-c", SessionTrackingInfo{UserID: "user-2", JTI: "jti-c", IP: "3.3.3.3", CreatedAt: time.Now()}, 0)

	sessions, err := h.GetUserSessions("user-1")
	if err != nil {
		t.Fatalf("GetUserSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
	// Should be sorted oldest first
	if sessions[0].JTI != "jti-a" {
		t.Errorf("expected oldest session first, got %s", sessions[0].JTI)
	}
}

func TestSessionHandler_GetUserSessions_NilBolt(t *testing.T) {
	h := NewSessionHandler(newMockStore(), nil, nil)

	sessions, err := h.GetUserSessions("user-1")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if sessions != nil {
		t.Error("expected nil sessions when bolt is nil")
	}
}

func TestSessionHandler_revokeAllUserSessions(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewSessionHandler(newMockStore(), bolt, nil)

	bolt.Set("user_sessions", "user-1:jti-abcdef", SessionTrackingInfo{UserID: "user-1", JTI: "jti-abcdef"}, 0)
	bolt.Set("user_sessions", "user-1:jti-xyz123", SessionTrackingInfo{UserID: "user-1", JTI: "jti-xyz123"}, 0)

	err := h.revokeAllUserSessions(context.Background(), "user-1")
	if err != nil {
		t.Errorf("revokeAllUserSessions: %v", err)
	}

	// Verify sessions are deleted
	keys, _ := bolt.List("user_sessions")
	if len(keys) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(keys))
	}
}

func TestSessionHandler_LogoutAll(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewSessionHandler(newMockStore(), bolt, nil)

	bolt.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a"}, 0)

	req := httptest.NewRequest("POST", "/api/v1/auth/logout-all", nil)
	req = withClaims(req, "user-1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.LogoutAll(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSessionHandler_LogoutAll_NoClaims(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewSessionHandler(newMockStore(), bolt, nil)

	req := httptest.NewRequest("POST", "/api/v1/auth/logout-all", nil)
	rr := httptest.NewRecorder()
	h.LogoutAll(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSessionHandler_ListSessions(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewSessionHandler(newMockStore(), bolt, nil)

	bolt.Set("user_sessions", "user-1:jti-abcdef", SessionTrackingInfo{UserID: "user-1", JTI: "jti-abcdef", IP: "127.0.0.1", UserAgent: "TestAgent", CreatedAt: time.Now()}, 0)

	req := httptest.NewRequest("GET", "/api/v1/auth/sessions", nil)
	req = withClaims(req, "user-1", "t1", "role_admin", "admin@test.com")
	req.Header.Set("User-Agent", "TestAgent")
	rr := httptest.NewRecorder()
	h.ListSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 1 {
		t.Errorf("expected 1 session, got %v", resp["count"])
	}
}

func TestSessionHandler_ListSessions_NoClaims(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewSessionHandler(newMockStore(), bolt, nil)

	req := httptest.NewRequest("GET", "/api/v1/auth/sessions", nil)
	rr := httptest.NewRecorder()
	h.ListSessions(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}
