# sc-open-redirect Results

## Summary
Open redirect security scan.

## Findings

No active platform-level open redirect findings are verified in the current working tree.

### OR-001: Ingress HTTPS Redirect Validates Host Header
- **Severity:** Info
- **Confidence:** 90
- **File:** `internal/ingress/module.go`
- **Description:** The ingress gateway validates the Host header before redirecting to HTTPS. `isValidRedirectHost` rejects newlines, URL-looking hosts, authentication spoofing (`@`), invalid hostnames, and public IP redirect hosts.
- **Remediation:** No action required.

### OR-002: App Redirect Rules Are Constrained
- **Severity:** Info
- **Confidence:** 85
- **File:** `internal/api/handlers/redirects.go`
- **Description:** Per-app redirect rules are an intentional product feature. Rule creation now rejects non-path sources, protocol-relative destinations, non-http(s) absolute destinations, CRLF injection, unknown rule types, and external destinations for internal rewrite rules.
- **Remediation:** No action required. Continue treating tenant-configured external redirects as product behavior that should be visible in app configuration/audit trails.

## Positive Security Patterns Observed
- Ingress Host validation rejects `\r\n`, `http://`, `https://`, and `@`.
- Hostname length validation enforces max hostname/label sizes.
- Redirect rule destinations are limited to absolute paths or `http`/`https` URLs.
- Rewrite rules can only target absolute paths.
