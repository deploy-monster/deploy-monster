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
	previousKeys  [][]byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewJWTService creates a new JWT service. previousSecrets are old keys kept
// for graceful rotation — tokens signed with them are still accepted for
// validation but new tokens are always signed with the primary secret.
func NewJWTService(secret string, previousSecrets ...string) *JWTService {
	prev := make([][]byte, 0, len(previousSecrets))
	for _, s := range previousSecrets {
		if s != "" {
			prev = append(prev, []byte(s))
		}
	}
	return &JWTService{
		secretKey:     []byte(secret),
		previousKeys:  prev,
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
// Tries the active key first, then falls back to previous keys for graceful rotation.
func (j *JWTService) ValidateAccessToken(tokenStr string) (*Claims, error) {
	for _, key := range j.allKeys() {
		token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
			return key, nil
		})
		if err != nil {
			continue
		}
		claims, ok := token.Claims.(*Claims)
		if !ok || !token.Valid {
			continue
		}
		return claims, nil
	}
	return nil, jwt.ErrTokenSignatureInvalid
}

// RefreshTokenClaims holds the validated claims from a refresh token.
type RefreshTokenClaims struct {
	UserID string
	JTI    string
}

// RefreshTokenTTLSeconds is the refresh token lifetime used for revocation entry TTL.
const RefreshTokenTTLSeconds = 7 * 24 * 60 * 60 // 7 days

// ValidateRefreshToken parses and validates a refresh token.
// Returns the user ID (Subject) and the token ID (JTI) for revocation tracking.
// Tries the active key first, then falls back to previous keys for graceful rotation.
func (j *JWTService) ValidateRefreshToken(tokenStr string) (*RefreshTokenClaims, error) {
	for _, key := range j.allKeys() {
		token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
			return key, nil
		})
		if err != nil {
			continue
		}
		claims, ok := token.Claims.(*jwt.RegisteredClaims)
		if !ok || !token.Valid {
			continue
		}
		return &RefreshTokenClaims{
			UserID: claims.Subject,
			JTI:    claims.ID,
		}, nil
	}
	return nil, jwt.ErrTokenSignatureInvalid
}

// allKeys returns the active key followed by any previous keys.
func (j *JWTService) allKeys() [][]byte {
	keys := make([][]byte, 0, 1+len(j.previousKeys))
	keys = append(keys, j.secretKey)
	keys = append(keys, j.previousKeys...)
	return keys
}

func generateTokenID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
