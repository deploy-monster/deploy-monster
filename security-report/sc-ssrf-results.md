# sc-ssrf Results

## Summary
Server-side request forgery security scan.

## Findings

No active SSRF findings are verified in the current working tree.

## Resolved / Revalidated Items

### SSRF-001: Git Clone URL Validation Bypass Risk
- **Previous Severity:** Medium
- **Status:** RESOLVED / CURRENTLY CONTROLLED
- **Files:** `internal/build/builder.go`, `internal/api/handlers/import_export.go`
- **Notes:** `ValidateGitURL` now rejects `git://`, `http://`, `file://`, shell metacharacters, private/link-local/loopback IPs, and unsafe resolved DNS targets. Local absolute git paths are rejected by default and require explicit `MONSTER_ALLOW_LOCAL_GIT_PATHS=true` for development. App import validation now reuses the same policy instead of accepting weaker manifest-only URL checks.

### SSRF-002: Outbound Webhook Deliveries May Reach Internal Networks
- **Previous Severity:** Medium
- **Status:** RESOLVED / CURRENTLY CONTROLLED
- **Files:** `internal/api/handlers/event_webhooks.go`, `internal/notifications/providers.go`
- **Notes:** Outbound event webhooks and Slack/Discord notification webhooks require HTTPS and reject localhost, private IP ranges, link-local/cloud metadata IPs, multicast IPs, and common metadata/internal hostnames. Notification webhook HTTP clients also revalidate redirect targets before following redirects.

## Positive Security Patterns Observed
- `ValidateGitURL` blocks unsafe schemes, local file access, private IPs, shell metacharacters, and DNS rebinding targets.
- Outbound webhook configuration and redirect-following paths validate destination safety.
- ACME challenge traffic is constrained to expected Let's Encrypt workflows.
