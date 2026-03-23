package auth

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

func TestGenerateTOTP(t *testing.T) {
	cfg, err := GenerateTOTP("user@example.com", "DeployMonster")
	if err != nil {
		t.Fatalf("GenerateTOTP: %v", err)
	}

	if cfg.Secret == "" {
		t.Error("secret should not be empty")
	}
	if cfg.URL == "" {
		t.Error("URL should not be empty")
	}
	if len(cfg.Recovery) != 8 {
		t.Errorf("expected 8 recovery codes, got %d", len(cfg.Recovery))
	}

	// URL should be otpauth:// format
	if !strings.HasPrefix(cfg.URL, "otpauth://totp/") {
		t.Error("URL should start with otpauth://totp/")
	}

	// Secret should be valid base32
	_, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(cfg.Secret)
	if err != nil {
		t.Errorf("secret should be valid base32: %v", err)
	}
}

func TestValidateTOTP_SelfGenerated(t *testing.T) {
	cfg, _ := GenerateTOTP("test@test.com", "Test")

	// Decode secret and generate a code for current window
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(cfg.Secret))
	counter := time.Now().Unix() / 30
	code := generateCode(key, counter)

	if !ValidateTOTP(cfg.Secret, code) {
		t.Error("self-generated code should validate")
	}
}

func TestValidateTOTP_BadSecret(t *testing.T) {
	if ValidateTOTP("!!!invalid!!!", "123456") {
		t.Error("invalid base32 secret should fail")
	}
}

func TestRecoveryCodes_Unique(t *testing.T) {
	cfg, _ := GenerateTOTP("test@test.com", "Test")
	seen := make(map[string]bool)
	for _, code := range cfg.Recovery {
		if seen[code] {
			t.Errorf("duplicate recovery code: %s", code)
		}
		seen[code] = true
	}
}
