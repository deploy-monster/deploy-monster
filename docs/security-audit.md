# Security audit report

- **Date:** 2026-04-10
- **Version audited:** v0.0.1 (105 hardening tiers applied)
- **Tools:** `govulncheck`, `staticcheck`, `gosec`, `go vet`
- **Auditor:** Ersin KOÇ

This document records the findings and resolutions of the pre-release security
sweep. It is meant as a baseline — future audits should reference this file and
record deltas rather than re-auditing from scratch.

## Summary

| Category         | Tool          | Raw findings | After triage | Status |
|------------------|---------------|:------------:|:------------:|--------|
| CVEs (reachable) | govulncheck   | 6            | 2            | 4 fixed (toolchain bump), 2 upstream |
| Style / bugs     | staticcheck   | 41           | 7 fixed      | 34 test-only nil-context warnings deferred |
| Security rules   | gosec (HIGH)  | 27           | 3 fixed      | 24 false positive / intentional |
| `go vet`         | go vet        | 0            | 0            | clean |

## govulncheck

Ran `govulncheck ./...` across the full module.

### Before the toolchain bump (6 findings)

| ID             | Package                              | Severity | Disposition |
|----------------|--------------------------------------|----------|-------------|
| GO-2026-4947   | `crypto/x509` (stdlib)               | high     | Fixed — bumped Go to 1.26.2 |
| GO-2026-4946   | `crypto/x509` (stdlib)               | high     | Fixed — bumped Go to 1.26.2 |
| GO-2026-4870   | `crypto/tls` (stdlib)                | high     | Fixed — bumped Go to 1.26.2 |
| GO-2026-4866   | `crypto/x509` (stdlib)               | high     | Fixed — bumped Go to 1.26.2 |
| GO-2026-4887   | `github.com/docker/docker@v28.5.2+incompatible` | medium | **Upstream** — Moby daemon-side AuthZ plugin bypass. DeployMonster calls the Docker daemon as a *client* and never runs as the daemon, so the vulnerable code path is not reachable through our binary. No upstream fix available at audit time. |
| GO-2026-4883   | `github.com/docker/docker@v28.5.2+incompatible` | medium | **Upstream** — Moby daemon-side plugin-privilege off-by-one. Same analysis as 4887. |

### Fix applied

A `toolchain` directive was added to `go.mod`, pinning the build toolchain at
1.26.2 while leaving the minimum language version at 1.26.1:

```
go 1.26.1

toolchain go1.26.2
```

With `GOTOOLCHAIN=auto` (the default), Go automatically downloads 1.26.2 on the
first build via the toolchain directive. CI jobs pin `go-version: '1.26'` via
`setup-go`, which resolves to the latest 1.26.x patch on each runner — the
toolchain directive in `go.mod` then acts as a hard floor so any runner that
somehow landed on 1.26.1 still pulls 1.26.2 before compiling. Using a
toolchain directive instead of raising the `go` line keeps downstream module
consumers free to stay on 1.26.1 while our compiled binary always gets the
patched stdlib.

This fix was reconciled against reality in Tier 95 after `govulncheck` surfaced
that the original documentation claim (bumping the `go` line) was never
actually committed — only the toolchain pin landed.

### Post-fix

```
$ govulncheck ./...
Vulnerability #1: GO-2026-4887  (docker/docker — daemon-side, not applicable)
Vulnerability #2: GO-2026-4883  (docker/docker — daemon-side, not applicable)
Your code is affected by 2 vulnerabilities from 1 module.
```

### CI integration

A `govulncheck` step was added to the `lint` job in `.github/workflows/ci.yml`.
It runs `govulncheck ./...` and fails the build on any **new** vulnerability
not in the allow-list. The two unresolved Docker SDK findings above are
allow-listed pending upstream fixes.

## staticcheck

Ran `staticcheck ./...`. 41 findings, of which 3 were in production code and
the rest in tests.

### Production-code fixes

| Site                                      | Rule   | Fix |
|-------------------------------------------|--------|-----|
| `internal/backup/s3.go:229`               | SA4006 | Removed dead initial assignment of `listURL`; declared once in the branching block. |
| `internal/deploy/graceful.go:37`          | U1000  | Removed unused `lastCheck time.Time` field from `HealthChecker`. |
| `internal/dns/sync.go:27`                 | U1000  | Removed unused `sync.Mutex` field from `SyncQueue` — the queue is channel-serialized and never needed it. |

