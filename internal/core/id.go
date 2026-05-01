package core

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"math/big"
)

// GenerateID returns a 16-character hex random ID.
func GenerateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fail closed: crypto/rand must be available for ID generation.
		// Returning an empty or predictable ID would cause security issues
		// (JWT JTIs, API keys, etc.). Panic to fail fast and alert operators.
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// GenerateSecret returns a crypto-random base64 string of the given byte length.
func GenerateSecret(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.URLEncoding.EncodeToString(b)
}

// GeneratePassword returns a crypto-random alphanumeric password.
func GeneratePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	max := big.NewInt(int64(len(charset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic("crypto/rand unavailable: " + err.Error())
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}
