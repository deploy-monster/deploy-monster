# sc-clickjacking Results

## Summary
Clickjacking security scan.

## Findings

No active clickjacking findings are verified in the current working tree.

## Resolved / Revalidated Items

### CJ-001: Missing Frame Protection Headers
- **Previous Severity:** Medium
- **Status:** RESOLVED
- **Files:** `internal/api/middleware/security_headers.go`, `internal/api/spa.go`
- **Notes:** API/security middleware sets `X-Frame-Options: DENY` and CSP `frame-ancestors 'none'`. The SPA index response also sets CSP with `frame-ancestors 'none'`.

## Positive Security Patterns Observed
- API responses receive `X-Frame-Options: DENY`.
- API responses receive CSP with `frame-ancestors 'none'`.
- SPA HTML receives nonce-backed CSP with `frame-ancestors 'none'`.
- HSTS is sent on TLS responses.
