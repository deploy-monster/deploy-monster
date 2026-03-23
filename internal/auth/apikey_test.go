package auth

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	pair, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	// Key should start with dm_
	if !strings.HasPrefix(pair.Key, "dm_") {
		t.Errorf("key should start with dm_, got %q", pair.Key[:10])
	}

	// Prefix should be first 11 chars (dm_ + 8 hex)
	if !strings.HasPrefix(pair.Key, pair.Prefix) {
		t.Error("prefix should be start of key")
	}

	// Hash should not be empty
	if pair.Hash == "" {
		t.Error("hash should not be empty")
	}

	// Hash should match
	if HashAPIKey(pair.Key) != pair.Hash {
		t.Error("HashAPIKey should produce same hash")
	}
}

func TestGenerateAPIKey_Unique(t *testing.T) {
	pair1, _ := GenerateAPIKey()
	pair2, _ := GenerateAPIKey()

	if pair1.Key == pair2.Key {
		t.Error("two API keys should be different")
	}
	if pair1.Hash == pair2.Hash {
		t.Error("two API key hashes should be different")
	}
}

func TestHashAPIKey_Deterministic(t *testing.T) {
	key := "dm_abc123def456"
	h1 := HashAPIKey(key)
	h2 := HashAPIKey(key)

	if h1 != h2 {
		t.Error("HashAPIKey should be deterministic")
	}
}
