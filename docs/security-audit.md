# Security audit report

- **Date:** 2026-04-10
- **Version audited:** Unreleased (post-v1.6.0, 60 hardening tiers applied)
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

`go.mod` was bumped from `go 1.26.1` to `go 1.26.2`:

```
// Bumped to 1.26.2 to pull the crypto/tls and crypto/x509 fixes for
// GO-2026-4866, GO-2026-4870, GO-2026-4946, GO-2026-4947 (see
// `govulncheck ./...` and docs/security-audit.md).
go 1.26.2
```

With `GOTOOLCHAIN=auto` (the default), Go automatically downloads 1.26.2 on the
first build. CI was updated to pin `go-version: '1.26.2'` in all jobs so the
upgrade is deterministic.

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

**G101 — hardcoded credentials in `internal/auth/oauth.go` (2 sites).**
gosec flags `NewGoogleOAuth(clientID, clientSecret)` and its GitHub twin
because the struct literal contains a field literally named `ClientSecret`.
The value is passed in as a parameter and comes from the user's config file.
False positives — no fix applied.

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
