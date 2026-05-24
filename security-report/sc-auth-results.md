# SC-Auth Results: Authentication Flaw Detection

## Scan Scope
- Password hashing and password policy
- Brute force and per-account login throttling
- First-run super admin bootstrap behavior
- MFA/TOTP enrollment and login enforcement
- API-key authentication
- Frontend login and registration auth flows

## Current Findings

No active authentication findings remain in the scanned paths.

## Resolved Findings

### AUTH-001: Bootstrap Credential File Exposure
- **Status:** Resolved
- **Evidence:** First-run setup no longer writes a `.credentials` file. Generated bootstrap passwords are logged once, `MONSTER_ADMIN_PASSWORD` is unset after use, and `/etc/deploymonster/deploymonster.env` is removed if it contains bootstrap admin credentials.
- **Files:** `internal/auth/module.go:139-164`, `internal/auth/module.go:169-188`

### AUTH-002: MFA/TOTP Login Enforcement
- **Status:** Resolved
- **Evidence:** Login accepts `totp_code` and requires it when `user.TOTPEnabled` is true. TOTP enrollment, status, disable, and backup-code endpoints exist. The frontend login flow now prompts for an authentication code after the backend returns `TOTP code required`.
- **Files:** `internal/api/handlers/auth.go:227-241`, `internal/api/handlers/sessions.go:280-405`, `web/src/pages/Login.tsx`, `web/src/stores/auth.ts`

### AUTH-003: Password Policy Hardening
- **Status:** Resolved
- **Evidence:** `ValidatePasswordStrength` now defaults to 12 characters and requires uppercase, lowercase, digit, special character, and a common-password blocklist check. Frontend registration and settings copy now match the backend policy.
- **Files:** `internal/auth/password.go:76-117`, `web/src/pages/Register.tsx`, `web/src/pages/Settings.tsx`

### AUTH-004: TOTP Backup Code Persistence
- **Status:** Resolved
- **Evidence:** Backup-code hashes are stored via the database implementations, returned plaintext codes remain one-time display only, and login validation consumes a matching backup code so it cannot be reused.
- **Files:** `internal/auth/totp_service.go`, `internal/db/users.go`, `internal/db/postgres.go`, `internal/db/migrations/0007_totp_backup_codes.sql`

## Checks Passed
- Passwords hashed with bcrypt cost 13
- Login, register, and refresh routes have rate limiting
- Per-account login failure tracking is present
- Generic login errors prevent account enumeration on password failure
- TOTP validation fails closed if no validator or vault is configured
- TOTP backup codes are persisted as hashes and consumed on successful use
- API client no longer masks `/auth/login` 401 domain errors as session expiry
- First-run super admin email is generated when `MONSTER_ADMIN_EMAIL` is unset
- No plaintext password storage detected

## Summary
- **Open Findings:** 0
- **Resolved Findings:** 4
- **Overall Status:** No active authentication findings remain in the scanned paths.
