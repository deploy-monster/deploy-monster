# sc-websocket Results

## Summary
WebSocket security scan.

## Findings

### Finding: WS-001
- **Title:** WebSocket Origin Validation Configurable but Not Strictly Enforced
- **Severity:** Low
- **Confidence:** 70
- **File:** internal/api/ws/deploy_hub.go (assumed)
- **Description:** Deploy progress WebSocket uses `SetAllowedOrigins` from CORS config. If origins are misconfigured to `*`, cross-origin WebSocket connections would be accepted.
- **Remediation:** Reject `*` for WebSocket origins. Enforce explicit origin matching.

## Positive Security Patterns Observed
- Terminal uses SSE+POST pattern instead of raw WebSocket for broader compatibility and simpler auth
- Deploy progress WebSocket protected by `protected` middleware (JWT required)
- Auth middleware enforced before WebSocket upgrade
