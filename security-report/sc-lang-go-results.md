# sc-lang-go Results

## Summary
Go-specific security scan of the DeployMonster backend.

## Findings

### Finding: GO-001
- **Title:** Panic on Short JWT Secret at Startup
- **Severity:** Medium
- **Confidence:** 95
- **File:** internal/auth/jwt.go:54
- **Description:** `NewJWTService` panics if the JWT secret is shorter than 32 characters. While this prevents weak secrets, a panic on startup can cause denial of service if an operator misconfigures the secret.
- **Remediation:** Return an error instead of panicking to allow graceful shutdown with a clear error message.

### Finding: GO-002
- **Title:** Panic on Random Generation Failure
- **Severity:** Low
- **Confidence:** 90
- **File:** internal/auth/jwt.go:268
- **Description:** `generateTokenID` panics if `crypto/rand.Read` fails. This could cause a cascading failure under entropy exhaustion.
- **Remediation:** Return an error up the call stack instead of panicking.

### Finding: GO-003
- **Title:** Docker Client v28.5.2 Known Vulnerabilities
- **Severity:** High
- **Confidence:** 85
- **File:** go.mod:9
- **Description:** The `go.mod` explicitly acknowledges AuthZ bypass and plugin privilege issues in Docker v28.5.2.
- **Remediation:** Upgrade to Docker client v29+ when available.

### Finding: GO-004
- **Title:** Unhandled Error in logWriter Write
- **Severity:** Info
- **Confidence:** 70
- **File:** internal/build/builder.go:95
- **Description:** `fmt.Fprintf(logWriter, ...)` error return is ignored. In a build pipeline, log writer failures could mask disk-full errors.
- **Remediation:** Check and handle the error return.

## Positive Security Patterns Observed
- No `unsafe` package usage
- No CGO (`CGO_ENABLED=0` in build)
- `context.Context` passed correctly throughout
- Structured logging with `log/slog`
- Proper error wrapping with `fmt.Errorf("...: %w", err)`
- Race detection enabled in tests (`make test`)
