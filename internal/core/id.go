package core

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	mrand "math/rand/v2"
)

// GenerateID returns a 16-character hex random ID.
func GenerateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// GenerateSecret returns a crypto-random base64 string of the given byte length.
func GenerateSecret(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.URLEncoding.EncodeToString(b)
}

// GeneratePassword returns a crypto-random alphanumeric password.
func GeneratePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[mrand.IntN(len(charset))]
	}
	return string(b)
}
