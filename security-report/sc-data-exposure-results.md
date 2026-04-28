# SC-Data-Exposure Results — DeployMonster

## Summary
Sensitive data exposure controls are largely effective. Error responses are sanitized, secret values are masked in API responses, and debug mode is not enabled. A few low-severity observations are noted around error-message detail and internal stack traces in logs.

## Findings

### Finding: EXPOSE-001 — Validation errors expose internal error text (Low)
- **File:** `internal/api/handlers/apps.go:68`, `internal/api/handlers/app_rename.go:35`, `internal/api/handlers/branding.go:40`, `internal/api/handlers/mcp_endpoint.go:53`, `internal/api/handlers/sessions.go:151`
- **Severity:** Low
- **Confidence:** 80
- **Vulnerability Type:** CWE-209 (Error Message Information Leak)
- **Description:** Several handlers return `err.Error()` directly in 400 Bad Request responses for validation failures (e.g., `validateAppName`, `validateCustomCSS`, `ValidatePasswordStrength`). These errors are user-facing validation messages, not internal system errors, but the pattern could be misused if an underlying library returns internal details.
- **Impact:** Low — current messages are safe, but the pattern is fragile.
- **Remediation:** Audit all `writeError(w, 400, err.Error())` call sites to ensure no internal error paths leak through.
- **References:** https://cwe.mitre.org/data/definitions/209.html

### Finding: EXPOSE-002 — Stack traces logged internally on panic (Low)
- **File:** `internal/api/handlers/helpers.go:248-251`, `internal/core/safego.go`
- **Severity:** Low
- **Confidence:** 90
- **Vulnerability Type:** CWE-532 (Log File Information Disclosure)
- **Description:** Panic recovery middleware logs `debug.Stack()` internally. This is correct behavior (stack traces must not reach clients), but log files containing stack traces should be protected with restrictive permissions.
- **Impact:** Low — stack traces remain server-side.
- **Remediation:** Ensure log files are written with `0600` or `0640` permissions and are not world-readable.
- **References:** https://cwe.mitre.org/data/definitions/532.html

### Finding: EXPOSE-003 — Env var export returns plaintext values (Info)
- **File:** `internal/api/handlers/env_import.go:114-119`
- **Severity:** Info
- **Confidence:** 95
- **Vulnerability Type:** CWE-200 (Information Disclosure)
- **Description:** The `Export` endpoint returns environment variables in `.env` format as a downloadable attachment. This is by design (authenticated users exporting their own app env vars), but the endpoint should require explicit authorization.
- **Impact:** Info — authenticated endpoint; no broader exposure.
- **Remediation:** Ensure RBAC checks restrict this to app owners/admins.

### Finding: EXPOSE-004 — Secret values masked in API responses (Safe)
- **File:** `internal/api/handlers/envvars.go:43-48`, `internal/api/handlers/env_compare.go:66-68`, `internal/api/handlers/secrets.go:129-148`
- **Severity:** Info
- **Description:** Env var values are masked (`maskValue`, `maskShort`). Secret list endpoints return metadata only, never encrypted values. Webhook secrets are stored as SHA-256 hashes and only shown once at creation.

### Finding: EXPOSE-005 — No debug mode in production (Safe)
- **File:** `internal/core/config.go:411-436`
- **Severity:** Info
- **Description:** No `DEBUG = True` equivalent. Log level defaults to `info` and is configurable via `MONSTER_LOG_LEVEL`. Pprof is opt-in (`enable_pprof`).

### Finding: EXPOSE-006 — `.credentials` file with 0600 perms (Safe)
- **File:** `internal/auth/module.go:128`
- **Severity:** Info
- **Description:** Auto-generated admin credentials are written to `.credentials` with `0600` permissions and are gitignored.

## Verdict
No critical or high data-exposure issues found. One low-severity pattern (err.Error() in 400s) should be reviewed.
