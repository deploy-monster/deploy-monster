package auth

import (
	"fmt"
	"unicode"

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

// ValidatePasswordStrength checks if a password meets minimum requirements.
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
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return fmt.Errorf("password must contain uppercase, lowercase, digit, and special character")
	}
	return nil
}
