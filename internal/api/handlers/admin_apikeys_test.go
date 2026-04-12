package handlers

import (
	"testing"
	"time"
)

func TestCleanupExpiredKeys_RemovesExpired(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAdminAPIKeyHandler(nil, bolt)

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	expiredRec := apiKeyRecord{
		Prefix:    "exp_abc",
		Hash:      "hash1",
		Type:      "platform",
		CreatedBy: "user1",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: &past,
	}
	validRec := apiKeyRecord{
		Prefix:    "val_xyz",
		Hash:      "hash2",
		Type:      "platform",
		CreatedBy: "user1",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: &future,
	}

	bolt.Set("api_keys", "exp_abc", expiredRec, 0)
	bolt.Set("api_keys", "val_xyz", validRec, 0)
	bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"exp_abc", "val_xyz"}}, 0)

	removed := h.CleanupExpiredKeys()
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	// Verify only valid key remains in index
	var idx apiKeyIndex
	if err := bolt.Get("api_keys", "_index", &idx); err != nil {
		t.Fatalf("failed to get index: %v", err)
	}
	if len(idx.Prefixes) != 1 || idx.Prefixes[0] != "val_xyz" {
		t.Errorf("expected [val_xyz], got %v", idx.Prefixes)
	}

	// Verify expired key is deleted from store
	var check apiKeyRecord
	if err := bolt.Get("api_keys", "exp_abc", &check); err == nil {
		t.Error("expected expired key to be deleted from store")
	}
}

func TestCleanupExpiredKeys_NoExpiry(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAdminAPIKeyHandler(nil, bolt)

	// Key with no expiry (nil ExpiresAt) should not be removed
	rec := apiKeyRecord{
		Prefix:    "perm_abc",
		Hash:      "hash1",
		Type:      "platform",
		CreatedBy: "user1",
		CreatedAt: time.Now(),
		ExpiresAt: nil,
	}
	bolt.Set("api_keys", "perm_abc", rec, 0)
	bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"perm_abc"}}, 0)

	removed := h.CleanupExpiredKeys()
	if removed != 0 {
		t.Errorf("expected 0 removed for non-expiring key, got %d", removed)
	}
}

func TestCleanupExpiredKeys_EmptyIndex(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAdminAPIKeyHandler(nil, bolt)

	removed := h.CleanupExpiredKeys()
	if removed != 0 {
		t.Errorf("expected 0 removed for missing index, got %d", removed)
	}
}

func TestCleanupExpiredKeys_AllExpired(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAdminAPIKeyHandler(nil, bolt)

	past1 := time.Now().Add(-2 * time.Hour)
	past2 := time.Now().Add(-1 * time.Hour)

	bolt.Set("api_keys", "exp1", apiKeyRecord{Prefix: "exp1", Hash: "h1", ExpiresAt: &past1}, 0)
	bolt.Set("api_keys", "exp2", apiKeyRecord{Prefix: "exp2", Hash: "h2", ExpiresAt: &past2}, 0)
	bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"exp1", "exp2"}}, 0)

	removed := h.CleanupExpiredKeys()
	if removed != 2 {
		t.Fatalf("expected 2 removed, got %d", removed)
	}

	var idx apiKeyIndex
	if err := bolt.Get("api_keys", "_index", &idx); err != nil {
		t.Fatalf("failed to get index: %v", err)
	}
	if len(idx.Prefixes) != 0 {
		t.Errorf("expected empty index, got %v", idx.Prefixes)
	}
}
