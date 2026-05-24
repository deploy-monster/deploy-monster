# sc-header-injection Results

## Summary
HTTP header injection security scan.

## Findings

No active HTTP header injection findings are verified in the current working tree.

### HI-001: Host Header Validation Prevents Header Injection
- **Severity:** Info
- **Confidence:** 90
- **File:** `internal/ingress/module.go`
- **Description:** `isValidRedirectHost` explicitly rejects hosts containing `\r` or `\n`, which prevents HTTP response splitting and header injection via the Host header before HTTPS redirects are emitted.
- **Remediation:** No action required.

### HI-002: User-Derived Header Values Are Sanitized Or Constrained
- **Severity:** Info
- **Confidence:** 85
- **Files:** `internal/api/handlers/helpers.go`, `internal/api/handlers/redirects.go`
- **Description:** Download filenames flow through `safeFilename` before `Content-Disposition`. Redirect rules now reject CRLF in source/destination values before they can be persisted for downstream redirect behavior.
- **Remediation:** No action required.

### HI-003: SMTP Header Injection Hardened
- **Severity:** Low
- **Confidence:** 90
- **File:** `internal/notifications/smtp.go`
- **Status:** Resolved
- **Description:** Email subject, recipient header, and configured sender display name now reject CR, LF, and NUL bytes before RFC 5322 headers are assembled. The SMTP envelope recipient is also taken from the parsed email address rather than the raw header string.
- **Remediation:** No action required.

## Positive Security Patterns Observed
- `middleware.SecurityHeaders` sets standard security headers.
- `X-Accel-Buffering: no` is used for SSE streams.
- Host header is validated before redirect use.
- Content-Disposition filenames are normalized to alphanumeric, dot, hyphen, and underscore.
- SMTP notification headers reject CR/LF/NUL injection characters.
