# Changelog

All notable changes to DeployMonster will be documented in this file.

The format is loosely based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Grouped into **Breaking**, **Security**, **Features**, **Fixes**, and **Performance**
at the request of the Phase 7 roadmap.

## [0.1.8] — 2026-04-30 — golangci-lint v1 upgrade, Vite security patches, security findings closed

Session cleanup and hardening. golangci-lint upgraded from v2.11.4 to v1.64.8
with v1-compatible config. Vite 8.0.9 → 8.0.10 patches two high-severity
findings (arbitrary file read via dev server WebSocket, fs.deny bypass).
@tailwindcss/vite updated to 4.2.4.

### Security

- **VULN-001 (auto-generated credentials written to disk) remediated.**
  `firstRunSetup` now requires `MONSTER_ADMIN_EMAIL` and
  `MONSTER_ADMIN_PASSWORD` via environment variables — refuses to start
  if either is absent. No credentials written to disk.
- **VULN-003 (predictable default admin email) remediated.** Default
  `admin@deploy.monster` fallback removed. Same env-var enforcement applies.
- **Security report top-risks updated** to reflect the above and correctly
  count VULN-002 (Docker client upstream) as the sole remaining High.

### Fixes

- **golangci-lint v1 config** — dropped unsupported `version: 2` top-level
  key; converted all `path-regexp` exclude rules to `path` (v1 field).
- **Dead `response` variable removed** in `event_webhooks.go` Create method —
  triggered `unusedwrite` govet error.
- **97 stale UI chunks/assets deleted** — refreshed with `pnpm run build`.

### Dependencies

- Vite 8.0.9 → 8.0.10 (2 High severity patched)
- @tailwindcss/vite 4.2.2 → 4.2.4

Sprint 1 of the post-audit roadmap. Four failing tests turned green, one
security regression from v0.1.4 reverted, one canonical defense added.
All 40 Go test packages now green on `go test -short -count=1 ./...`.

### Security

- **CORS-001 regression from v0.1.4 reverted.** Commit `a72550d` collapsed
  the CORS middleware to "always wildcard, never credentials," which
  **defeated the entire `server.cors_origins` allowlist** — any configured
  list was silently ignored. The new two-mode contract honors both shapes
  safely: public mode (`*` or empty) emits `Access-Control-Allow-Origin: *`
  and never `Allow-Credentials` (browsers reject wildcard+creds per the
  fetch spec); allowlist mode echoes the request `Origin` only if it
  matches an entry and emits `Allow-Credentials: true`. New regression
  test `TestCORS_NeverWildcardWithCredentials` walks four failure shapes
  to lock the invariant. `internal/api/middleware/middleware.go`,
  `internal/api/middleware/cors_test.go`.
- **JWT alg-pinning hardened (AUTH-001 follow-up).** Added the canonical
  `jwt.WithValidMethods([]string{"HS256"})` option to both
  `ValidateAccessToken` and `ValidateRefreshToken` in
  `internal/auth/jwt.go`. The pre-existing post-parse `token.Method !=
  HS256` check is kept as belt-and-suspenders. This closes the last
  theoretical `alg=none`/alg-confusion window between `ParseWithClaims`
  and the post-parse guard.

### Fixes

- **Portable ephemeral-port test fixtures.** Four tests hardcoded
  `127.0.0.1:1` as an "always refuses connection" address, which was
  false on the dev workstation (the port answers with HTTP 404 — likely
  an endpoint-protection agent) and on any box with a listener on that
  port. Replaced with the portable `listen on :0 → close → reuse addr`
  pattern: reserve an ephemeral port from the kernel, close it, and use
  the now-free address. Affected: `internal/discovery/health_test.go`
  (`TestHealthChecker_CheckAll_TCPUnhealthy`),
  `internal/ingress/proxy_test.go`
  (`TestReverseProxy_CircuitBreaker_RecordsFailure`),
  `internal/ingress/coverage_boost_test.go`
  (`TestReverseProxy_ServeHTTP_BackendConnectionError`),
  `internal/swarm/swarm_coverage_test.go`
  (`TestAgentClient_Dial_DefaultPort` — the latter additionally used
  `SetDefaultPort` to inject the closed port rather than asserting on
  the occupied-by-DeployMonster default `:8443`).

### Documentation

- **Audit triad published under `.project/`.** `ANALYSIS.md` (honest
  feature/security/code-quality snapshot), `ROADMAP.md` (three-path
  decision framework + sprint plan), `PRODUCTIONREADY.md` (CONDITIONAL
  GO verdict with blocker list). Supersedes the aspirational 100/100
  claim in the repo-root `PRODUCTION-READY.md`.

## [0.1.6] — 2026-04-15 — Comprehensive UX overhaul

Three-phase UX sweep across marketplace, modal/dialog patterns, and the
topology editor. 96 files changed; no schema or API breaks.

### Features

- **Marketplace.** 12 templates gained icons + config schemas; new
  dynamic config-form generator renders variables from the schema;
  featured-templates section on the marketplace index; new
  `/marketplace/:slug` template detail page with services list, resource
  requirements, and compose preview.
- **Modal → Sheet + AlertDialog migration.** New `Sheet` (slide-over
  panel) and `AlertDialog` primitives. `window.confirm()` replaced on
  six pages. Create flows for Servers, Databases, and Git Sources moved
  from `Dialog` to `Sheet` for the wider inputs these forms need.
- **Topology editor fixes.** Removed the dual-state pattern between
  ReactFlow's internal state and the Zustand store — single source of
  truth eliminates the lost-edits class of bugs. Config panel widened
  from `w-72` to `w-96`. Empty-state card shown when the canvas has
  zero nodes instead of an empty grid.

## [0.1.5] — 2026-04-15 — Duplicate tag

Tag points to the same commit as `v0.1.4` (`a72550d`). Left in place
to preserve installer URL stability; no binary difference.

## [0.1.4] — 2026-04-15 — CORS rewrite (later reverted in v0.1.7)

### Changed

- CORS middleware rewritten to always emit wildcard, no origin
  validation, HTTP-only install default. **This change defeated the
  `server.cors_origins` allowlist and was reverted in v0.1.7.**
  Operators on v0.1.4–v0.1.6 should upgrade to v0.1.7 if they rely on
  a configured origin list. `internal/api/middleware/middleware.go`,
  `scripts/install.sh`.

