# Authentication Security Scan Results

**Scan Date:** 2026-04-14  
**Scope:** DeployMonster Authentication System  
**Files Analyzed:**
- `internal/auth/jwt.go`
- `internal/auth/password.go`
- `internal/auth/apikey.go`
- `internal/auth/module.go`
- `internal/api/handlers/auth.go`
- `internal/api/handlers/sessions.go`
- `internal/api/middleware/middleware.go`
- `internal/api/middleware/ratelimit.go`
- `internal/api/middleware/csrf.go`
- `internal/api/router.go`
- `internal/db/bolt.go`
- `internal/core/config.go`

---

## Summary

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High | 1 |
| Medium | 4 |
| Low | 3 |
| Info | 4 |

**Overall Security Posture:** GOOD - The authentication system has been well-hardened with many security best practices already implemented. Most findings are medium/low severity with clear remediation paths.

---

## Critical Findings (0)

No critical vulnerabilities identified.

---

## High Severity Findings (1)

### AUTH-001: Missing Signing Method Verification in Refresh Token Validation

**Location:** `internal/auth/jwt.go:208-226`  
**Severity:** HIGH  
**Confidence:** HIGH

**Issue Description:**
The `ValidateRefreshToken` function does not explicitly verify the JWT signing method, unlike `ValidateAccessToken` which checks `token.Method != jwt.SigningMethodHS256` at line 157. This could potentially allow algorithm confusion attacks where an attacker presents a token signed with a different algorithm (e.g., `alg=none`) that the parser might accept.

**Current Code:**
```go
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
        // Missing: token.Method verification
        return &RefreshTokenClaims{
            UserID: claims.Subject,
            JTI:    claims.ID,
        }, nil
    }
    return nil, jwt.ErrTokenSignatureInvalid
}
```

**Remediation:**
Add explicit signing method verification after parsing, matching the pattern used in `ValidateAccessToken`:

```go
if token.Method != jwt.SigningMethodHS256 {
    return nil, jwt.ErrTokenSignatureInvalid
}
```

**References:**
- JWT Algorithm Confusion (RFC 7515): https://tools.ietf.org/html/rfc7515
- OWASP JWT Security Cheat Sheet

---

## Medium Severity Findings (4)

### AUTH-002: bcrypt Cost Factor Not Configurable

**Location:** `internal/auth/password.go:10`  
**Severity:** MEDIUM  
**Confidence:** HIGH

**Issue Description:**
The bcrypt cost factor is hardcoded to 13 in a constant. While this is a reasonable default (providing ~0.3s hash time on modern hardware), it cannot be adjusted based on deployment requirements or threat models. Organizations with higher security requirements cannot increase the cost factor without code modification.

**Current Code:**
```go
const bcryptCost = 13
```

**Remediation:**
Make the bcrypt cost factor configurable via environment variable or config file, with a sensible default:

```go
func getBcryptCost() int {
    if v := os.Getenv("MONSTER_BCRYPT_COST"); v != "" {
        if cost, err := strconv.Atoi(v); err == nil && cost >= 10 && cost <= 31 {
            return cost
        }
    }
    return 13 // default
}
```

---

### AUTH-003: Refresh Token JTI Validation Missing in Token Rotation Flow

**Location:** `internal/api/handlers/auth.go:270-341`  
**Severity:** MEDIUM  
**Confidence:** MEDIUM

**Issue Description:**
In the `Refresh` handler, after validating a refresh token, the code checks if the JTI exists in the revocation list, but it does not verify that the JTI in the token claims actually matches the expected format or is non-empty. An empty JTI could potentially bypass the revocation check (line 298-303) since an empty string key lookup might behave unexpectedly.

**Current Code:**
```go
// Check if token has been revoked
if h.bolt != nil && rtClaims.JTI != "" {  // Only checks if not empty
    var revoked bool
    if err := h.bolt.Get("revoked_tokens", rtClaims.JTI, &revoked); err == nil && revoked {
        writeError(w, http.StatusUnauthorized, "token has been revoked")
        return
    }
}
```

**Remediation:**
Explicitly validate that JTI is present and properly formatted before proceeding:

```go
if rtClaims.JTI == "" {
    writeError(w, http.StatusUnauthorized, "invalid token format")
    return
}
```

---

### AUTH-004: API Key Prefix Length Insufficient for Uniqueness

**Location:** `internal/auth/apikey.go:10, 28`  
**Severity:** MEDIUM  
**Confidence:** MEDIUM