### Test-code fixes

| Site                                         | Rule   | Fix |
|-----------------------------------------------|--------|-----|
| `internal/gitsources/providers/providers_final_test.go:26` | SA4006 | Removed dead `err :=` assignment; the test now uses `_ = gh` to show the intent. |
| `internal/ingress/router_extra_test.go:131`  | SA5011 | `t.Error` → `t.Fatal` so the subsequent nil-pointer dereference is unreachable. |
| `internal/marketplace/registry_test.go:85`   | SA5011 | Same pattern — `t.Error` → `t.Fatal`. |
| `internal/db/postgres_test.go:2195`          | SA4023 | Replaced tautological `if store == nil` runtime check with the compile-time `var _ core.Store = (*PostgresDB)(nil)` assertion. |

### Deferred

34 SA1012 warnings across 11 test files: `do not pass a nil Context`. These
are test scaffolding that passes `nil` as the context argument to functions
whose implementations discard the ctx. Fixing each site to `context.TODO()`
changes no semantics but churns a lot of files; these are tracked as a future
cleanup and excluded from the CI lint gate for now.

## gosec

Ran `gosec -exclude-dir=web -exclude-dir=tests ./...`. 243 findings total:
**27 HIGH**, 33 MEDIUM, 183 LOW.

### HIGH-severity bug fixes (3)

| Site                                    | Rule  | Fix |
|------------------------------------------|-------|-----|
| `internal/ingress/lb/balancer.go:RoundRobin.Next` | G115 (and panic) | Added `if len(backends) == 0 { return "" }` guard. Without this, a route with zero healthy backends would panic on divide-by-zero. |
| `internal/ingress/lb/balancer.go:LeastConn.Next`  | G115 (and crash) | Same empty-slice guard. Also prevents dereferencing an unset `best` after the loop. |
| `internal/ingress/lb/balancer.go:IPHash.Next`     | G115 (and panic) | Same empty-slice guard; the remaining `uint32(len(backends))` conversion is annotated `#nosec G115` because `len(backends)` is bounded to the small number of backends configured per route. |
| `internal/ingress/lb/balancer.go:Random.Next`     | G115 (and panic) | Same empty-slice guard; the `uint32(counter)` truncation is intentional (rotating counter window). |

### HIGH-severity false positives / intentional (24)

**G115 — integer overflow in metrics conversions (10 sites).**
All in `internal/deploy/docker.go` and `internal/resource/collector.go`,
converting `uint64` byte counts from Docker stats (`stats.MemoryStats.Usage`,
`iface.RxBytes`, `memStats.Sys`, etc.) to `int64` for JSON serialization.
These values represent physical memory/network/block-IO counters that cannot
approach `math.MaxInt64` (~9.2 exabytes) in any conceivable deployment. No
fix applied — these are excluded from the CI gate via `gosec.yaml`.

**G115 — TOTP time-window and rate-limit bucketing (4 sites).**
`internal/auth/totp.go:66` (`int64 -> uint64` on a positive time step) and
`internal/ingress/middleware/ratelimit_bolt.go:65-66` (`int64 -> rune` for
bucket-key byte encoding). Values are positive and well within range. No fix
applied.

**G101 — hardcoded credentials in marketplace templates (6 sites).**
`internal/marketplace/builtins_100.go` contains Docker Compose YAML templates
with example passwords like `POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}`.
These are *template defaults* meant to be overridden at deploy time via the
marketplace form, not real credentials. False positives — no fix applied.

**G118 — goroutine uses `context.Background()` while request-scoped ctx is
available (2 sites).** `internal/ingress/module.go:120` starts the ACME
renewal loop and `internal/discovery/module.go:81` starts the Docker event
watcher. Both run for the lifetime of the module, not the lifetime of the
`Start(ctx)` call that spawned them. Using the short-lived startup context
would cause the loops to exit as soon as `Start` returned. Both modules
implement their own `Stop()` that cleanly terminates the goroutine. These are
intentional lifecycle patterns — no fix applied.

**G402 — `InsecureSkipVerify: true` in `cmd/deploymonster/main.go:319`.**
This is the new `deploymonster health` subcommand introduced to support the
distroless Docker `HEALTHCHECK` directive. The default is `--insecure=true`
so the local healthcheck works against the server's self-signed certificate
on `127.0.0.1:8443`. The subcommand only connects to localhost. Annotated
with `#nosec G402` and a rationale comment.

