package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// JWTService.RevokeAllPreviousKeys
// ---------------------------------------------------------------------------

func TestJWTService_RevokeAllPreviousKeys(t *testing.T) {
	j := MustNewJWTService("primary-key-32-bytes-long-abcdefg")

	t.Run("returns 0 on empty ring", func(t *testing.T) {
		if got := j.RevokeAllPreviousKeys(); got != 0 {
			t.Fatalf("RevokeAllPreviousKeys() = %d, want 0", got)
		}
	})

	t.Run("clears all rotated keys and reports count", func(t *testing.T) {
		// Seed two previous keys via the public API so we exercise the same
		// code path AddPreviousKey uses.
		j.AddPreviousKey("first-rotated-key-32-bytes-long-1")
		j.AddPreviousKey("second-rotated-key-32-bytes-long-")
		if len(j.previousKeys) != 2 {
			t.Fatalf("setup failed: previousKeys=%d, want 2", len(j.previousKeys))
		}

		got := j.RevokeAllPreviousKeys()
		if got != 2 {
			t.Fatalf("RevokeAllPreviousKeys() = %d, want 2", got)
		}
		if len(j.previousKeys) != 0 {
			t.Fatalf("previousKeys not cleared: len=%d", len(j.previousKeys))
		}
		if len(j.previousAdded) != 0 {
			t.Fatalf("previousAdded not cleared: len=%d", len(j.previousAdded))
		}
	})

	t.Run("a token signed with a revoked key now fails validation", func(t *testing.T) {
		oldSecret := "rotated-key-32-bytes-long-1234567"
		newSecret := "primary-key-32-bytes-long-abcdefg"
		oldSvc := MustNewJWTService(oldSecret)
		pair, err := oldSvc.GenerateTokenPair("user-1", "tenant-1", "role-1", "u@example.com")
		if err != nil {
			t.Fatalf("GenerateTokenPair: %v", err)
		}

		newSvc := MustNewJWTService(newSecret)
		newSvc.AddPreviousKey(oldSecret)
		if _, err := newSvc.ValidateAccessToken(pair.AccessToken); err != nil {
			t.Fatalf("token must validate before revoke: %v", err)
		}

		count := newSvc.RevokeAllPreviousKeys()
		if count != 1 {
			t.Fatalf("RevokeAllPreviousKeys() = %d, want 1", count)
		}
		if _, err := newSvc.ValidateAccessToken(pair.AccessToken); err == nil {
			t.Fatal("expected validation failure after RevokeAllPreviousKeys")
		}
	})
}

// ---------------------------------------------------------------------------
// TOTPService — coverage for Validate / Disable / Status / error paths
// ---------------------------------------------------------------------------

// fakeStore is a richer mock than totpServiceStore: every method is a function
// field so tests can inject specific failures without writing whole types.
type fakeTOTPStore struct {
	core.Store
	getUser           func(ctx context.Context, id string) (*core.User, error)
	updateTOTPEnabled func(ctx context.Context, id string, enabled bool, secret string) error
	updateTOTPCalls   int
	lastEnabled       bool
	lastStoredSecret  string
}

func (f *fakeTOTPStore) GetUser(ctx context.Context, id string) (*core.User, error) {
	if f.getUser != nil {
		return f.getUser(ctx, id)
	}
	return nil, errors.New("getUser not configured")
}

func (f *fakeTOTPStore) UpdateTOTPEnabled(ctx context.Context, id string, enabled bool, secret string) error {
	f.updateTOTPCalls++
	f.lastEnabled = enabled
	f.lastStoredSecret = secret
	if f.updateTOTPEnabled != nil {
		return f.updateTOTPEnabled(ctx, id, enabled, secret)
	}
	return nil
}

type erroringVault struct{ decryptErr error }

func (erroringVault) Encrypt(value string) (string, error) { return "enc:" + value, nil }
func (v erroringVault) Decrypt(string) (string, error)     { return "", v.decryptErr }

func enrolledUserWithSecret(t *testing.T) (string, *core.User) {
	t.Helper()
	secret, _, err := GenerateTOTPSecret("u1", "alice@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	return secret, &core.User{
		ID:          "u1",
		Email:       "alice@example.com",
		TOTPEnabled: true,
		TOTPSecret:  "enc:" + secret,
	}
}

func TestTOTPService_Validate_FailsWithoutVault(t *testing.T) {
	svc := NewTOTPService(&fakeTOTPStore{})
	if svc.Validate("u1", "000000") {
		t.Fatal("Validate should refuse when vault is unset")
	}
	if svc.ValidateContext(context.Background(), "u1", "000000") {
		t.Fatal("ValidateContext should refuse when vault is unset")
	}
}

