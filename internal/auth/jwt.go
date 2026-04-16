package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// RotationGracePeriod is how long previous keys remain valid after rotation.
// Access tokens are valid for 15 min, so we accept previous keys for 20 min
// to cover the worst-case: a token issued just before rotation is valid for
// up to 15 min, plus clock skew (2-3 min), plus a small buffer.
// SECURITY FIX (SESS-006): Reduced from 1 hour to 20 minutes to limit exposure window.
const RotationGracePeriod = 20 * time.Minute

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
	previousKeys  [][]byte    // ordered list of old keys
	previousAdded []time.Time // wall-clock when each key was added
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewJWTService creates a new JWT service. previousSecrets are old keys kept
// for graceful rotation — tokens signed with them are still accepted for
// validation but only within RotationGracePeriod of when they were added.
// This prevents indefinitely-valid compromised keys.
// SECURITY: Enforces minimum secret length of 32 characters (256 bits).
func NewJWTService(secret string, previousSecrets ...string) *JWTService {
	// SECURITY FIX (JWT-002): Enforce minimum secret length
	const minSecretLength = 32 // 256 bits minimum
	if len(secret) < minSecretLength {
		panic(fmt.Sprintf("JWT secret must be at least %d characters (256 bits) for security", minSecretLength))
	}

	prev := make([][]byte, 0, len(previousSecrets))
	added := make([]time.Time, 0, len(previousSecrets))
	now := time.Now()
	for _, s := range previousSecrets {
		if s != "" {
			prev = append(prev, []byte(s))
			added = append(added, now) // assume all provided previous keys are fresh
		}
	}
	return &JWTService{
		secretKey:     []byte(secret),
		previousKeys:  prev,
		previousAdded: added,
		accessExpiry:  15 * time.Minute,
		refreshExpiry: 7 * 24 * time.Hour,
	}
}

// AddPreviousKey adds an old key that was rotated out. Call this during
// key rotation to register the old primary as a fallback with its add time.
// The key will be accepted for validation only within RotationGracePeriod.
func (j *JWTService) AddPreviousKey(secret string) {
	if secret == "" {
		return
	}
	j.previousKeys = append(j.previousKeys, []byte(secret))
	j.previousAdded = append(j.previousAdded, time.Now())
	j.purgeExpiredPreviousKeys()
}

// purgeExpiredPreviousKeys removes keys older than RotationGracePeriod.
// Called before every validation and after key additions.
func (j *JWTService) purgeExpiredPreviousKeys() {
	cutoff := time.Now().Add(-RotationGracePeriod)
	validIdx := 0
	for i, t := range j.previousAdded {
		if t.After(cutoff) {
			if validIdx != i {
				j.previousKeys[validIdx] = j.previousKeys[i]
				j.previousAdded[validIdx] = j.previousAdded[i]
			}
			validIdx++
		}
	}
	if validIdx < len(j.previousKeys) {
		j.previousKeys = j.previousKeys[:validIdx]
		j.previousAdded = j.previousAdded[:validIdx]
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
			NotBefore: jwt.NewNumericDate(now),
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
// SECURITY: Caller should check if token JTI is in revocation list via IsAccessTokenRevoked.
func (j *JWTService) ValidateAccessToken(tokenStr string) (*Claims, error) {
	for _, key := range j.allKeys() {
		// WithValidMethods rejects the token before the keyfunc runs if
		// the alg header is not HS256 — the canonical defense against
		// alg=none and alg-confusion attacks.
		token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
			return key, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
		if err != nil {
			continue
		}
		claims, ok := token.Claims.(*Claims)
		if !ok || !token.Valid {
			continue
		}
		// Belt-and-suspenders: the parse option above already enforces
		// this, but we double-check in case the library behavior changes.
		if token.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return claims, nil
	}
	return nil, jwt.ErrTokenSignatureInvalid
}

// AccessTokenRevocation stores a revoked access token JTI with expiration time.
type AccessTokenRevocation struct {
	JTI       string    `json:"jti"`
	ExpiresAt time.Time `json:"expires_at"`
	UserID    string    `json:"user_id"`
}

// RevokeAccessToken marks an access token as revoked by storing its JTI.
// The revocation entry has the same TTL as the token's remaining lifetime.
// This is typically called during logout to immediately invalidate the access token.
func (j *JWTService) RevokeAccessToken(boltStorer interface {
	Set(bucket, key string, value any, ttlSeconds int64) error
}, jti, userID string, expiresAt time.Time) error {
	revocation := AccessTokenRevocation{
		JTI:       jti,
		ExpiresAt: expiresAt,
		UserID:    userID,
	}
	// Calculate remaining TTL
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return nil // Token already expired, no need to revoke
	}
	return boltStorer.Set("revoked_access_tokens", jti, revocation, int64(remaining.Seconds()))
}

// IsAccessTokenRevoked checks if an access token JTI is in the revocation list.
func (j *JWTService) IsAccessTokenRevoked(boltStorer interface {
	Get(bucket, key string, dest any) error
}, jti string) bool {
	// If no revocation store provided, assume token is not revoked
	if boltStorer == nil {
		return false
	}
	var revocation AccessTokenRevocation
	err := boltStorer.Get("revoked_access_tokens", jti, &revocation)
	return err == nil // If no error, token is revoked
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
		// WithValidMethods rejects the token before the keyfunc runs if
		// the alg header is not HS256 — mirrors ValidateAccessToken.
		token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
			return key, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
		if err != nil {
			continue
		}
		claims, ok := token.Claims.(*jwt.RegisteredClaims)
		if !ok || !token.Valid {
			continue
		}
		if token.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return &RefreshTokenClaims{
			UserID: claims.Subject,
			JTI:    claims.ID,
		}, nil
	}
	return nil, jwt.ErrTokenSignatureInvalid
}

// allKeys returns the active key followed by previous keys that are still
// within RotationGracePeriod. Expired keys are purged before returning.
func (j *JWTService) allKeys() [][]byte {
	j.purgeExpiredPreviousKeys()
	keys := make([][]byte, 0, 1+len(j.previousKeys))
	keys = append(keys, j.secretKey)
	keys = append(keys, j.previousKeys...)
	return keys
}

func generateTokenID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// SECURITY: Panic on random generation failure - this should never happen
		// and indicates a serious system issue (entropy exhaustion)
		panic("failed to generate token ID: " + err.Error())
	}
	return hex.EncodeToString(b)
}