### MEDIUM / LOW findings

- **G104 (178 LOW) — unchecked error returns.** The majority are intentional
  `h.events.Publish(...)` and `flag.FlagSet.Parse(...)` calls where the
  error is uninteresting (event-bus publishes are best-effort; `flag.FlagSet`
  constructed with `flag.ExitOnError` never returns a non-nil error from
  Parse). Not addressed. Suppressing them would require touching ~180 sites
  for zero security benefit.
- **G204 (16 MEDIUM) — command execution with non-constant arg.** All are in
  the build pipeline (`docker build`, `git clone`) where the non-constant
  argument is a validated path or an image tag. Reviewed — no injection vector.
- **G304 (10 MEDIUM) — file inclusion via variable.** All are the backup
  download handler, the config loader, and the template loader, which open
  paths that are already validated against traversal by
  `internal/core/pathutil.go`. No fix needed.
- **G306 (4 MEDIUM) — file permissions too permissive.** All write mode `0644`
  for non-sensitive files (logs, exported data). Acceptable.
- **G402 (1 MEDIUM) — covered above (the health-check).**
- **G505 (1 MEDIUM) — weak crypto (SHA-1).** Used for Git commit hash display,
  not for security. Acceptable.
- **G118 (2 MEDIUM) — covered above.**

## `go vet`

Clean. No findings.

## Phase 1–5 hardening fixes (closed)

The scanner findings above are static-analysis-level issues. A
separate class of security-relevant defects — lifecycle races,
resource leaks, context cancellation gaps, denial-of-service vectors —
is not caught by any scanner in the toolchain but does show up as
real vulnerabilities under load. Phases 1–5 closed a batch of these
across 88 hardening tiers; the ones with security impact are
captured here as resolved findings.

| ID       | Tier | Subsystem               | Class                   | Fix summary |
|----------|------|-------------------------|-------------------------|-------------|
| H-001    | 65   | `internal/discovery`    | goroutine leak          | Docker event watcher was spawned from `Start(ctx)` but never bound to a lifecycle cancellation. Added `stopCtx`, wait-group drain, and panic recovery so `Stop()` terminates the watcher deterministically. |
| H-002    | 66   | `internal/vps`          | context-cancellation gap| VPS provisioning goroutines ignored ctx cancellation during long-running provider calls. All provider paths now plumb the request context and abort on cancel. |
| H-003    | 67   | `internal/backup`       | scheduler lifecycle     | Backup scheduler could emit a backup-start event after `Stop()`. Guarded with a `closed` flag + wait-group + context-cancellation on scheduled runs. |
| H-004    | 68   | `internal/billing`      | Stripe webhook replay   | Stripe webhook replay protection was incomplete; a replayed webhook with the same event ID was re-applied. Added deduplication via BBolt-keyed idempotency store + 24 h retention. |
| H-005    | 69   | `internal/build`        | builder lifecycle       | Builder goroutines could outlive `Stop(ctx)` if a Docker-build streaming read was mid-frame. Added close-propagation through the `io.Reader` chain and panic recovery. |
| H-006    | 70   | `internal/core`         | scheduler race          | Scheduler registration + `Stop()` race-conditioned a `nil map` panic under contention. Rewritten to use `sync.Map` + atomic stopped flag. |
| H-007    | 71   | `internal/dns`          | DNS sync queue          | DNS sync goroutines did not drain on shutdown; a pending propagation check could leak a goroutine per restart. Added `stopCtx` + wait-group drain. |
| H-008    | 72   | `internal/api/middleware`| body-limit bypass      | `BodyLimit(10MB)` middleware used `http.MaxBytesReader` but did not fully drain on reject, leaving a small window where a client could wedge a connection. Fixed with explicit `io.Copy(io.Discard, …)` on reject. |
| H-009    | 73   | `internal/ingress`      | ingress gateway lifecycle | ACME renewal loop ran on `context.Background()` and did not exit when the ingress module stopped. Added module-scoped `stopCtx` + wait-group drain + ACME ctx plumbing. |
| H-010    | 74   | `internal/deploy`       | auto-rollback drain     | Auto-rollback goroutines could outlive `Stop()` during a rollback-mid-flight and write to a closed DB handle. Wait-group drain + closed flag. |
| H-011    | 75   | `internal/resource`     | resource monitor        | Resource monitor's tick loop ignored cancellation and published `resource.sample` events after `Stop()`. Added `stopCtx` and stop-flag gate before publish. |
| H-012    | 76   | `internal/swarm`        | AgentServer lifecycle   | Swarm `AgentServer` could accept a new agent mid-shutdown, panicking on the closed listener. Added panic recovery wrapper, `stopCtx`, wait-group drain, closed-flag gate. |
| H-013    | 77   | `internal/api/ws`       | WebSocket DeployHub     | DeployHub concurrent writes could race the frame encoder, producing malformed frames that disconnect every subscriber. Added per-client write mutex + dead-client eviction + hub `Shutdown()` that drains in-flight writes. |
| H-014    | 78–82| `internal/auth`         | JWT, TOTP, OAuth PKCE   | Hardening of the auth stack: JWT key rotation with `PreviousSecretKeys` (graceful), TOTP time-window G115 corrections, OAuth PKCE enforcement in provider paths, fuzz targets added. |
| H-015    | 83   | `internal/api`          | request-scope leak      | `RequestLogger` middleware captured the body for structured logging and did not always close the tee reader. Fixed with `defer r.Body.Close()` + tee reset. |
| H-016    | 84   | `internal/build`        | tenant queue fairness   | Per-tenant build queue fairness was not proven under load. Added `tenant_queue_bench_test.go` (4 microbenchmarks) + a standalone harness at `tests/loadtest/build_queue/` that measures `max-tenant-p99 / median-tenant-p99` fairness — 16 tenants × 4× oversubscription shows 1.00x fairness. Not a fix *per se*, but closes a DoS question: a runaway tenant cannot starve the queue. |
| H-017    | 85   | `tests/loadtest`        | regression gate         | HTTP loadtest now compares against a committed baseline (`tests/loadtest/baselines/http.json`) and fails CI on ≥10% throughput drop or p95 increase. Defends against performance regressions being merged silently. |
| H-018    | 86   | `tests/soak`            | drift detection         | 24-hour soak harness added with three drift gates (goroutine leak, heap climb, DB bloat). Runtime metrics exposed on the existing `/metrics/api` endpoint via `runtime.ReadMemStats` — no new pprof auth surface. See [tests/soak/README.md](../tests/soak/README.md). |

