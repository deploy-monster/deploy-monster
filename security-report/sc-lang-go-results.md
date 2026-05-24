# sc-lang-go Results

## Summary
Go-specific security scan of the DeployMonster backend.

## Findings

### GO-001: Panic On Random Generation Failure
- **Severity:** Low
- **Confidence:** 90
- **File:** `internal/auth/jwt.go`, `internal/core/id.go`, `internal/api/middleware/requestid.go`
- **Description:** A few cryptographic ID generation paths panic if `crypto/rand` fails. This is defensible for unrecoverable entropy failures, but it can still terminate request handling or startup paths under extreme OS entropy/provider failure.
- **Remediation:** For request-scoped token/request ID generation, return errors up the stack where practical. Keep startup-only fail-fast behavior documented.

### GO-002: Ignored Build Log Writer Errors
- **Severity:** Info
- **Confidence:** 70
- **File:** `internal/build/builder.go`
- **Description:** Build progress writes intentionally ignore `fmt.Fprintf(logWriter, ...)` errors. If the log writer points to persistent build logs, failures could hide disk/logging problems; if it points to a client stream, failing the build may be too strict.
- **Remediation:** Decide log-writer semantics. If persistent logs are authoritative, fail the build on write errors; if streaming is best-effort, keep ignoring but document it.

## Resolved / Revalidated Items

### GO-003: Panic On Short JWT Secret At Startup
- **Previous Severity:** Medium
- **Status:** RESOLVED
- **File:** `internal/auth/jwt.go`
- **Notes:** `NewJWTService` returns an error for secrets shorter than 32 characters instead of panicking.

### GO-004: Docker Client v28.5.2 Known Vulnerabilities
- **Previous Severity:** High
- **Status:** RESOLVED / NOT CURRENT
- **File:** `go.mod`
- **Notes:** The legacy Docker SDK module is no longer imported; current Docker integration uses split Moby modules. `govulncheck ./...` reports 0 called vulnerabilities.

## Positive Security Patterns Observed
- No `unsafe` package usage in reviewed source.
- No CGO in the release build path.
- `context.Context` passed correctly throughout.
- Structured logging with `log/slog`.
- Proper error wrapping with `fmt.Errorf("...: %w")`.
- Race detection is part of the main test target.
