# Verification Report — 2026-07-06

## Scope

Verified the current `master` working tree for DeployMonster after the recent hardening and test-fix work, then continued by repairing stale tests and runtime failures found during verification.

- Repository: `github.com/deploy-monster/deploy-monster`
- Branch: `master`
- Starting HEAD observed: `e9bbc48 fix: update mock stores for tenantID-scoped Store interface signatures`
- Go module target: `go 1.26.1`, `toolchain go1.26.4`
- This report documents the commands actually run and the results they support. It does **not** claim “100% zero issues”; the monolithic `go test ./...` command still exceeded the tool timeout, although package groups passed when run separately.

## Verification Summary

| Area | Command | Result |
|---|---|---|
| Go production build | `go build ./...` | Passed |
| Go compile-only tests | `go test -run '^$' ./...` | Passed |
| DB package tests | `go test ./internal/db` | Passed |
| Backup package tests | `go test ./internal/backup` | Passed |
| Secrets package tests | `go test ./internal/secrets/...` | Passed |
| API handlers targeted tests | `go test ./internal/api/handlers -run 'TestNotificationTest|TestWebhookRotateHandler_Rotate'` | Passed |
| API handlers full package | `go test ./internal/api/handlers -timeout 60s` | Passed |
| API middleware package | `go test ./internal/api/middleware` | Passed |
| Database engines package | `go test ./internal/database/engines` | Passed |
| Command packages | `go test ./cmd/... -timeout 120s` | Passed |
| Internal packages | `go test ./internal/... -timeout 120s` | Passed |
| Test packages | `go test ./tests/... -timeout 120s` | Passed |
| Full Go suite monolithic | `go test ./... -timeout 120s` | Not completed by tool; operation aborted due tool timeout |
| Web unit tests | `cd web && pnpm test` | Passed earlier — 44 files, 405 tests |
| Web production build | `cd web && pnpm build` | Passed earlier |

## Fixes Applied During Verification

### Go test compile blockers

The initial run found stale test code that no longer matched tenant-scoped interfaces. These were fixed:

- Added missing parentheses from botched tenant-scoping refactor call sites.
- Updated `internal/backup` mocks for `UpdateBackupStatus(ctx, id, status, sizeBytes, tenantID)`.
- Updated `internal/backup/scheduler_boost_test.go` `markFailed` calls to include tenant ID.
- Updated DB tests for tenant-scoped signatures: `ListAppsByProject`, `ListDomainsByApp`, `DeleteDomainsByApp`, `UpdateBackupStatus`, `DeleteTenant`, `DeleteApp`, and `DeleteDomain`.

### DB runtime failures

The continued run found and fixed 3 `internal/db` runtime failures:

1. `TestSQLite_DeleteDomain`
   - Cause: test passed `dom.AppID` where `DeleteDomain` now expects tenant ID.
   - Fix: pass `tenantID`.

2. `TestSQLite_AtomicNextDeployVersion`
   - Cause: implementation called `BEGIN IMMEDIATE` inside `s.Tx`, nesting a transaction under SQLite.
   - Fix: perform `BEGIN IMMEDIATE` / `COMMIT` directly for that operation and rollback on error.

3. `TestHashAppIDToLockID_Stable`
   - Cause: test contract expected `hashAppIDToLockID("") == 0`; implementation hashed empty string to a non-zero FNV value.
   - Fix: return `0` for empty app ID.

### Full-suite runtime failures outside DB

The next full run found stale tests in non-DB packages. These were fixed to match current security behavior:

1. `internal/api/handlers`
   - Notification handler tests now include auth claims because `NotificationHandler.Test` requires authenticated context.
   - Webhook rotate test now expects the persisted secret to be a hash, not the one-time plaintext response secret.

2. `internal/api/middleware`
   - `realIP` test now expects spoofable `X-Forwarded-For` to be ignored, matching the current audit-log forgery hardening.

3. `internal/database/engines`
   - Redis/MongoDB connection-string tests now verify that secrets/hosts/databases are not exposed, matching current redaction behavior.

All changed Go files were formatted with `gofmt`.

## Evidence Details

Passing checks observed after fixes:

```text
go build ./...                                      -> passed
go test -run '^$' ./...                            -> passed
go test ./internal/db                              -> passed, 7.637s
go test ./internal/backup                          -> passed
go test ./internal/secrets/...                     -> passed
go test ./internal/api/handlers -timeout 60s       -> passed, 28.934s
go test ./internal/api/middleware                  -> passed
go test ./internal/database/engines                -> passed
go test ./cmd/... -timeout 120s                    -> passed
go test ./internal/... -timeout 120s               -> passed
go test ./tests/... -timeout 120s                  -> passed
```

Earlier frontend checks also passed:

```text
cd web && pnpm test
# Test Files  44 passed (44)
# Tests       405 passed (405)

cd web && pnpm build
# vite build completed successfully
```

## Remaining Caveats / Issues

### 1. Monolithic `go test ./...` did not complete under the tool timeout

Command:

```bash
go test ./... -timeout 120s
```

Result: tool operation aborted due timeout.

Important nuance: the same package universe was then split into groups and passed:

```bash
go test ./cmd/... -timeout 120s
 go test ./internal/... -timeout 120s
 go test ./tests/... -timeout 120s
```

So the evidence supports that the grouped Go package tests passed, but it does **not** support saying the exact monolithic `go test ./...` invocation completed successfully in this environment.

### 2. Web pnpm configuration warning remains

Earlier web commands passed, but pnpm emitted:

```text
[WARN] The "pnpm" field in package.json is no longer read by pnpm. The following keys were ignored: "pnpm.overrides". See https://pnpm.io/settings for the new home of each setting.
```

This warning does not block tests/builds, but it means the intended overrides may not be active under the installed pnpm version.

## Conclusion

Current status is substantially improved:

- **Build:** passed (`go build ./...`, web build).
- **Compile:** passed (`go test -run '^$' ./...`).
- **Backend grouped tests:** passed for `cmd`, `internal`, and `tests` package groups.
- **Previously failing DB/API/middleware/database-engine packages:** now pass in targeted runs.
- **Frontend tests:** passed earlier.
- **Caveat:** monolithic `go test ./... -timeout 120s` did not complete before the tool timeout, so avoid claiming that exact command is green.

Do **not** describe the repository as having “100% zero issues.” A supported statement is: production builds pass, compile-only verification passes, targeted and grouped Go tests pass, and frontend tests/build pass; however, the single monolithic full-suite command was not observed completing successfully in this tool environment.

## Recommended Follow-up

1. Run `go test ./... -timeout 120s` directly in a local shell/CI environment without the agent tool timeout to confirm the monolithic command completes.
2. Move `web/package.json` `pnpm.overrides` into pnpm’s supported configuration location if those overrides are intended to be enforced.
3. Commit the test fixes and verification report as a scoped change.