## [0.1.3] — 2026-04-15 — Installer + static-asset CORS bypass

### Fixes

- **install.sh** — auto-generate `secret_key` in generated config so
  fresh installs start cleanly without a manual secret edit.
- **CORS on static assets** — the middleware no longer intercepts
  responses for the embedded SPA asset routes (`/assets/*`,
  `/chunks/*`). Eliminates the class of bugs where aggressive CORS
  preflight rewriting broke cached SPA bundles on certain browsers.

### Documentation

- **README** — quickstart install command and status badge bumped to
  the new tag.

## [0.1.2] — 2026-04-14 — Hotfix: Install script variable scope

Fixes install script regression where variables were not accessible
outside the generate_config function.

### Fixed

- **install.sh** — Fixed variable scope issue:
  - Removed `local` keyword from `domain`, `acme_email`, `admin_email`, `admin_password`
  - Variables now accessible in main() scope after generate_config() call
  - Fixes "unbound variable" errors for domain (lines 482, 500)
  - Added explicit initialization for `GENERATED_ADMIN_EMAIL` and `GENERATED_ADMIN_PASSWORD`

## [0.1.1] — 2026-04-14 — Hotfix: Install script

Quick patch to fix install script regression in v0.1.0.

### Fixed

- **install.sh** — Fixed "unbound variable" error for `domain` in non-interactive mode (line 482, 500)
- Used `${domain:-}` default syntax to handle unset variable gracefully

## [0.1.0] — 2026-04-14 — Production Release

**First production-ready release.** All blockers resolved, 100% test pass rate,
comprehensive documentation complete.

### Highlights

- **Production Readiness Score: 100/100** (was 87/100)
- **Test Coverage: 88.4%** (CI-enforced 85% gate)
- **All 312 test files passing** (20 previously failing handler tests fixed)
- **E2E tests stabilized** (WebSocket origin validation + timing fixes)
- **Security audit complete** (13 findings remediated)

### Fixed

- **WebSocket origin validation test** — Security-compliant behavior, empty Origin rejected
- **JWT nil pointer** — `IsAccessTokenRevoked` handles nil store gracefully
- **Handler test suite** — 20 tests fixed with mock data + auth context:
  - PortHandler tests (4 tests)
  - DomainHandler tests (8 tests) 
  - HealthCheckHandler tests (5 tests)
  - Final handler tests (3 tests)
- **E2E test drift** — Added `data-testid` attributes, fixed auth initialization timing

### Documentation

- **PRODUCTIONREADY.md** — Updated to GO FOR PRODUCTION (92/100 → 100/100)
- **ROADMAP.md** — Phase 1-2 marked complete
- **GAPS_ANALYSIS.md** — All blockers resolved

## [0.0.2] — 2026-04-14 — Security hardenting follow-up

Second release focused on security audit remediation and dependency updates.
All 13 Phase 3 verified findings from the security audit have been resolved.

### Security

- **13 security findings remediated** (Phase 3 audit, all verified):
  - XFF injection in IPHash load balancer — parseXFF() via net.ParseIP validation
  - Webhook secret plaintext storage — SHA-256 hash storage, shown once at creation
  - SSRF DNS rebinding window — validateResolvedHost() re-validates at clone time
  - Bulk operations without rollback — original status tracking + rollback on partial failure
  - JWT key rotation no expiration — RotationGracePeriod (1h) + auto-purge on every validation
  - CSRF SameSite=LaxMode — SameSiteStrictMode on access and refresh cookies
  - Rate limiter XFF spoof — validateIP() rejects private/loopback/link-local IPs
  - Global webhook limit (no per-tenant) — per-tenant keys + 20 limit
  - Stripe webhook no rate limit — 30 req/min per IP in-memory limiter
  - API key entropy increase — 32 random bytes (256-bit entropy)
  - bcrypt cost 12→13
  - Credentials file write error not fatal → startup fails on error
  - Webhook list no pagination — paginateSlice() + writePaginatedJSON()
- **modernc.org/sqlite bumped 1.48.0 → 1.48.2** — 13 bugfixes including memory corruption fixes, data races, resource leaks, ABI fixes
- **golang.org/x/crypto bumped 0.49.0 → 0.50.0**
- **github.com/mattn/go-isatty bumped 0.0.20 → 0.0.21**

### Web UI Dependencies

- **react 19.2.4 → 19.2.5** (React Server Components cycle protections)
- **lucide-react 1.7.0 → 1.8.0** (new icons, aria-hidden fix)
- **globals 17.4.0 → 17.5.0**
- **typescript-eslint 8.57.2 → 8.58.1** (TypeScript 6 support)

### Infrastructure

- **Production-ready report added** — PRODUCTION-READY.md with full audit summary
- **Security reports updated** — Full 4-phase audit documented in security-report/

## [0.0.1] — 2026-04-12 — Initial public release

The first release to ship after the Phase 1–6 audit and the 105-tier
hardening sweep. Every line below is measured against HEAD, not against the
pre-audit aspirational numbers from the v1.4.0 era.

### Breaking

_No breaking config or database changes._ Existing installs upgrade in place with
`0002_add_indexes` applied additively. Min-from is `v1.0.0` per
`docs/upgrade-guide.md`; installs older than `v0.5.0` must step through `v0.5.2`
first so the historical schema is flattened correctly.

### Security

- **17 Dependabot alerts closed** (20 → 3). Tier 91 closed them in
  `pnpm-lock.yaml` via `pnpm.overrides`; Tier 96 actually made GitHub's
  scanner see them as closed by deleting the stale `web/package-lock.json`
  (a leftover from a pre-pnpm era) that Dependabot had silently been
  scanning instead of `pnpm-lock.yaml`. `scripts/build.sh` and
  `scripts/ci-local.sh` rewritten to use `pnpm install --frozen-lockfile`
  instead of `npm ci`, and `.gitignore` updated to ignore
  `web/package-lock.json` so the drift can't reintroduce itself. Remaining
  3 alerts are all upstream-blocked (`fixed=null`): 2 against
  `github.com/docker/docker` (daemon-side CVEs, duplicates of R-001) and 1
  against `go.etcd.io/bbolt` (GHSA-6jwv-w5xf-7j27, no upstream fix published
  yet).
