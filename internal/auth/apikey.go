package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const apiKeyPrefix = "dm_"

// APIKeyPair contains a generated API key and its hash.
type APIKeyPair struct {
	Key     string // Full key (only shown once): dm_xxxx...
	Hash    string // SHA-256 hash (stored in DB)
	Prefix  string // First 8 chars for display: dm_xxxx
}

// GenerateAPIKey creates a new API key with the dm_ prefix.
func GenerateAPIKey() (*APIKeyPair, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}

	key := apiKeyPrefix + hex.EncodeToString(b)
	hash := HashAPIKey(key)
	prefix := key[:len(apiKeyPrefix)+8]

	return &APIKeyPair{
		Key:    key,
		Hash:   hash,
		Prefix: prefix,
	}, nil
}

// HashAPIKey creates a SHA-256 hash of an API key.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
