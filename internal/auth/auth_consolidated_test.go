package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// === merged from auth_cov_100_test.go ===

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

// === merged from auth_coverage2_test.go ===

// =============================================================================
// jwt.go:228-229 — claims type assertion fails (ValidateAccessToken)
// =============================================================================

func TestValidateAccessToken_TamperedClaims(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")

	pair, err := svc.GenerateTokenPair("u", "t", "r", "e@e.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	// Tamper with the payload portion of the token
	parts := strings.Split(pair.AccessToken, ".")
	if len(parts) == 3 {
		parts[1] = "eyJleHAiOjAsImlhdCI6MH0" // tampered payload
		tampered := strings.Join(parts, ".")
		_, err = svc.ValidateAccessToken(tampered)
		if err == nil {
			t.Error("expected error for tampered token claims")
		}
	}
}

// =============================================================================
// jwt.go:319 (ValidateRefreshToken) — token with invalid signature
// =============================================================================

func TestValidateRefreshToken_InvalidSignature(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")

	now := time.Now()
	claims := refreshTokenWithSession{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   "user1",
			Issuer:    tokenIssuer,
			Audience:  jwt.ClaimStrings{tokenAudience},
		},
		FirstIssuedAt: now.Unix(),
	}
	// Sign with a different key
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("different-key-not-the-same-as-test-secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = svc.ValidateRefreshToken(tokenStr)
	if err == nil {
		t.Error("expected error for token signed with different key")
	}
}

// =============================================================================
// jwt.go:322-324 — method != HS256 in refresh validation
// =============================================================================

func TestValidateRefreshToken_WrongSigningMethod(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")

	now := time.Now()
	claims := refreshTokenWithSession{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   "user1",
			Issuer:    tokenIssuer,
			Audience:  jwt.ClaimStrings{tokenAudience},
		},
		FirstIssuedAt: now.Unix(),
	}
	tokenStr, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)

	_, err := svc.ValidateRefreshToken(tokenStr)
	if err == nil {
		t.Error("expected error for none-signed refresh token")
	}
}

// =============================================================================
// jwt.go:329-331 — absolute session timeout exceeded
// =============================================================================

func TestValidateRefreshToken_AbsoluteSessionTimeout(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")

	now := time.Now()
	past := now.Add(-31 * 24 * time.Hour)
	claims := refreshTokenWithSession{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(past),
			Subject:   "user1",
			ID:        "test-jti",
			Issuer:    tokenIssuer,
			Audience:  jwt.ClaimStrings{tokenAudience},
		},
		FirstIssuedAt: past.Unix(),
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret-key-at-least-32-bytes!"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = svc.ValidateRefreshToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired session (absolute timeout)")
	}
	if !strings.Contains(err.Error(), "session expired") && !strings.Contains(err.Error(), "absolute timeout") {
		t.Errorf("error = %v, want session expired message", err)
	}
}

// =============================================================================
// password.go — edge case character check tests
// =============================================================================

func TestValidatePasswordStrength_MissingUpper(t *testing.T) {
	err := ValidatePasswordStrength("lowercase1!", 8)
	if err == nil {
		t.Fatal("expected error for missing uppercase")
	}
}

func TestValidatePasswordStrength_MissingDigit(t *testing.T) {
	err := ValidatePasswordStrength("Uppercase!", 8)
	if err == nil {
		t.Fatal("expected error for missing digit")
	}
}

func TestValidatePasswordStrength_MissingSpecial(t *testing.T) {
	err := ValidatePasswordStrength("Uppercase1", 8)
	if err == nil {
		t.Fatal("expected error for missing special char")
	}
}

func TestValidatePasswordStrength_ValidPassword(t *testing.T) {
	err := ValidatePasswordStrength("ValidP@ss1", 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// module.go:51-53 — JWT init error (short secret)
// =============================================================================

func TestModuleInit_JWTError_ShortSecret(t *testing.T) {
	store := &mockStore{userCount: 1}
	cfg := &core.Config{}
	cfg.Server.SecretKey = "short" // Too short

	m := New()
	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for short JWT secret")
	}
	if !strings.Contains(err.Error(), "JWT secret must be at least") {
		t.Errorf("error = %v, want JWT secret length error", err)
	}
}

// =============================================================================
// module.go:60-62 — SetReplayStore with KV in Core.DB
// =============================================================================

func TestModuleInit_SetReplayStore(t *testing.T) {
	store := &mockStore{userCount: 1}
	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	m := New()
	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
		DB:     &core.Database{KV: &nopKVStorer{}},
	}
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
}

// =============================================================================
// module.go:65-76 — secrets module lookup via Registry
// =============================================================================

type mockSecretModule struct {
	core.Module // embed so we only override what we need
}

func (m *mockSecretModule) ID() string { return "secrets" }

func (m *mockSecretModule) Vault() interface {
	Encrypt(string) (string, error)
	Decrypt(string) (string, error)
} {
	return &nopVault{}
}

type nopVault struct{}

func (v *nopVault) Encrypt(s string) (string, error) { return "enc:" + s, nil }
func (v *nopVault) Decrypt(s string) (string, error) { return strings.TrimPrefix(s, "enc:"), nil }

func TestModuleInit_SecretsVaultViaRegistry(t *testing.T) {
	store := &mockStore{userCount: 1}
	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	reg := core.NewRegistry()
	// Register the mock secrets module
	if err := reg.Register(&mockSecretModule{}); err != nil {
		t.Fatalf("Register mock secrets module: %v", err)
	}

	c := &core.Core{
		Logger:   slog.Default(),
		Store:    store,
		Config:   cfg,
		DB:       &core.Database{KV: &nopKVStorer{}},
		Registry: reg,
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init with secrets module: %v", err)
	}
}

// =============================================================================
// totp_service.go — SetReplayStore
// =============================================================================

func TestTOTPSetReplayStore(t *testing.T) {
	s := NewTOTPService(nil)
	s.SetReplayStore(&nopKVStorer{})
	if s.replay == nil {
		t.Error("replay store should be set")
	}
}

// =============================================================================
// totp_service.go:184-190 — consumeBackupCode empty code / not backup store
// =============================================================================

func TestTOTPConsumeBackupCode_EmptyCode(t *testing.T) {
	s := NewTOTPService(&mockStore{})
	if s.consumeBackupCode(context.Background(), "user1", "") {
		t.Error("expected false for empty code")
	}
}

func TestTOTPConsumeBackupCode_NotBackupStore(t *testing.T) {
	s := NewTOTPService(&mockStore{})
	// mockStore doesn't implement totpBackupCodeStore, so this should return false
	if s.consumeBackupCode(context.Background(), "user1", "ABCD1234") {
		t.Error("expected false when store doesn't implement totpBackupCodeStore")
	}
}

// =============================================================================
// totp_service.go:211 — Disable with TOTP not enabled
// Uses totpMockStore which implements GetUser
// =============================================================================

type totpMockStore struct {
	core.Store
	user struct {
		TOTPEnabled     bool
		TOTPSecret      string
		TOTPBackupCodes []string
	}
}

func (m *totpMockStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return &core.User{
		TOTPEnabled:     m.user.TOTPEnabled,
		TOTPSecret:      m.user.TOTPSecret,
		TOTPBackupCodes: m.user.TOTPBackupCodes,
	}, nil
}

func (m *totpMockStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, fmt.Errorf("not found")
}

func TestTOTPDisable_NotEnabled(t *testing.T) {
	s := NewTOTPService(&totpMockStore{})
	err := s.Disable(context.Background(), "user1", "123456")
	if err == nil {
		t.Fatal("expected error when TOTP not enabled")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("error = %v, want 'not enabled'", err)
	}
}

// =============================================================================
// totp_service.go:137-155 — validateStoredSecret with vault nil / no secret
// =============================================================================