**Issue Description:**
API keys use a 12-character prefix (8 chars after the "dm_" prefix) for lookup. With hex encoding, this provides only 32 bits of entropy (8 hex chars * 4 bits), which increases the collision probability when many keys are generated. For high-volume deployments with thousands of API keys, this could lead to prefix collisions requiring full scan fallback.

**Current Code:**
```go
const apiKeyPrefix = "dm_"
// ...
prefix := key[:len(apiKeyPrefix)+12]  // dm_ + 12 hex chars = 15 chars total
```

**Remediation:**
Increase the prefix length to 16 characters after "dm_" to provide 64 bits of entropy:

```go
prefix := key[:len(apiKeyPrefix)+16]  // dm_ + 16 hex chars = 19 chars total
```

Or use a base32 encoding to maintain readability while increasing entropy per character.

---

### AUTH-005: Session Fingerprinting for Security Monitoring

**Location:** `internal/api/handlers/sessions.go:364-389`  
**Severity:** MEDIUM  
**Confidence:** LOW

**Issue Description:**
The session tracking system stores IP address and User-Agent but does not perform any security analysis on these values during session validation. Potential security events like:
- Sudden IP geolocation changes
- User-Agent string changes
- Impossible travel scenarios

Are not detected or logged, reducing the ability to detect account compromise.

**Remediation:**
Add optional security event detection during session operations:

```go
// During refresh/login - compare current session with stored session
if session.IP != currentIP {
    slog.Warn("session ip change detected",
        "user_id", userID,
        "old_ip", session.IP,
        "new_ip", currentIP,
    )
    // Optionally require re-authentication for sensitive operations
}
```

---

## Low Severity Findings (3)

### AUTH-006: Cookie Domain Not Explicitly Set

**Location:** `internal/api/handlers/auth.go:49-70`  
**Severity:** LOW  
**Confidence:** MEDIUM

**Issue Description:**
The authentication cookies (`dm_access`, `dm_refresh`) do not explicitly set the `Domain` attribute. This means they default to the exact host of the request. While this is the most secure default, it may cause unexpected behavior in multi-subdomain deployments where users expect cookies to be shared across subdomains.

**Current Code:**
```go
http.SetCookie(w, &http.Cookie{
    Name:     cookieAccess,
    Value:    tokens.AccessToken,
    Path:     "/api",
    MaxAge:   tokens.ExpiresIn,
    HttpOnly: true,
    Secure:   secure,
    SameSite: http.SameSiteStrictMode,
    // Domain not set - defaults to exact host
})
```

**Remediation:**
Consider making the cookie domain configurable:

```go
if cfg.CookieDomain != "" {
    cookie.Domain = cfg.CookieDomain
}
```

**Note:** Do not set this to a wildcard without careful consideration of security implications.

---

### AUTH-007: Password Validation Does Not Check Special Characters

**Location:** `internal/auth/password.go:27-50`  
**Severity:** LOW  
**Confidence:** HIGH

**Issue Description:**
The `ValidatePasswordStrength` function requires uppercase, lowercase, and digits, but does not require special characters. While this provides basic entropy, modern password policies often require at least one special character to increase the search space for brute force attacks.

**Current Code:**
```go
var hasUpper, hasLower, hasDigit bool
for _, r := range password {
    switch {
    case unicode.IsUpper(r):
        hasUpper = true
    case unicode.IsLower(r):
        hasLower = true
    case unicode.IsDigit(r):
        hasDigit = true
    }
}
if !hasUpper || !hasLower || !hasDigit {
    return fmt.Errorf("password must contain uppercase, lowercase, and digit")
}
```

**Remediation:**
Add optional special character requirement or make it configurable:

```go
var hasUpper, hasLower, hasDigit, hasSpecial bool
for _, r := range password {
    switch {
    case unicode.IsUpper(r):
        hasUpper = true
    case unicode.IsLower(r):
        hasLower = true
    case unicode.IsDigit(r):
        hasDigit = true
    case unicode.IsPunct(r) || unicode.IsSymbol(r):
        hasSpecial = true
    }
}
// Make special character requirement configurable
if requireSpecial && !hasSpecial {
    return fmt.Errorf("password must contain at least one special character")
}
```

---

### AUTH-008: JWT Token ID Generation Uses Weak Randomness

**Location:** `internal/auth/jwt.go:238-242`  
**Severity:** LOW  
**Confidence:** LOW

