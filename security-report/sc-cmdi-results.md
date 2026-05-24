# sc-cmdi Results

## Summary
Command injection security scan.

## Findings

No active OS command injection findings remain in the scanned runtime paths.

## Resolved Findings

### CMDI-001: Command Blocklist Bypass Potential in Container Exec
- **Status:** Resolved
- **Evidence:** Container exec and terminal commands use an allowlist for the primary executable and block shell operators before dispatching direct argv to the runtime.
- **Files:** `internal/api/handlers/exec.go`, `internal/api/ws/terminal.go`

### CMDI-002: Explicit Args Shell/Eval Flag Bypass
- **Status:** Resolved
- **Evidence:** The exec handler now validates the full argv, including explicit `args`, and blocks shell/eval flags such as `bash -c`, `sh -lc`, `python -c`, and `node --eval`. Terminal, one-off app commands, cron jobs, and node executors apply the same shared policy.
- **Files:** `internal/core/command_safety.go`, `internal/api/handlers/exec.go`, `internal/api/handlers/commands.go`, `internal/api/ws/terminal.go`, `internal/cron/module.go`, `internal/swarm/local.go`, `internal/swarm/remote.go`, `internal/api/handlers/exec_security_test.go`, `internal/api/ws/terminal_security_test.go`, `internal/cron/module_test.go`, `internal/swarm/swarm_test.go`

### CMDI-003: Runtime Command Paths Used `sh -c`
- **Status:** Resolved
- **Evidence:** `/apps/{id}/commands`, app cron jobs, and local/remote node executors now tokenize requested commands, validate them with the shared argv policy, and send direct argv to the container runtime instead of `["sh", "-c", command]`.
- **Files:** `internal/api/handlers/commands.go`, `internal/cron/module.go`, `internal/swarm/local.go`, `internal/swarm/remote.go`, `internal/api/handlers/commands_handler_test.go`, `internal/cron/module_test.go`, `internal/swarm/swarm_test.go`, `internal/swarm/swarm_coverage_test.go`

## Positive Security Patterns Observed
- Shared command policy for exec, terminal, one-off app commands, cron, and node executors
- Full argv validation covers explicit command arguments and parsed command strings
- `exec.CommandContext` used with timeout
- `ValidateGitURL` blocks shell metacharacters in git URLs
- `validateBuildArg` prevents control characters and flag injection in Docker build args
- `validateDockerImageTag` restricts image tag format
