# sc-cors Results

## Summary
Cross-origin resource sharing security scan.

## Findings

### Finding: CORS-001
- **Title:** CORS Origins Configurable by Administrator
- **Severity:** Info
- **Confidence:** 90
- **File:** internal/api/router.go:89
- **Description:** CORS origins are loaded from `monster.yaml` `Server.CORSOrigins`. A misconfiguration (e.g., `*`) could allow unwanted cross-origin access.
- **Remediation:** Validate CORS origins at startup — reject `*` or empty lists in production mode.

## Positive Security Patterns Observed
- CORS middleware applied globally
- Credentials policy respects HTTPS configuration
- No `Access-Control-Allow-Origin: *` hardcoded
- WebSocket/SSE endpoints respect CORS origins via `SetAllowedOrigins`
