package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const apiKeyPrefix = "dm_"

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

// HashAPIKey creates a bcrypt hash of an API key.
// SECURITY FIX (CRYPTO-001): Changed from SHA-256 to bcrypt to prevent rainbow table attacks.
// bcrypt's adaptive cost factor makes offline attacks computationally expensive.
func HashAPIKey(key string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
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
