package auth

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT token claims.
type Claims struct {
	jwt.RegisteredClaims
	UserID   string `json:"uid"`
	TenantID string `json:"tid"`
	RoleID   string `json:"rid"`
	Email    string `json:"email"`
}

// TokenPair contains access and refresh tokens.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// JWTService handles JWT token generation and validation.
type JWTService struct {
	secretKey     []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewJWTService creates a new JWT service.
func NewJWTService(secret string) *JWTService {
	return &JWTService{
		secretKey:     []byte(secret),
		accessExpiry:  15 * time.Minute,
		refreshExpiry: 7 * 24 * time.Hour,
	}
}

// GenerateTokenPair creates a new access/refresh token pair.
func (j *JWTService) GenerateTokenPair(userID, tenantID, roleID, email string) (*TokenPair, error) {
	now := time.Now()

	// Access token
	accessClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(j.accessExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        generateTokenID(),
		},
		UserID:   userID,
		TenantID: tenantID,
		RoleID:   roleID,
		Email:    email,
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(j.secretKey)
	if err != nil {
		return nil, err
	}

	// Refresh token
	refreshClaims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(now.Add(j.refreshExpiry)),
		IssuedAt:  jwt.NewNumericDate(now),
		Subject:   userID,
		ID:        generateTokenID(),
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(j.secretKey)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(j.accessExpiry.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// ValidateAccessToken parses and validates an access token.
func (j *JWTService) ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		return j.secretKey, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

// ValidateRefreshToken parses and validates a refresh token.
func (j *JWTService) ValidateRefreshToken(tokenStr string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		return j.secretKey, nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return "", jwt.ErrTokenInvalidClaims
	}
	return claims.Subject, nil
}

func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
