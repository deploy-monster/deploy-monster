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

	"golang.org/x/crypto/bcrypt"
)

// recoveryBcryptCost is intentionally lower than password bcryptCost:
// recovery codes are single-use and already have >64 bits of entropy
// (8 hex chars × 2 halves), so a cost of 10 is sufficient and keeps
// the login path responsive when MFA falls back to a recovery code.
const recoveryBcryptCost = 10

// TOTPConfig holds TOTP 2FA configuration returned to a newly
// enrolling user.
//
// Recovery holds the human-facing plaintext codes — the caller MUST
// display these exactly once and never persist them. RecoveryHashes
// holds the bcrypt hashes that should be stored server-side; each
// element is the hash of the corresponding Recovery entry.
type TOTPConfig struct {
	Secret         string   `json:"secret"`
	URL            string   `json:"url"` // otpauth:// URI for QR code
	Recovery       []string `json:"recovery"`
	RecoveryHashes []string `json:"-"` // bcrypt hashes for server-side storage
}

// GenerateTOTP creates a new TOTP secret and provisioning URI.
func GenerateTOTP(email, issuer string) (*TOTPConfig, error) {
	secret := generateTOTPSecret(20)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(secret))

	uri := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		issuer, email, encoded, issuer)

	// Generate 8 recovery codes plus their bcrypt hashes. The plaintext
	// list is handed to the caller once (for the user to print/save);
	// persistence must only ever keep the hashes.
	recovery := make([]string, 8)
	hashes := make([]string, 8)
	for i := range recovery {
		recovery[i] = fmt.Sprintf("%s-%s", randomHex(4), randomHex(4))
		h, err := HashRecoveryCode(recovery[i])
		if err != nil {
			return nil, fmt.Errorf("hash recovery code: %w", err)
		}
		hashes[i] = h
	}

	return &TOTPConfig{
		Secret:         encoded,
		URL:            uri,
		Recovery:       recovery,
		RecoveryHashes: hashes,
	}, nil
}

// HashRecoveryCode hashes a single recovery code with bcrypt. The code
// is normalized (lowercased, whitespace trimmed) before hashing so the
// verification path does not depend on how the user typed it in.
func HashRecoveryCode(code string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(normalizeRecoveryCode(code)), recoveryBcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash recovery code: %w", err)
	}
	return string(hash), nil
}

// VerifyRecoveryCode checks a user-entered code against a slice of
// stored hashes in constant time with respect to the correct hash.
// On success it returns (matchedIndex, true) — the caller is expected
// to delete or invalidate hashes[matchedIndex] so the code cannot be
// reused. Returns (-1, false) if no hash matches.
func VerifyRecoveryCode(hashes []string, code string) (int, bool) {
	norm := []byte(normalizeRecoveryCode(code))
	for i, h := range hashes {
		if h == "" {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(h), norm) == nil {
			return i, true
		}
	}
	return -1, false
}

func normalizeRecoveryCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
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
	_, _ = rand.Read(b)
	return string(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	const hex = "0123456789abcdef"
	out := make([]byte, n*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out[:n])
}
