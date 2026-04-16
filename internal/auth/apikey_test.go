package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
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

// TestHashAPIKey_CostIsPinnedToPasswordCost asserts the bcrypt cost embedded
// in new API-key hashes matches the password cost. If this fails, someone
// silently weakened (or diverged) API-key hashing from password hashing —
// which is an asymmetry we explicitly don't want: an attacker who dumps the
// DB should not find API keys cheaper to crack than user passwords.
func TestHashAPIKey_CostIsPinnedToPasswordCost(t *testing.T) {
	hash, err := HashAPIKey("dm_costpintest")
	if err != nil {
		t.Fatalf("HashAPIKey: %v", err)
	}

	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost: %v", err)
	}
	if cost != bcryptCost {
		t.Errorf("API-key bcrypt cost = %d, want %d (== password bcryptCost). "+
			"API keys must not be cheaper to crack than passwords.", cost, bcryptCost)
	}
	if cost != apiKeyBcryptCost {
		t.Errorf("API-key bcrypt cost = %d, want %d (apiKeyBcryptCost constant)", cost, apiKeyBcryptCost)
	}
}

// TestVerifyAPIKey_AcceptsLegacyLowerCostHashes guards the no-migration
// promise: hashes written with bcrypt.DefaultCost (10) — the value this
// code used before Sprint 2 raised the cost to 13 — must still verify.
// bcrypt reads the cost from the hash prefix so this should just work, but
// the test pins the behaviour so a future refactor can't quietly break
// every API key in existing deployments.
func TestVerifyAPIKey_AcceptsLegacyLowerCostHashes(t *testing.T) {
	key := "dm_legacycost10"
	legacyHash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword (legacy): %v", err)
	}
	if cost, _ := bcrypt.Cost(legacyHash); cost != bcrypt.DefaultCost {
		t.Fatalf("legacy hash cost = %d, want %d — setup error", cost, bcrypt.DefaultCost)
	}
	if !VerifyAPIKey(key, string(legacyHash)) {
		t.Error("VerifyAPIKey must accept legacy cost-10 hashes — failing this " +
			"test means we'd brick every API key issued before the cost bump.")
	}
}
