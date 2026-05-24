# sc-api-security Results

## Summary
REST API security scan.

## Findings

### API-001: Large API Surface Increases Review Burden
- **Severity:** Info
- **Confidence:** 95
- **File:** `internal/api/router.go`
- **Description:** The API exposes 200+ routes, which increases the chance that future endpoints accidentally miss authentication, RBAC, CSRF, idempotency, or tenant scoping wrappers.
- **Remediation:** Keep router-level regression tests in sync with new route additions. Continue expanding table-driven checks for protected mutating routes and tenant-scoped route fuzzing.

### API-002: Public Validation Errors Are Intentionally Detailed
- **Severity:** Low
- **Confidence:** 70
- **Files:** `internal/api/handlers/auth.go`, `internal/api/handlers/sessions.go`, `internal/api/handlers/apps.go`
- **Description:** Some public/authentication and validation paths return detailed user-facing messages. Current reviewed examples are local validation errors rather than raw internal errors, and TOTP service error reflection was fixed. This remains a policy tradeoff between UX and enumeration resistance.
- **Remediation:** Keep raw service/store errors sanitized. For public auth endpoints, prefer generic credential failures and rate-limit/lockout controls over detailed account state.

## Positive Security Patterns Observed
- OpenAPI drift check validates router/spec alignment.
- Admin route table tests assert non-super-admin users receive 403.
- Viewer-role tests cover key mutating routes that require permissions.
- Cross-tenant mutation tests exercise app-scoped mutation routes.
- API route fuzzing checks tenant-scoped GET route behavior.
- ETag caching applies to idempotent GETs.
- Idempotency middleware exists for mutation endpoints.
- Consistent JSON response format and `/api/v1/` versioning.
- Request timeout and 10 MB body limit are configured.
