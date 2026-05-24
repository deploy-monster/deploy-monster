# sc-rate-limiting Results

## Summary
Rate limiting security scan.

## Findings

### Finding: RL-001
- **Title:** Global Rate Limit Excludes SPA Assets but Includes All API Traffic
- **Severity:** Info
- **Confidence:** 90
- **File:** internal/api/router.go:69
- **Description:** Rate limiting is correctly scoped to `/api/` and `/hooks/` prefixes. Static SPA assets are excluded to prevent browser sessions from exhausting limits.
- **Remediation:** No action required. Current implementation is correct.

### Finding: RL-002
- **Title:** Login/Register Rate Limits Are Generous
- **Severity:** Low
- **Confidence:** 75
- **File:** internal/api/handlers/auth.go:627-717
- **Status:** Resolved
- **Description:** Login still has a generous per-IP limiter for CI/browser compatibility, but successful code now also enforces per-account lockout: after 5 failed attempts, the account is locked for 15 minutes and responses include rate-limit headers.
- **Remediation:** No action required. Keep both per-IP and per-account controls in place.

## Positive Security Patterns Observed
- Global per-IP rate limit: 120 req/min
- Per-tenant rate limit: 100 req/min
- Auth-specific limits: login 120/min, register 120/min, refresh 5/min
- Per-account login lockout: 5 failed attempts / 15 minutes
- Rate limiter uses SQLite-backed KV storage for durable state
- Prefix-based rate limiting (`/api/`, `/hooks/`)
