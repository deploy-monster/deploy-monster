# Verification Report — 2026-07-06

## Scope

Verified the current `master` working tree for DeployMonster after the recent hardening and test-fix work, then continued by repairing stale tests and runtime failures found during verification.

- Repository: `github.com/deploy-monster/deploy-monster`
- Branch: `master`
- Starting HEAD observed: `e9bbc48 fix: update mock stores for tenantID-scoped Store interface signatures`
- Go module target: `go 1.26.1`, `toolchain go1.26.4`
- This report documents the commands actually run and the results they support. The monolithic `go test ./...` command was initially unresolved due to tool timeout; it has since been confirmed passing (see update below).

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
| Full Go suite monolithic | `go test ./... -timeout 120s` | Initially unresolved; confirmed passing on 2026-07-08 (see update below) |
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

## Post-Report Updates

### 2026-07-08 — Monolithic `go test ./...` confirmed passing

After the fix branches were merged into `master`, the full suite was re-run:

```bash
go test -count=1 ./... -timeout 240s
```

Result: **all 44 packages pass** (42 `ok`, 2 with no test files, 0 `FAIL`). Total runtime ~2m30s.

Key package timings:
- `internal/db` — 10.020s
- `internal/api/handlers` — 32.071s
- `internal/api/middleware` — 9.122s
- `internal/auth` — 27.351s
- `internal/backup` — 1.343s
- `internal/secrets` — 8.007s
- `internal/deploy` — 5.260s
- `internal/swarm` — 8.242s

Additionally, `go vet ./...` passed clean and the OpenAPI drift check reports **236/236 routes matching** with an empty allowlist.

This closes the primary caveat from the original report.

### Still open
- **Release artifact/image publication** — verified for v0.1.9 (GitHub release + GHCR image published, Trivy-scanned).
- **Staging validation** on real infrastructure remains the final pre-launch gate.

### 2026-07-10 — Full investigation + remaining caveats closed

A comprehensive project investigation was performed on the working tree:

**Verified still passing (all re-run live):**
- `go build ./...` ✅ | `go vet ./...` (all tag variants) ✅
- `go test -count=1 ./... -timeout 240s` ✅ (44 packages, 0 FAIL)
- OpenAPI drift: 236/236 routes, allowlist=0 ✅
- Coverage: 85.1% filtered (≥85% gate) ✅
- Web tests: 44 files / 405 tests ✅
- Web build: 933ms ✅

**Resolved caveats from this report:**
- **pnpm overrides warning** ✅ — already fixed in commit `5e3459d` via `web/.npmrc` migration; verified clean (`pnpm install` emits no warning)
- **VERSION file** ✅ — was `v0.1.8`, updated to `v0.1.9` to match the released tag

**New deliverable:**
- `PRODUCTION-STATUS.md` — comprehensive English production status document (345 lines, 10 sections) consolidating executive verdict + detailed evidence

**Single remaining open item:**
- Staging validation on real infrastructure (per `docs/staging-validation.md`) — requires a disposable staging host with Docker, DNS control, and a smoke-test account. Cannot be executed from this environment.

## Remaining Caveats / Issues

### 1. ~~Monolithic `go test ./...` did not complete under the tool timeout~~ ✅ RESOLVED

The monolithic suite was confirmed passing on 2026-07-08 — see update above.

### 2. ~~Web pnpm configuration warning~~ ✅ RESOLVED

The `pnpm.overrides` warning was resolved by commit `5e3459d` which migrated overrides
from `package.json` to `web/.npmrc` using pnpm 10+'s `override.*` directive syntax.
Verified clean on 2026-07-10: `pnpm install` emits no warning and the overrides
(esbuild, braces, vite@7, ws) are active.

## Conclusion

Current status is substantially improved:

- **Build:** passed (`go build ./...`, web build).
- **Compile:** passed (`go test -run '^$' ./...`).
- **Backend grouped tests:** passed for `cmd`, `internal`, and `tests` package groups.
- **Previously failing DB/API/middleware/database-engine packages:** now pass in targeted runs.
- **Frontend tests:** passed earlier.
- **Monolithic suite:** `go test -count=1 ./... -timeout 240s` — **all 44 packages pass** (42 ok, 2 no-test-files, 0 FAIL). Previously this command could not be confirmed due to tool timeout; it is now verified green.

As of 2026-07-10: production builds pass, go vet (all tag variants) is clean, all 44 Go test packages pass the monolithic suite, coverage is 85.1% (above the 85% CI gate), the OpenAPI drift gate is clean (236/236 routes), frontend tests/build pass, the pnpm overrides warning is resolved, the VERSION file is synced, and release v0.1.9 is published with Trivy-scanned Docker image. The single remaining open item is staging validation on real infrastructure.

## Recommended Follow-up

1. ~~Run `go test ./... -timeout 120s` directly in a local shell/CI environment~~ ✅ Done — confirmed passing with `go test -count=1 ./... -timeout 240s` on 2026-07-08.
2. ~~Move `web/package.json` `pnpm.overrides` into pnpm’s supported configuration location~~ ✅ Done — already migrated in commit `5e3459d`, verified clean on 2026-07-10.
3. Execute [`docs/staging-validation.md`](staging-validation.md) on a disposable staging host before declaring the release production-ready for hosted SaaS.
