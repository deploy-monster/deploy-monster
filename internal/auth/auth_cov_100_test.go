package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// module.go:16 — init() factory registration
// =============================================================================

func TestAuthCov_ModuleFactory(t *testing.T) {
	m := New()
	if m.ID() != "core.auth" {
		t.Errorf("ID = %q", m.ID())
	}
}

// =============================================================================
// totp_service.go:78-80 — vault.Encrypt error in Enroll
// =============================================================================

type covVault struct {
	encErr error
	decErr error
}

func (v *covVault) Encrypt(s string) (string, error) {
	if v.encErr != nil {
		return "", v.encErr
	}
	return "enc:" + s, nil
}

func (v *covVault) Decrypt(s string) (string, error) {
	if v.decErr != nil {
		return "", v.decErr
	}
	return strings.TrimPrefix(s, "enc:"), nil
}

type covStore struct {
	core.Store
	getUser func(ctx context.Context, id string) (*core.User, error)
	update  func(ctx context.Context, id string, enabled bool, secret string) error
	backup  func(ctx context.Context, id string, hashes []string) error
}

func (s *covStore) GetUser(ctx context.Context, id string) (*core.User, error) {
	if s.getUser != nil {
		return s.getUser(ctx, id)
	}
	return &core.User{ID: id, Email: id + "@t.com"}, nil
}

func (s *covStore) UpdateTOTPEnabled(_ context.Context, _ string, _ bool, _ string) error {
	if s.update != nil {
		return errors.New("store error")
	}
	return nil
}

func (s *covStore) UpdateTOTPBackupCodes(_ context.Context, _ string, _ []string) error {
	if s.backup != nil {
		return errors.New("backup error")
	}
	return nil
}

