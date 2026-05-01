package auth

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

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

func (m *mockStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	if m.createUserErr != nil {
		return "", m.createUserErr
	}
	return m.createUserID, nil
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
