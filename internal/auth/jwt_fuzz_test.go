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

// FuzzValidateAccessTokenUntrusted feeds arbitrary strings to the access
// token validator to ensure it never panics and never leaks a non-nil
// claims pointer alongside an error. Random bytes effectively cannot forge
// an HMAC, so the acceptance path is unreachable — the real invariant is
// no-panic + clean error semantics on the rejection path.
func FuzzValidateAccessTokenUntrusted(f *testing.F) {
	svc := NewJWTService("fuzz-test-jwt-secret-at-least-32-bytes!")

	f.Add("")
	f.Add("not.a.jwt")
	f.Add("a.b.c")
	f.Add("eyJhbGciOiJub25lIn0..")                  // alg=none attack shape
	f.Add("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..") // header only
	f.Add("..")
	f.Add("a")

	f.Fuzz(func(t *testing.T, token string) {
		claims, err := svc.ValidateAccessToken(token)
		if err != nil {
			// Rejection path: claims must be nil so no caller can mistake
			// a garbage input for a valid session.
			if claims != nil {
				t.Errorf("error returned but claims non-nil: %v", claims)
			}
			return
		}
		// Acceptance path — not reachable in practice for random input
		// because the HMAC cannot be forged. If we ever land here, the
		// claims struct must be non-nil (the acceptance contract).
		if claims == nil {
			t.Error("ValidateAccessToken returned nil claims with nil error")
		}
	})
}

// FuzzValidateRefreshTokenUntrusted mirrors the access-token fuzzer for the
// refresh-token path. Same invariant: no panic, and on rejection the claims
// pointer is nil.
func FuzzValidateRefreshTokenUntrusted(f *testing.F) {
	svc := NewJWTService("fuzz-test-jwt-secret-at-least-32-bytes!")

	f.Add("")
	f.Add("garbage")
	f.Add("a.b.c")
	f.Add("..")

	f.Fuzz(func(t *testing.T, token string) {
		claims, err := svc.ValidateRefreshToken(token)
		if err != nil {
			if claims != nil {
				t.Errorf("error returned but claims non-nil: %v", claims)
			}
			return
		}
		if claims == nil {
			t.Error("ValidateRefreshToken returned nil claims with nil error")
		}
	})
}
