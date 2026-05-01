package auth

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"hash"
	"net/url"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"golang.org/x/crypto/bcrypt"
)

// TOTPConfig holds TOTP configuration.
type TOTPConfig struct {
	Digits    int // 6 or 8
	Period    int // seconds, typically 30
	Algorithm string // "SHA1", "SHA256", "SHA512"
}

// DefaultTOTPConfig is the standard TOTP configuration (Google Authenticator compatible).
var DefaultTOTPConfig = TOTPConfig{
	Digits:    6,
	Period:    30,
	Algorithm: "SHA1",
}

// GenerateTOTPSecret generates a new TOTP secret for a user.
// Returns the secret (base32 encoded) and a provisioning URI for QR code.
func GenerateTOTPSecret(userID, email string) (secret string, provisioningURI string, err error) {
	// Generate 20 bytes of random entropy (160 bits, standard TOTP)
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate totp secret: %w", err)
	}

	// Base32 encode the secret (no padding for Google Authenticator compatibility)
	secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)

	// Build provisioning URI (RFC 6238)
	// otpauth://totp/Issuer:Email?secret=XXX&issuer=XXX&digits=6&period=30
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", "DeployMonster")
	params.Set("digits", fmt.Sprintf("%d", DefaultTOTPConfig.Digits))
	params.Set("period", fmt.Sprintf("%d", DefaultTOTPConfig.Period))
	params.Set("algorithm", DefaultTOTPConfig.Algorithm)

	issuer := "DeployMonster"
	accountName := email
	provisioningURI = fmt.Sprintf("otpauth://totp/%s:%s?%s",
		url.PathEscape(issuer), url.PathEscape(accountName), params.Encode())

	return secret, provisioningURI, nil
}

// ValidateTOTP validates a TOTP token against a secret.
// Supports compatibility with Google Authenticator, Authy, and other TOTP apps.
// Returns true if the token is valid within a window of ±1 period (for clock drift).
func ValidateTOTP(token string, secret string) bool {
	if len(token) != 6 && len(token) != 8 {
		return false
	}

	secret = strings.ToUpper(strings.TrimSpace(secret))
	secretBytes, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		// Try standard encoding with padding
		secretBytes, err = base32.StdEncoding.DecodeString(secret + "==")
		if err != nil {
			return false
		}
	}

	period := DefaultTOTPConfig.Period
	window := 1 // Allow ±1 period for clock drift

	now := time.Now().Unix()

	// Check current and adjacent periods
	for offset := -window; offset <= window; offset++ {
		expected := generateTOTP(secretBytes, now, period, 6)
		if constantTimeCompare(token, expected) {
			return true
		}
	}

	return false
}

// generateTOTP generates a TOTP value using HMAC-SHA1 (RFC 6238).
func generateTOTP(secret []byte, counter int64, period int, digits int) string {
	// Pack counter into 8 bytes (big-endian)
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, uint64(counter/ int64(period)))

	// HMAC-SHA1
	mac := hmacSHA1(secret, msg)

	// Dynamic truncation (RFC 4226)
	offset := mac[len(mac)-1] & 0x0F
	truncated := binary.BigEndian.Uint32(mac[offset:offset+4]) & 0x7FFFFFFF

	// Modulo 10^digits
	code := truncated % 1000000

	// Pad with leading zeros
	result := fmt.Sprintf("%0*d", digits, code)
	return result
}

// hmacSHA1 computes HMAC-SHA1.
func hmacSHA1(key, message []byte) []byte {
	var h hash.Hash
	h.Write(key)
	h.Write(message)
	return h.Sum(nil)
}

// constantTimeCompare compares two strings in constant time to prevent timing attacks.
func constantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

// GenerateBackupCodes generates a set of backup codes for account recovery.
// Returns 10 backup codes, each 8 characters. Each code is hashed with bcrypt
// for storage. The plain text codes are returned only once to the user.
const backupCodesCount = 10
const backupCodeLength = 8

// BackupCodes holds a set of hashed backup codes and the plain text versions (for display).
type BackupCodes struct {
	Hashes  []string // bcrypt hashes for storage
	Plain   []string // Plain text codes (show to user once only)
}

// GenerateBackupCodes generates a new set of backup codes.
func GenerateBackupCodes() (*BackupCodes, error) {
	codes := &BackupCodes{
		Hashes: make([]string, backupCodesCount),
		Plain:  make([]string, backupCodesCount),
	}

	for i := 0; i < backupCodesCount; i++ {
		// Generate 8 random bytes, encode as alphanumeric string
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generate backup code: %w", err)
		}

		// Use base32 (no padding) for alphanumeric codes
		plain := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)[:8]
		codes.Plain[i] = plain

		// Hash with bcrypt (cost 10 for backup codes - lower than passwords since they're high entropy)
		hash, err := bcrypt.GenerateFromPassword([]byte(plain), 10)
		if err != nil {
			return nil, fmt.Errorf("hash backup code: %w", err)
		}
		codes.Hashes[i] = string(hash)
	}

	return codes, nil
}

// VerifyBackupCode checks a plain text backup code against stored hashes.
// Returns the index of the matched code, or -1 if no match.
// Also invalidates the used code by replacing its hash.
func VerifyBackupCode(plain string, hashes []string) int {
	for i, hash := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil {
			return i
		}
	}
	return -1
}

// CanEnableTOTP checks if TOTP can be enabled for a user.
// Returns an error if TOTP is already enabled or if the user has other issues.
func CanEnableTOTP(store core.Store, userID string) error {
	user, err := store.GetUser(nil, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	if user.TOTPEnabled {
		return fmt.Errorf("TOTP is already enabled")
	}
	return nil
}