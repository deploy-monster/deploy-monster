# sc-open-redirect Results

## Summary
Open redirect security scan.

## Findings

### Finding: OR-001
- **Title:** Ingress HTTPS Redirect Validates Host Header
- **Severity:** Info
- **Confidence:** 90
- **File:** internal/ingress/module.go:234-246
- **Description:** The ingress gateway validates the Host header before redirecting to HTTPS. `isValidRedirectHost` rejects newlines, URLs with schemes, authentication spoofing (`@`), and invalid hostnames.
- **Remediation:** No action required. The validation is comprehensive.

## Positive Security Patterns Observed
- `isValidRedirectHost` rejects `\r\n`, `http://`, `https://`, and `@`
- Hostname length validation (max 253 chars, label max 63)
- IP validation: allows loopback/private but rejects public IPs in redirects
- `validateHostname` enforces alphanumeric start/end and hyphens only
- `urlParseSafe` rejects opaque URLs
