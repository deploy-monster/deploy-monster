# sc-cmdi Results

## Summary
Command injection security scan.

## Findings

### Finding: CMDI-001
- **Title:** Command Blocklist Bypass Potential in Container Exec
- **Severity:** Medium
- **Confidence:** 75
- **File:** internal/api/handlers/exec.go:18-31, internal/api/ws/terminal.go:36-40
- **Description:** `isCommandSafe` uses a substring blocklist (`rm -rf /`, `mkfs`, `curl | sh`, etc.). Blocklists are inherently incomplete. An attacker could use equivalent commands (`/bin/rm -rf /`, `wget -O- | bash`, `nc -e /bin/sh`, etc.) to bypass restrictions.
- **Impact:** If an attacker compromises a user account, they may execute dangerous commands inside their own container.
- **Remediation:** Replace blocklist with an allowlist approach (e.g., only permit specific safe commands) or use a restricted shell. Alternatively, validate commands against a known-safe command registry.

### Finding: CMDI-002
- **Title:** splitCommand Does Not Prevent All Shell Metacharacters
- **Severity:** Low
- **Confidence:** 65
- **File:** internal/api/handlers/exec.go:48-81
- **Description:** `splitCommand` tokenizes by spaces/quotes but passes tokens directly to `runtime.Exec`. While this prevents `sh -c` injection, certain Docker exec implementations may still interpret some tokens. The function also does not validate individual tokens against dangerous binaries.
- **Remediation:** Sanitize each token against a whitelist of allowed characters/binaries.

## Positive Security Patterns Observed
- No `sh -c` usage in exec handler — commands passed as direct exec arguments
- `exec.CommandContext` used with timeout
- `ValidateGitURL` blocks shell metacharacters in git URLs
- `validateBuildArg` prevents control characters and flag injection in Docker build args
- `validateDockerImageTag` restricts image tag format
