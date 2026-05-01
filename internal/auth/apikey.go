package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const apiKeyPrefix = "dm_"

// apiKeyBcryptCost mirrors the password-hashing cost (13) so API keys and
// passwords have the same offline-attack economics — an attacker who dumps
// the DB should not find API keys cheaper to crack than user passwords.
// Older hashes generated at bcrypt.DefaultCost (10) continue to verify
// correctly because bcrypt encodes the cost into the hash string itself;
// no migration is required. New keys generated from now on use cost 13.
const apiKeyBcryptCost = 13

// APIKeyPair contains a generated API key and its hash.
type APIKeyPair struct {
	Key    string // Full key (only shown once): dm_xxxx...
	Hash   string // bcrypt hash (stored in DB)
	Prefix string // First 12 chars for display: dm_xxxx...
}

// GenerateAPIKey creates a new API key with the dm_ prefix.
func GenerateAPIKey() (*APIKeyPair, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}

	key := apiKeyPrefix + hex.EncodeToString(b)
	hash, err := HashAPIKey(key)
	if err != nil {
		return nil, fmt.Errorf("hash api key: %w", err)
	}
	prefix := key[:len(apiKeyPrefix)+12]

	return &APIKeyPair{
		Key:    key,
		Hash:   hash,
		Prefix: prefix,
	}, nil
}

// HashAPIKey creates a bcrypt hash of an API key at apiKeyBcryptCost. Old
// hashes written at a lower cost still verify via VerifyAPIKey; bcrypt reads
// the cost from the hash prefix, so no migration is needed.
func HashAPIKey(key string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(key), apiKeyBcryptCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(hash), nil
}

// VerifyAPIKey compares a provided API key against a stored bcrypt hash.
// Uses constant-time comparison to prevent timing attacks.
func VerifyAPIKey(key, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(key))
	return err == nil
}

// SECURITY NOTE (AUTH-003): API key prefix lookup is intentionally O(n)
// across all stored keys to authenticate requests. While this allows
// prefix enumeration (given a prefix, you can find matching keys), this is
// expected design — the full key is required to use the API key, and the
// prefix alone provides no actionable information without the secret portion.
// The lookup is bounded and rate-limited via per-IP and per-account limits.
