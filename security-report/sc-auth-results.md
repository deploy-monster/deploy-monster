# SC-Auth Results: Authentication Flaw Detection

## Scan Scope
- Password hashing: bcrypt with cost 13
- Brute force protection: per-IP rate limiting on login/register/refresh
- Timing-safe comparison: bcrypt for passwords and API keys
- Account enumeration: generic error messages on login
- First-run super admin creation with auto-generated credentials
- API key authentication via X-API-Key header

## Findings

### AUTH-001: Auto-Generated Super Admin Credentials Written to Unencrypted File
- **Severity:** High
- **Confidence:** 95
- **File:** `internal/auth/module.go:126-135`
- **Vulnerability Type:** CWE-798 (Hardcoded Credentials)
- **Description:** During first-run setup, if no `MONSTER_ADMIN_PASSWORD` env var is set, a 16-character password is auto-generated and written to a `.credentials` file with `0600` permissions. While the permissions are restrictive, the file is stored on disk next to the database path. There is no mechanism to automatically delete or rotate this file after the admin first logs in, leaving credentials at rest indefinitely.
- **Impact:** An attacker with filesystem read access (e.g., via path traversal, backup extraction, or host compromise) can obtain the super admin password and gain full platform control.
- **Remediation:**
  1. Print the credentials to stdout/stderr during first-run setup instead of writing to a file (or use both with a strong warning).
  2. Add a startup task that deletes the `.credentials` file once the super admin has logged in at least once.
  3. Alternatively, force the user to set `MONSTER_ADMIN_PASSWORD` via environment variable before first run and refuse to start if unset.
- **References:** https://cwe.mitre.org/data/definitions/798.html

### AUTH-002: No Multi-Factor Authentication (MFA/TOTP)
- **Severity:** Medium
- **Confidence:** 90
- **File:** `internal/api/handlers/auth.go` (Login handler)
- **Vulnerability Type:** CWE-308 (Use of Single-factor Authentication)
- **Description:** The login flow accepts only email and password. The `users` table has a `totp_enabled` column (seen in `internal/db/users.go`), but there is no TOTP verification step during login, no enrollment endpoint, and no backup code mechanism. This leaves all accounts, including the super admin, protected by a single factor.
- **Impact:** Credential stuffing, phishing, or password reuse attacks can lead directly to account takeover without a second factor to mitigate.
- **Remediation:** Implement TOTP enrollment (QR code generation), TOTP verification during login when `totp_enabled` is true, and backup codes for account recovery.
- **References:** https://cwe.mitre.org/data/definitions/308.html

### AUTH-003: Weak Password Policy (8 chars, no special char requirement)
- **Severity:** Medium
- **Confidence:** 85
- **File:** `internal/auth/password.go:27-50`
- **Vulnerability Type:** CWE-521 (Weak Password Requirements)
- **Description:** `ValidatePasswordStrength` enforces only length >= 8, one uppercase, one lowercase, and one digit. It does not require special characters and allows common patterns like `Password1` which would pass validation. The 8-character minimum is below modern NIST/OWASP recommendations (12+ characters).
- **Impact:** Users can set easily guessable passwords, increasing susceptibility to brute-force and dictionary attacks.
- **Remediation:** Increase minimum length to 12 (or 16 for admin accounts). Add a check against a common-password dictionary (e.g., Have I Been Pwned API or a local top-100k list). Consider using zxcvbn for entropy scoring.
- **References:** https://cwe.mitre.org/data/definitions/521.html

## Checks Passed
- Passwords hashed with bcrypt (adaptive cost, cost=13)
- Brute force protection present: login 120/min, register 120/min, refresh 5/min
- Timing-safe comparison used via `bcrypt.CompareHashAndPassword`
- Generic login errors ("invalid credentials") prevent account enumeration
- No plaintext password storage detected
- No hardcoded credentials in source code (env vars used)

## Summary
- **Total Findings:** 3 (1 High, 2 Medium)
- **Overall Status:** Issues found; see above for details.
