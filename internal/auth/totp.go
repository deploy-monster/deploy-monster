package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

// TOTPConfig holds TOTP 2FA configuration.
type TOTPConfig struct {
	Secret   string `json:"secret"`
	URL      string `json:"url"`      // otpauth:// URI for QR code
	Recovery []string `json:"recovery"` // Backup codes
}

// GenerateTOTP creates a new TOTP secret and provisioning URI.
func GenerateTOTP(email, issuer string) (*TOTPConfig, error) {
	secret := generateTOTPSecret(20)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(secret))

	uri := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		issuer, email, encoded, issuer)

	// Generate 8 recovery codes
	recovery := make([]string, 8)
	for i := range recovery {
		recovery[i] = fmt.Sprintf("%s-%s",
			randomHex(4), randomHex(4))
	}

	return &TOTPConfig{
		Secret:   encoded,
		URL:      uri,
		Recovery: recovery,
	}, nil
}

// ValidateTOTP checks if a 6-digit code matches the current or adjacent time window.
func ValidateTOTP(secret, code string) bool {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return false
	}

	now := time.Now().Unix() / 30

	// Check current window and ±1 for clock drift
	for _, offset := range []int64{-1, 0, 1} {
		expected := generateCode(key, now+offset)
		if expected == code {
			return true
		}
	}
	return false
}

// generateCode computes a TOTP code for a given time counter.
func generateCode(key []byte, counter int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code := truncated % uint32(math.Pow10(6))

	return fmt.Sprintf("%06d", code)
}

func generateTOTPSecret(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return string(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	const hex = "0123456789abcdef"
	out := make([]byte, n*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out[:n])
}
