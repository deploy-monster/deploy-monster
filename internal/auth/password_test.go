package auth

import (
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	password := "MySecureP@ss123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if hash == "" {
		t.Error("hash should not be empty")
	}
	if hash == password {
		t.Error("hash should not equal plaintext")
	}

	// Verify correct password
	if err := VerifyPassword(hash, password); err != nil {
		t.Errorf("VerifyPassword should succeed: %v", err)
	}

	// Verify wrong password
	if err := VerifyPassword(hash, "wrong-password"); err == nil {
		t.Error("VerifyPassword should fail for wrong password")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid", "MyPass123!@#", false},
		{"too short", "Ab1!", true},
		{"no uppercase", "mypass123!", true},
		{"no lowercase", "MYPASS123!", true},
		{"no digit", "MyPassword!", true},
		{"no special", "MyPassword123", true},
		{"minimum valid", "Abcdefgh1!@#", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tt.password, 8)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePasswordStrength(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePasswordStrength_DefaultMinLength(t *testing.T) {
	// When minLength is 0, should default to 12
	err := ValidatePasswordStrength("Short1A!", 0)
	if err == nil {
		t.Error("expected error for 8-char password when minLength defaults to 12")
	}

	err = ValidatePasswordStrength("LongEnuf1!@#", 0)
	if err != nil {
		t.Errorf("unexpected error for 12-char password with default minLength: %v", err)
	}
}

func TestHashPassword_DifferentPasswords(t *testing.T) {
	hash1, err := HashPassword("Password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	hash2, err := HashPassword("Password2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Different passwords should produce different hashes
	if hash1 == hash2 {
		t.Error("different passwords should produce different hashes")
	}
}

func TestHashPassword_SamePasswordDifferentHashes(t *testing.T) {
	// bcrypt generates different salts, so same password produces different hashes
	hash1, err := HashPassword("SamePass1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	hash2, err := HashPassword("SamePass1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Same password should produce different hashes (due to salt)
	if hash1 == hash2 {
		t.Error("same password should produce different hashes (bcrypt salting)")
	}

	// But both should verify against the original password
	if err := VerifyPassword(hash1, "SamePass1"); err != nil {
		t.Errorf("hash1 should verify: %v", err)
	}
	if err := VerifyPassword(hash2, "SamePass1"); err != nil {
		t.Errorf("hash2 should verify: %v", err)
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	// Empty passwords should still hash (bcrypt allows this)
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword empty: %v", err)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	// Verify with malformed hash should fail
	err := VerifyPassword("not-a-valid-hash", "anypassword")
	if err == nil {
		t.Error("expected error for invalid hash format")
	}
}
