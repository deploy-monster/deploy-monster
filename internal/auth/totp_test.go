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

func TestRecoveryCodes_Hashed(t *testing.T) {
	cfg, err := GenerateTOTP("user@example.com", "Test")
	if err != nil {
		t.Fatalf("GenerateTOTP: %v", err)
	}

	if len(cfg.RecoveryHashes) != len(cfg.Recovery) {
		t.Fatalf("RecoveryHashes len = %d, want %d",
			len(cfg.RecoveryHashes), len(cfg.Recovery))
	}

	// Hashes must not equal the plaintext (that would defeat the point).
	for i, h := range cfg.RecoveryHashes {
		if h == cfg.Recovery[i] {
			t.Errorf("hash[%d] equals plaintext — not hashed", i)
		}
		if !strings.HasPrefix(h, "$2") {
			t.Errorf("hash[%d] does not look like bcrypt: %q", i, h)
		}
	}

	// Correct code verifies and returns its index.
	idx, ok := VerifyRecoveryCode(cfg.RecoveryHashes, cfg.Recovery[3])
	if !ok || idx != 3 {
		t.Errorf("VerifyRecoveryCode(correct) = %d,%v; want 3,true", idx, ok)
	}

	// Case / whitespace normalization.
	idx, ok = VerifyRecoveryCode(cfg.RecoveryHashes, "  "+strings.ToUpper(cfg.Recovery[0])+" ")
	if !ok || idx != 0 {
		t.Errorf("VerifyRecoveryCode(normalized) = %d,%v; want 0,true", idx, ok)
	}

	// A wrong code returns -1,false.
	idx, ok = VerifyRecoveryCode(cfg.RecoveryHashes, "aaaa-bbbb")
	if ok || idx != -1 {
		t.Errorf("VerifyRecoveryCode(wrong) = %d,%v; want -1,false", idx, ok)
	}

	// An empty hash slot is skipped (simulating a consumed code).
	hashes := append([]string(nil), cfg.RecoveryHashes...)
	hashes[3] = ""
	idx, ok = VerifyRecoveryCode(hashes, cfg.Recovery[3])
	if ok || idx != -1 {
		t.Errorf("VerifyRecoveryCode(consumed) = %d,%v; want -1,false", idx, ok)
	}
}

func TestHashRecoveryCode_Deterministic(t *testing.T) {
	// bcrypt is non-deterministic — two hashes of the same code should
	// differ, but both should verify.
	h1, err := HashRecoveryCode("abcd-1234")
	if err != nil {
		t.Fatalf("HashRecoveryCode: %v", err)
	}
	h2, err := HashRecoveryCode("abcd-1234")
	if err != nil {
		t.Fatalf("HashRecoveryCode: %v", err)
	}
	if h1 == h2 {
		t.Error("two bcrypt hashes of the same code should differ (random salt)")
	}
	if _, ok := VerifyRecoveryCode([]string{h1, h2}, "abcd-1234"); !ok {
		t.Error("both hashes should verify the same plaintext")
	}
}