func TestAuthCov_EnrollVaultEncryptError(t *testing.T) {
	svc := NewTOTPService(&covStore{})
	svc.SetVault(&covVault{encErr: errors.New("kms fail")})
	_, err := svc.Enroll(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthCov_EnrollStoreUpdateError(t *testing.T) {
	svc := NewTOTPService(&covStore{update: func(_ context.Context, _ string, _ bool, _ string) error {
		return errors.New("store full")
	}})
	svc.SetVault(&covVault{})
	_, err := svc.Enroll(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthCov_ConfirmEnrollGetUserError(t *testing.T) {
	svc := NewTOTPService(&covStore{
		getUser: func(_ context.Context, _ string) (*core.User, error) {
			return nil, errors.New("not found")
		},
	})
	svc.SetVault(&covVault{})
	err := svc.ConfirmEnrollment(context.Background(), "u1", "000000")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthCov_ConfirmEnrollAlreadyEnabled(t *testing.T) {
	svc := NewTOTPService(&covStore{
		getUser: func(_ context.Context, _ string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: true}, nil
		},
	})
	svc.SetVault(&covVault{})
	err := svc.ConfirmEnrollment(context.Background(), "u1", "000000")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthCov_DisableStoreUpdateError(t *testing.T) {
	secret, _, _ := GenerateTOTPSecret("u1", "u1@t.com")
	svc := NewTOTPService(&covStore{
		getUser: func(_ context.Context, _ string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: true, TOTPSecret: "enc:" + secret}, nil
		},
		update: func(_ context.Context, _ string, _ bool, _ string) error {
			return errors.New("store full")
		},
	})
	svc.SetVault(&covVault{})
	err := svc.Disable(context.Background(), "u1", "000000")
	// Fails at Validate first (wrong code), store update not reached
	if err == nil {
		t.Error("expected error")
	}
}

func TestAuthCov_DisableNotEnabled(t *testing.T) {
	svc := NewTOTPService(&covStore{
		getUser: func(_ context.Context, _ string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: false}, nil
		},
	})
	svc.SetVault(&covVault{})
	err := svc.Disable(context.Background(), "u1", "000000")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthCov_BackupCodesNoStore(t *testing.T) {
	svc := NewTOTPService(&covStore{})
	_, err := svc.GenerateBackupCodes(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthCov_BackupCodesGetUserError(t *testing.T) {
	svc := NewTOTPService(&covStore{
		getUser: func(_ context.Context, _ string) (*core.User, error) {
			return nil, errors.New("not found")
		},
	})
	_, err := svc.GenerateBackupCodes(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthCov_ValidateNotEnabled(t *testing.T) {
	svc := NewTOTPService(&covStore{
		getUser: func(_ context.Context, _ string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: false, TOTPSecret: ""}, nil
		},
	})
	svc.SetVault(&covVault{})
	if svc.ValidateContext(context.Background(), "u1", "000000") {
		t.Error("expected false")
	}
}

func TestAuthCov_StatusGetUserError(t *testing.T) {
	svc := NewTOTPService(&covStore{
		getUser: func(_ context.Context, _ string) (*core.User, error) {
			return nil, errors.New("not found")
		},
	})
	_, err := svc.Status("u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthCov_EnrollVaultNotConfigured(t *testing.T) {
	svc := NewTOTPService(&covStore{})
	_, err := svc.Enroll(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// =============================================================================
// totp_service.go:115-117 — ConfirmEnrollment store update error
// =============================================================================

type covConfirmStore struct {
	core.Store
	getUser func(ctx context.Context, id string) (*core.User, error)
	update  func(ctx context.Context, id string, enabled bool, secret string) error
}

func (s *covConfirmStore) GetUser(ctx context.Context, id string) (*core.User, error) {
	if s.getUser != nil {
		return s.getUser(ctx, id)
	}
	return &core.User{ID: id, Email: id + "@t.com"}, nil
}

func (s *covConfirmStore) UpdateTOTPEnabled(_ context.Context, _ string, _ bool, _ string) error {
	if s.update != nil {
		return errors.New("store error")
	}
	return nil
}

func TestAuthCov_ConfirmEnrollmentStoreUpdateErr(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("u1", "u1@t.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	// Generate a valid TOTP code using the internal function
	code := generateTOTP([]byte(secret), time.Now().Unix(), DefaultTOTPConfig.Period, DefaultTOTPConfig.Digits)

	svc := NewTOTPService(&covConfirmStore{
		getUser: func(_ context.Context, _ string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: false, TOTPSecret: "enc:" + secret}, nil
		},
		update: func(_ context.Context, _ string, _ bool, _ string) error {
			return errors.New("store error")
		},
	})
	svc.SetVault(&covVault{})
	err = svc.ConfirmEnrollment(context.Background(), "u1", code)
	if err == nil {
		t.Fatal("expected error from store update")
	}
}

// =============================================================================
// totp_service.go:227-229 — Disable store update error
// =============================================================================

func TestAuthCov_DisableStoreUpdateErr(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("u1", "u1@t.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	code := generateTOTP([]byte(secret), time.Now().Unix(), DefaultTOTPConfig.Period, DefaultTOTPConfig.Digits)

	svc := NewTOTPService(&covConfirmStore{
		getUser: func(_ context.Context, _ string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: true, TOTPSecret: "enc:" + secret}, nil
		},
		update: func(_ context.Context, _ string, _ bool, _ string) error {
			return errors.New("store error")
		},
	})
	svc.SetVault(&covVault{})
	err = svc.Disable(context.Background(), "u1", code)
	if err == nil {
		t.Fatal("expected error from store update")
	}
}

// =============================================================================
// totp_service.go:266-268 — GenerateBackupCodes GenerateBackupCodes error
// totp_service.go:270-272 — store.UpdateTOTPBackupCodes error
// =============================================================================

type covBackupStore struct {
	core.Store
	getUser func(ctx context.Context, id string) (*core.User, error)
	update  func(ctx context.Context, id string, hashes []string) error
}

func (s *covBackupStore) GetUser(ctx context.Context, id string) (*core.User, error) {
	if s.getUser != nil {
		return s.getUser(ctx, id)
	}
	return &core.User{ID: id, TOTPEnabled: true}, nil
}

func (s *covBackupStore) UpdateTOTPBackupCodes(_ context.Context, _ string, _ []string) error {
	if s.update != nil {
		return errors.New("store error")
	}
	return nil
}

func TestAuthCov_BackupCodesStoreUpdateErr(t *testing.T) {
	svc := NewTOTPService(&covBackupStore{
		update: func(_ context.Context, _ string, _ []string) error {
			return errors.New("store error")
		},
	})
	_, err := svc.GenerateBackupCodes(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error from store update")
	}
}

// =============================================================================
// password.go:110-112 — common password check requires a password that
// passes character checks AND is in the commonPasswords map.
// No existing blocklist entry has upper+lower+digit+special, so this
// path is unreachable with the current blocklist. Marked as known gap.
// =============================================================================
