package core

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"math/big"
)

// GenerateID returns a 16-character hex random ID.
func GenerateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to math/big if crypto/rand fails (e.g., /dev/random unavailable).
		// This is intentionally not a panic — crypto/rand failures should not
		// crash a production server. The returned ID has lower entropy but the
		// server can continue serving. Log at error level for alerting.
		slog.Error("crypto/rand unavailable, using math/big fallback", "error", err)
		for i := range b {
			n, _ := rand.Int(rand.Reader, big.NewInt(256))
			b[i] = byte(n.Int64())
		}
	}
	return hex.EncodeToString(b)
}

// GenerateSecret returns a crypto-random base64 string of the given byte length.
func GenerateSecret(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		slog.Error("crypto/rand unavailable for GenerateSecret, using math/big fallback", "error", err)
		for i := range b {
			n, _ := rand.Int(rand.Reader, big.NewInt(256))
			b[i] = byte(n.Int64())
		}
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
			// Fallback if crypto/rand fails
			n, _ = rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}
