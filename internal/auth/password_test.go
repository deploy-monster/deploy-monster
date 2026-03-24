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
		{"valid", "MyPass123", false},
		{"too short", "Ab1", true},
		{"no uppercase", "mypass123", true},
		{"no lowercase", "MYPASS123", true},
		{"no digit", "MyPassword", true},
		{"minimum valid", "Abcdefg1", false},
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
	// When minLength is 0, should default to 8
	err := ValidatePasswordStrength("Short1A", 0)
	if err == nil {
		t.Error("expected error for 7-char password when minLength defaults to 8")
	}

	err = ValidatePasswordStrength("LongEnuf1", 0)
	if err != nil {
		t.Errorf("unexpected error for 9-char password with default minLength: %v", err)
	}
}