- **`go.opentelemetry.io/otel*` bumped `1.42.0 → 1.43.0`** — CVE-2026-39882
  (OTLP HTTP exporter reads unbounded HTTP response body).
- **`vite` bumped `8.0.3 → 8.0.8`** (direct devDep) — GHSA-p9ff-h696-f583,
  GHSA-v2wj-q39q-566r, GHSA-4w7w-66w2-5vf9 (dev-server path traversal +
  middleware bypass). `pnpm.overrides` added to pin transitive `vite@7` to
  `^7.3.2` via `vitest@3.2.4` → `@vitest/mocker` → `vite@7` chain.
- **`lodash` pinned `^4.18.0`** via `pnpm.overrides` — GHSA-r5fr-rjxr-66jc
  (prototype pollution) and GHSA-f23m-r3pf-42rh (ReDoS). Reached via the
  abandoned `dagre@0.8.5` → `graphlib@2.1.8` chain used by the topology editor.
- **Go toolchain bumped to `1.26.2`** via a `toolchain go1.26.2` directive in
  `go.mod` (keeping `go 1.26.1` as the minimum language version so downstream
  module consumers aren't forced to bump their own floor). Closes stdlib CVEs
  **GO-2026-4870** (unauthenticated TLS 1.3 `KeyUpdate` record → persistent
  connection retention + DoS in `crypto/tls`) and **GO-2026-4866** (case-
  sensitive `excludedSubtrees` name constraints → auth bypass in `crypto/x509`).
  Surfaced in Tier 95 by a `govulncheck ./...` run that caught the original
  Tier 91 documentation claim (bumping the `go` line itself) had never actually
  been committed. `GOTOOLCHAIN=auto` downloads 1.26.2 automatically; CI jobs
  continue to pin `go-version: '1.26'` via `setup-go` (resolves to latest
  1.26.x) with the toolchain directive acting as a hard floor.
- **105 hardening tiers landed** across lifecycle, context-cancellation, replay,
  and DoS vectors that static analyzers cannot catch. Representative fixes:
  ws `DeployHub` Shutdown + concurrent-write mutex + dead-client eviction
  (Tier 77); swarm `AgentServer` wg drain, recover, stopCtx, closed flag
  (Tier 76); resource monitor lifecycle + stopCtx plumbing (Tier 75); deploy
  manager lifecycle + auto-rollback drain (Tier 74); ingress gateway lifecycle
  + ACME ctx plumbing (Tier 73); body-limit bypass fix (Tier 72); auth
  JWT/TOTP/OAuth PKCE hardening (Tiers 78-82); request-scope leak (Tier 83);
  tenant queue fairness (Tier 84). Tail of the sweep (Tiers 100-105):
  deployment status persistence across restart (Tier 100); scheduler WG-reuse
  race + compose nil-logger + lifecycle drain-sleep removal + `-race` CI fix
  across every lifecycle WG + Postgres FK null-scan + notifications-drain
  race (Tier 101); tenant-queue shutdown-unblock deflake + mock docker client
  keepalive close-between-requests + **global rate limiter scoped to `/api/`
  and `/hooks/` only** (previously leaked into `/hooks/ws` and `/metrics`) +
  SPA 404 for missing `/assets/` and `/chunks/` (Tier 102); **auth cookie
  `Secure` flag now gated on request transport** so dev-mode HTTP is reachable
  without losing production HTTPS hardening (Tier 103); SPA embed invariants
  + full-router integration guards (Tier 104); e2e setup fails loudly on a
  broken auth pipeline instead of silent-skip (Tier 105). Full list in
  `docs/security-audit.md`.
- **Argon2id + AES-256-GCM vault** with per-install random salt persisted in
  BBolt at `vault/salt`. Legacy-salt migration path via `migrateLegacyVault()`
  for pre-Phase-2 installs (idempotent). Documented in
  [ADR 0008](docs/adr/0008-encryption-key-strategy.md).
- **`Module.RotateEncryptionKey`** — single-step re-encryption of every secret
  under a new master key.
- **Admin middleware wired at the router (Phase 7.0 closure)** — 20 routes
  (`/api/v1/admin/*` + `POST /api/v1/apps/{id}/transfer`) now pass through a
  single `adminOnly = protected ∘ RequireSuperAdmin` chain in
  `internal/api/router.go`. Previously `middleware.RequireSuperAdmin` was
  defined and unit-tested in `internal/api/middleware/admin.go` but *not wired
  to any route*; inline `claims.RoleID != "role_super_admin"` checks in 7 of
  the 20 handlers were the only gate, and the unguarded 13 — including
  `GET /admin/tenants` — were a cross-tenant enumeration oracle for any
  authenticated user. Inline checks removed from `admin_apikeys.go`,
  `announcements.go`, `db_backup.go`, `migrations.go`, `platform_stats.go`,
  `tenant_ratelimit.go`, `transfer.go`. Four new router-level authorization
  tests in `internal/api/router_test.go` (63 subtest cases total) walk every
  admin route with developer / viewer / unauthenticated / super-admin tokens,
  guaranteeing the wiring cannot drift. Grep proof: `RequireSuperAdmin`
  inside `internal/api/router.go` must return ≥ 1 match (currently 1, via the
  `adminOnly` helper at the top of the route block). 8 stale handler-level
  auth tests (duplicate coverage at the wrong layer) deleted from
  `admin_apikeys_handler_test.go`, `announcements_handler_test.go`,
  `db_backup_handler_test.go`, `final90_test.go`, `final95_test.go`,
  `coverage_boost2_test.go`, `coverage_boost4_test.go`, `handlers_final_test.go`.
- **Cross-tenant authorization fuzz target (Phase 7.11 closure)** — new
  `FuzzRouter_CrossTenant` + `TestRouter_CrossTenantSeedCorpus` in
  `internal/api/router_fuzz_test.go` enumerate 42 resource-scoped GET routes
  from `internal/api/router.go` (every `/api/v1/apps/{id}/*` sub-route,
  `/api/v1/projects/{id}`, `/api/v1/agents/{id}`, etc.) and walk each one
  with a `role_developer` access token minted for `tenant-A` against a
  `crossTenantStore` that pre-seeds `tenant-B`-owned resources. The oracle
  is strict: any 2xx response is a cross-tenant leak (SPA HTML fallthrough
  is the only exception, detected by sniffing `<!doctype html` / `<html`
  prefixes). `url.PathEscape` on the fuzz input keeps raw control bytes
  from panicking `httptest.NewRequest`'s URL parser, and handler panics
  are recovered inside the per-route loop so the fuzzer keeps walking
  after an unstubbed sub-interface method crashes. The fuzz seed corpus is
  promoted to a `Test*` so CI runs it on every `go test ./internal/api/...`
  without requiring `-fuzz`; the `FuzzRouter_CrossTenant` target stays
  local-exploratory because Go's `-fuzztime` is a soft target unsuitable
  for a blocking CI gate. **Two real leaks caught on first run and fixed
  immediately:** (1) `RollbackHandler.ListVersions` /
  `RollbackHandler.Rollback` in `internal/api/handlers/rollback.go` called
  the deploy engine directly with the raw `{id}` from the URL and never
  checked `claims.TenantID` — `GET /api/v1/apps/{id}/versions` returned
  `200 {"data":[]}` for a foreign tenant's app ID. Fixed by routing both
  methods through the existing `requireTenantApp(w, r, h.store)` guard,
  which fetches the application and returns 404 when the tenant mismatches.
  (2) `AgentStatusHandler.GetAgent` in
  `internal/api/handlers/agent_status.go` echoed any requested server ID
  back as a synthetic `AgentNodeStatus` with `status: "unknown"` — a 200
  response that leaked existence of arbitrary IDs to any authenticated
  caller. No remote-agent registry lookup is wired up yet, so the method
  now returns 404 for every non-`"local"` ID; the comment references Phase
  7.11 so a future registry implementation knows to remove the stub. New
  regression test `TestRollbackHandler_ListVersions_CrossTenant` locks in
  the rollback fix (tenant-A claims reading a tenant-B app ID → 404), and
  `TestAgentStatus_GetAgent_Success` was renamed to
  `TestAgentStatus_GetAgent_RemoteReturns404` to codify the hardened
  behavior. Six collateral tests in `coverage_boost2_test.go` that exercised
  the pre-fix rollback handler without injecting claims were updated to
  seed `store.addApp(...)` and `withClaims(...)` so they traverse the new
  tenant guard. Stress-verified: `go test -fuzz FuzzRouter_CrossTenant
  -fuzztime 30s ./internal/api/` → **155,869 execs, 92 interesting inputs,
  zero leaks**.
- **Writers-under-load benchmark + gate (Phase 7.10 closure)** — the
  `MaxOpenConns(1)` ceiling in `internal/db/sqlite.go` is now quantified and
  guarded. New `BenchmarkStore_ConcurrentWrites_64Workers` in
  `internal/db/sqlite_bench_test.go` fans `b.N` `DeploymentStore.Create`
  calls across 64 goroutines through a buffered channel, records per-op
  latency, and emits p50/p95/p99 via `b.ReportMetric` so
  `go test -bench=BenchmarkStore_ConcurrentWrites` surfaces the full
  distribution. New `TestStore_ConcurrentWrites_BaselineGate` in
  `internal/db/concurrent_writes_gate_test.go` runs the same shape with a
  fixed 1024 ops / 64 workers, compares p95 to the committed baseline at
  `internal/db/testdata/concurrent_writes_baseline.json`, and fails on a
  > 10 % regression. The gate is opt-in via `DM_DB_GATE=1` (`go test ./...`
  skips by default to keep local runs fast); `DM_DB_GATE_UPDATE=1` rewrites
  the baseline after an intentional tuning change and `DM_DB_GATE_VERBOSE=1`
  always logs the measurement. Two new Makefile targets: `make db-gate`
  runs the gate, `make db-gate-baseline` re-captures. Initial dev-machine
  baseline: 1024 ops / 2.04 s, 503 ops/s, p50=91ms p95=359ms p99=611ms on
  a Ryzen 9 16-core workstation — confirming the single-writer queue tail
  under burst load. A non-blocking `Writers-under-load gate` step was
  added to the `test` job in `.github/workflows/ci.yml`
  (`continue-on-error: true`) so the gate reports numbers on every CI run;
  flipping it to enforcing is a one-line change paired with a CI-runner
  baseline recapture via `workflow_dispatch`, tracked as a post-v1.6 P2.
- **Secrets scanning in CI (Phase 7.9 closure)** — new `secrets` job in
  `.github/workflows/ci.yml` runs gitleaks v8.30.1 on every push and pull
  request. PRs scan only `origin/<base_ref>..HEAD`; pushes to main scan the
  full git history. The binary is downloaded from the official release URL
  and verified against the committed SHA256
  `551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb` —
  `gitleaks-action` was rejected because it requires a paid
  `GITLEAKS_LICENSE` for any GitHub organization, even public repos, and a
  SHA256-pinned binary is a stronger supply chain than an action pinned by
  commit SHA. New `.gitleaks.toml` at the repo root extends the built-in
  ruleset with narrow allowlists for test files (`_test.go`,
  `web/src/**/*.test.ts(x)`, `web/src/**/__tests__/**`, `web/e2e/*.ts`),
  built SPA assets, API docs under `docs/examples/`, placeholder PEM bodies
  (`MIIB...`/`MIIE...`), the AWS-documented canonical example key pair
  `AKIAIOSFODNN7EXAMPLE`/`wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`, Stripe
  `sk_live_abc123` placeholders, and a generic `test|fake|dummy|example|
  changeme|placeholder` stopword pattern. Locally verified: `gitleaks git`
  scans 305 commits / 19.26 MB in ~1.4 s with zero findings; `gitleaks dir`
  scans the 33.88 MB working tree in ~7 s, also zero findings.
- **Content-Security-Policy tightened on SPA responses (Phase 7.8 closure)** —
  `internal/api/middleware/security_headers.go` dropped the loose `ws: wss:`
  scheme sources from `connect-src` and the `data:` source from `font-src`.
  Same-origin WebSocket traffic (`web/src/hooks/useDeployProgress.ts` builds
  URLs from `window.location.host`) is covered by `connect-src 'self'` per
  CSP 3, and the built SPA `internal/api/static/assets/` has no `url(data:` or
  `@font-face` rules. The stricter `object-src 'none'` was kept. Final
  directive: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'`.
  The global `middleware.SecurityHeaders` chain at `internal/api/router.go:82`
  already wrapped the SPA mux, so CSP was applied to `/` responses all along —
  Phase 7.8 was a directive tightening, not a new wiring. New
  `TestSecurityHeaders_CSPDirective` locks in the exact string across `/`,
  `/dashboard`, and `/api/v1/apps` so any future relaxation trips CI.

### Features

- **OpenAPI drift checker** (`cmd/openapi-gen`, `make openapi-check`). Parses
  `internal/api/router.go` via regex scan of `r.mux.Handle*` calls and
  `docs/openapi.yaml` via `gopkg.in/yaml.v3`, diffs the two sets, fails CI on
  drift. Ratcheting allowlist at `docs/openapi-drift-allowlist.txt` ensures
  the file cannot rot silently: stale entries (drift closed but line remains)
  also fail the check. Wired into the `lint` job in CI. Known gap on first
  run: 232 routes in code vs 88 in spec (144-route backlog, parked in the
  allowlist and expected to shrink over time as operators fill in missing
  spec entries).
- **24-hour soak-test harness** (`tests/soak`, `make soak-test`) + 5-minute
  CI smoke variant (`make soak-test-short`). Sample interval, duration,
  concurrency all configurable.
- **Loadtest regression gate** (`tests/loadtest`, `make loadtest-check`) —
  fails on ≥10% p95 latency regression against a committed baseline at
  `tests/loadtest/baselines/http.json`.
- **Prometheus runtime-metric block on `/metrics/api`** — Go runtime stats
  (goroutines, GC, heap) + per-handler request counters and latency histograms.
- **`internal/db/migrations/0002_add_indexes.sql`** — 30+ indexes on hot query
  paths (`apps`, `deployments`, `audit_log`, `secrets`, `usage_records`).
  Additive and reversible via the paired `.down.sql`.
- **PostgreSQL store contract parity** — full end-to-end CRUD test suite at
  `internal/db/store_contract_test.go` runs against both SQLite and Postgres
  backends on every push via `sqlite` and `pgintegration` build tags.
- **Compile-time `var _ core.Store = (*SQLiteDB)(nil)`** contract assertions
  in both backends catch interface drift at `go build` time. Documented in
  [ADR 0009](docs/adr/0009-store-interface-composition.md).

### Fixes

- **`cmd/openapi-gen`** regex-filters router registrations against the
  `routeMethods` allowlist so string matches on unknown methods like
  `PROPFIND` don't pollute the drift set.
- **`web/src/hooks/useApi.ts`** — `useMutation` narrows by method so
  `api.delete` (which takes `(path, opts)` not `(path, body, opts)`) type-checks
  cleanly.
- **`web/src/stores/__tests__/topologyStore.test.ts`** — custom-type test used
  the literal `'connection'` which is not in the `TopologyEdgeType` union
  (`'default' | 'dependency' | 'mount' | 'dns'`). Switched to `'mount'`.
- **Pre-existing TypeScript drift** fixed as a side effect of the Tier 91
  `pnpm build` verification pass.

### Performance

- **Loadtest baseline** at `tests/loadtest/baselines/http.json` captures the
  committed p50/p95/p99/throughput against which regressions are measured.
- **30+ new indexes** from `0002_add_indexes` — expected impact on hot-path
  reads is quantified in the soak run output, not in this changelog.
- **85%+ CI-enforced test coverage gate** — coverage below 85% fails the
  `test` job in `.github/workflows/ci.yml`.

### Documentation

- **Two new ADRs**: [0008-encryption-key-strategy.md](docs/adr/0008-encryption-key-strategy.md)
  and [0009-store-interface-composition.md](docs/adr/0009-store-interface-composition.md).
  ADR 0010 (in-process event bus) was deliberately skipped because
  [ADR 0006](docs/adr/0006-event-bus-in-process.md) already covers the topic
  completely.
- **`docs/upgrade-guide.md`** — per-version compatibility matrix covering every
  shipped tag from `v0.1.0` through `v0.0.1` (min-from, min Go, config
  break, DB migrations, agent protocol) + three-phase migration checklist
  (pre-flight, during-upgrade, post-upgrade).
- **`docs/security-audit.md`** — "Phase 1–5 hardening fixes (closed)" table
  with 18 rows (H-001 through H-018) + "Residual risk register" with 9 rows
  (R-001 through R-009) ranked by severity.
- **`README.md` + `CLAUDE.md` accuracy pass** — every metric measured against
  HEAD: 240 API endpoints (was 224), 222 handlers (was 115), ~50K Go source
  (was 27K), ~117K Go tests (was 47K), 85%+ CI-enforced coverage (was
  "97% avg"), 56 marketplace templates across 16 categories (was "116",
  which was a naive-grep double-count), ~24MB binary (was 22MB). CLAUDE.md
  state section fixed: the project uses the custom `useApi`/`useMutation`/
  `usePaginatedApi` hook family, **not** TanStack React Query
  (no `@tanstack/react-query` in `package.json`).

### Release engineering

- **`scripts/install.sh` hardened for the curl-pipe entrypoint (Phase 7.5
  prep closure)** — the curl-pipe installer was audited end-to-end and
  rewritten. The pre-audit script passed
  a visual checklist but had three real supply-chain/UX holes: (a) it
  downloaded the release tarball with zero SHA256 verification, so a
  corrupted download or a tampered CDN would install whatever bytes
  showed up; (b) on any download error it silently fell back to
  `go install github.com/.../cmd/deploymonster@latest`, which ships a
  binary **without** the embedded React SPA because `go install` fetches
  the module and builds it without running `pnpm run build` + embed-copy;
  (c) it had no `uninstall` mode at all, despite the roadmap §7.5
  requirement that `curl … | bash -s -- uninstall` must work. The rewrite
  adds: a `verify_checksum()` step that downloads the release's
  `checksums.txt` alongside the archive, extracts the expected hash via
  `awk '$2 == name'` (tolerant of extra whitespace + other entries),
  computes the actual via `sha256sum`/`shasum -a 256` (Linux/macOS), and
  errors out with a precise mismatch message on failure (exit 1); a full
  `do_uninstall()` path that stops + disables + removes the systemd unit
  and binary but intentionally preserves `/var/lib/deploymonster` to
  avoid silent dataloss of bolt buckets, SQLite DBs, vault salt, and
  uploaded secrets; removal of the broken `go install` fallback; a
  hardened `get_latest_version()` that fails loudly on API errors and
  validates the parsed tag against `^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$`
  instead of silently falling through to `VERSION=latest`; a `--version=X`
  override so users can pin a specific release without an API round-trip;
  a `--force` reinstall guard that warns on an existing
  `/usr/local/bin/deploymonster` instead of silently overwriting; a
  `preflight()` check that requires `uname`/`curl`/`tar`/`mktemp` up
  front with a clear `Missing required command: <name>` error; TTY-aware
  color escapes (empty strings when stdin isn't a terminal — keeps logs
  clean under `curl … | bash`); `tar -xzf --no-same-owner` to avoid
  preserving the GitHub Actions runner UID on sudo installs; and a fix
  for the one real shellcheck finding on the pre-audit script (SC2064 —
  `trap "rm -rf ${TMP_DIR}" EXIT` → `trap 'rm -rf "${tmp_dir}"' EXIT`
  so expansion happens at signal time, not at trap-definition time). The
  systemd unit gained `After=network-online.target`, `Wants=network-online.target`,
  `LimitNOFILE=65536`, `Documentation=<repo-url>`, an automatic
  `systemctl enable` so the service survives reboot (the old version only
  ran `daemon-reload`), and an explicit comment explaining why `User=` is
  **not** set (the daemon needs `/var/run/docker.sock` + the ability to
  bind :80/:443 for ACME, so a dedicated unprivileged account is
  operator-configurable but not the default). New `.gitattributes` at
  repo root forces `*.sh text eol=lf` (plus `*.go`, `Makefile`, `*.yml`,
  `Dockerfile`, `go.mod/go.sum`) so Windows contributors with
  `core.autocrlf=true` cannot silently commit CRLF and break the
  curl-pipe installer on the very first variable assignment it hits.
  Locally verified: `shellcheck` → zero findings; `bash -n` → clean;
  `verify_checksum()` exercised against a synthetic good + tampered
  tarball (correct accept + correct reject-with-mismatch); version
  regex exercised against five valid tag shapes + one garbage input.
  End-to-end dry-run on a real VM is Phase 7.5's remaining work and
  requires a staging URL the user owns.
- **`.goreleaser.yml` migrated to v2 schema** — `archives.format: tar.gz` →
  `formats: [tar.gz]` and `archives.format_overrides.format: zip` →
  `formats: [zip]` (both deprecated in goreleaser v2).
- **`goreleaser release --snapshot --clean --skip=before,docker` validated
  end-to-end (Phase 7.2 closure)** — 6 platforms built in ~24 seconds
  (`linux/darwin/windows × amd64/arm64`), archives contain binary + LICENSE +
  README + CHANGELOG + example config, `checksums.txt` covers every archive
  and every SBOM, per-archive SBOMs generated via `syft` (6 files,
  `*_linux_amd64.sbom.json` etc.), binary metadata (`./deploymonster version`)
  returns the expected `version/commit/built/go` block from the ldflags. The
  `--skip=before,docker` flags intentionally offload the full `go test ./...`
  pass to CI (where it already runs) and defer the docker image build to
  Phase 7.6, keeping the local snapshot fast (~24s) and reproducible without
  a running Docker daemon.
- **`Makefile:release-snapshot`** updated to the validated invocation so
  `make release-snapshot` produces the same `dist/` layout a fresh contributor
  would see in CI.
- **`.github/workflows/release.yml`** — new workflow, fires on `v*.*.*` tag
  push. Builds the React UI first (`pnpm install --frozen-lockfile` +
  `pnpm run build`) so the embed.FS is fresh, sets up Go 1.26 with cache,
  downloads `syft v1.18.1` for SBOMs, logs in to `ghcr.io` using the
  workflow's `GITHUB_TOKEN`, then runs `goreleaser-action@v6` with
  `distribution: goreleaser, version: v2.15.2, args: release --clean`.
  Permissions scoped to `contents: write, packages: write`.
- **GHCR image + trivy gate wired up (Phase 7.6 closure)** — the release
  path now produces a scratch-based Docker image, pushes it to
  `ghcr.io/deploy-monster/deploy-monster:{version}` and `:latest`, and
  blocks the workflow on any new HIGH/CRITICAL trivy finding. Root
  `Dockerfile` rewritten as a three-stage build: `node:22-alpine` → SPA
  via `pnpm run build`, `golang:1.26-alpine` + `ca-certificates` + `tzdata`
  → Go binary with `-trimpath -ldflags "-s -w -X main.version/commit/date"`
  and a pre-chowned `/rootfs/var/lib/deploymonster` (scratch has no shell,
  so the VOLUME mount point is chowned at build time), `scratch` runtime
  with `USER 65534:65534`, `VOLUME ["/var/lib/deploymonster"]`, full OCI
  label set (`title`, `description`, `url`, `source`, `vendor`, `licenses`,
  plus dynamic `version`/`revision`/`created`). The Go-builder stage
  **overwrites `internal/api/static/*` with the freshly built SPA from
  stage 1** so a release image can never ship a stale UI even if someone
  forgets to re-run `scripts/build.sh` before tagging. No `curl`, no
  package manager, no shell — HEALTHCHECK intentionally dropped since
  orchestrators define readiness probes and shipping curl just for a
  Docker HEALTHCHECK contradicts the minimal-surface posture. Local build
  produces a **32.1 MB image** (8.3 MB binary layer) on `docker build
  --build-arg VERSION=0.0.1-local --build-arg COMMIT=… --build-arg
  DATE=…`; `docker run deploy-monster:local-phase76 version` reports the
  injected ldflags correctly. `.goreleaser.yml` `dockers:` section
  corrected: `ghcr.io/deploy-monster/deploymonster:*` →
  `ghcr.io/deploy-monster/deploy-monster:*` (the old path silently drifted
  from the `github.com/deploy-monster/deploy-monster` repo slug because
  Phase 7.2 validated with `--skip=before,docker` and never touched the
  image tags), new `--build-arg=DATE={{.Date}}` so `main.date` matches
  goreleaser's timestamp instead of the docker-build wall-clock, explicit
  `--pull` + `--platform=linux/amd64`, full OCI label set baked in via
  `--label` build flags. `.github/workflows/release.yml` grew a `Trivy —
  scan GHCR image` step using `aquasecurity/trivy-action@0.28.0` with
  `severity: HIGH,CRITICAL`, `ignore-unfixed: true`,
  `pkg-types: os,library`, `exit-code: '1'`, scanning the `:latest` tag
  (deterministic within a single workflow run because goreleaser always
  sets it). `grype` was evaluated and rejected — trivy's DB already
  supersets it for Go stdlib + OS packages, and running two scanners in
  the same gate doubles the flake surface without adding coverage. New
  `.trivyignore` at repo root with a single suppression: **CVE-2026-34040**
  (`github.com/docker/docker` Moby daemon authorization bypass) is
  mis-attributed by trivy's DB to the Go client SDK path — `go list -m
  -versions github.com/docker/docker` confirms `v28.5.2+incompatible` is
  still the latest Go tag as of 2026-04-12 and there is no v29 SDK
  release, so the "Fixed Version: 29.3.1" trivy reports refers to the
  daemon binary users run, not to a consumable Go client fix. This is the
  exact false-positive pattern already tracked as upstream-blocked R-001
  in `docs/security-audit.md`. The `.trivyignore` file header documents
  the policy: suppressions are **by exact CVE ID only**, never by
  severity or package name. Locally verified end-to-end: `docker build`
  produces the image, `goreleaser check` passes (only the pre-existing
  `dockers → dockers_v2` deprecation notice), and
  `aquasec/trivy:0.58.0 image --ignorefile .trivyignore --exit-code 1`
  exits 0 with "Some vulnerabilities have been ignored/suppressed".
  Multi-arch (`linux/arm64`) is explicitly deferred to Phase 8 alongside
  the `dockers_v2` migration. `deployments/Dockerfile` left untouched —
  it's the dev-only target for `deployments/docker-compose.dev.yaml`,
  optimized for hot rebuilds not release minimalism.
- **`dockers` → `dockers_v2` migration deferred** — goreleaser v2.15.2 prints
  a soft deprecation warning on `dockers`/`docker_manifests`. The current
  syntax still works; migration is bundled with the Phase 8 multi-arch
  push (`linux/arm64` alongside `linux/amd64`) since both require
  touching the same config block.
- **Known operator footgun**: tags `v1.0.0` through `v1.5.0` are topologically
  orphaned. They were cut on 2026-03-25 pointing to aspirational pre-audit
  commits, but `v0.5.2` was cut later (2026-03-26) from what became the real
  trunk. `git describe --tags --abbrev=0` from HEAD returns `v0.5.2`, not
  `v1.5.0`, because v0.5.2 is the *nearest* reachable tag (200 commits back
  from HEAD at `7add828`). When cutting `v0.0.1`, the tag lands on the
  real ancestry line and `goreleaser` will correctly pick it up. The orphan
  `v1.x` tags should be **left in place** (deleting them is destructive to
  any user who installed from them).

---

## [1.5.0] - 2026-03-29

### Highlights
- **56 marketplace templates** across 16 categories (Grafana, Keycloak,
  Home Assistant, etc.). Earlier drafts of this entry claimed "116" — that
  figure was a naive grep artifact that double-counted `Slug:` references in
  `Related`/`Featured` lists. The authoritative count comes from
  `marketplace.LoadBuiltins()` → `marketplace.Count()` at runtime.
- **Repository migrated** to `github.com/deploy-monster/deploy-monster`
- **Competitive positioning** — Full comparison table vs Coolify, Dokploy, CapRover, Railway
- **Admin roles documentation** — System Admin vs Client Admin clarified

### Added
- Marketplace templates across 16 categories
- Competitive comparison table in README
- Multi-tenancy documentation with admin role examples
- System Admin vs Client Admin role distinction

### Changed
- Repository URL: `github.com/deploy-monster/deploy-monster`
- GoReleaser config updated for new org
- Docker image: `ghcr.io/deploy-monster/deploymonster`
- All documentation URLs updated

### Categories (representative templates)
- **CMS**: Drupal, Strapi, Payload CMS
- **E-commerce**: Medusa, PrestaShop, Sylius
- **Monitoring**: Grafana, Prometheus, Loki, Tempo, Jaeger, cAdvisor
- **Communication**: Matrix Synapse, Rocket.Chat, Mattermost, Zulip
- **Media**: Jellyfin, Immich, Navidrome, PhotoPrism, Audiobookshelf
- **Productivity**: Paperless-NGX, BookStack, Wiki.js, Outline, NocoDB, Baserow
- **Security**: Keycloak, Authentik, Authelia, Portainer
- **AI/ML**: Open WebUI, LocalAI, Stable Diffusion
- **Automation**: Node-RED, ActivePieces, Huginn, Trigger.dev
- **DevTools**: GitLab CE, Gogs, Drone CI, Woodpecker CI, IT Tools
- **Storage**: Seafile, File Browser, ProjectSend
- **Analytics**: Umami, Matomo
- **Finance**: Actual Budget, Ghostfolio
- **IoT**: Home Assistant

### Visual topology editor (landed as part of the 1.5 line, previously
miscredited to an aspirational 1.6 entry)
- Topology Editor page with React Flow canvas (`/topology`)
- Custom node components: AppNode, DatabaseNode, DomainNode, VolumeNode, WorkerNode
- Component palette for drag-and-drop infrastructure design
- Configuration panel for selected node properties
- Topology deployment API (`POST /api/v1/topology/deploy`)
- Auto-layout feature using dagre algorithm
- Environment selector (production, staging, development)
- `@xyflow/react` and `dagre` npm dependencies

---

## [1.4.0] - 2026-03-27

### Highlights
- **97% avg test coverage** across 34 packages (up from 92.8%)
- **Comprehensive ARCHITECTURE.md** with ASCII diagrams
- **247 Go test files** (up from 194)

### Added
- ARCHITECTURE.md with system diagrams, module dependencies, event taxonomy
- Resource module `collectOnce()` method for testability
- Webhooks Trigger error path test coverage
- Ingress ACME manager checkRenewals/issueCertificate tests

### Changed
- Updated README with ECOSTACK TECHNOLOGY OÜ branding
- Added creator info (Ersin KOÇ) with TR/EE context
- Updated Docker image paths to deploy-monster org
- Consolidated .gitignore patterns

### Fixed
- Resource collectionLoop coverage via extracted method
- Gitignore now properly ignores *.test, *.tmp, *.log files

## [1.3.0] - 2026-03-25

### Highlights
- **92.8% avg test coverage** across 20 packages (3 at 100%)
- **194 Go test files** + 6 React test files (50 tests)
- **115/115 handlers** wired to real services (zero placeholders)
- **Enterprise-grade UI** with shadcn/ui, hover transitions, micro-interactions

### Added
- Container exec API (POST /apps/{id}/exec) with real Docker SDK
- Container stats API (real-time CPU/RAM/network/IO per container)
- Docker image management (pull, list, remove, cleanup dangling)
- Docker network and volume listing APIs
- Deploy pipeline: webhook → build → deploy orchestration
- BBolt KV persistence for 30+ config/state buckets
- React component tests: Button, Card, Badge, Input (50 tests total)
- 7 Go fuzz tests for security-critical packages
- 38 Go benchmark functions
- OpenAPI 3.0.3 specification (docs/openapi.yaml)

### Changed
- All 115 handlers now use real services (SQLite Store, BBolt KV, Docker SDK)
- React UI completely redesigned with shadcn/ui components
- Login: gradient branding, glass-effect features, password toggle
- Dashboard: greeting banner, stat cards with trends, quick actions
- Marketplace: category-colored icons, Featured badges, deploy dialog
- Sidebar: collapsible groups, glow logo, theme toggle, Cmd+B shortcut
- All 19 pages: hover transitions, skeleton loading, rich empty states

### Fixed
- Compose parser nil pointer dereference (found by fuzzing)
- Marketplace nil pointer (module init order dependency)
- useApi hook double response unwrapping
- audit_log table name mismatch (audit_logs → audit_log)

## [0.1.0] - 2026-03-24

### Added

#### Core Platform
- Module system with auto-registration via `init()` and topological dependency sort
- EventBus with sync/async handlers, prefix matching, typed payloads
- Store interface (DB-agnostic) — SQLite default, PostgreSQL ready
- Services registry for cross-module communication
- Agent protocol for master/worker architecture
- Core scheduler for recurring tasks
- Configuration validation on startup
- ASCII art startup banner with system info

#### API (223 endpoints)
- Authentication: JWT, refresh tokens, 2FA TOTP, SSO OAuth (Google, GitHub)
- Applications: full CRUD, deploy, scale, rollback, clone, suspend/resume, transfer
- Deployments: versioning, diff, preview, scheduling, approval workflow
- Docker Compose: YAML parser, interpolation, dependency-ordered stack deploy
- Domains: CRUD, DNS verification, SSL status check, wildcard certificates
- Databases: managed PostgreSQL, MySQL, MariaDB, Redis, MongoDB provisioning
- Backups: local + S3 storage, cron scheduler, retention policies
- Secrets: AES-256-GCM vault with Argon2id KDF, ${SECRET:name} resolver
- Servers: Hetzner, DigitalOcean, Vultr, Linode, Custom SSH providers
- Git sources: GitHub, GitLab, Gitea, Bitbucket API providers
- Team: RBAC (6 roles), invitations, audit log
- Marketplace: 25 one-click templates
- Billing: plans, usage metering, Stripe client, quota enforcement
- MCP: 9 AI-callable tools with HTTP transport
- Admin: system info, tenants, branding, license, updates, API keys
- Monitoring: metrics, alerts, Prometheus /metrics endpoint

#### Networking
- Custom reverse proxy (no Traefik/Nginx dependency)
- ACME certificate manager with auto-renewal
- 5 load balancer strategies (round-robin, least-conn, IP-hash, random, weighted)
- Service discovery via Docker label watcher
- Backend health checking (HTTP/TCP)
- Per-app middleware: rate limiting, CORS, compression, basic auth, headers

#### Build Engine
- 14 project type auto-detection
- 12 Dockerfile templates (Node.js, Next.js, Go, Python, Rust, PHP, Java, .NET, Ruby, Vite, Nuxt, static)
- Git clone pipeline with token injection
- Concurrent build worker pool

#### Security
- AES-256-GCM secret encryption with Argon2id key derivation
- Per-app IP whitelist/denylist
- Request ID tracing on every request
- API key authentication (X-API-Key header)
- Audit logging middleware
- Quota enforcement middleware
- Request body size limiting (10MB)
- Request timeout (30s)
- GDPR data export and right to erasure

#### React UI (19 pages)
- Login, Register, Onboarding (5-step wizard)
- Dashboard with real-time stats, activity feed, announcements
- Applications: list (auto-refresh), detail (6 tabs), deploy wizard
- Marketplace with deploy dialog and config vars
- Databases, Servers, Domains, Settings (functional)
- Team (members, roles, audit log), Billing (plan comparison)
- Git Sources, Backups, Secrets, Admin (3 tabs)
- 404 page, error boundary, toast notifications
- CMD+K global search, dark/light/system theme
- Pagination component, loading spinners

#### Operations
- Single binary (22MB) with embedded React UI
- CLI: serve, version, config, init, --agent
- GitHub Actions CI pipeline
- GoReleaser configuration
- curl | bash installer script
- Docker HEALTHCHECK
- monster.example.yaml template

### Performance
- RoundRobin LB: 3.6 ns/op (0 allocations)
- IPHash LB: 26 ns/op (0 allocations)
- LeastConn LB: 55 ns/op (0 allocations)
- JWT Generate: 4.1 μs/op
- JWT Validate: 4.2 μs/op
- AES-256 Encrypt: 633 ns/op
- AES-256 Decrypt: 489 ns/op
- Compose Parse: 17.6 μs/op
- SQLite GetApp: 41 μs/op
