package auth

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/golang-jwt/jwt/v5"
)

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