Every one of the above has test coverage pinning the fix. Regression
of any of them fails `make test` on the matching `tier*_hardening_test.go`
file.

## Residual risk register

This is the honest list of what is **not** fixed, ranked by severity.
Future audits should either drive these to closed or justify each as
accepted risk again.

| ID    | Class                  | Severity | Owner      | Notes |
|-------|------------------------|----------|------------|-------|
| R-001 | Docker SDK CVEs (2)    | MEDIUM   | upstream   | `GO-2026-4887` + `GO-2026-4883`. Daemon-side only; DeployMonster is always the client. Re-check monthly; bump `docker/docker` as soon as upstream releases a fixed version. |
| R-002 | Single master encryption key | MEDIUM | product | Attacker with DB file + config file decrypts every tenant's secrets. Mitigated by file permissions + optional separate `secrets.encryption_key` but no KMS tier exists for self-hosted. Revisit if compliance or hosted-tier requirements change. See [ADR 0008](adr/0008-encryption-key-strategy.md). |
| R-003 | No per-tenant key isolation | LOW | product | All tenants share one master key. A full-cluster compromise is equally bad for everyone. Acceptable for self-hosted single-operator; not for multi-tenant BYOK/enterprise. |
| R-004 | No ciphertext version marker | LOW | engineering | The vault format is raw `[nonce ‖ AES-256-GCM output]` with no algorithm version byte. A future algorithm swap needs either a format-version byte or a full `RotateEncryptionKey` pass. Harmless today — there is exactly one algorithm — but locks in a future ADR before any second algorithm lands. |
| R-005 | Vault rotation has no previous-key grace window | LOW | operations | If `RotateEncryptionKey` is interrupted, some secret versions are on the old key and some on the new. The code retries correctly on the next call, but the operator-facing story is "don't interrupt it." JWT rotation *does* have a previous-key grace window — vault does not. |
| R-006 | staticcheck SA1012 in tests (34 sites) | INFO | engineering | `nil` passed as `context.Context` in tests whose implementations discard the ctx. Churny to fix; excluded from the CI lint gate. Addresses itself the next time a test touches one of these files. |
| R-007 | gosec G104 on event-bus `Publish` (178 sites) | INFO | engineering | Event-bus publishes are best-effort by design. Suppressing the error would require touching 178 sites for zero security benefit. Excluded via `.gosec.yaml`. |
| R-008 | Audit log is not encrypted at rest | LOW | product | Actor, action, target, IP, timestamp are plaintext in SQLite. Fine for self-hosted; not fine for every compliance regime (HIPAA, FedRAMP). Encrypt the audit log body if a compliance requirement emerges. |
| R-009 | 3 residual Dependabot alerts (17 closed in Tiers 91 + 96) | LOW | upstream | As of 2026-04-11 GitHub reported 20 open alerts (11 HIGH + 9 MODERATE) against `master`. Tier 91 closed 17 of them in the working tree by bumping dependencies: `go.opentelemetry.io/otel*` 1.42.0 → 1.43.0 (CVE-2026-39882, HTTP OTLP exporter unbounded body), `vite` 8.0.3 → 8.0.8 (GHSA-p9ff-h696-f583 + GHSA-v2wj-q39q-566r + GHSA-4w7w-66w2-5vf9, dev-server path-traversal and middleware bypass), plus pnpm `overrides` forcing transitive `vite@7` → `7.3.2` (same three GHSAs via `vitest@3.2.4` → `@vitest/mocker` → `vite@7`) and transitive `lodash@4.17.23` → `4.18.1` (GHSA-r5fr-rjxr-66jc + GHSA-f23m-r3pf-42rh, prototype pollution and ReDoS, reached via the abandoned `dagre@0.8.5` → `graphlib@2.1.8` chain used by the topology editor). **Tier 96 discovered that Dependabot was still reporting 11 of those alerts as open** after the Tier 91 push, because `web/package-lock.json` (a stale lockfile from a pre-pnpm era) was still tracked in git alongside `web/pnpm-lock.yaml`. Dependabot scanned the stale npm lockfile, which never received the `pnpm.overrides` transformations, and kept re-raising alerts against `4.17.21`/`7.0.0`/`8.0.3` that had already been closed in `pnpm-lock.yaml`. Root cause: `scripts/build.sh` used `npm ci` / `npm install` / `npm run build` while CI and `CLAUDE.md` both prescribed `pnpm`, so every local `make build` regenerated the stale npm lockfile. Fix: `scripts/build.sh` and `scripts/ci-local.sh` rewritten to use `pnpm install --frozen-lockfile` + `pnpm run build`, `web/package-lock.json` deleted, `.gitignore` updated to ignore `web/package-lock.json` so a stray `npm install` can't reintroduce the drift. The 3 still-open alerts after Tier 96 are upstream-blocked: 2 against `github.com/docker/docker` (duplicates of R-001, daemon-side CVEs, no fixed version listed), and 1 against `go.etcd.io/bbolt` (GHSA-6jwv-w5xf-7j27, no upstream fix published yet — tracked, re-check monthly). Severity dropped from MEDIUM to LOW because the remaining alerts are all `fixed=null` upstream waits, not unpatched dependency drift. |

