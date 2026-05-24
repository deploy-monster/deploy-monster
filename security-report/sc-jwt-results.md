# SC-JWT Results: JWT Implementation Flaw Detection

## Scan Scope
- Algorithm confusion (`alg=none`, RS256->HS256)
- Weak signing secrets
- Expiration, audience, and issuer validation
- Token storage
- Key rotation and previous-key grace period
- JTI revocation

## Findings

### JWT-001: Emergency Previous-Key Revocation Not Exposed Operationally
- **Severity:** Low
- **Confidence:** 65
- **File:** `internal/auth/jwt.go`
- **Description:** `RotationGracePeriod` is 20 minutes, and `RevokeAllPreviousKeys` exists in the JWT service. However, no reviewed admin endpoint or documented runtime operation exposes emergency revocation of previous keys if a signing key is known compromised.
- **Impact:** A compromised previous key may remain accepted until the grace period expires unless the process is restarted/reconfigured to drop previous keys.
- **Remediation:** Expose an authenticated super-admin operation or documented operational flag to call `RevokeAllPreviousKeys` and force active-key-only validation during incident response.

## Resolved / Revalidated Items

### JWT-002: Missing Audience (aud) and Issuer (iss) Validation
- **Previous Severity:** Low
- **Status:** RESOLVED
- **File:** `internal/auth/jwt.go`
- **Notes:** Access and refresh tokens include issuer/audience claims, and validation uses `jwt.WithIssuer(tokenIssuer)` and `jwt.WithAudience(tokenAudience)`.

### JWT-003: Short JWT Secret Panic
- **Previous Severity:** Medium
- **Status:** RESOLVED
- **File:** `internal/auth/jwt.go`
- **Notes:** `NewJWTService` returns an error for secrets shorter than 32 characters rather than panicking.

## Checks Passed
- Algorithm confusion defended with `jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name})`.
- Belt-and-suspenders method check enforces HS256.
- Minimum secret length enforced at 32 characters / 256 bits.
- Access tokens expire after 15 minutes.
- Refresh tokens expire after 7 days and have a 30-day absolute session timeout.
- Token IDs are cryptographically random.
- Access token revocation list is implemented via SQLite-backed KV storage.
- Previous secret keys are purged after `RotationGracePeriod`.
- Tokens are stored in `HttpOnly` cookies, not localStorage.
- No `alg=none` acceptance.
- No weak/default JWT secrets in source.

## Summary
- **Total Findings:** 1 Low
- **Overall Status:** Low residual operational hardening remains.
