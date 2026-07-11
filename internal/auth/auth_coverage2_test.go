package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
	"github.com/golang-jwt/jwt/v5"
)

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
// module.go:60-62 — SetReplayStore with Bolt in Core.DB
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
		DB:     &core.Database{Bolt: &nopBoltStorer{}},
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
		DB:       &core.Database{Bolt: &nopBoltStorer{}},
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
	s.SetReplayStore(&nopBoltStorer{})
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
		TOTPEnabled bool
		TOTPSecret  string
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
// nopBoltStorer — minimal BoltStorer implementation for tests
// =============================================================================

type nopBoltStorer struct{}

func (n *nopBoltStorer) Set(bucket, key string, value any, ttlSeconds int64) error { return nil }
func (n *nopBoltStorer) BatchSet(items []core.BoltBatchItem) error                  { return nil }
func (n *nopBoltStorer) Get(bucket, key string, dest any) error {
	return fmt.Errorf("not found")
}
func (n *nopBoltStorer) Delete(bucket, key string) error                { return nil }
func (n *nopBoltStorer) List(bucket string) ([]string, error)          { return nil, nil }
func (n *nopBoltStorer) Close() error                                   { return nil }
func (n *nopBoltStorer) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	return nil, fmt.Errorf("not found")
}
func (n *nopBoltStorer) GetWebhookSecret(webhookID string) (string, error) {
	return "", fmt.Errorf("not found")
}
