# SC-Session Results: Session Management Flaw Detection

## Scan Scope
- Session fixation prevention
- Cookie security attributes (HttpOnly, Secure, SameSite)
- Session invalidation on logout and password change
- Concurrent session limiting
- Refresh token rotation and access token revocation

## Findings

### SESS-001: SameSite=None on Cookies Without Strict Origin Validation
- **Severity:** Medium
- **Confidence:** 85
- **File:** `internal/api/handlers/auth.go:49-76`
- **Vulnerability Type:** CWE-1275 (Sensitive Cookie with Improper SameSite Attribute)
- **Description:** `setTokenCookies` sets `SameSite=NoneMode` when `secure` is true (HTTPS). While `Secure` is also set in this case, the code does not validate that the request origin matches the expected application domain. In deployments behind misconfigured reverse proxies or with spoofed `X-Forwarded-Proto: https`, an attacker could trick a browser into sending cookies in a cross-site context.
- **Impact:** CSRF attacks or session hijacking via cross-site requests if the origin validation is missing at the ingress layer.
- **Remediation:**
  1. Default to `SameSite=LaxMode` unless cross-site API usage is explicitly required.
  2. If `SameSite=None` is necessary, enforce strict origin allowlisting in the CORS middleware (already partially present) and document the requirement for a correctly configured reverse proxy.
  3. Consider making the cookie SameSite mode configurable via environment variable.
- **References:** https://cwa.mitre.org/data/definitions/1275.html

### SESS-002: No Absolute Session Timeout for Refresh Tokens
- **Severity:** Low
- **Confidence:** 70
- **File:** `internal/auth/jwt.go:71`
- **Vulnerability Type:** CWE-613 (Insufficient Session Expiration)
- **Description:** Refresh tokens are valid for 7 days with no absolute maximum lifetime or sliding-session cap. A stolen refresh token can be rotated indefinitely (each rotation produces a new valid refresh token) as long as the user does not explicitly log out or change their password. There is no "maximum session duration" beyond which the user must re-authenticate.
- **Impact:** Long-lived stolen sessions that persist even if the user is inactive.
- **Remediation:** Implement an absolute session timeout (e.g., 30 days) stored alongside the refresh token JTI. During validation, reject tokens whose absolute creation time exceeds the platform-wide maximum. Alternatively, track the original session creation time and enforce a hard cutoff.
- **References:** https://cwe.mitre.org/data/definitions/613.html

## Checks Passed
- Session fixation prevention: `clearTokenCookies` is called before login
- Access token revocation on logout and refresh (SESS-001 regression tests pass)
- Password change invalidates all refresh tokens (`revokeAllUserSessions`) and the current access token
- Concurrent session limit enforced (max 10 sessions per user)
- `LogoutAll` endpoint revokes all sessions for the current user
- `ListSessions` endpoint allows users to review active sessions
- Cookies are `HttpOnly` and `Secure` (when HTTPS)
- Refresh token rotation implemented (old refresh token revoked on rotation)

## Summary
- **Total Findings:** 2 (1 Medium, 1 Low)
- **Overall Status:** Issues found; see above for details.