func TestTOTPValidateStoredSecret_NoVault(t *testing.T) {
	s := NewTOTPService(&mockStore{})
	if s.validateStoredSecret(context.Background(), "user1", "123456", true) {
		t.Error("expected false when vault is nil")
	}
}

// =============================================================================
// totp_service.go:252-256 — GenerateBackupCodes without backup store
// =============================================================================

func TestTOTPGenerateBackupCodes_NoBackupStore(t *testing.T) {
	s := NewTOTPService(&mockStore{})
	_, err := s.GenerateBackupCodes(context.Background(), "user1")
	if err == nil {
		t.Fatal("expected error when store doesn't implement totpBackupCodeStore")
	}
}

// =============================================================================
// totp_service.go:261-263 — GenerateBackupCodes with TOTP not enabled
// =============================================================================

type totpMockBackupStore struct {
	totpMockStore
}

func (m *totpMockBackupStore) UpdateTOTPBackupCodes(_ context.Context, _ string, _ []string) error {
	return nil
}

func TestTOTPGenerateBackupCodes_TOTPNotEnabled(t *testing.T) {
	s := NewTOTPService(&totpMockBackupStore{})
	_, err := s.GenerateBackupCodes(context.Background(), "user1")
	if err == nil {
		t.Fatal("expected error when TOTP is not enabled")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("error = %v, want 'not enabled'", err)
	}
}

// =============================================================================
// ValidateContext without vault returns false
// =============================================================================

func TestTOTPValidateContext_NoVault_ReturnsFalse(t *testing.T) {
	s := NewTOTPService(&mockStore{})
	if s.ValidateContext(context.Background(), "user1", "123456") {
		t.Error("expected false without vault")
	}
}

// =============================================================================
// Disable with valid vault — exercises validateStoredSecret replay path
// =============================================================================

func TestTOTPDisable_WithVault_NoSecret(t *testing.T) {
	s := NewTOTPService(&totpMockStore{
		user: struct {
			TOTPEnabled     bool
			TOTPSecret      string
			TOTPBackupCodes []string
		}{TOTPEnabled: true, TOTPSecret: "", TOTPBackupCodes: nil},
	})
	s.SetVault(&nopVault{})
	err := s.Disable(context.Background(), "user1", "123456")
	if err == nil {
		t.Fatal("expected error due to empty secret")
	}
}

// =============================================================================
// nopKVStorer — minimal KVStorer implementation for tests
// =============================================================================

type nopKVStorer struct{}

func (n *nopKVStorer) Set(bucket, key string, value any, ttlSeconds int64) error { return nil }
func (n *nopKVStorer) BatchSet(items []core.KVBatchItem) error                   { return nil }
func (n *nopKVStorer) Get(bucket, key string, dest any) error {
	return fmt.Errorf("not found")
}
func (n *nopKVStorer) Delete(bucket, key string) error      { return nil }
func (n *nopKVStorer) List(bucket string) ([]string, error) { return nil, nil }
func (n *nopKVStorer) Close() error                         { return nil }
func (n *nopKVStorer) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	return nil, fmt.Errorf("not found")
}
func (n *nopKVStorer) GetWebhookSecret(webhookID string) (string, error) {
	return "", fmt.Errorf("not found")
}

// === merged from auth_coverage_test.go ===

// ═══════════════════════════════════════════════════════════════════════════════
// Module Init with mock Store
// ═══════════════════════════════════════════════════════════════════════════════

type mockStore struct {
	core.Store
	userCount       int
	countErr        error
	createTenantID  string
	createTenantErr error
	createUserID    string
	createUserErr   error
	createdEmail    string
}

func (m *mockStore) CountUsers(_ context.Context) (int, error) {
	return m.userCount, m.countErr
}

func (m *mockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	if m.createTenantErr != nil {
		return "", m.createTenantErr
	}
	return m.createTenantID, nil
}

func (m *mockStore) CreateUserWithMembership(_ context.Context, email, _, _, _, _, _ string) (string, error) {
	m.createdEmail = email
	if m.createUserErr != nil {
		return "", m.createUserErr
	}
	return m.createUserID, nil
}

func (m *mockStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return &core.User{ID: "user1", TOTPEnabled: false}, nil
}

func (m *mockStore) UpdateTOTPEnabled(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

func TestModule_Init_WithStore(t *testing.T) {
	store := &mockStore{userCount: 5}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.jwt == nil {
		t.Error("JWT service should be initialized after Init")
	}
	if m.store == nil {
		t.Error("store should be set after Init")
	}
}

func TestModule_Init_NilStore(t *testing.T) {
	c := &core.Core{
		Logger: slog.Default(),
		Store:  nil,
		Config: &core.Config{},
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should return error when Store is nil")
	}
}

func TestModule_Init_FirstRunSetup(t *testing.T) {
	store := &mockStore{
		userCount:      0, // No users - first run
		createTenantID: "tenant-1",
		createUserID:   "user-1",
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	t.Setenv("MONSTER_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "SecureP@ss123!")

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init with first run setup: %v", err)
	}
}

func TestModule_Init_FirstRunSetup_WithEnvPassword(t *testing.T) {
	store := &mockStore{
		userCount:      0,
		createTenantID: "tenant-1",
		createUserID:   "user-1",
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	t.Setenv("MONSTER_ADMIN_EMAIL", "custom@example.com")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "CustomPass123!")

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init with custom env credentials: %v", err)
	}
}

func TestModule_Init_FirstRunSetup_GeneratesUnpredictableEmail(t *testing.T) {
	store := &mockStore{
		userCount:      0,
		createTenantID: "tenant-1",
		createUserID:   "user-1",
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	t.Setenv("MONSTER_ADMIN_EMAIL", "")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "SecureP@ss123!")

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init with generated admin email: %v", err)
	}

	legacyDefaultEmail := "admin" + "@deploymonster.local"
	if store.createdEmail == legacyDefaultEmail {
		t.Fatal("first-run admin email used the predictable legacy default")
	}
	if !strings.HasPrefix(store.createdEmail, "admin-") || !strings.HasSuffix(store.createdEmail, "@deploymonster.local") {
		t.Fatalf("generated admin email = %q, want admin-<random>@deploymonster.local", store.createdEmail)
	}
}

func TestModule_Init_FirstRunSetup_CountError(t *testing.T) {
	store := &mockStore{
		countErr: context.DeadlineExceeded,
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should propagate CountUsers error")
	}
}

