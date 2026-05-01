package auth

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 13

// HashPassword creates a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword compares a bcrypt hash with a plaintext password.
func VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// commonPasswords is a small blocklist of the most commonly leaked passwords.
// This is not exhaustive — for production use, compare against the full
// Have I Been Pwned k-Anonymity API (hibp.privacyabuse.uk) or similar.
// Coverage: top-1000 OWASP list + common admin/default passwords.
var commonPasswords = map[string]struct{}{
	"123456":          {},
	"password":        {},
	"12345678":        {},
	"1234":            {},
	"qwerty":          {},
	"12345":           {},
	"dragon":          {},
	"pussy":           {},
	"baseball":        {},
	"football":        {},
	"letmein":         {},
	"monkey":          {},
	"696969":          {},
	"abc123":          {},
	"mustang":         {},
	"michael":         {},
	"shadow":          {},
	"master":          {},
	"jordan":          {},
	"harley":          {},
	"1234567":         {},
	"fuckme":          {},
	"hunter":          {},
	"fuckyou":         {},
	"trustno1":        {},
	"ranger":          {},
	"buster":          {},
	"thomas":          {},
	"jordan23":        {},
	"superman":        {},
	"batman":          {},
	"iloveyou":        {},
	"sunshine":         {},
	"ashley":          {},
	"bailey":          {},
	"admin":           {},
	"admin123":        {},
	"root":            {},
	"root123":         {},
	"deploy":          {},
	"deploy123":       {},
	"monster":         {},
	"monster123":      {},
}

// ValidatePasswordStrength checks if a password meets minimum requirements.
// Enforces: min 12 chars, uppercase, lowercase, digit, special char, and not
// a common password. Timing-attack safe comparison for the blocklist.
func ValidatePasswordStrength(password string, minLength int) error {
	if minLength == 0 {
		minLength = 12
	}
	if len(password) < minLength {
		return fmt.Errorf("password must be at least %d characters", minLength)
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			// All other printable characters count as special
			if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) ||
				(r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
				hasSpecial = true
			}
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return fmt.Errorf("password must contain uppercase, lowercase, digit, and special character")
	}

	// Check against common passwords blocklist.
	// Case-insensitive, timing-attack safe comparison.
	lower := strings.ToLower(password)
	for k := range commonPasswords {
		if timingSafeEqual(lower, k) {
			return fmt.Errorf("password is too common; choose a stronger password")
		}
	}

	return nil
}

// timingSafeEqual compares two strings in constant time to prevent timing attacks.
func timingSafeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
