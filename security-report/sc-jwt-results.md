# SC-JWT Results: JWT Implementation Flaw Detection

## Scan Scope
- Algorithm confusion (alg=none, RS256->HS256)
- Weak signing secrets
- Missing expiration/audience/issuer validation
- Token storage (cookies vs localStorage)
- Key rotation and previous key grace period
- JTI revocation

## Findings

### JWT-001: Missing Audience (aud) and Issuer (iss) Validation
- **Severity:** Low
- **Confidence:** 75
- **File:** `internal/auth/jwt.go:108-147` (GenerateTokenPair), `internal/auth/jwt.go:152-175` (ValidateAccessToken)
- **Vulnerability Type:** CWE-345 (Insufficient Verification of Data Authenticity)
- **Description:** JWT claims do not include `aud` (audience) or `iss` (issuer) fields, and validation does not check them. In multi-service or multi-tenant deployments, a token issued by one DeployMonster instance could potentially be replayed against another if they share the same secret key. Similarly, there is no domain scoping.
- **Impact:** Token replay across environments or services that share secrets.
- **Remediation:** Add `aud` (e.g., "deploymonster-api") and `iss` (e.g., hostname or deployment ID) to claims generation, and validate them during `ValidateAccessToken` and `ValidateRefreshToken`.
- **References:** https://cwe.mitre.org/data/definitions/345.html

### JWT-002: Key Rotation Grace Period Could Be Exploited During Rapid Rotation
- **Severity:** Low
- **Confidence:** 60
- **File:** `internal/auth/jwt.go:12-17`, `internal/auth/jwt.go:75-85`
- **Vulnerability Type:** CWE-347 (Improper Verification of Cryptographic Signature)
- **Description:** `RotationGracePeriod` is 20 minutes. If an attacker compromises a secret and the admin rotates the key, the attacker can still forge tokens with the old secret for up to 20 minutes. While this is a necessary trade-off for zero-downtime rotation, there is no emergency "revoke all previous keys immediately" API or flag.
- **Impact:** Brief window where a compromised key remains valid after rotation.
- **Remediation:** Provide an admin API endpoint or configuration flag to immediately invalidate all previous keys (accept only the active key), accepting that this may cause brief 401s for in-flight requests.
- **References:** https://cwe.mitre.org/data/definitions/347.html

## Checks Passed
- Algorithm confusion defended: `jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name})` used
- Belt-and-suspenders check: `token.Method != jwt.SigningMethodHS256` enforced
- Minimum secret length enforced (32 chars / 256 bits) with panic on violation
- Access tokens expire after 15 minutes
- Refresh tokens expire after 7 days
- Token IDs (JTI) are cryptographically random (16 bytes from `crypto/rand`)
- Access token revocation list implemented via BBolt
- Previous secret keys are purged after `RotationGracePeriod`
- Tokens stored in `HttpOnly` cookies, not localStorage
- No `alg=none` acceptance
- No weak/default secrets in code (env-driven)

## Summary
- **Total Findings:** 2 (2 Low)
- **Overall Status:** Issues found; see above for details.
