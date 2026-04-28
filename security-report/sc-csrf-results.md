# sc-csrf Results

## Summary
Cross-site request forgery security scan.

## Findings

### Finding: CSRF-001
- **Title:** SameSite=None on Authentication Cookies
- **Severity:** Medium
- **Confidence:** 85
- **File:** internal/api/handlers/auth.go (setTokenCookies)
- **Description:** Cookies are set with `SameSite=None` when HTTPS is enabled. While `Secure` is also set, this is the most permissive setting and increases CSRF surface area if origin validation is imperfect.
- **Remediation:** Default to `SameSite=Lax` unless cross-site API access is explicitly required.

## Positive Security Patterns Observed
- `middleware.CSRFProtect` is present in the global middleware chain
- `CSRFProtect` applied to all routes
- Cookies use `HttpOnly` flag
- `Secure` flag set on cookies when HTTPS is enabled
- Token-based auth (Bearer JWT) for API requests is inherently CSRF-resistant for XHR/fetch
