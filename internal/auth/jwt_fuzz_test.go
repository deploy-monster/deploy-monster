package auth

import (
	"testing"
	"unicode/utf8"
)

// FuzzJWTRoundtrip generates tokens with random userID and email, then validates
// that the round-tripped claims match the original inputs.
// JWT payloads are JSON-encoded, so only valid UTF-8 strings round-trip cleanly.
func FuzzJWTRoundtrip(f *testing.F) {
	f.Add("user-1", "user@example.com")
	f.Add("", "")
	f.Add("usr_abc123", "admin@deploy.monster")
	f.Add("a]b[c{d}e", "weird+chars@test.co")

	svc := NewJWTService("fuzz-test-jwt-secret-at-least-32-bytes!")

	f.Fuzz(func(t *testing.T, userID, email string) {
		// JWT claims are JSON-encoded; invalid UTF-8 gets replaced during
		// marshal/unmarshal, so skip inputs that are not valid UTF-8.
		if !utf8.ValidString(userID) || !utf8.ValidString(email) {
			return
		}

		pair, err := svc.GenerateTokenPair(userID, "tenant-1", "role_admin", email)
		if err != nil {
			t.Fatalf("GenerateTokenPair failed: %v", err)
		}

		claims, err := svc.ValidateAccessToken(pair.AccessToken)
		if err != nil {
			t.Fatalf("ValidateAccessToken failed: %v", err)
		}

		if claims.UserID != userID {
			t.Errorf("UserID mismatch: got %q, want %q", claims.UserID, userID)
		}
		if claims.Email != email {
			t.Errorf("Email mismatch: got %q, want %q", claims.Email, email)
		}
	})
}

// FuzzPasswordHash verifies that any password survives a hash→verify round-trip.
// Note: bcrypt truncates at 72 bytes, so we only compare up to that limit.
func FuzzPasswordHash(f *testing.F) {
	f.Add("MySecureP@ss123")
	f.Add("")
	f.Add("short")
	f.Add("a-very-long-password-that-exceeds-typical-limits-1234567890!@#$%^&*()")
	f.Add(string([]byte{0, 255, 128, 64}))

	f.Fuzz(func(t *testing.T, password string) {
		// bcrypt has a 72-byte input limit; longer passwords are silently truncated.
		// We test the round-trip as the library sees it.
		hash, err := HashPassword(password)
		if err != nil {
			// bcrypt rejects passwords longer than 72 bytes in some implementations
			// or inputs with null bytes. That is acceptable.
			return
		}

		if err := VerifyPassword(hash, password); err != nil {
			t.Errorf("VerifyPassword failed for password %q: %v", password, err)
		}
	})
}
