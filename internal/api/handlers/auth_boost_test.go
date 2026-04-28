package handlers

import (
	"strconv"
	"testing"
	"time"
)

func TestEnforceSessionLimit_NoBolt(t *testing.T) {
	h := NewAuthHandler(nil, nil, nil)
	// Should not panic
	h.enforceSessionLimit("user-1")
}

func TestEnforceSessionLimit_ListError(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAuthHandler(nil, nil, bolt)
	// user_sessions bucket doesn't exist — List returns error
	h.enforceSessionLimit("user-1")
	// No panic = pass
}

func TestEnforceSessionLimit_UnderLimit(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAuthHandler(nil, nil, bolt)

	for i := 0; i < 5; i++ {
		session := map[string]any{
			"user_id":    "user-1",
			"jti":        "jti-" + strconv.Itoa(i),
			"created_at": time.Now().Add(-time.Duration(i) * time.Hour),
		}
		bolt.Set("user_sessions", "user-1:jti-"+strconv.Itoa(i), session, 0)
	}

	h.enforceSessionLimit("user-1")

	keys, _ := bolt.List("user_sessions")
	if len(keys) != 5 {
		t.Errorf("expected 5 sessions, got %d", len(keys))
	}
}

func TestEnforceSessionLimit_OverLimit(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAuthHandler(nil, nil, bolt)

	for i := 0; i < 12; i++ {
		session := map[string]any{
			"user_id":    "user-1",
			"jti":        "jti-" + strconv.Itoa(i),
			"created_at": time.Now().Add(-time.Duration(i) * time.Hour),
		}
		bolt.Set("user_sessions", "user-1:jti-"+strconv.Itoa(i), session, 0)
	}

	h.enforceSessionLimit("user-1")

	keys, _ := bolt.List("user_sessions")
	if len(keys) != 10 {
		t.Errorf("expected 10 sessions after pruning, got %d", len(keys))
	}

	// Oldest 2 sessions (jti-10, jti-11) should be revoked
	var revoked bool
	if err := bolt.Get("revoked_tokens", "jti-10", &revoked); err != nil || !revoked {
		t.Error("expected jti-10 to be revoked")
	}
	if err := bolt.Get("revoked_tokens", "jti-11", &revoked); err != nil || !revoked {
		t.Error("expected jti-11 to be revoked")
	}
}

func TestEnforceSessionLimit_OtherUserNotAffected(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAuthHandler(nil, nil, bolt)

	// 12 sessions for user-1
	for i := 0; i < 12; i++ {
		session := map[string]any{
			"user_id":    "user-1",
			"jti":        "jti-" + strconv.Itoa(i),
			"created_at": time.Now().Add(-time.Duration(i) * time.Hour),
		}
		bolt.Set("user_sessions", "user-1:jti-"+strconv.Itoa(i), session, 0)
	}

	// 5 sessions for user-2
	for i := 0; i < 5; i++ {
		session := map[string]any{
			"user_id":    "user-2",
			"jti":        "jti-u2-" + strconv.Itoa(i),
			"created_at": time.Now().Add(-time.Duration(i) * time.Hour),
		}
		bolt.Set("user_sessions", "user-2:jti-u2-"+strconv.Itoa(i), session, 0)
	}

	h.enforceSessionLimit("user-1")

	keys, _ := bolt.List("user_sessions")
	if len(keys) != 15 {
		t.Errorf("expected 15 sessions total, got %d", len(keys))
	}
}