Residual risks flagged `MEDIUM` or above are blockers for a `v2.0`
tag. `LOW` and `INFO` are acceptable through `v1.x` provided each
one has a written reason and an owner in this table.

## Test suite

`make test-short` ran to completion with no failures after all fixes were
applied. No regression from the toolchain bump or the production-code fixes.

## Allow-list / ongoing tracking

The following items are accepted risk as of this audit and should be revisited
on each subsequent audit:

1. **`github.com/docker/docker@v28.5.2+incompatible` — GO-2026-4887,
   GO-2026-4883.** Re-check upstream monthly. Bump the module and re-audit as
   soon as a fixed version is published.
2. **staticcheck SA1012 in test files.** Convert to `context.TODO()` as part
   of the next test-file cleanup pass. No runtime impact.
3. **gosec G115 on metrics conversions.** Excluded in `.gosec.yaml`. Leave
   excluded unless a memory/stats backend emerges where overflow is plausible.
4. **gosec G104 on event-bus publishes and `flag.Parse`.** Excluded in
   `.gosec.yaml` pattern list. Not a defect.

## How to re-run the audit

```bash
# Toolchain
go version   # must be >= 1.26.2

# Vulnerability scan (CI-gated)
govulncheck ./...

# Static analysis
staticcheck ./...
go vet ./...

# Security linter
gosec -exclude-dir=web -exclude-dir=tests ./...

# Full test suite with race detector
make test
```

Any delta against the baseline in this document should be triaged and either
fixed or justified in a new section dated below.
