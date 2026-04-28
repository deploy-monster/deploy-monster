# sc-api-security Results

## Summary
REST API security scan.

## Findings

### Finding: API-001
- **Title:** Large API Surface Increases Attack Surface
- **Severity:** Info
- **Confidence:** 95
- **File:** internal/api/router.go
- **Description:** 200+ endpoints create a large attack surface. New endpoints may inadvertently bypass security controls if developers forget middleware wrappers.
- **Remediation:** Maintain the CI test that walks all admin routes and asserts 403. Consider adding a similar test for all protected routes to ensure none are accidentally public.

### Finding: API-002
- **Title:** Some Public Endpoints Return Detailed Error Messages
- **Severity:** Low
- **Confidence:** 60
- **File:** internal/api/handlers/ (various)
- **Description:** Public endpoints like auth may return detailed validation errors that could aid attackers.
- **Remediation:** Use generic error messages for public endpoints and log details server-side.

## Positive Security Patterns Observed
- OpenAPI spec served at `/api/v1/openapi.json`
- ETag caching on idempotent GETs
- Idempotency middleware for mutation endpoints
- Consistent JSON response format
- API versioning (`/api/v1/`)
- Request timeout (30s) and body limit (10MB)
