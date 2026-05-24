# sc-csrf Results

## Summary
Cross-site request forgery security scan.

## Findings

No active CSRF findings remain in the scanned API middleware surface.

## Resolved Findings

### Finding: CSRF-001
- **Title:** Legacy SameSite=None Authentication Cookies
- **Severity:** Medium
- **Confidence:** 85
- **File:** internal/api/handlers/auth.go (setTokenCookies)
- **Status:** Resolved
- **Description:** Authentication cookies now use `SameSite=Strict`, `HttpOnly`, and transport-gated `Secure`. The previous SameSite=None behavior is no longer present.
- **Remediation:** N/A

### Finding: CSRF-002
- **Title:** Token Cookie Could Bypass CSRF When CSRF Cookie Was Missing
- **Severity:** Low
- **Confidence:** 80
- **File:** internal/api/middleware/csrf.go
- **Status:** Resolved
- **Description:** Mutating requests with an auth token cookie but without a CSRF cookie previously passed through CSRF middleware. This could affect migrated or partially cleared browser sessions.
- **Remediation:** `CSRFProtect` now rejects mutating cookie-auth requests when `dm_access` or `dm_refresh` is present but `__Host-dm_csrf` is absent or empty. Bearer/API-key requests remain exempt.

## Positive Security Patterns Observed
- `middleware.CSRFProtect` is present in the global middleware chain
- `CSRFProtect` applied to all routes
- Cookies use `HttpOnly` flag
- Auth cookies use `SameSite=Strict`
- `Secure` flag set on cookies when HTTPS is used
- Token-based auth (Bearer JWT) for API requests is inherently CSRF-resistant for XHR/fetch