func TestTOTPService_Validate_GetUserFailureReturnsFalse(t *testing.T) {
	store := &fakeTOTPStore{
		getUser: func(context.Context, string) (*core.User, error) {
			return nil, errors.New("db down")
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})
	if svc.Validate("u1", "000000") {
		t.Fatal("Validate must be false when GetUser errors")
	}
}

func TestTOTPService_Validate_RejectsWhenTOTPDisabled(t *testing.T) {
	secret, _, _ := GenerateTOTPSecret("u1", "alice@example.com")
	store := &fakeTOTPStore{
		getUser: func(context.Context, string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: false, TOTPSecret: "enc:" + secret}, nil
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})
	if svc.Validate("u1", currentTOTPCode(t, secret)) {
		t.Fatal("Validate must be false when TOTPEnabled is false")
	}
}

func TestTOTPService_Validate_DecryptFailureReturnsFalse(t *testing.T) {
	store := &fakeTOTPStore{
		getUser: func(context.Context, string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: true, TOTPSecret: "ciphertext"}, nil
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(erroringVault{decryptErr: errors.New("kms unreachable")})
	if svc.Validate("u1", "000000") {
		t.Fatal("Validate must be false when decrypt fails")
	}
}

func TestTOTPService_Validate_HappyPathWithCorrectCode(t *testing.T) {
	secret, user := enrolledUserWithSecret(t)
	store := &fakeTOTPStore{
		getUser: func(context.Context, string) (*core.User, error) { return user, nil },
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})

	if !svc.Validate("u1", currentTOTPCode(t, secret)) {
		t.Fatal("Validate must accept current code")
	}
	if svc.Validate("u1", "000000") {
		t.Fatal("Validate must reject obviously-wrong code")
	}
}

func TestTOTPService_Disable_RequiresEnabledAndValidCode(t *testing.T) {
	secret, user := enrolledUserWithSecret(t)
	store := &fakeTOTPStore{
		getUser: func(context.Context, string) (*core.User, error) { return user, nil },
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})

	t.Run("rejects bad code", func(t *testing.T) {
		err := svc.Disable(context.Background(), "u1", "000000")
		if err == nil || !strings.Contains(err.Error(), "invalid TOTP code") {
			t.Fatalf("Disable wrong-code: err=%v, want 'invalid TOTP code'", err)
		}
		if store.updateTOTPCalls != 0 {
			t.Fatal("UpdateTOTPEnabled must not be called on bad code")
		}
	})

	t.Run("accepts current code and clears secret", func(t *testing.T) {
		store.updateTOTPCalls = 0
		if err := svc.Disable(context.Background(), "u1", currentTOTPCode(t, secret)); err != nil {
			t.Fatalf("Disable: %v", err)
		}
		if store.updateTOTPCalls != 1 {
			t.Fatalf("UpdateTOTPEnabled calls = %d, want 1", store.updateTOTPCalls)
		}
		if store.lastEnabled {
			t.Fatal("Disable must call UpdateTOTPEnabled with enabled=false")
		}
		if store.lastStoredSecret != "" {
			t.Fatal("Disable must clear the stored secret")
		}
	})

	t.Run("rejects when not enabled", func(t *testing.T) {
		disabledStore := &fakeTOTPStore{
			getUser: func(context.Context, string) (*core.User, error) {
				return &core.User{ID: "u1", TOTPEnabled: false}, nil
			},
		}
		disabledSvc := NewTOTPService(disabledStore)
		disabledSvc.SetVault(testTOTPVault{})
		err := disabledSvc.Disable(context.Background(), "u1", "000000")
		if err == nil || !strings.Contains(err.Error(), "not enabled") {
			t.Fatalf("Disable when off: err=%v, want 'not enabled'", err)
		}
	})

	t.Run("propagates GetUser error", func(t *testing.T) {
		errStore := &fakeTOTPStore{
			getUser: func(context.Context, string) (*core.User, error) {
				return nil, errors.New("db down")
			},
		}
		errSvc := NewTOTPService(errStore)
		errSvc.SetVault(testTOTPVault{})
		err := errSvc.Disable(context.Background(), "u1", "000000")
		if err == nil || !strings.Contains(err.Error(), "get user") {
			t.Fatalf("Disable GetUser-error: err=%v, want wrapped 'get user'", err)
		}
	})
}

