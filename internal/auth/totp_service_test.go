package auth

import (
	"context"
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"golang.org/x/crypto/bcrypt"
)

type totpServiceStore struct {
	core.Store
	user *core.User
}

func (s *totpServiceStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return s.user, nil
}

func (s *totpServiceStore) UpdateTOTPEnabled(_ context.Context, _ string, enabled bool, secret string) error {
	s.user.TOTPEnabled = enabled
	s.user.TOTPSecret = secret
	return nil
}

func (s *totpServiceStore) UpdateTOTPBackupCodes(_ context.Context, _ string, hashes []string) error {
	s.user.TOTPBackupCodes = append([]string{}, hashes...)
	return nil
}

type testTOTPVault struct{}

func (testTOTPVault) Encrypt(value string) (string, error) {
	return "enc:" + value, nil
}

func (testTOTPVault) Decrypt(value string) (string, error) {
	return strings.TrimPrefix(value, "enc:"), nil
}

func currentTOTPCode(t *testing.T, secret string) string {
	t.Helper()
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatalf("decode TOTP secret: %v", err)
	}
	return generateTOTP(secretBytes, time.Now().Unix(), DefaultTOTPConfig.Period, 6)
}

func TestTOTPService_EnrollRequiresConfirmationBeforeEnabled(t *testing.T) {
	store := &totpServiceStore{user: &core.User{ID: "u1", Email: "alice@example.com"}}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})

	uri, err := svc.Enroll(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if uri == "" {
		t.Fatal("expected provisioning URI")
	}
	if store.user.TOTPEnabled {
		t.Fatal("TOTP must not be enabled until a generated code is confirmed")
	}
	if store.user.TOTPSecret == "" {
		t.Fatal("expected pending encrypted TOTP secret")
	}

	secret := strings.TrimPrefix(store.user.TOTPSecret, "enc:")
	if err := svc.ConfirmEnrollment(context.Background(), "u1", currentTOTPCode(t, secret)); err != nil {
		t.Fatalf("ConfirmEnrollment: %v", err)
	}
	if !store.user.TOTPEnabled {
		t.Fatal("TOTP should be enabled after confirming a valid generated code")
	}
}

func TestTOTPService_ValidateConsumesBackupCode(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("ABCD1234"), 10)
	if err != nil {
		t.Fatalf("hash backup code: %v", err)
	}
	store := &totpServiceStore{
		user: &core.User{
			ID:              "u1",
			Email:           "alice@example.com",
			TOTPEnabled:     true,
			TOTPSecret:      "bad-ciphertext",
			TOTPBackupCodes: []string{string(hash)},
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})

	if !svc.ValidateContext(context.Background(), "u1", "ABCD1234") {
		t.Fatal("expected backup code to validate")
	}
	if len(store.user.TOTPBackupCodes) != 0 {
		t.Fatalf("backup code was not consumed: %d remaining", len(store.user.TOTPBackupCodes))
	}
	if svc.ValidateContext(context.Background(), "u1", "ABCD1234") {
		t.Fatal("backup code must be one-time use")
	}
}
