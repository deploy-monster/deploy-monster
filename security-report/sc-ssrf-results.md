# sc-ssrf Results

## Summary
Server-side request forgery security scan.

## Findings

### Finding: SSRF-001
- **Title:** Git Clone URL Validation Bypass Risk
- **Severity:** Medium
- **Confidence:** 70
- **File:** internal/build/builder.go:206-258
- **Description:** `ValidateGitURL` blocks `file://`, `http://`, shell metacharacters, and private IPs. However, it allows `git://` and `ssh://` schemes. Some `git://` implementations may redirect to local protocols. Also, `isAbsPath` allows local absolute paths for development, which could be abused if the build environment has sensitive files.
- **Remediation:** Disable `git://` scheme (insecure, no encryption). Restrict local paths to a whitelist in production. Validate SSH host keys for `ssh://` URLs.

### Finding: SSRF-002
- **Title:** Outbound Webhook Deliveries May Reach Internal Networks
- **Severity:** Medium
- **Confidence:** 65
- **File:** internal/webhooks/ (sender/delivery)
- **Description:** Outbound webhook URLs are configured by authenticated users. If URL validation does not block private IP ranges, webhooks could be used to scan or attack internal services.
- **Remediation:** Validate all outbound webhook URLs against private/blocked IP ranges before delivery (similar to `ValidateGitURL`).

## Positive Security Patterns Observed
- `ValidateGitURL` blocks `file://`, `http://`, private IPs, and shell metacharacters
- `validateResolvedHost` performs real-time DNS lookup to block DNS rebinding
- `isPrivateOrBlockedIP` covers loopback, link-local (cloud metadata), private, and unspecified ranges
- ACME challenges go to Let's Encrypt (trusted external)
