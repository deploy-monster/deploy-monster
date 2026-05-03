package auth

import (
	"encoding/base32"
	"testing"
	"time"
)

func TestGenerateTOTP_RFC6238SHA1Vector(t *testing.T) {
	secret := []byte("12345678901234567890")

	got := generateTOTP(secret, 59, 30, 8)
	if got != "94287082" {
		t.Fatalf("generateTOTP() = %q, want %q", got, "94287082")
	}
}

func TestValidateTOTP_CurrentAndAdjacentWindows(t *testing.T) {
	secretBytes := []byte("12345678901234567890")
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)
	now := time.Now().Unix()

	tests := []struct {
		name  string
		token string
	}{
		{name: "current", token: generateTOTP(secretBytes, now, DefaultTOTPConfig.Period, 6)},
		{name: "previous", token: generateTOTP(secretBytes, now-int64(DefaultTOTPConfig.Period), DefaultTOTPConfig.Period, 6)},
		{name: "next", token: generateTOTP(secretBytes, now+int64(DefaultTOTPConfig.Period), DefaultTOTPConfig.Period, 6)},
		{name: "eight_digits", token: generateTOTP(secretBytes, now, DefaultTOTPConfig.Period, 8)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !ValidateTOTP(tt.token, secret) {
				t.Fatalf("ValidateTOTP(%q) = false, want true", tt.token)
			}
		})
	}
}

func TestValidateTOTP_RejectsInvalidToken(t *testing.T) {
	secretBytes := []byte("12345678901234567890")
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)

	if ValidateTOTP("000000", secret) {
		t.Fatal("ValidateTOTP accepted invalid token")
	}
}
