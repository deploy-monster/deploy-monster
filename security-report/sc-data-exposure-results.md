# SC-Data-Exposure Results — DeployMonster

## Summary
Sensitive data exposure controls are largely effective. Error responses are sanitized, secret values are masked in API responses, and debug mode is not enabled. A few low-severity observations are noted around intentional validation detail and internal stack traces in logs.

## Findings

### Finding: EXPOSE-001 — Validation errors expose detailed user-facing text (Low)
- **File:** `internal/api/handlers/apps.go:70`, `internal/api/handlers/app_rename.go:35`, `internal/api/handlers/branding.go:40`, `internal/api/handlers/mcp_endpoint.go:53`, `internal/api/handlers/sessions.go:152`
- **Severity:** Low
- **Confidence:** 80
- **Vulnerability Type:** CWE-209 (Error Message Information Leak)
- **Description:** Several handlers return `err.Error()` directly in 400 Bad Request responses for local validation failures (e.g., `validateAppName`, `validateCustomCSS`, `ValidatePasswordStrength`, webhook URL validation, certificate domain validation). These errors are user-facing validation messages, not internal system errors. A TOTP handler path that could expose wrapped store/vault errors was fixed during this pass.
- **Impact:** Low — remaining messages are validation-oriented, but the pattern should remain constrained to known-safe validators.
- **Remediation:** Keep `writeError(w, 400, err.Error())` limited to local validators. For service calls, use explicit allowlists or generic server-side messages as done for TOTP.
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

### Finding: EXPOSE-006 — Legacy `.credentials` file behavior removed (Safe)
- **File:** `internal/auth/module.go`
- **Severity:** Info
- **Description:** Current first-run setup no longer writes a runtime `.credentials` file. Generated bootstrap passwords are logged once, `MONSTER_ADMIN_EMAIL` and `MONSTER_ADMIN_PASSWORD` are unset after use, and `/etc/deploymonster/deploymonster.env` is removed if it contains bootstrap admin credentials. The installer may use that env file transiently for non-interactive bootstrap.

### Resolved: EXPOSE-007 — TOTP service errors leaked wrapped internal details
- **File:** `internal/api/handlers/sessions.go`
- **Severity:** Low
- **Status:** Resolved
- **Description:** TOTP enrollment, confirmation, and disable paths previously returned raw service errors as 400 responses. Wrapped internal failures such as `get user: <store error>` could disclose backend implementation details.
- **Remediation:** TOTP handlers now allowlist expected user-facing states (`invalid TOTP code`, already enabled, not enabled, enrollment not started) and return generic 500 messages for internal failures. Regression tests verify internal store errors are not reflected.

## Verdict
No critical or high data-exposure issues found. Remaining direct `err.Error()` 400 responses are constrained to validation messages; service-backed TOTP error leakage has been fixed.