**Issue Description:**
The `generateTokenID` function uses `crypto/rand.Read` which is cryptographically secure, but the 16-byte output (128 bits) is encoded to hex producing 32 characters. While sufficient for uniqueness, this is more than necessary and increases token size. More importantly, there's no check for the error return from `rand.Read`.

**Current Code:**
```go
func generateTokenID() string {
    b := make([]byte, 16)
    _, _ = rand.Read(b)  // Error ignored
    return hex.EncodeToString(b)
}
```

**Remediation:**
Check for errors from `rand.Read`:

```go
func generateTokenID() string {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil {
        // Fall back to time-based with random suffix or panic in secure context
        return fmt.Sprintf("%d-%x", time.Now().UnixNano(), make([]byte, 8))
    }
    return hex.EncodeToString(b)
}
```

---

## Informational Findings (4)

### AUTH-INFO-001: Secure Implementation - HS256 Algorithm Enforcement

**Location:** `internal/auth/jwt.go:156-159`  
**Status:** SECURE

The code explicitly verifies that tokens use the expected signing algorithm (HS256), preventing algorithm confusion attacks. This is a critical security control that is correctly implemented.

### AUTH-INFO-002: Secure Implementation - Key Rotation Support

**Location:** `internal/auth/jwt.go:11-97`  
**Status:** SECURE

The JWT service implements graceful key rotation with a configurable grace period (20 minutes). Previous keys are tracked with timestamps and automatically purged after expiration. This is an enterprise-grade security feature.

### AUTH-INFO-003: Secure Implementation - Constant-Time API Key Comparison

**Location:** `internal/api/middleware/middleware.go:236`  
**Status:** SECURE

API key validation uses `subtle.ConstantTimeCompare` to prevent timing attacks on key comparison. This is the correct approach for secure credential comparison.

### AUTH-INFO-004: Secure Implementation - Comprehensive Rate Limiting

**Location:** `internal/api/middleware/ratelimit.go`, `internal/api/router.go:125-127`  
**Status:** SECURE

The system implements per-IP rate limiting for authentication endpoints:
- Login: 5 requests/minute
- Register: 3 requests/minute
- Refresh: 5 requests/minute

This provides effective brute force protection.

---

## Appendix: Security Controls Summary

### Implemented Security Controls

| Control | Status | Location |
|---------|--------|----------|
| JWT algorithm enforcement | OK | jwt.go:157 |
| Key rotation support | OK | jwt.go:11-97 |
| Token revocation | OK | jwt.go:175-194 |
| Refresh token rotation | OK | auth.go:320-325 |
| Session fixation prevention | OK | auth.go:124 |
| Concurrent session limiting | OK | sessions.go:201-309 |
| Password hashing (bcrypt) | OK | password.go:13-19 |
| Password strength validation | OK | password.go:27-50 |
| API key hashing (SHA-256) | OK | apikey.go:38-41 |
| Constant-time key comparison | OK | middleware.go:236 |
| httpOnly cookies | OK | auth.go:57, 66 |
| Secure cookie flag | OK | auth.go:58, 67 |
| SameSite Strict cookies | OK | auth.go:59, 68 |
| CSRF protection | OK | csrf.go:17-78 |
| Rate limiting (auth endpoints) | OK | ratelimit.go, router.go |
| X-Forwarded-For validation | OK | ratelimit.go:87-101 |
| Brute force protection | OK | auth.go:139-142 (max password length) |
| Secret key length validation | OK | config.go:256-258 |

### Test Coverage

- JWT validation tests: `internal/auth/jwt_fuzz_test.go`, `internal/auth/auth_final_test.go`
- Password hashing tests: `internal/auth/password_test.go`
- API key tests: `internal/auth/apikey_test.go`
- Algorithm confusion test: `auth_final_test.go:75-89`

---

## Remediation Priority

1. **HIGH:** AUTH-001 - Add signing method verification to refresh token validation
2. **MEDIUM:** AUTH-002 - Make bcrypt cost configurable
3. **MEDIUM:** AUTH-003 - Add JTI validation in refresh flow
4. **MEDIUM:** AUTH-004 - Increase API key prefix length
5. **LOW:** AUTH-006 - Consider making cookie domain configurable
6. **LOW:** AUTH-007 - Consider adding special character requirement to passwords
7. **LOW:** AUTH-008 - Add error handling to token ID generation

---

*Report generated by Claude Code Security Scanner*
