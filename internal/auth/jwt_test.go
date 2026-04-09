package auth

import (
	"testing"
)

const testSecret = "test-secret-key-at-least-32-bytes-long!"

func TestJWT_GenerateAndValidate(t *testing.T) {
	svc := NewJWTService(testSecret)

	pair, err := svc.GenerateTokenPair("user-1", "tenant-1", "role_admin", "test@example.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	if pair.AccessToken == "" {
		t.Error("access token is empty")
	}
	if pair.RefreshToken == "" {
		t.Error("refresh token is empty")
	}
	if pair.TokenType != "Bearer" {
		t.Errorf("expected Bearer token type, got %q", pair.TokenType)
	}
	if pair.ExpiresIn <= 0 {
		t.Error("expires_in should be positive")
	}

	// Validate access token
	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if claims.UserID != "user-1" {
		t.Errorf("expected user-1, got %q", claims.UserID)
	}
	if claims.TenantID != "tenant-1" {
		t.Errorf("expected tenant-1, got %q", claims.TenantID)
	}
	if claims.RoleID != "role_admin" {
		t.Errorf("expected role_admin, got %q", claims.RoleID)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected test@example.com, got %q", claims.Email)
	}
}

func TestJWT_ValidateInvalidToken(t *testing.T) {
	svc := NewJWTService(testSecret)

	_, err := svc.ValidateAccessToken("invalid-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestJWT_ValidateWrongSecret(t *testing.T) {
	svc1 := NewJWTService("secret-one-at-least-32-bytes-long!")
	svc2 := NewJWTService("secret-two-at-least-32-bytes-long!")

	pair, _ := svc1.GenerateTokenPair("user-1", "tenant-1", "role_admin", "test@example.com")

	_, err := svc2.ValidateAccessToken(pair.AccessToken)
	if err == nil {
		t.Error("expected error when validating with different secret")
	}
}

func TestJWT_RefreshToken(t *testing.T) {
	svc := NewJWTService(testSecret)

	pair, _ := svc.GenerateTokenPair("user-1", "tenant-1", "role_admin", "test@example.com")

	rtClaims, err := svc.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if rtClaims.UserID != "user-1" {
		t.Errorf("expected user-1, got %q", rtClaims.UserID)
	}
}

func TestJWT_KeyRotation_AccessToken(t *testing.T) {
	oldSecret := "old-secret-key-at-least-32-bytes-long!"
	newSecret := "new-secret-key-at-least-32-bytes-long!"

	// Generate token with the old key
	oldSvc := NewJWTService(oldSecret)
	pair, err := oldSvc.GenerateTokenPair("user-1", "tenant-1", "role_admin", "test@example.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	// After rotation: new service has new key as active, old key as previous
	rotatedSvc := NewJWTService(newSecret, oldSecret)

	// Token signed with old key should still validate
	claims, err := rotatedSvc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("old-key access token should validate after rotation: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Errorf("expected user-1, got %q", claims.UserID)
	}

	// New tokens should be signed with new key and also validate
	newPair, err := rotatedSvc.GenerateTokenPair("user-2", "tenant-2", "role_user", "u2@example.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair with new key: %v", err)
	}
	claims2, err := rotatedSvc.ValidateAccessToken(newPair.AccessToken)
	if err != nil {
		t.Fatalf("new-key access token should validate: %v", err)
	}
	if claims2.UserID != "user-2" {
		t.Errorf("expected user-2, got %q", claims2.UserID)
	}

	// Service with only new key (no previous) should reject old token
	newOnlySvc := NewJWTService(newSecret)
	_, err = newOnlySvc.ValidateAccessToken(pair.AccessToken)
	if err == nil {
		t.Error("old-key token should fail without previous key configured")
	}
}

func TestJWT_KeyRotation_RefreshToken(t *testing.T) {
	oldSecret := "old-secret-key-at-least-32-bytes-long!"
	newSecret := "new-secret-key-at-least-32-bytes-long!"

	oldSvc := NewJWTService(oldSecret)
	pair, err := oldSvc.GenerateTokenPair("user-1", "tenant-1", "role_admin", "test@example.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	// Rotated service accepts old refresh token
	rotatedSvc := NewJWTService(newSecret, oldSecret)
	rtClaims, err := rotatedSvc.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("old-key refresh token should validate after rotation: %v", err)
	}
	if rtClaims.UserID != "user-1" {
		t.Errorf("expected user-1, got %q", rtClaims.UserID)
	}
}

func TestJWT_KeyRotation_MultiplePreviousKeys(t *testing.T) {
	key1 := "secret-key-one-at-least-32-bytes-long!"
	key2 := "secret-key-two-at-least-32-bytes-long!"
	key3 := "secret-key-three-at-least-32-bytes!!"

	// Generate tokens with each key
	svc1 := NewJWTService(key1)
	pair1, _ := svc1.GenerateTokenPair("u1", "t", "r", "e@e.com")

	svc2 := NewJWTService(key2)
	pair2, _ := svc2.GenerateTokenPair("u2", "t", "r", "e@e.com")

	// Current service has key3 as active, key1 and key2 as previous
	svc3 := NewJWTService(key3, key1, key2)

	// All three should validate
	c1, err := svc3.ValidateAccessToken(pair1.AccessToken)
	if err != nil {
		t.Fatalf("key1 token should validate: %v", err)
	}
	if c1.UserID != "u1" {
		t.Errorf("expected u1, got %q", c1.UserID)
	}

	c2, err := svc3.ValidateAccessToken(pair2.AccessToken)
	if err != nil {
		t.Fatalf("key2 token should validate: %v", err)
	}
	if c2.UserID != "u2" {
		t.Errorf("expected u2, got %q", c2.UserID)
	}

	// Token from key3 (active)
	pair3, _ := svc3.GenerateTokenPair("u3", "t", "r", "e@e.com")
	c3, err := svc3.ValidateAccessToken(pair3.AccessToken)
	if err != nil {
		t.Fatalf("key3 token should validate: %v", err)
	}
	if c3.UserID != "u3" {
		t.Errorf("expected u3, got %q", c3.UserID)
	}
}

func TestJWT_KeyRotation_EmptyPreviousKeysIgnored(t *testing.T) {
	svc := NewJWTService(testSecret, "", "", "")
	pair, err := svc.GenerateTokenPair("user-1", "t", "r", "e@e.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}
	_, err = svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
}

func TestJWT_UniqueTokenIDs(t *testing.T) {
	svc := NewJWTService(testSecret)

	pair1, _ := svc.GenerateTokenPair("user-1", "t", "r", "e@e.com")
	pair2, _ := svc.GenerateTokenPair("user-1", "t", "r", "e@e.com")

	if pair1.AccessToken == pair2.AccessToken {
		t.Error("two token pairs should have different access tokens")
	}
}
