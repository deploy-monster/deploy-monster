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
- **File:** internal/api/router.go:128-130
- **Description:** Login and register share the global 120/min per-IP limit, which is high enough to allow significant credential stuffing volume.
- **Remediation:** Reduce per-account login rate limit (e.g., 5 attempts per 15 minutes per account) independently of per-IP limits.

## Positive Security Patterns Observed
- Global per-IP rate limit: 120 req/min
- Per-tenant rate limit: 100 req/min
- Auth-specific limits: login 120/min, register 120/min, refresh 5/min
- Rate limiter uses BBolt for distributed state (works across replicas)
- Prefix-based rate limiting (`/api/`, `/hooks/`)