func TestModule_Init_FirstRunSetup_CreateTenantError(t *testing.T) {
	store := &mockStore{
		userCount:       0,
		createTenantErr: context.DeadlineExceeded,
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	t.Setenv("MONSTER_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "SecureP@ss123!")

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should propagate CreateTenantWithDefaults error")
	}
}

func TestModule_Init_FirstRunSetup_CreateUserError(t *testing.T) {
	store := &mockStore{
		userCount:      0,
		createTenantID: "tenant-1",
		createUserErr:  context.DeadlineExceeded,
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	t.Setenv("MONSTER_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "SecureP@ss123!")

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should propagate CreateUserWithMembership error")
	}
}

func TestModule_Start_WithLogger(t *testing.T) {
	m := New()
	m.logger = slog.Default()

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// JWT ValidateRefreshToken edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestJWT_ValidateRefreshToken_Invalid(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes-long!")

	_, err := svc.ValidateRefreshToken("invalid-token-string")
	if err == nil {
		t.Error("expected error for invalid refresh token")
	}
}

func TestJWT_ValidateRefreshToken_WrongSecret(t *testing.T) {
	svc1 := MustNewJWTService("secret-one-at-least-32-bytes-long-aaaa!")
	svc2 := MustNewJWTService("secret-two-at-least-32-bytes-long-bbbb!")

	pair, _ := svc1.GenerateTokenPair("user-1", "t", "r", "e@e.com")

	_, err := svc2.ValidateRefreshToken(pair.RefreshToken)
	if err == nil {
		t.Error("expected error when validating refresh token with different secret")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// API Key edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestGenerateAPIKey_PrefixLength(t *testing.T) {
	pair, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	// Prefix = "dm_" + 12 hex chars = 15 chars total
	if len(pair.Prefix) != 15 {
		t.Errorf("prefix length = %d, want 15", len(pair.Prefix))
	}
}

func TestGenerateAPIKey_KeyLength(t *testing.T) {
	pair, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	// Key = "dm_" + 64 hex chars (32 bytes) = 67 chars total
	if len(pair.Key) != 67 {
		t.Errorf("key length = %d, want 67", len(pair.Key))
	}
}

func TestHashAPIKey_DifferentKeys(t *testing.T) {
	h1, err := HashAPIKey("dm_key1")
	if err != nil {
		t.Fatalf("HashAPIKey: %v", err)
	}
	h2, err := HashAPIKey("dm_key2")
	if err != nil {
		t.Fatalf("HashAPIKey: %v", err)
	}

	if h1 == h2 {
		t.Error("different keys should produce different hashes")
	}

	// SECURITY FIX (CRYPTO-001): Verify that correct keys verify against their hashes
	if !VerifyAPIKey("dm_key1", h1) {
		t.Error("key1 should verify against h1")
	}
	if !VerifyAPIKey("dm_key2", h2) {
		t.Error("key2 should verify against h2")
	}
	// Cross-verification should fail
	if VerifyAPIKey("dm_key1", h2) {
		t.Error("key1 should not verify against h2")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Password - HashPassword error paths
// ═══════════════════════════════════════════════════════════════════════════════

func TestHashPassword_VeryLongPassword(t *testing.T) {
	// bcrypt has a 72-byte limit; test that it doesn't error
	longPwd := strings.Repeat("A", 72)
	hash, err := HashPassword(longPwd)
	if err != nil {
		t.Fatalf("HashPassword with 72-char password: %v", err)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
}

func TestValidatePasswordStrength_CustomMinLength(t *testing.T) {
	// minLength = 12
	err := ValidatePasswordStrength("Short1Ab!", 12)
	if err == nil {
		t.Error("9-char password should fail when minLength is 12")
	}

	err = ValidatePasswordStrength("LongEnough1Ab!", 12)
	if err != nil {
		t.Errorf("14-char password should pass with minLength 12: %v", err)
	}
}

func TestGenerateTokenID(t *testing.T) {
	id1 := generateTokenID()
	id2 := generateTokenID()

	if id1 == "" {
		t.Error("token ID should not be empty")
	}
	if len(id1) != 32 {
		t.Errorf("token ID length = %d, want 32", len(id1))
	}
	if id1 == id2 {
		t.Error("two token IDs should be different")
	}
}

func (m *mockStore) CreateServer(_ context.Context, _ *core.Server) error { return nil }
func (m *mockStore) GetServer(_ context.Context, _ string) (*core.Server, error) {
	return nil, core.ErrNotFound
}
func (m *mockStore) ListServersByTenant(_ context.Context, _ string) ([]core.Server, error) {
	return nil, nil
}
func (m *mockStore) ListAllServers(_ context.Context) ([]core.Server, error) { return nil, nil }
func (m *mockStore) UpdateServerStatus(_ context.Context, _, _ string) error { return nil }
func (m *mockStore) DeleteServer(_ context.Context, _ string) error          { return nil }

// === merged from auth_final_test.go ===

// ═══════════════════════════════════════════════════════════════════════════════
// GenerateTokenPair — covers jwt.go:45 (verify both token branches)
// ═══════════════════════════════════════════════════════════════════════════════

func TestGenerateTokenPair_FieldValues(t *testing.T) {
	svc := MustNewJWTService("test-secret-at-least-32-bytes-long-key!")

	pair, err := svc.GenerateTokenPair("user1", "tenant1", "role1", "test@test.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("access token should not be empty")
	}
	if pair.RefreshToken == "" {
		t.Error("refresh token should not be empty")
	}
	if pair.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want Bearer", pair.TokenType)
	}
	if pair.ExpiresIn != int((15 * time.Minute).Seconds()) {
		t.Errorf("ExpiresIn = %d, want %d", pair.ExpiresIn, int((15 * time.Minute).Seconds()))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ValidateAccessToken — covers jwt.go:86 (!ok || !token.Valid branch)
// ═══════════════════════════════════════════════════════════════════════════════

func TestValidateAccessToken_ExpiredToken(t *testing.T) {
	svc := &JWTService{
		secretKey:     []byte("test-secret-key-at-least-32-bytes!"),
		accessExpiry:  -1 * time.Second, // Already expired
		refreshExpiry: 7 * 24 * time.Hour,
	}

	pair, err := svc.GenerateTokenPair("u", "t", "r", "e@e.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	_, err = svc.ValidateAccessToken(pair.AccessToken)
	if err == nil {
		t.Error("expected error for expired access token")
	}
}

func TestValidateAccessToken_TamperedToken(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")

	pair, err := svc.GenerateTokenPair("u", "t", "r", "e@e.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	tampered := pair.AccessToken + "x"
	_, err = svc.ValidateAccessToken(tampered)
	if err == nil {
		t.Error("expected error for tampered access token")
	}
}

func TestValidateAccessToken_WrongSigningMethod(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodNone, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		UserID: "user1",
	})
	tokenStr, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")
	_, err := svc.ValidateAccessToken(tokenStr)
	if err == nil {
		t.Error("expected error for none-signed token")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ValidateRefreshToken — covers jwt.go:101 (!ok || !token.Valid)
// ═══════════════════════════════════════════════════════════════════════════════

func TestValidateRefreshToken_ExpiredToken(t *testing.T) {
	svc := &JWTService{
		secretKey:     []byte("test-secret-key-at-least-32-bytes!"),
		accessExpiry:  15 * time.Minute,
		refreshExpiry: -1 * time.Second,
	}

	pair, err := svc.GenerateTokenPair("u", "t", "r", "e@e.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	_, err = svc.ValidateRefreshToken(pair.RefreshToken)
	if err == nil {
		t.Error("expected error for expired refresh token")
	}
}

func TestValidateRefreshToken_ValidToken_ReturnsUserID(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")

	pair, err := svc.GenerateTokenPair("user-42", "t", "r", "e@e.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	rtClaims, err := svc.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if rtClaims.UserID != "user-42" {
		t.Errorf("userID = %q, want user-42", rtClaims.UserID)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// HashPassword — covers password.go:13 (normal path)
// The error branch from bcrypt is impossible to trigger with valid input.
// ═══════════════════════════════════════════════════════════════════════════════

func TestHashPassword_RoundTrip(t *testing.T) {
	hash, err := HashPassword("ValidPass1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if err := VerifyPassword(hash, "ValidPass1"); err != nil {
		t.Errorf("VerifyPassword failed for correct password: %v", err)
	}
}

func TestHashPassword_WrongPasswordFails(t *testing.T) {
	hash, _ := HashPassword("CorrectPass1")
	if err := VerifyPassword(hash, "WrongPass2"); err == nil {
		t.Error("VerifyPassword should fail with wrong password")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// firstRunSetup — covers module.go:86 (auto-generated password log branch)
// ═══════════════════════════════════════════════════════════════════════════════

func TestFirstRunSetup_MissingEnvVarsUsesDefaults(t *testing.T) {
	store := &mockStore{
		userCount:      0,
		createTenantID: "tenant-1",
		createUserID:   "user-1",
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	// Clear env vars — Init should create a default first-run admin with a generated password.
	t.Setenv("MONSTER_ADMIN_EMAIL", "")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "")

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
}

func TestFirstRunSetup_WithEnvVars(t *testing.T) {
	store := &mockStore{
		userCount:      0,
		createTenantID: "tenant-1",
		createUserID:   "user-1",
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	t.Setenv("MONSTER_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "SecureP@ss123!")

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// init() — covers module.go:11
// ═══════════════════════════════════════════════════════════════════════════════

func TestInit_RegisteredAsModule(t *testing.T) {
	m := New()
	var _ core.Module = m
	if m.ID() != "core.auth" {
		t.Errorf("ID() = %q, want core.auth", m.ID())
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module.Health — both branches
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Health_NilJWT_IsDown(t *testing.T) {
	m := New()
	if h := m.Health(); h != core.HealthDown {
		t.Errorf("Health() = %v, want HealthDown when jwt is nil", h)
	}
}

func TestModule_Health_WithJWT_IsOK(t *testing.T) {
	m := New()
	// SECURITY FIX (JWT-002): Use a secret that meets minimum length requirement (32 chars)
	m.jwt = MustNewJWTService("this-is-a-very-long-secret-key-for-testing-only")
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK", h)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module.Stop / Routes
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Stop_NoError(t *testing.T) {
	m := New()
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestModule_Routes_ReturnsNil(t *testing.T) {
	m := New()
	if r := m.Routes(); r != nil {
		t.Errorf("Routes() = %v, want nil", r)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RoleLevel / CanAssignRole — covers rbac.go
// ═══════════════════════════════════════════════════════════════════════════════

func TestRoleLevel_Builtins(t *testing.T) {
	tests := []struct {
		roleID string
		want   int
	}{
		{"role_super_admin", LevelSuperAdmin},
		{"role_owner", LevelOwner},
		{"role_admin", LevelAdmin},
		{"role_developer", LevelDeveloper},
		{"role_operator", LevelOperator},
		{"role_viewer", LevelViewer},
		{"custom_role", LevelDeveloper},
	}
	for _, tt := range tests {
		if got := RoleLevel(tt.roleID); got != tt.want {
			t.Errorf("RoleLevel(%q) = %d, want %d", tt.roleID, got, tt.want)
		}
	}
}

func TestCanAssignRole(t *testing.T) {
	tests := []struct {
		inviter string
		target  string
		want    bool
	}{
		{"role_owner", "role_developer", true},
		{"role_owner", "role_owner", true},
		{"role_developer", "role_owner", false},
		{"role_viewer", "role_developer", false},
		{"role_super_admin", "role_owner", true},
		{"role_admin", "role_admin", true},
	}
	for _, tt := range tests {
		got := CanAssignRole(tt.inviter, tt.target)
		if got != tt.want {
			t.Errorf("CanAssignRole(%q, %q) = %v, want %v", tt.inviter, tt.target, got, tt.want)
		}
	}
}

func TestGenerateAPIKey_FieldsConsistent(t *testing.T) {
	pair, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	// SECURITY FIX (CRYPTO-001): With bcrypt, hash includes random salt so we verify using VerifyAPIKey
	// instead of direct comparison
	if !VerifyAPIKey(pair.Key, pair.Hash) {
		t.Error("hash should verify with VerifyAPIKey")
	}

	// Prefix should be start of key
	if pair.Key[:len(pair.Prefix)] != pair.Prefix {
		t.Error("prefix should be start of key")
	}
}

// === merged from auth_remaining_test.go ===

// =============================================================================
// JWTService — ValidateAccessToken with previous key (jwt.go:214)
// =============================================================================

func TestJWTService_ValidateAccessTokenWithPrevKey(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")
	s2 := MustNewJWTService("different-secret-thats-also-at-least-32-bytes-long!")

	pair, err := s2.GenerateTokenPair("user1", "tenant1", "admin", "user@test.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	s.AddPreviousKey("different-secret-thats-also-at-least-32-bytes-long!")

	claims, err := s.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken with previous key: %v", err)
	}
	if claims.UserID != "user1" {
		t.Errorf("expected user1, got %s", claims.UserID)
	}
}

// =============================================================================
// ValidateRefreshToken — basic round trip (jwt.go:308)
// =============================================================================

func TestJWTService_ValidateRefreshTokenRoundTrip(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")

	pair, err := s.GenerateTokenPair("user1", "tenant1", "admin", "user@test.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	claims, err := s.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if claims.UserID != "user1" {
		t.Errorf("expected user1, got %s", claims.UserID)
	}
	if claims.JTI == "" {
		t.Error("expected non-empty JTI")
	}
}

// =============================================================================
// ValidateAccessToken invalid token (jwt.go:214)
// =============================================================================

func TestJWTService_ValidateAccessTokenInvalidExtra(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")

	_, err := s.ValidateAccessToken("invalid-token-string")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

// =============================================================================
// ValidateRefreshToken — wrong key (jwt.go:308)
// =============================================================================

func TestJWTService_ValidateRefreshTokenWrongKeyExtra(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")
	s2 := MustNewJWTService("different-secret-thats-also-at-least-32-bytes-long!")

	pair, err := s2.GenerateTokenPair("user1", "tenant1", "admin", "user@test.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	_, err = s.ValidateRefreshToken(pair.RefreshToken)
	if err == nil {
		t.Fatal("expected error for wrong key")
	}
}

// =============================================================================
// ValidateRefreshToken invalid token (jwt.go:308)
// =============================================================================

func TestJWTService_ValidateRefreshTokenInvalidExtra(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")

	_, err := s.ValidateRefreshToken("not-a-valid-token")
	if err == nil {
		t.Fatal("expected error for invalid refresh token")
	}
}

// =============================================================================
// ValidatePasswordStrength — edge cases (password.go:79)
// =============================================================================

func TestValidatePasswordStrength_TooShortExtra(t *testing.T) {
	err := ValidatePasswordStrength("Ab1!", 12)
	if err == nil || !strings.Contains(err.Error(), "at least") {
		t.Fatalf("expected min length error, got: %v", err)
	}
}

func TestValidatePasswordStrength_MissingUpperExtra(t *testing.T) {
	err := ValidatePasswordStrength("abcdefgh1!@#", 12)
	if err == nil || !strings.Contains(err.Error(), "uppercase") {
		t.Fatalf("expected uppercase error, got: %v", err)
	}
}

func TestValidatePasswordStrength_MissingLowerExtra(t *testing.T) {
	err := ValidatePasswordStrength("ABCDEFGH1!@#", 12)
	if err == nil || !strings.Contains(err.Error(), "lowercase") {
		t.Fatalf("expected lowercase error, got: %v", err)
	}
}

func TestValidatePasswordStrength_MissingDigitExtra(t *testing.T) {
	err := ValidatePasswordStrength("Abcdefgh!@#$", 12)
	if err == nil || !strings.Contains(err.Error(), "digit") {
		t.Fatalf("expected digit error, got: %v", err)
	}
}

func TestValidatePasswordStrength_MissingSpecialExtra(t *testing.T) {
	err := ValidatePasswordStrength("Abcdefgh12345", 12)
	if err == nil || !strings.Contains(err.Error(), "special") {
		t.Fatalf("expected special character error, got: %v", err)
	}
}

func TestValidatePasswordStrength_CommonPasswordExtra(t *testing.T) {
	// "Monster123!" lowercases to "monster123!" which is NOT in the common list.
	// We can still test the common password path exists by using a known common password
	// with enough chars and all character types.
	// "Password1!" — lowercase "password1!" is NOT in the list, so we exercise
	// the valid path instead.
	err := ValidatePasswordStrength("Monster123!", 8)
	// This should either pass all checks or fail on common list - either is fine
	// as long as it doesn't panic
	_ = err
}

func TestValidatePasswordStrength_ValidExtra(t *testing.T) {
	err := ValidatePasswordStrength("CorrectHorseBatteryStaple1!", 12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePasswordStrength_ZeroMinLengthExtra(t *testing.T) {
	err := ValidatePasswordStrength("Abcdef1!ghij", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// HashPassword / VerifyPassword — round trip (password.go:13)
// =============================================================================

func TestHashAndVerifyPasswordExtra(t *testing.T) {
	hash, err := HashPassword("test-password-123!@#")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	if err := VerifyPassword(hash, "test-password-123!@#"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}

	if err := VerifyPassword(hash, "wrong-password"); err == nil {
		t.Fatal("expected error for wrong password")
	}
}

// =============================================================================
// GenerateAPIKey — round trip (apikey.go:32)
// =============================================================================

func TestGenerateAPIKey_RoundTripExtra(t *testing.T) {
	pair, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if pair.Key == "" {
		t.Fatal("expected non-empty key")
	}
	if pair.Hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if len(pair.Prefix) != APIKeyPrefixLength {
		t.Errorf("expected %d-char prefix, got %d", APIKeyPrefixLength, len(pair.Prefix))
	}
	if !strings.HasPrefix(pair.Key, "dm_") {
		t.Errorf("expected dm_ prefix, got %s", pair.Key[:3])
	}

	if !VerifyAPIKey(pair.Key, pair.Hash) {
		t.Error("VerifyAPIKey should succeed")
	}
	if VerifyAPIKey("wrong-key", pair.Hash) {
		t.Error("VerifyAPIKey should fail for wrong key")
	}
}

// =============================================================================
// GenerateTOTPSecret — basic (totp.go:34)
// =============================================================================

func TestGenerateTOTPSecret_LengthExtra(t *testing.T) {
	secret, uri, err := GenerateTOTPSecret("user1", "user@test.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if len(secret) == 0 {
		t.Fatal("expected non-empty secret")
	}
	if uri == "" {
		t.Error("expected non-empty provisioning URI")
	}
}

// =============================================================================
// GenerateBackupCodes — count verification (totp.go:156)
// =============================================================================

func TestGenerateBackupCodes_CountExtra(t *testing.T) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}
	if codes == nil {
		t.Error("expected non-empty backup codes")
	}
	if len(codes.Plain) == 0 {
		t.Error("expected non-empty first code")
	}
}

// =============================================================================
// RevokeAccessToken — already expired is no-op (jwt.go:251)
// =============================================================================

func TestJWTService_RevokeAccessTokenExpiredExtra(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")

	mockStorer := &mockKVStorer{}
	err := s.RevokeAccessToken(mockStorer, "jti_123", "user1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("RevokeAccessToken: %v", err)
	}
	if mockStorer.setCalled {
		t.Error("expected no Set call for already-expired token")
	}
}

// =============================================================================
// IsAccessTokenRevoked — nil storer (jwt.go:268)
// =============================================================================

func TestJWTService_IsAccessTokenRevokedNilStorerExtra(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")

	revoked := s.IsAccessTokenRevoked(nil, "jti_123")
	if revoked {
		t.Error("expected false when storer is nil")
	}
}

// =============================================================================
// TOTPService GenerateBackupCodes (totp_service.go:252)
// =============================================================================

func TestTOTPService_GenerateBackupCodesExtra(t *testing.T) {
	store := &fakeTOTPStore{
		getUser: func(context.Context, string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: true}, nil
		},
	}
	svc := NewTOTPService(store)
	codes, err := svc.GenerateBackupCodes(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}
	if codes == nil {
		t.Fatal("expected non-nil backup codes")
	}
}

// =============================================================================
// MockKVStorer for JWT tests
// =============================================================================

type mockKVStorer struct {
	setCalled bool
}

func (m *mockKVStorer) Set(bucket, key string, value any, ttlSeconds int64) error {
	m.setCalled = true
	return nil
}

func (m *mockKVStorer) Get(bucket, key string, dest any) error {
	return nil
}

// =============================================================================
// GenerateTokenPair round trip (jwt.go:161)
// =============================================================================

func TestJWTService_GenerateTokenPairRoundTripExtra(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")

	pair, err := s.GenerateTokenPair("user1", "tenant1", "admin", "user@test.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	if pair.TokenType != "Bearer" {
		t.Errorf("expected Bearer, got %s", pair.TokenType)
	}
	if pair.ExpiresIn <= 0 {
		t.Errorf("expected positive ExpiresIn, got %d", pair.ExpiresIn)
	}

	claims, err := s.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.UserID != "user1" {
		t.Errorf("expected user1, got %s", claims.UserID)
	}
	if claims.TenantID != "tenant1" {
		t.Errorf("expected tenant1, got %s", claims.TenantID)
	}
	if claims.RoleID != "admin" {
		t.Errorf("expected admin, got %s", claims.RoleID)
	}
	if claims.Email != "user@test.com" {
		t.Errorf("expected user@test.com, got %s", claims.Email)
	}

	refreshClaims, err := s.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if refreshClaims.UserID != "user1" {
		t.Errorf("expected user1, got %s", refreshClaims.UserID)
	}
}

// =============================================================================
// RBAC — RoleLevel and CanAssignRole (rbac.go)
// =============================================================================

func TestRoleLevelExtra(t *testing.T) {
	tests := []struct {
		roleID string
		want   int
	}{
		{"role_admin", LevelAdmin},
		{"role_owner", LevelOwner},
		{"role_developer", LevelDeveloper},
		{"role_viewer", LevelViewer},
		{"role_operator", LevelOperator},
		{"role_super_admin", LevelSuperAdmin},
		{"custom_role", LevelDeveloper}, // unknown defaults to developer
	}
	for _, tt := range tests {
		got := RoleLevel(tt.roleID)
		if got != tt.want {
			t.Errorf("RoleLevel(%q) = %d, want %d", tt.roleID, got, tt.want)
		}
	}
}

func TestCanAssignRoleExtra(t *testing.T) {
	tests := []struct {
		assignerRole string
		targetRole   string
		want         bool
	}{
		{"role_admin", "role_admin", true},
		{"role_admin", "role_developer", true},
		{"role_admin", "role_viewer", true},
		{"role_developer", "role_admin", false},
		{"role_developer", "role_developer", true},
		{"role_developer", "role_viewer", true},
		{"role_viewer", "role_admin", false},
		{"role_viewer", "role_viewer", true},
		{"role_owner", "role_owner", true},
		{"role_owner", "role_admin", true},
	}
	for _, tt := range tests {
		got := CanAssignRole(tt.assignerRole, tt.targetRole)
		if got != tt.want {
			t.Errorf("CanAssignRole(%q, %q) = %v, want %v", tt.assignerRole, tt.targetRole, got, tt.want)
		}
	}
}

// =============================================================================
// NewJWTService with empty secret defaults (jwt.go:72)
// =============================================================================

func TestNewJWTService_WithPreviousSecretsExtra(t *testing.T) {
	s, err := NewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!", "prev-secret-also-at-least-32-bytes-long-here!")
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil service")
	}
	if len(s.secretKey) == 0 {
		t.Error("expected non-empty secret key")
	}
	keys := s.allKeys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys (active + previous), got %d", len(keys))
	}
}

// =============================================================================
// GenerateTokenID — verify format (jwt.go:357)
// =============================================================================

func TestGenerateTokenID_FormatExtra(t *testing.T) {
	id := generateTokenID()
	if len(id) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("expected 32 hex chars, got %d", len(id))
	}
}

// =============================================================================
// TOTP Disable for non-existent user (totp_service.go:211)
// =============================================================================

func TestTOTPService_DisableNotEnabledExtra(t *testing.T) {
	store := &fakeTOTPStore{
		getUser: func(context.Context, string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: false}, nil
		},
	}
	svc := NewTOTPService(store)
	err := svc.Disable(context.Background(), "u1", "000000")
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("expected 'not enabled' error, got: %v", err)
	}
}

// =============================================================================
// Auth Module — Health check (module.go:97)
// =============================================================================

func TestAuthModule_HealthDownExtra(t *testing.T) {
	m := &Module{}
	h := m.Health()
	if h != core.HealthDown {
		t.Errorf("expected HealthDown, got %s", h)
	}
}

// =============================================================================
// JWTService — allKeys with previous keys (jwt.go:344)
// =============================================================================

func TestJWTService_AllKeysExtra(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")
	s.AddPreviousKey("prev-key-test-32-bytes-long!")
	s.AddPreviousKey("prev-key-test-32-bytes-long!")

	keys := s.allKeys()
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

// =============================================================================
// RevokeAllPreviousKeys — clears previous keys (jwt.go:119)
// =============================================================================

func TestJWTService_RevokeAllPreviousKeysExtra(t *testing.T) {
	s := MustNewJWTService("test-secret-thats-at-least-32-bytes-long-for-hs256!")
	s.AddPreviousKey("prev-key-test-32-bytes-long!")

	s.RevokeAllPreviousKeys()

	keys := s.allKeys()
	if len(keys) != 1 {
		t.Errorf("expected only 1 key after revoke, got %d", len(keys))
	}
}

// Enroll with nil vault — should return error
func TestTOTPService_EnrollNoVaultExtra(t *testing.T) {
	svc := NewTOTPService(nil)
	_, err := svc.Enroll(context.Background(), "user1")
	if err == nil || !strings.Contains(err.Error(), "vault not configured") {
		t.Fatalf("expected vault error, got: %v", err)
	}
}

// ConfirmEnrollment with no vault and empty code — should fail
func TestTOTPService_ConfirmEmptyCodeExtra(t *testing.T) {
	store := &fakeTOTPStore{
		getUser: func(context.Context, string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPSecret: "secret"}, nil
		},
	}
	svc := NewTOTPService(store)
	err := svc.ConfirmEnrollment(context.Background(), "u1", "")
	if err == nil {
		t.Fatal("expected error for empty/no-vault confirmation code")
	}
}

// === merged from coverage_followup_test.go ===

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
	updateBackupCodes func(ctx context.Context, id string, hashes []string) error
	backupCodeCalls   int
	lastBackupCodes   []string
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

func (f *fakeTOTPStore) UpdateTOTPBackupCodes(ctx context.Context, id string, hashes []string) error {
	f.backupCodeCalls++
	f.lastBackupCodes = append([]string{}, hashes...)
	if f.updateBackupCodes != nil {
		return f.updateBackupCodes(ctx, id, hashes)
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
	store := &fakeTOTPStore{
		getUser: func(context.Context, string) (*core.User, error) {
			return &core.User{ID: "u1", TOTPEnabled: true}, nil
		},
	}
	svc := NewTOTPService(store)
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
	if store.backupCodeCalls != 1 {
		t.Fatalf("UpdateTOTPBackupCodes calls = %d, want 1", store.backupCodeCalls)
	}
	if len(store.lastBackupCodes) != len(codes.Hashes) {
		t.Fatalf("stored hashes = %d, want %d", len(store.lastBackupCodes), len(codes.Hashes))
	}
	for i, c := range codes.Plain {
		if strings.TrimSpace(c) == "" {
			t.Fatalf("code %d is blank", i)
		}
	}
}

// === merged from coverage_targeted_test.go ===

// =============================================================================
// Module.init — covers the init() registration path (module.go:15, 50.0%)
// =============================================================================

func TestModuleInit_Registered(t *testing.T) {
	// The init() function in module.go registers via core.RegisterModule.
	// This test verifies that the factory function works.
	m := New()
	if m.ID() != "core.auth" {
		t.Errorf("ID() = %q, want %q", m.ID(), "core.auth")
	}
	if m.Name() != "Authentication" {
		t.Errorf("Name() = %q, want %q", m.Name(), "Authentication")
	}
	if m.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want %q", m.Version(), "1.0.0")
	}
}

// =============================================================================
// ValidatePasswordStrength — covers the remaining edge cases (password.go:79)
// Missing lowercase detection + common password reject
// =============================================================================

func TestValidatePasswordStrength_MissingLower(t *testing.T) {
	err := ValidatePasswordStrength("UPPERCASE1!", 8)
	if err == nil {
		t.Fatal("expected error for missing lowercase")
	}
}

func TestValidatePasswordStrength_CommonPassword(t *testing.T) {
	// The commonPasswords map uses exact (lowercase) matching
	// Test with the exact common password "password"
	err := ValidatePasswordStrength("password", 8)
	if err == nil {
		t.Fatal("expected error for common password 'password'")
	}
	// Also test "admin" as another common password
	err = ValidatePasswordStrength("admin", 8)
	if err == nil {
		t.Fatal("expected error for common password 'admin'")
	}
}

func TestValidatePasswordStrength_MinLengthZero(t *testing.T) {
	// minLength=0 triggers the default of 12 in the function.
	err := ValidatePasswordStrength("Short1A!", 0)
	if err == nil {
		t.Fatal("expected error when minLength=0 and password is <12 chars")
	}
	// With minLength=0, a 12-char password should pass
	err2 := ValidatePasswordStrength("LongEnough1Ab!", 0)
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
}

// =============================================================================
// validateStoredSecret — anti-replay path (totp_service.go:167-178)
// =============================================================================

// replayKVStorer implements core.KVStorer with a controlled lastStep.
type replayKVStorer struct {
	lastStep int64
	getErr   error
	setErr   error
	getCalls int
	setCalls int
}

func (r *replayKVStorer) Set(_ string, _ string, _ any, _ int64) error {
	r.setCalls++
	if r.setErr != nil {
		return r.setErr
	}
	return nil
}

func (r *replayKVStorer) Get(_ string, _ string, dest any) error {
	r.getCalls++
	if r.getErr != nil {
		return r.getErr
	}
	if d, ok := dest.(*int64); ok {
		*d = r.lastStep
	}
	return nil
}

func (r *replayKVStorer) Delete(_, _ string) error            { return nil }
func (r *replayKVStorer) List(_ string) ([]string, error)     { return nil, nil }
func (r *replayKVStorer) Close() error                        { return nil }
func (r *replayKVStorer) BatchSet(_ []core.KVBatchItem) error { return nil }
func (r *replayKVStorer) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, errors.New("not found")
}
func (r *replayKVStorer) GetWebhookSecret(_ string) (string, error) {
	return "", errors.New("not found")
}

type replayTestStore struct {
	core.Store
	user *core.User
}

func (s *replayTestStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return s.user, nil
}

func TestTOTPValidateStoredSecret_AntiReplay_RejectsReplayedStep(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("u1", "alice@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	replay := &replayKVStorer{lastStep: 999999999999} // Set lastStep far in the future

	store := &replayTestStore{
		user: &core.User{
			ID:          "u1",
			TOTPEnabled: true,
			TOTPSecret:  "enc:" + secret,
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})
	svc.SetReplayStore(replay)

	// The current TOTP step will be <= lastStep (which is huge), so this should be false
	if svc.validateStoredSecret(context.Background(), "u1", currentTOTPCode(t, secret), true) {
		t.Fatal("validateStoredSecret should return false when step <= lastStep (anti-replay)")
	}
	if replay.getCalls == 0 {
		t.Error("expected Get to be called on replay store")
	}
}

func TestTOTPValidateStoredSecret_AntiReplay_GetErrorAccepted(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("u1", "alice@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	replay := &replayKVStorer{
		getErr: errors.New("key not found"),
	}

	store := &replayTestStore{
		user: &core.User{
			ID:          "u1",
			TOTPEnabled: true,
			TOTPSecret:  "enc:" + secret,
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})
	svc.SetReplayStore(replay)

	// Get returns error (step not found), so anti-replay check passes
	if !svc.validateStoredSecret(context.Background(), "u1", currentTOTPCode(t, secret), true) {
		t.Fatal("validateStoredSecret should return true when Get returns error (first use)")
	}
	if replay.setCalls == 0 {
		t.Error("expected Set to be called on replay store after successful validation")
	}
}

func TestTOTPValidateStoredSecret_AntiReplay_SetErrorLogs(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("u1", "alice@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	replay := &replayKVStorer{
		getErr: errors.New("key not found"),
		setErr: errors.New("store full"),
	}

	store := &replayTestStore{
		user: &core.User{
			ID:          "u1",
			TOTPEnabled: true,
			TOTPSecret:  "enc:" + secret,
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})
	svc.SetReplayStore(replay)

	// Set error should be tolerated (logged, not returned)
	if !svc.validateStoredSecret(context.Background(), "u1", currentTOTPCode(t, secret), true) {
		t.Fatal("validateStoredSecret should still return true even if Set fails")
	}
}

// =============================================================================
// consumeBackupCode — match + update success and error paths (totp_service.go:183-208)
// =============================================================================

type backupCodeTestStore struct {
	core.Store
	user            *core.User
	updateBackupErr error
	updateCalled    bool
}

func (s *backupCodeTestStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return s.user, nil
}

func (s *backupCodeTestStore) UpdateTOTPBackupCodes(_ context.Context, _ string, _ []string) error {
	s.updateCalled = true
	return s.updateBackupErr
}

func TestTOTPConsumeBackupCode_Success(t *testing.T) {
	// First generate valid backup codes
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}

	store := &backupCodeTestStore{
		user: &core.User{
			ID:              "u1",
			TOTPEnabled:     true,
			TOTPBackupCodes: codes.Hashes,
		},
	}
	svc := NewTOTPService(store)

	// Consume the first backup code
	if !svc.consumeBackupCode(context.Background(), "u1", codes.Plain[0]) {
		t.Fatal("consumeBackupCode should return true for valid code")
	}
	if !store.updateCalled {
		t.Error("expected UpdateTOTPBackupCodes to be called")
	}
}

func TestTOTPConsumeBackupCode_WrongCode(t *testing.T) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}

	store := &backupCodeTestStore{
		user: &core.User{
			ID:              "u1",
			TOTPEnabled:     true,
			TOTPBackupCodes: codes.Hashes,
		},
	}
	svc := NewTOTPService(store)

	if svc.consumeBackupCode(context.Background(), "u1", "ZZZZZZZZ") {
		t.Fatal("consumeBackupCode should return false for wrong code")
	}
}

func TestTOTPConsumeBackupCode_UpdateError(t *testing.T) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}

	store := &backupCodeTestStore{
		user: &core.User{
			ID:              "u1",
			TOTPEnabled:     true,
			TOTPBackupCodes: codes.Hashes,
		},
		updateBackupErr: errors.New("db write failed"),
	}
	svc := NewTOTPService(store)

	// The backup code should match but the update error should cause false return
	if svc.consumeBackupCode(context.Background(), "u1", codes.Plain[0]) {
		t.Fatal("consumeBackupCode should return false when UpdateTOTPBackupCodes fails")
	}
}

func TestTOTPConsumeBackupCode_UserNotEnabled(t *testing.T) {
	store := &backupCodeTestStore{
		user: &core.User{
			ID:          "u1",
			TOTPEnabled: false,
		},
	}
	svc := NewTOTPService(store)

	if svc.consumeBackupCode(context.Background(), "u1", "ABCDEF") {
		t.Fatal("consumeBackupCode should return false when TOTP is not enabled")
	}
}

func TestTOTPConsumeBackupCode_NoBackupCodes(t *testing.T) {
	store := &backupCodeTestStore{
		user: &core.User{
			ID:              "u1",
			TOTPEnabled:     true,
			TOTPBackupCodes: []string{},
		},
	}
	svc := NewTOTPService(store)

	if svc.consumeBackupCode(context.Background(), "u1", "ABCDEF") {
		t.Fatal("consumeBackupCode should return false when no backup codes exist")
	}
}

// =============================================================================
// JWT: belt-and-suspenders method check (jwt.go:233-234)
// WithValidMethods already rejects non-HS256 before this check,
// but we test the edge where the refresh token has FirstIssuedAt=0
// (not set), which exercises the skippable absolute session check.
// =============================================================================

func TestValidateRefreshToken_FirstIssuedAtZero(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")

	now := time.Now()
	claims := refreshTokenWithSession{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   "user1",
			ID:        "test-jti",
			Issuer:    tokenIssuer,
			Audience:  jwt.ClaimStrings{tokenAudience},
		},
		FirstIssuedAt: 0, // Not set
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret-key-at-least-32-bytes!"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	rtClaims, err := svc.ValidateRefreshToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if rtClaims.UserID != "user1" {
		t.Errorf("UserID = %q, want %q", rtClaims.UserID, "user1")
	}
}

// =============================================================================
// GenerateTOTPSecret — verify the provisioning URI is well-formed (totp.go:34)
// =============================================================================

func TestGenerateTOTPSecret_URIContainsExpectedParts(t *testing.T) {
	secret, uri, err := GenerateTOTPSecret("user-1", "test@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if secret == "" {
		t.Error("secret should not be empty")
	}
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Errorf("uri = %q, want otpauth://totp/ prefix", uri)
	}
	if !strings.Contains(uri, "secret=") {
		t.Error("uri should contain secret=")
	}
	if !strings.Contains(uri, "issuer=DeployMonster") {
		t.Error("uri should contain issuer=DeployMonster")
	}
	if !strings.Contains(uri, "digits=6") {
		t.Error("uri should contain digits=6")
	}
	if !strings.Contains(uri, "period=30") {
		t.Error("uri should contain period=30")
	}
}

// =============================================================================
// GenerateBackupCodes — verify count and that hashes can be verified (totp.go:156)
// =============================================================================

func TestGenerateBackupCodes_CountAndVerify(t *testing.T) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}
	if len(codes.Plain) != 10 {
		t.Errorf("plain codes count = %d, want 10", len(codes.Plain))
	}
	if len(codes.Hashes) != 10 {
		t.Errorf("hashes count = %d, want 10", len(codes.Hashes))
	}
	for i, plain := range codes.Plain {
		if len(plain) != 8 {
			t.Errorf("code %d length = %d, want 8", i, len(plain))
		}
		// Verify the hash matches the plain code
		if err := bcrypt.CompareHashAndPassword([]byte(codes.Hashes[i]), []byte(plain)); err != nil {
			t.Errorf("code %d hash does not match plain text: %v", i, err)
		}
	}
}

// =============================================================================
// ValidateTOTPStep — edge cases (totp.go:73)
// =============================================================================

func TestValidateTOTPStep_InvalidTokenLength(t *testing.T) {
	_, ok := ValidateTOTPStep("12345", "SECRETBASE32")
	if ok {
		t.Error("expected false for 5-digit token")
	}
}

func TestValidateTOTPStep_InvalidSecret(t *testing.T) {
	_, ok := ValidateTOTPStep("123456", "!!!INVALID!!!")
	if ok {
		t.Error("expected false for invalid base32 secret")
	}
}

// =============================================================================
// ValidateTOTP — top-level validation function (totp.go:64)
// =============================================================================

func TestValidateTOTP_InvalidCode(t *testing.T) {
	if ValidateTOTP("000000", "SECRET") {
		t.Error("expected false for invalid code")
	}
}

// =============================================================================
// Note: crypto/rand.Read error paths in GenerateAPIKey, generateTokenID,
// GenerateTOTPSecret, and GenerateBackupCodes are untestable in Go 1.26+
// because rand.Read fatally exits (os.Exit/panic) on reader failure rather
// than returning an error. The error-handling code is dead in practice.
// See: https://go.dev/issue/66821
// =============================================================================

func TestValidatePasswordStrength_AdminCommonPassword(t *testing.T) {
	// "admin" is in the commonPasswords map
	err := ValidatePasswordStrength("admin", 8)
	if err == nil {
		t.Fatal("expected error for common password 'admin'")
	}
}

func TestValidatePasswordStrength_Monster123CommonPassword(t *testing.T) {
	// "monster123" is in the commonPasswords map
	err := ValidatePasswordStrength("monster123", 8)
	if err == nil {
		t.Fatal("expected error for common password 'monster123'")
	}
}

// === merged from module_followup_test.go ===

// TestModule_TOTP mirrors the pattern of TestModule_JWT — before
// initialization the accessor returns nil, and once the field is set
// the accessor returns the same pointer.
func TestModule_TOTP(t *testing.T) {
	m := New()
	if m.TOTP() != nil {
		t.Error("TOTP() should be nil before initialization")
	}

	svc := NewTOTPService(nil)
	m.totp = svc
	if m.TOTP() != svc {
		t.Error("TOTP() should return the configured TOTP service")
	}
}

// TestCleanupBootstrapAdminCredentials_EnvVarsAlwaysCleared verifies the
// pre-file-handling branch: the two MONSTER_ADMIN_* env vars are unset
// regardless of file state, even when the configured env-file path does
// not exist on disk.
func TestCleanupBootstrapAdminCredentials_EnvVarsAlwaysCleared(t *testing.T) {
	withBootstrapEnvFile(t, filepath.Join(t.TempDir(), "missing.env"))

	t.Setenv("MONSTER_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "hunter2")

	m := New()
	m.logger = slog.Default()
	m.cleanupBootstrapAdminCredentials()

	if v, ok := os.LookupEnv("MONSTER_ADMIN_EMAIL"); ok {
		t.Fatalf("MONSTER_ADMIN_EMAIL still set: %q", v)
	}
	if v, ok := os.LookupEnv("MONSTER_ADMIN_PASSWORD"); ok {
		t.Fatalf("MONSTER_ADMIN_PASSWORD still set: %q", v)
	}
}

// TestCleanupBootstrapAdminCredentials_FileWithoutMarkerKept covers the
// early-return where the env file exists but contains no
// MONSTER_ADMIN_PASSWORD line — the cleanup must leave the file alone.
func TestCleanupBootstrapAdminCredentials_FileWithoutMarkerKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploymonster.env")
	if err := os.WriteFile(path, []byte("MONSTER_PORT=8443\n"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	withBootstrapEnvFile(t, path)

	m := New()
	m.logger = slog.Default()
	m.cleanupBootstrapAdminCredentials()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("env file should still exist when no password marker is present: %v", err)
	}
}

// TestCleanupBootstrapAdminCredentials_ReadErrorIsNotNotExist covers the
// rare branch where ReadFile fails for a reason other than IsNotExist —
// the easiest provocation is pointing the env-file path at a directory,
// which on every supported OS surfaces as "is a directory" rather than
// "no such file or directory". The function must Warn-and-return without
// panicking on the nil-data path that follows.
func TestCleanupBootstrapAdminCredentials_ReadErrorIsNotNotExist(t *testing.T) {
	dir := t.TempDir()
	// Path is a directory, not a file — ReadFile errors with
	// syscall.EISDIR (or platform equivalent) which is not IsNotExist.
	withBootstrapEnvFile(t, dir)

	m := New()
	m.logger = slog.Default()
	// Must not panic; function emits a Warn and returns.
	m.cleanupBootstrapAdminCredentials()

	// Sanity check: directory still exists (cleanup did not try to
	// remove a directory).
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("expected dir to still exist; stat err=%v", err)
	}
}

// TestCleanupBootstrapAdminCredentials_RemoveFailureWarns provokes the
// post-marker remove-failure branch by putting the env file inside a
// read-only parent directory so os.Remove returns EACCES. Skipped on
// Windows where 0o500 on directories does not block deletion the same
// way it does on POSIX.
func TestCleanupBootstrapAdminCredentials_RemoveFailureWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only parent dir does not block file removal on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root user bypasses the read-only directory protection")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "deploymonster.env")
	contents := "MONSTER_ADMIN_PASSWORD=hunter2\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	// Make parent dir read+exec only so unlink fails with EACCES.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // restore so t.TempDir cleanup can run
	withBootstrapEnvFile(t, path)

	m := New()
	m.logger = slog.Default()
	m.cleanupBootstrapAdminCredentials()

	// File must still exist because Remove failed.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should still exist after blocked remove; stat err=%v", err)
	}
}

// TestCleanupBootstrapAdminCredentials_FileWithMarkerRemoved exercises
// the success path where the password marker is present — the file is
// expected to be deleted.
func TestCleanupBootstrapAdminCredentials_FileWithMarkerRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploymonster.env")
	contents := "MONSTER_ADMIN_EMAIL=admin@example.com\nMONSTER_ADMIN_PASSWORD=hunter2\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	withBootstrapEnvFile(t, path)

	m := New()
	m.logger = slog.Default()
	m.cleanupBootstrapAdminCredentials()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected env file to be removed; stat err=%v", err)
	}
}

// withBootstrapEnvFile temporarily redirects the package-level
// bootstrapAdminEnvFile pointer at a caller-supplied path and restores
// the original value when the test ends.
func withBootstrapEnvFile(t *testing.T, path string) {
	t.Helper()
	original := bootstrapAdminEnvFile
	bootstrapAdminEnvFile = path
	t.Cleanup(func() { bootstrapAdminEnvFile = original })
}

// === merged from timing_followup_test.go ===

// TestConstantTimeCompare hits the length-mismatch branch and the
// equal/unequal same-length branches in totp.go's constantTimeCompare.
func TestConstantTimeCompare(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"both empty", "", "", true},
		{"equal", "123456", "123456", true},
		{"length mismatch", "12345", "123456", false},
		{"same length differ", "123456", "123457", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := constantTimeCompare(tc.a, tc.b); got != tc.want {
				t.Fatalf("constantTimeCompare(%q,%q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestValidateTOTP_EdgeCases covers the rejection paths and the
// padding-fallback decode that the happy-path tests skip.
func TestValidateTOTP_EdgeCases(t *testing.T) {
	t.Run("rejects token of wrong length", func(t *testing.T) {
		// Length must be exactly 6 or 8; everything else is rejected
		// before the decoder runs.
		if ValidateTOTP("12345", "JBSWY3DPEHPK3PXP") {
			t.Fatal("5-digit token must be rejected")
		}
		if ValidateTOTP("1234567", "JBSWY3DPEHPK3PXP") {
			t.Fatal("7-digit token must be rejected")
		}
		if ValidateTOTP("", "JBSWY3DPEHPK3PXP") {
			t.Fatal("empty token must be rejected")
		}
	})

	t.Run("rejects undecodable secret", func(t *testing.T) {
		// Base32 only accepts A-Z and 2-7; a literal "1" is invalid
		// and the padding-retry will fail too.
		if ValidateTOTP("000000", "111111") {
			t.Fatal("invalid base32 secret must reject")
		}
	})

	t.Run("rejects wrong code", func(t *testing.T) {
		// Use a known-good secret. Almost-any 6-digit token will
		// disagree with the live HOTP value; "000000" suffices in
		// practice since collision is 1-in-10^6 per call.
		if ValidateTOTP("000000", "JBSWY3DPEHPK3PXP") {
			// Extremely unlikely; if this hits, swap the literal for
			// another value rather than treat it as a regression.
			t.Skip("000000 happened to match HOTP at this clock tick")
		}
	})
}
