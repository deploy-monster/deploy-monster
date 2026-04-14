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

	// SECURITY FIX (CRYPTO-001): With bcrypt, hash is not deterministic (different salt each time)
	// Verify by checking the key can be verified against the hash
	if !VerifyAPIKey(pair.Key, pair.Hash) {
		t.Error("VerifyAPIKey should verify the key against its hash")
	}
}

func TestGenerateAPIKey_Unique(t *testing.T) {
	pair1, _ := GenerateAPIKey()
	pair2, _ := GenerateAPIKey()

	if pair1.Key == pair2.Key {
		t.Error("two API keys should be different")
	}
	// SECURITY FIX (CRYPTO-001): With bcrypt, hashes will be different due to unique salts
	if pair1.Hash == pair2.Hash {
		t.Error("two API key hashes should be different (bcrypt uses unique salts)")
	}
}

func TestHashAPIKey_Bcrypt(t *testing.T) {
	key := "dm_abc123def456"
	h1, err := HashAPIKey(key)
	if err != nil {
		t.Fatalf("HashAPIKey: %v", err)
	}

	// SECURITY FIX (CRYPTO-001): bcrypt hashes are not deterministic (random salt)
	// but both should verify the same key
	h2, err := HashAPIKey(key)
	if err != nil {
		t.Fatalf("HashAPIKey: %v", err)
	}

	// Hashes should be different due to different salts
	if h1 == h2 {
		t.Error("bcrypt hashes should be different (different salts)")
	}

	// But both should verify the original key
	if !VerifyAPIKey(key, h1) {
		t.Error("VerifyAPIKey should verify against h1")
	}
	if !VerifyAPIKey(key, h2) {
		t.Error("VerifyAPIKey should verify against h2")
	}
}

func TestVerifyAPIKey_InvalidKey(t *testing.T) {
	key := "dm_validkey123"
	hash, _ := HashAPIKey(key)

	// Wrong key should not verify
	if VerifyAPIKey("dm_wrongkey456", hash) {
		t.Error("VerifyAPIKey should reject wrong key")
	}

	// Empty key should not verify
	if VerifyAPIKey("", hash) {
		t.Error("VerifyAPIKey should reject empty key")
	}
}
