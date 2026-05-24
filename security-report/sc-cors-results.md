# sc-cors Results

## Summary
Cross-origin resource sharing security scan.

## Findings

### Finding: CORS-001
- **Title:** CORS Origins Configurable by Administrator
- **Severity:** Info
- **Confidence:** 90
- **File:** internal/api/router.go:89
- **Description:** CORS origins are loaded from `monster.yaml` `Server.CORSOrigins`. Public mode (`""` or `*`) emits `Access-Control-Allow-Origin: *` without `Access-Control-Allow-Credentials`, so browsers reject credentialed cross-origin cookie requests.
- **Remediation:** Keep production deployments on explicit origin allowlists when browser credentialed access is required. WebSocket upgrades now reject wildcard origins separately.

## Positive Security Patterns Observed
- CORS middleware applied globally
- Credentials policy respects HTTPS configuration
- Wildcard public mode is not combined with `Access-Control-Allow-Credentials`
- WebSocket deploy progress rejects wildcard origins and requires exact origin matches
