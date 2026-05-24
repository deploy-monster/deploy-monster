# sc-websocket Results

## Summary
WebSocket security scan.

## Findings

No active WebSocket findings remain in the scanned API surface.

## Resolved Findings

### Finding: WS-001
- **Title:** WebSocket Origin Validation Configurable but Not Strictly Enforced
- **Severity:** Low
- **Confidence:** 70
- **File:** internal/api/ws/deploy.go
- **Status:** Resolved
- **Description:** Deploy progress WebSocket uses `SetAllowedOrigins` from CORS config. Previously, `*` accepted any Origin during WebSocket upgrade.
- **Remediation:** WebSocket origin checks now reject empty origins and wildcard `*`, and only allow exact configured origins.

## Positive Security Patterns Observed
- Terminal uses SSE+POST pattern instead of raw WebSocket for broader compatibility and simpler auth
- Deploy progress WebSocket protected by `protected` middleware (JWT required)
- Auth middleware enforced before WebSocket upgrade
- Deploy progress WebSocket rejects wildcard origins and requires exact origin matches
