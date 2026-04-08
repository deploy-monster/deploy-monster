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

func TestJWT_UniqueTokenIDs(t *testing.T) {
	svc := NewJWTService(testSecret)

	pair1, _ := svc.GenerateTokenPair("user-1", "t", "r", "e@e.com")
	pair2, _ := svc.GenerateTokenPair("user-1", "t", "r", "e@e.com")

	if pair1.AccessToken == pair2.AccessToken {
		t.Error("two token pairs should have different access tokens")
	}
}