func TestTOTPService_Status(t *testing.T) {
	t.Run("reports enabled flag from store", func(t *testing.T) {
		_, user := enrolledUserWithSecret(t)
		store := &fakeTOTPStore{
			getUser: func(context.Context, string) (*core.User, error) { return user, nil },
		}
		svc := NewTOTPService(store)
		enabled, err := svc.Status("u1")
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if !enabled {
			t.Fatal("Status must mirror user.TOTPEnabled")
		}
	})

	t.Run("StatusContext returns false on store error", func(t *testing.T) {
		store := &fakeTOTPStore{
			getUser: func(context.Context, string) (*core.User, error) {
				return nil, errors.New("db down")
			},
		}
		svc := NewTOTPService(store)
		enabled, err := svc.StatusContext(context.Background(), "u1")
		if err == nil || !strings.Contains(err.Error(), "get user") {
			t.Fatalf("StatusContext: err=%v, want wrapped 'get user'", err)
		}
		if enabled {
			t.Fatal("StatusContext must report false on error")
		}
	})
}

func TestTOTPService_Enroll_ErrorPaths(t *testing.T) {
	t.Run("vault not configured", func(t *testing.T) {
		svc := NewTOTPService(&fakeTOTPStore{})
		_, err := svc.Enroll(context.Background(), "u1")
		if err == nil || !strings.Contains(err.Error(), "vault not configured") {
			t.Fatalf("Enroll without vault: err=%v", err)
		}
	})

	t.Run("rejects when TOTP already enabled", func(t *testing.T) {
		_, user := enrolledUserWithSecret(t)
		store := &fakeTOTPStore{
			getUser: func(context.Context, string) (*core.User, error) { return user, nil },
		}
		svc := NewTOTPService(store)
		svc.SetVault(testTOTPVault{})
		_, err := svc.Enroll(context.Background(), "u1")
		if err == nil || !strings.Contains(err.Error(), "already enabled") {
			t.Fatalf("Enroll already-enabled: err=%v", err)
		}
	})

	t.Run("propagates GetUser error", func(t *testing.T) {
		store := &fakeTOTPStore{
			getUser: func(context.Context, string) (*core.User, error) {
				return nil, errors.New("db down")
			},
		}
		svc := NewTOTPService(store)
		svc.SetVault(testTOTPVault{})
		_, err := svc.Enroll(context.Background(), "u1")
		if err == nil || !strings.Contains(err.Error(), "get user") {
			t.Fatalf("Enroll GetUser-error: err=%v", err)
		}
	})
}

func TestTOTPService_ConfirmEnrollment_ErrorPaths(t *testing.T) {
	t.Run("rejects when already enabled", func(t *testing.T) {
		_, user := enrolledUserWithSecret(t)
		store := &fakeTOTPStore{
			getUser: func(context.Context, string) (*core.User, error) { return user, nil },
		}
		svc := NewTOTPService(store)
		svc.SetVault(testTOTPVault{})
		err := svc.ConfirmEnrollment(context.Background(), "u1", "000000")
		if err == nil || !strings.Contains(err.Error(), "already enabled") {
			t.Fatalf("ConfirmEnrollment already-enabled: err=%v", err)
		}
	})

	t.Run("rejects when no pending secret", func(t *testing.T) {
		store := &fakeTOTPStore{
			getUser: func(context.Context, string) (*core.User, error) {
				return &core.User{ID: "u1", TOTPEnabled: false, TOTPSecret: ""}, nil
			},
		}
		svc := NewTOTPService(store)
		svc.SetVault(testTOTPVault{})
		err := svc.ConfirmEnrollment(context.Background(), "u1", "000000")
		if err == nil || !strings.Contains(err.Error(), "not been started") {
			t.Fatalf("ConfirmEnrollment no-pending: err=%v", err)
		}
	})

	t.Run("rejects invalid code", func(t *testing.T) {
		secret, _, _ := GenerateTOTPSecret("u1", "alice@example.com")
		store := &fakeTOTPStore{
			getUser: func(context.Context, string) (*core.User, error) {
				return &core.User{ID: "u1", TOTPEnabled: false, TOTPSecret: "enc:" + secret}, nil
			},
		}
		svc := NewTOTPService(store)
		svc.SetVault(testTOTPVault{})
		err := svc.ConfirmEnrollment(context.Background(), "u1", "000000")
		if err == nil || !strings.Contains(err.Error(), "invalid TOTP code") {
			t.Fatalf("ConfirmEnrollment bad-code: err=%v", err)
		}
	})
}

func TestTOTPService_GenerateBackupCodes(t *testing.T) {
	svc := NewTOTPService(&fakeTOTPStore{})
	codes, err := svc.GenerateBackupCodes(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}
	if codes == nil {
		t.Fatal("expected non-nil BackupCodes")
	}
	if len(codes.Plain) == 0 {
		t.Fatal("expected at least one plain code")
	}
	if len(codes.Plain) != len(codes.Hashes) {
		t.Fatalf("plain/hashed length mismatch: %d vs %d", len(codes.Plain), len(codes.Hashes))
	}
	for i, c := range codes.Plain {
		if strings.TrimSpace(c) == "" {
			t.Fatalf("code %d is blank", i)
		}
	}
}

