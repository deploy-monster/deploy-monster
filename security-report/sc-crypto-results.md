# SC-Crypto Results — DeployMonster

## Summary
Cryptographic implementations are generally sound. Strong algorithms (AES-256-GCM, bcrypt cost 13, TLS 1.2+) are used. One medium-severity finding relates to a fallback to `math/big` when `crypto/rand` fails.

## Findings

### Finding: CRYPTO-001 — JWT signed with HS256 (Medium)
- **File:** `internal/auth/jwt.go:124`, `internal/auth/jwt.go:136`
- **Severity:** Medium
- **Confidence:** 90
- **Vulnerability Type:** CWE-327 (Broken Crypto)
- **Description:** JWT access and refresh tokens are signed with `jwt.SigningMethodHS256` (HMAC-SHA-256). The code correctly enforces a minimum secret length of 32 characters (256 bits), uses `WithValidMethods` to reject alg-confusion attacks, and implements key rotation with a 20-minute grace period. HS256 is acceptable for symmetric-key deployments, but asymmetric algorithms (RS256/ES256) would provide better separation between signing and verification keys.
- **Impact:** If the single shared secret is compromised, an attacker can forge tokens. The rotation mechanism limits the exposure window.
- **Remediation:** Consider migrating to RS256 or ES256 for production deployments where token verification happens on multiple services.
- **References:** https://cwe.mitre.org/data/definitions/327.html

### Finding: CRYPTO-002 — `crypto/rand` fallback to `math/big` (Medium)
- **File:** `internal/core/id.go:15-23`, `internal/core/id.go:32-36`
- **Severity:** Medium
- **Confidence:** 85
- **Vulnerability Type:** CWE-338 (Weak PRNG)
- **Description:** `GenerateID()` and `GenerateSecret()` fall back to `math/big` with `crypto/rand.Reader` if `rand.Read` fails. The fallback path still uses `crypto/rand.Reader` via `rand.Int`, so entropy quality remains high, but the pattern is unusual and could be misread as using weak randomness.
- **Impact:** Low — the fallback does not actually weaken randomness because it still reads from `crypto/rand.Reader`.
- **Remediation:** Simplify by panicking on `crypto/rand` failure (as done in `generateTokenID`) to eliminate any ambiguity.
- **References:** https://cwe.mitre.org/data/definitions/338.html

### Finding: CRYPTO-003 — SMTP `InsecureSkipVerify` is config-opt-in (Low)
- **File:** `internal/notifications/smtp.go:180`, `internal/notifications/smtp.go:231`
- **Severity:** Low
- **Confidence:** 95
- **Vulnerability Type:** CWE-295 (Certificate Validation Disabled)
- **Description:** `InsecureSkipVerify` is exposed as a config option (`smtp.insecure_skip_verify`) for self-signed/internal SMTP relays. It defaults to `false`, is gated by `MinVersion: tls.VersionTLS12`, and emits a structured warning log when enabled.
- **Impact:** Low — only affects deployments that explicitly opt in.
- **Remediation:** Ensure documentation warns against enabling this in production.
- **References:** https://cwe.mitre.org/data/definitions/295.html

### Finding: CRYPTO-004 — TLS 1.2 minimum enforced (Safe)
- **File:** `internal/ingress/module.go:263`, `internal/notifications/smtp.go:181`, `internal/notifications/smtp.go:232`
- **Severity:** Info
- **Description:** `MinVersion: tls.VersionTLS12` is set on all TLS configurations. No deprecated protocols (SSLv3, TLS 1.0, TLS 1.1) are enabled.

### Finding: CRYPTO-005 — AES-256-GCM with random nonces (Safe)
- **File:** `internal/secrets/vault.go:71-91`
- **Severity:** Info
- **Description:** The secrets vault uses AES-256-GCM with per-encryption random nonces generated via `io.ReadFull(rand.Reader, nonce)`. Argon2id is used for key derivation.

### Finding: CRYPTO-006 — bcrypt cost 13 for passwords and API keys (Safe)
- **File:** `internal/auth/password.go:10`, `internal/auth/apikey.go:19`
- **Severity:** Info
- **Description:** Both passwords and API keys are hashed with bcrypt at cost 13. Legacy hashes at cost 10 continue to verify. Constant-time comparison is provided by `bcrypt.CompareHashAndPassword`.

## Verdict
No critical or high cryptography issues found. Two medium findings are noted for hardening.
