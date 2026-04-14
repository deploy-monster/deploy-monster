# Session Management Security Scan Report

**Scanner:** sc-session  
**Date:** 2026-04-14  
**Target:** DeployMonster GO Codebase

---

## Summary

Found **6 session management vulnerabilities** ranging from Medium to High severity.

| ID | Severity | Description |
|----|----------|-------------|
| SESS-001 | High | Access tokens cannot be revoked |
| SESS-002 | Medium | Session fixation vulnerability |
| SESS-003 | Medium | No concurrent session limit |
| SESS-004 | Medium | Password change doesn't invalidate other sessions |
| SESS-005 | Low | Missing active session listing/management |
| SESS-006 | Low | Refresh token rotation grace period risk |

---

## Detailed Findings

### SESS-001: Access Tokens Cannot Be Revoked (High)

**File:** `internal/api/middleware/middleware.go` line 166, 178  
**CWE:** CWE-384 / CWE-613  

Access tokens (15-minute lifetime) are validated without checking a revocation list. Stolen access tokens remain valid until expiration even after logout.

**Remediation:** Store issued access tokens with JTI in a short-lived cache and check revocation list in `RequireAuth` middleware.

---

### SESS-002: Session Fixation Vulnerability (Medium)

**File:** `internal/api/handlers/auth.go` line 118-176  
**CWE:** CWE-384  

The login endpoint does not regenerate session identifiers upon successful authentication.

**Remediation:** Clear any existing authentication cookies at the start of login.

---

### SESS-003: No Concurrent Session Limit (Medium)

**File:** `internal/api/handlers/auth.go` line 118-176  
**CWE:** CWE-287 / CWE-362  

Users can create unlimited concurrent sessions without any restriction.

**Remediation:** Implement session tracking per user with configurable maximum concurrent sessions.

---

### SESS-004: Password Change Does Not Invalidate Other Sessions (Medium)

**File:** `internal/api/handlers/sessions.go` line 104-156  
**CWE:** CWE-613  

Changing password does not invalidate existing refresh tokens or sessions.

**Remediation:** On password change, revoke ALL refresh tokens for the user.

---

### SESS-005: Missing Active Session Listing/Management (Low)

**File:** `internal/api/handlers/sessions.go`  
**CWE:** CWE-306  

No endpoint to list active sessions, view metadata, or terminate specific sessions.

---

### SESS-006: Refresh Token Rotation Grace Period Risk (Low)

**File:** `internal/auth/jwt.go` line 11-15, 69-76  
**CWE:** CWE-384  

The `RotationGracePeriod` (1 hour) allows previous signing keys to validate tokens, extending exposure window.

---

## Positive Security Controls

1. **HttpOnly Cookies**: Access and refresh tokens use httpOnly flag
2. **Secure Flag**: Cookies use Secure flag based on request scheme
3. **SameSite Strict**: Cookies use SameSiteStrictMode
4. **Refresh Token Rotation**: Old refresh tokens revoked when new ones issued
5. **Token Expiration**: Short-lived access tokens (15 min) and longer refresh tokens (7 days)
6. **CSRF Protection**: Double-submit cookie pattern
7. **Key Rotation Support**: Graceful key rotation with previous key validation
