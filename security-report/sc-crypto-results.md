# SC-Crypto Results — DeployMonster

## Summary
Cryptographic implementations are generally sound. Strong primitives are used for reviewed paths: AES-256-GCM with random nonces, Argon2id for vault key derivation, bcrypt cost 13 for passwords/API keys, HS256 JWTs with 256-bit minimum secret length, issuer/audience validation, and TLS 1.2+.

## Findings

### CRYPTO-001 — JWT Uses HS256 Symmetric Signing
- **Severity:** Low-Medium
- **Confidence:** 90
- **File:** `internal/auth/jwt.go`
- **Vulnerability Type:** CWE-327 (Broken/Risky Crypto Choice)
- **Description:** JWT access and refresh tokens are signed with HS256. The implementation correctly enforces a minimum secret length, rejects algorithm confusion, validates issuer/audience, and supports bounded key rotation. HS256 is acceptable for a single self-hosted control plane, but asymmetric signing would provide cleaner key separation if token verification is ever distributed across independently operated services.
- **Impact:** If the shared signing secret is compromised, an attacker can forge tokens until rotation/revocation takes effect.
- **Remediation:** Keep HS256 for simple self-hosted deployments, but plan RS256/ES256 support for distributed verification or multi-service deployments.

### CRYPTO-002 — SMTP InsecureSkipVerify Is Localhost-Only Opt-In
- **Severity:** Info
- **Confidence:** 95
- **File:** `internal/notifications/smtp.go`
- **Vulnerability Type:** CWE-295 (Certificate Validation Disabled)
- **Description:** `InsecureSkipVerify` exists for SMTP, but validation only permits it for localhost/loopback or `.local` hosts and logs a warning when enabled. Production SMTP servers must use valid TLS certificates.
- **Impact:** Low; the option is constrained to local/internal relay scenarios.
- **Remediation:** Keep documentation clear that this is for local relays only.

## Resolved / Revalidated Items

### CRYPTO-003 — crypto/rand fallback to math/big
- **Previous Severity:** Medium
- **Status:** RESOLVED / NOT CURRENT
- **File:** `internal/core/id.go`
- **Notes:** `GenerateID` and `GenerateSecret` now panic on `crypto/rand` failure instead of falling back. `GeneratePassword` uses `crypto/rand.Reader` through `rand.Int`; it does not use a weak PRNG.

## Safe Patterns Observed
- TLS configurations enforce `tls.VersionTLS12` minimum.
- Secrets vault uses AES-256-GCM with per-encryption random nonces.
- Vault key derivation uses Argon2id.
- Passwords and API keys are hashed with bcrypt cost 13.
- JWT validation uses `jwt.WithValidMethods`, issuer, and audience checks.
- Previous JWT keys are purged after a 20-minute grace period.

## Verdict
No critical or high cryptography issues found. Remaining items are architectural/operational hardening, not immediate vulnerabilities.
