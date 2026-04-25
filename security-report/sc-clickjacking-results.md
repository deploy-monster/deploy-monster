# sc-clickjacking Results

## Summary
Clickjacking security scan.

## Findings

### Finding: CJ-001
- **Title:** No X-Frame-Options or CSP frame-ancestors in Middleware Chain
- **Severity:** Medium
- **Confidence:** 75
- **File:** internal/api/router.go:76-94
- **Description:** The `middleware.SecurityHeaders` is present but no explicit `X-Frame-Options` or `Content-Security-Policy frame-ancestors` was confirmed in the middleware chain. The React SPA could be embedded in a malicious iframe.
- **Remediation:** Add `X-Frame-Options: DENY` or `SAMEORIGIN` to the security headers middleware. Add `Content-Security-Policy: frame-ancestors 'self'`.

## Positive Security Patterns Observed
- `middleware.SecurityHeaders` exists in the chain
- HSTS header present on HTTPS redirects
