# Authentication, Authorization, JWT, and Crypto Security Findings

**Audit Date**: 2026-04-13
**Phase**: 2 - HUNT
**Scope**: Authentication, Authorization, JWT, Session Management, Crypto

---

## Summary

| Severity | Count | Status |
|----------|-------|--------|
| HIGH     | 0     | -      |
| MEDIUM   | 2     | Findings below |
| LOW      | 3     | Findings below |
| INFO     | 5     | Positive findings |

---

## Findings

### Finding 1: JWT Algorithm Not Explicitly Restricted

**Severity**: MEDIUM
**File**: `internal/auth/jwt.go` (lines 144, 173)
**CWE**: [CWE-347](https://cwe.mitre.org/data/definitions/347.html) - Improper Verification of Cryptographic Signature

**Description**:
The JWT validation functions `ValidateAccessToken` and `ValidateRefreshToken` use `jwt.ParseWithClaims` without explicitly restricting valid algorithms via `WithValidMethods()`.

```go
// jwt.go:144
token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
    return key, nil
})
```

**Why it's a vulnerability**:
While the golang-jwt library rejects the "none" algorithm by default, not explicitly restricting algorithms could allow algorithm confusion attacks. An attacker could potentially craft a token using a different algorithm (e.g., RS256) if the key is known or guessable.

**Recommendation**:
Add explicit algorithm restriction using `WithValidMethods`:
```go
token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
    if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
        return nil, jwt.ErrSignatureInvalid
    }
    return key, nil
})
```

**Test Coverage**:
The codebase does have a test (`TestValidateAccessToken_WrongSigningMethod` in `internal/auth/auth_final_test.go:75-88`) that verifies "none" algorithm is rejected, but it does not test algorithm confusion attacks like HS256 vs RS256.

---

### Finding 2: Weak Minimum Secret Key Length for JWT

**Severity**: MEDIUM
**File**: `internal/core/config.go` (line 256)
**CWE**: [CWE-326](https://cwe.mitre.org/data/definitions/326.html) - Inadequate Encryption Strength

**Description**:
The configuration validation requires only 16 characters minimum for the JWT secret key:

```go
// config.go:256
if len(c.Server.SecretKey) < 16 {
    return fmt.Errorf("config: server.secret_key must be at least 16 characters")
}
```

**Why it's a vulnerability**:
HS256 (HMAC-SHA256) requires a 256-bit (32-byte) key for proper security. A 16-character ASCII secret provides only 128 bits of entropy. While this is not trivially breakable, it falls short of cryptographic best practices and the full security margin that HS256 is designed to provide.

**Recommendation**:
Increase the minimum secret key length to 32 characters:
```go
if len(c.Server.SecretKey) < 32 {
    return fmt.Errorf("config: server.secret_key must be at least 32 characters")
}
```

---

### Finding 3: Hardcoded Default Admin Email

**Severity**: LOW
**File**: `internal/auth/module.go` (line 99)
**CWE**: [CWE-798](https://cwe.mitre.org/data/definitions/798.html) - Use of Hard-coded Credentials

**Description**:
The first-run setup creates a default super admin with a predictable email address:

```go
// module.go:99
email := getEnvOrDefault("MONSTER_ADMIN_EMAIL", "admin@deploy.monster")
```

**Why it's a vulnerability**:
While the password is auto-generated and stored securely in a credentials file, the predictable email address reduces the search space for credential stuffing attacks. An attacker who can trigger first-run setup (e.g., on a fresh deployment with no users) would know the email format.

**Mitigating Factors**:
- Password is auto-generated with 16 crypto-random characters
- Password is written to a file with 0600 permissions (readable only by owner)
- First-run only executes when `CountUsers() == 0`
- The default email can be overridden via `MONSTER_ADMIN_EMAIL` environment variable

**Recommendation**:
Consider randomizing the default admin username or requiring `MONSTER_ADMIN_EMAIL` to be explicitly set in production.

---

### Finding 4: generatePassword Uses Limited Character Set

**Severity**: LOW
**File**: `internal/core/id.go` (lines 42-54)
**CWE**: [CWE-331](https://cwe.mitre.org/data/definitions/331.html) - Insufficient Entropy

**Description**:
The password generator uses a 62-character alphanumeric charset without special characters:

```go
// id.go:43
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
```

**Why it's a vulnerability**:
- Only 6 bits of entropy per character (log2(62) ≈ 5.95)
- A 16-character password has approximately 95 bits of entropy
- Missing special characters reduces the effective search space for certain attack vectors

**Mitigating Factors**:
- Uses `crypto/rand` for random number generation (secure)
- Fallback to `math/big` if `crypto/rand` fails (logged but continues)
- Passwords are used for system-generated credentials, not user-chosen passwords

**Recommendation**:
Consider adding special characters to increase entropy per character, or use base64 encoding which provides 6 bits per character with all printable characters.

---

### Finding 5: generateTokenID Silently Ignores rand.Read Errors

**Severity**: LOW
**File**: `internal/auth/jwt.go` (lines 201-204)
**CWE**: [CWE-755](https://cwe.mitre.org/data/definitions/755.html) - Improper Handling of Exceptional Conditions

**Description**:
The token ID generation silently ignores potential errors from `rand.Read`:

```go
// jwt.go:201-204
func generateTokenID() string {
    b := make([]byte, 16)
    _, _ = rand.Read(b)  // Error ignored!
    return hex.EncodeToString(b)
}
```

**Why it's a vulnerability**:
If `rand.Read` fails, the function returns a hex string of whatever bytes were allocated (likely zeros or garbage), potentially producing predictable token IDs. While `crypto/rand` rarely fails, this is still a silent failure mode.

**Mitigating Factors**:
- `crypto/rand` is a blocking read from `/dev/urandom` on Unix systems and is extremely reliable
- Failure would result in predictable tokens only if the buffer was not filled

**Recommendation**:
Handle the error explicitly and fallback to a safer behavior:
```go
func generateTokenID() string {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil {
        // Fall back to panic or alternative secure source
        panic("crypto/rand unavailable: " + err.Error())
    }
    return hex.EncodeToString(b)
}
```

---

## Positive Security Findings

The following security controls are properly implemented:

### 1. Password Hashing - SECURE
- **File**: `internal/auth/password.go`
- bcrypt with cost factor 13 is used for password hashing
- Proper truncation protection (256 char max in handlers)
- Password strength validation with uppercase, lowercase, and digit requirements

### 2. API Key Hashing - SECURE
- **File**: `internal/auth/apikey.go`
- SHA-256 hashing for API keys (appropriate for HMAC-based lookups)
- 32-byte cryptographically random keys generated via `crypto/rand`

### 3. CSRF Protection - SECURE
- **File**: `internal/api/middleware/csrf.go`
- Double-submit cookie pattern implemented
- Bearer token and API key authentication bypass CSRF (correct behavior)
- `X-CSRF-Token` header validation with proper error responses

### 4. Webhook Signature Verification - SECURE
- **File**: `internal/webhooks/receiver.go`
- HMAC-SHA256 used for GitHub, Gitea, Gogs
- `hmac.Equal()` used for constant-time comparison (prevents timing attacks)
- GitLab token comparison uses constant-time comparison

### 5. IDOR Protection - SECURE
- **File**: `internal/api/handlers/helpers.go`
- `requireTenantApp()` helper validates app ownership before operations
- Returns "application not found" (not "unauthorized") to prevent enumeration
- All app-specific handlers use this helper

### 6. Session Management - SECURE
- HTTP-only cookies with `SameSite=Strict`
- Secure flag set when request is over HTTPS
- Refresh token rotation implemented
- Token revocation via BBolt storage

### 7. Stripe Webhook Security - SECURE
- **File**: `internal/api/handlers/stripe_webhook.go`
- Signature verification via Stripe SDK
- Rate limiting per IP (30/minute)
- Request body size limit (512KB)

---

## Recommendations Summary

1. **[MEDIUM]** Restrict JWT algorithms explicitly in `ValidateAccessToken` and `ValidateRefreshToken`
2. **[MEDIUM]** Increase minimum `secret_key` length to 32 characters
3. **[LOW]** Randomize default admin email or require explicit configuration
4. **[LOW]** Consider expanding `GeneratePassword` charset
5. **[LOW]** Handle errors in `generateTokenID` explicitly

---

*Report generated during Phase 2: HUNT of security audit*
