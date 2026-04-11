# DeployMonster — Production Roadmap

> **Audit date:** 2026-04-11
> **Baseline commit:** `b36fba6`
> **Companion documents:** `.project/ANALYSIS.md`, `.project/PRODUCTIONREADY.md`

This roadmap translates the gap analysis in `ANALYSIS.md` into a concrete, sequenced plan to ship a production-quality `v1.0.0`. It is structured as **seven phases**. Each phase has a purpose, an ordered task list, the files and lines involved, an effort estimate in engineer-days, an exit criterion, and a risk profile.

The roadmap is opinionated about sequencing. Phases 1-3 address the concrete blockers; Phases 4-5 harden the platform; Phases 6-7 close the loop on release engineering and documentation. Phases can be partially overlapped by a team of two, but no later phase should start until its predecessor's **exit criterion** is green.

**Total bottom-line estimate:** ~58-78 engineer-days of focused work from HEAD to a defensible `v1.0.0` release, assuming one senior Go engineer + one React engineer + intermittent review from a third pair of eyes. A single experienced full-stack engineer should budget 10-13 calendar weeks.

---

## Phase 1 — Critical fixes: Make the build green and close the bypasses
**Purpose:** Get `go test ./...` passing and eliminate every finding that could be called a security bypass or a silent data-integrity failure. Nothing in later phases should be worked on until this phase's exit criterion holds, because those tasks legitimize a red tree.

**Effort:** 8-11 engineer-days.
**Risk profile:** LOW. Every task here is a scoped, well-understood bug fix.

### 1.1 Unblock the test suite
1. **Decide the admin middleware story** — `internal/api/middleware/admin_test.go` tests `RequireAdmin`, `RequireSuperAdmin`, `RequireOwnerOrAbove`. Either:
   - Implement the three middlewares against `jwtClaims.RoleID` and the `Store.RoleStore` role hierarchy (preferred), **or**
   - Delete the test file (not preferred — leaves the latent vulnerability in place).
   The preferred path is a new `internal/api/middleware/admin.go` with three functions matching the test signatures, each wrapping `http.Handler` and returning 403 on role mismatch. The tests in `admin_test.go` encode the expected behavior; follow them. **Effort: 1 day.**
2. **Implement `CertStore.ListCerts`** — `internal/ingress/cert_monitor_test.go:21` references this method. Add it to the `CertStore` struct in `internal/ingress/cert_store.go` (or wherever `CertStore` lives). Returns `[]*Cert, error`. **Effort: 0.5 day.**
3. **Fix `internal/backup/backup_coverage_test.go`** — the test references an undefined `goodStorage`. Either re-add the helper (if it was intended to survive) or delete the dead test stanzas that reference it. This is a mechanical fix, likely 30 minutes. **Effort: 0.25 day.**
4. **Fix the `TestSQLite_Rollback` deadlock** — `internal/db/sqlite_test.go:240` hangs because the `DOWN` migration does not drop the `tenants` table. Audit `internal/db/migrations/0001_init.down.sql` (it likely has a typo or omits `DROP TABLE IF EXISTS tenants CASCADE`) **and** audit the migration runner in `internal/db/migrations.go` for connection-leak on rollback failure. The deadlock is symptomatic of a `*sql.Tx` not being rolled back on migration error, leaving the single-writer SQLite lock held indefinitely. Fix both the SQL and the leak. **Effort: 1-1.5 days, mostly instrumentation and verification.**
5. **Make `make test` the CI gate.** It currently runs `go test -race -coverprofile=coverage.out ./...`. After the four fixes above, this should go green. Bake it in: if it's red, CI fails. **Effort: 0.25 day.**

**Exit:** `go build ./... && go vet ./... && go test -race -timeout 120s ./...` all pass.

### 1.2 Wire AWS SigV4 into Route53 and S3
6. **Route53 signing** — `internal/dns/providers/route53.go:98` has the comment `// AWS SigV4 signing would go here`. The helper at `internal/dns/providers/sigv4.go` exists, is tested against AWS test vectors, and was written for exactly this use. Replace the comment with a call like `signV4(req, xmlBodyBytes, r.accessKey, r.secretKey, "us-east-1", "route53", time.Now())`. Do the same in `findHostedZone()` (`route53.go:118`). Run `TestRoute53_SignsRequests` until green. **Effort: 0.5 day.**
7. **Extract SigV4 into `internal/aws/sigv4`** — the same helper is needed by S3 backup. Promote the file (along with its test and AWS test vectors) from `internal/dns/providers/sigv4.go` to a reusable package. **Effort: 0.5 day.**
8. **S3 backup signing** — `internal/backup/s3.go:155` has `// Note: Full AWS SigV4 signing would be implemented here.` Replace with a call into the new `internal/aws/sigv4.SignRequest(req, body, access, secret, region, "s3", time.Now())`. Cover all three methods (Upload, Download, Delete). Add a test that runs against localstack or mocks the HTTP with a signature verifier. **Effort: 1-1.5 days.**

**Exit:** Route53 test green, S3 backup test green, and a live `curl`-equivalent manual upload against an AWS test bucket succeeds.

### 1.3 Close the auth and webhook bypasses
9. **Bitbucket webhook signature verification** — `internal/webhooks/receiver.go:292-294` falls through to `return true` for any provider not in the switch. Add a `"bitbucket":` case. Bitbucket uses an `X-Hub-Signature` header (hex HMAC-SHA256 of the raw body with the configured webhook secret). Implement a `VerifyBitbucketSignature(body, secret, header)` helper and add a unit test with a known vector. **Effort: 0.5 day.**
10. **Default branch in `VerifySignature` should reject, not accept.** Current behavior: unknown provider returns `true`. Correct behavior: unknown provider returns `false` AND `receiver.HandleWebhook` returns 400. A "generic" provider that wants to accept unsigned bodies should be a first-class, explicitly configured option, not the default. **Effort: 0.25 day.**

### 1.4 Build log observability
11. **Stream build logs to WebSocket or SSE** — `internal/deploy/pipeline.go:80` passes `io.Discard`. Replace with a `core.BoltStorer`-backed ring buffer per `AppID` + a `*core.EventBus` emitter for `build.log.line`. The React `AppDetail` page should subscribe via the existing SSE/WS plumbing. `useEventSource` already exists. **Effort: 2 days** (backend ring buffer + event + SSE endpoint + React subscriber + Playwright coverage).

### 1.5 MCP advertise-vs-implement parity
12. **Either implement `scale_app` or remove it from the catalog.** Preferred: implement. Add a `case "scale_app":` in `internal/mcp/handler.go:48`, with a `scaleApp(ctx, input)` method that calls `runtime.UpdateContainerReplicas(appID, replicas)` via the deploy module. The test in `mcp_test.go:591` already lists `scale_app` in the expected tool set. **Effort: 0.5 day.**

**Phase 1 exit criterion (ALL must be true):**
- `go test ./...` passes with race detection and coverage, no timeouts.
- `TestRoute53_SignsRequests` green.
- `TestSQLite_Rollback` green and does not deadlock.
- A Playwright E2E test verifies users see build log output in the UI.
- MCP returns a valid response to every tool in `tools.go`.
- Admin-only endpoints are protected by `RequireAdmin` middleware, not handler-local checks.

---

## Phase 2 — Core completion: Fill the spec-vs-code gaps
**Purpose:** Close the feature gaps between what SPECIFICATION.md promises and what ships. This is the phase where users start getting features they were told existed.

**Effort:** 14-18 engineer-days.
**Risk profile:** MEDIUM. These are real feature additions, some with cross-module touch points.

### 2.1 Deploy strategies
1. **Rolling deploy grace period.** `internal/deploy/strategies/strategy.go:272` currently stops old replicas without SIGTERM grace. Pass the `graceful.Shutdown` helper (which already exists in `internal/deploy/graceful/`) with a configurable `StopGracePeriodSeconds` from `App.DeployConfig`. **Effort: 0.5 day.**
2. **Blue-green strategy.** New file `internal/deploy/strategies/bluegreen.go`. Brings up the new replica set at a shadow port, health-checks it, flips the ingress routing table via an `app.route.update` event, then tears down the old set. Requires extending the ingress module with a "secondary target" notion and a cutover primitive. **Effort: 3-4 days.**
3. **Canary strategy.** New file `internal/deploy/strategies/canary.go`. Traffic split via weighted LB (the weighted strategy already exists in `internal/ingress/lb/`). Progressive rollout from 10% → 50% → 100% gated on health-check green AND a pluggable success-rate metric from `internal/resource`. **Effort: 3-4 days.**
4. **Build `Stop()` method.** `internal/build/builder.go` does not expose a clean cancellation path. A deploy cancelled mid-build leaves `docker build` running. Add `Stop(ctx) error` on the `Builder` interface, implement via context cancellation + `docker build --force-rm` on the current build's `BuildKit` client. **Effort: 1 day.**

### 2.2 Ingress: DNS-01 and TLS hardening
5. **DNS-01 challenge support.** `internal/ingress/acme.go` implements HTTP-01 only. DNS-01 requires the ACME client to publish TXT records via the configured DNS provider (Cloudflare, Route53 once SigV4 is wired). Add `challengeDNS01(domain, token)` that calls `Services.DNSProvider(tenantID).CreateRecord(ctx, &DNSRecord{Type: "TXT", Name: "_acme-challenge." + domain, Value: digest})` and polls until propagation. Required for wildcard certs. **Effort: 2-3 days.**
6. **Enforce TLS by default.** Config default should be `ingress.enable_https = true` and `ingress.force_https = true` (HTTP→HTTPS redirect). Opt-out only for explicit local development. **Effort: 0.5 day.**
7. **WebSocket frame rate limiting.** The proxy is fine. The direct WS endpoints (`/api/v1/deploy/logs`, `/api/v1/swarm/agent`, `/api/v1/metrics/stream`) currently have no per-connection frame rate cap. Add a token-bucket middleware (`internal/api/middleware/ws_ratelimit.go`) that enforces 100 frames/sec per connection with a burst of 200. **Effort: 1 day.**

### 2.3 Auth hardening
8. **Hash recovery codes.** `internal/auth/totp.go` stores recovery codes plaintext. Hash with bcrypt cost 10 (not 12 — they are one-time) before storage and compare with `bcrypt.CompareHashAndPassword` on use. **Effort: 0.5 day.**
9. **PKCE on OAuth.** `internal/auth/oauth.go` builds the authorization URL without `code_challenge`. Add S256 PKCE: generate 32-byte random verifier, store in the state cookie (encrypted), send `code_challenge` on authorize, send `code_verifier` on token exchange. **Effort: 1 day.**
10. **Per-deployment vault salt.** `internal/secrets/vault.go` hardcodes `"deploymonster-vault-salt-v1"`. On first boot, generate a 32-byte random salt, persist in BBolt under `dm/vault/salt`, load on subsequent boots. Existing deployments: on first boot of the upgraded binary, re-derive the key with the new salt and re-encrypt all secrets inside a single transaction. **Effort: 1-1.5 days** (care needed around the re-encryption migration).

### 2.4 Notifications & monitoring completeness
11. **Email (SMTP) notification provider.** New file `internal/notifications/smtp.go`. Use `net/smtp` with STARTTLS. Configured via `monster.yaml` `notifications.email.{host,port,from,username,password,tls}`. Implement the `NotificationProvider` interface already used by Slack/Discord/Telegram. **Effort: 1 day.**
12. **Linux host `/proc` metrics.** `internal/resource/host_linux.go` (build-tagged). Parse `/proc/stat` for CPU, `/proc/meminfo` for memory, `/proc/diskstats` and `statfs()` for disk. Expose via the existing resource module's reporting channel so the dashboard and Prometheus endpoint stop reading zero. **Effort: 1-1.5 days.**

### 2.5 Swarm correctness
13. **Unhardcode the agent port.** `internal/swarm/client.go:119` hardcodes `8443`. Replace with a config read of `cfg.Server.Port` or a CLI flag default. **Effort: 0.25 day.**

**Phase 2 exit criterion:**
- All 4 deploy strategies implemented and covered by tests.
- A wildcard certificate can be issued end-to-end against Let's Encrypt staging.
- Recovery codes are hashed; PKCE is on the OAuth path; vault has per-deployment salt.
- Email notifications work against a real SMTP server (Mailhog in CI).
- Host metrics report non-zero on a Linux host.
- A non-default master port correctly accepts agent connections.

---

## Phase 3 — Hardening: Close the operational gaps
**Purpose:** The features are in. Now the system has to survive being run by someone who is not the author.

**Effort:** 10-14 engineer-days.
**Risk profile:** MEDIUM. This phase is where latent issues surface under load.

### 3.1 Graceful shutdown end-to-end
1. **Tier 73-77 style audit on every module with background work.** The recent hardening tiers covered ingress, deploy, swarm, resource, and ws DeployHub. Apply the same pattern (stopCtx, wg.Wait, defer recover, closed flag) to: `internal/build` (currently the worst offender), `internal/backup/scheduler`, `internal/notifications` (async sends). **Effort: 2-3 days.**
2. **Restart storm test.** Spawn 20 apps, kill master, restart master, verify all 20 reconnect and none are left in `deploying` or `building` status. Add as an integration test under `tests/` with a `t.Skip` gate on a `INTEGRATION=1` env var. **Effort: 1.5 days.**

### 3.2 Data durability
3. **Backup restore round-trip test.** There is a backup creation test; there is no test that (a) creates backup → (b) wipes disk → (c) restores from backup → (d) verifies everything is back. Add one. **Effort: 1.5 days.**
4. **SIGHUP config reload test.** `cmd/deploymonster/main.go:122-131` handles SIGHUP. No test exercises it end-to-end. Add an integration test that reloads while a deploy is in flight and verifies the deploy completes without corruption. **Effort: 1 day.**
5. **Stripe webhook replay test.** Use the Stripe CLI in CI to simulate a webhook, replay the same webhook, and verify the second call is a no-op (idempotency). **Effort: 1 day.**

### 3.3 Multi-tenant safety under contention
6. **Resource quota enforcement test.** Tenant A with 100-app limit cannot create the 101st. Easy. But also: tenant A cannot consume CPU/RAM beyond quota even when tenant B is idle. This requires wiring the resource module's per-tenant aggregates into a blocking check in the deploy pipeline. **Effort: 1-2 days.**
7. **Docker daemon exhaustion defense.** If tenant A hammers the builder, tenant B's deploys should still make progress. Today, `max_concurrent_builds` is a global cap. Make it per-tenant (N per tenant, M global) with a waiting queue. **Effort: 1-1.5 days.**

### 3.4 Frontend resilience
8. **Per-request timeout in `web/src/api/client.ts`.** `fetch()` calls have no `AbortController`. Add a 30s default with an override per call. **Effort: 0.25 day.**
9. **Retry with backoff on 5xx.** Retry on 502/503/504 up to 2 times with jitter. **Effort: 0.5 day.**
10. **Cap refresh-token attempts.** Currently `tryRefresh()` can loop. Cap at 1 attempt per 401 with a 30s cooldown. **Effort: 0.25 day.**

**Phase 3 exit criterion:**
- No goroutine leaks detected under `go test -race` across 3 consecutive runs.
- Backup restore completes round-trip from a clean slate.
- Tenant A cannot exhaust tenant B's resources in a stress test.
- Frontend survives 50% of `/api/v1/*` requests returning 503 without a refresh storm.

---

## Phase 4 — Testing: Fill the test gaps
**Purpose:** Raise actual test coverage to match the README claim and, more importantly, close the behavior-coverage gaps identified in §6.3 of ANALYSIS.md.

**Effort:** 7-10 engineer-days.
**Risk profile:** LOW, time-consuming.

### 4.1 Integration tests
1. ~~**Master/agent end-to-end.** Spawn a real master process, spawn a real agent process in a separate goroutine with the `--agent` flag, exercise the full protocol (ping/pong/container.*/metrics.collect/health.check). Today this is mocked at the wire level.~~ **DONE.** Added `internal/swarm/master_agent_integration_test.go` (gated `//go:build integration`) that spins up a real `AgentServer` on a random loopback port and a real `AgentClient` pointed at it, then walks `Ping` (both `SendPing` and `RemoteExecutor.Ping`), `metrics.collect`, `container.list`/`create`/`stop`/`restart`/`remove`/`logs`, raw `health.check`, and an unknown-command error path. The test found and fixed a real bug: `AgentServer.handleAgentMessage` was logging `AgentMsgPong` but never routing it to the `pending` channel, so every `Send`-backed ping round-trip (including the heartbeat loop's per-tick pings) was burning its 5s context deadline. Pong now routes through `pending` identically to `result`/`error`. New CI step in `test-integration` runs it under `-tags integration -race`. **Effort: 2-3 days.**
2. ~~**Let's Encrypt staging integration.** Request a cert from LE staging during CI. Gated on an env var so CI doesn't abuse the staging quota.~~ **DONE.** Added `internal/ingress/le_staging_integration_test.go` (gated `//go:build integration` + env `LE_STAGING_TEST=1`) that drives `golang.org/x/crypto/acme` directly against `https://acme-staging-v02.api.letsencrypt.org/directory`: exercises Discover, Register with `AcceptTOS`, and AuthorizeOrder for a throwaway hostname. Rate-limit responses are converted to `t.Skip` so the runner is never marked broken by external quota. New CI step in `test-integration` runs the smoke test conditionally on repo variable `LE_STAGING_TEST=1`. Full `ACMEManager.issueCertificate` path stays stubbed pending real crypto/acme integration in Phase 5+. **Effort: 1.5 days.**
3. ~~**PostgreSQL store integration test.** `internal/db/postgres_integration_test.go` exists (untracked) for a PostgreSQL `core.Store` that does not yet exist. Plan B: keep the test file, implement the store in Phase 5. For this phase, just run the existing SQLite integration tests against a fresh database in Docker CI to catch regressions.~~ **DONE.** Added `internal/db/sqlite_integration_test.go` (gated `//go:build integration`) that mirrors the Postgres pgintegration flow end-to-end on a fresh file-backed SQLite DB: tenants, users, projects, apps, deployments, domains, secrets, audit, `CreateTenantWithDefaults` tx helper, unique-constraint rollback, and close/reopen persistence. New `test-integration` CI job runs SQLite + SSL + API full-app-startup integration tests under `-tags integration`. **Effort: 1 day.**

### 4.2 Fuzz coverage
4. ~~**Add fuzzers for** webhook signature verification, JWT parse, config loading, marketplace template rendering. Currently 7 fuzzers; target 15.~~ **DONE.** 15 fuzzers now exist: +FuzzVerifyBitbucketSignature, +FuzzVerifySignature (webhooks), +FuzzValidateAccessTokenUntrusted, +FuzzValidateRefreshTokenUntrusted (auth), +FuzzLoadConfig, +FuzzConfigYAMLUnmarshal (core), +FuzzValidateTemplate, +FuzzValidateTemplateNil (marketplace). **Effort: 1-1.5 days.**

### 4.3 Coverage honesty
5. ~~**Measure real coverage after Phase 1-3.** Generate an HTML report, inspect the 5 lowest-coverage files, write targeted tests for the highest-risk uncovered branches (error paths, boundary conditions). Avoid `*_coverage_test.go` files that exercise code without asserting behavior.~~ **DONE.** Targeted the 5 lowest-coverage files (logger.go 28.6% → 100%, retry.go 58.3% → 100%, sigv4.go 34.6% → 99%, restore.go 39% → 89%, topology/deployer.go 40% → 50%+) with behavior-asserting tests. Total coverage rose from 86.4% → 86.7%. **Effort: 1-2 days.**
6. ~~**Raise the CI gate from 80% to 85%** once achieved honestly, and update the README to match reality instead of the 97% aspirational number.~~ **DONE.** CI gate bumped to 85% in `.github/workflows/ci.yml`; README coverage claims (badge, metrics table, project stats) updated from 97% aspirational to 86% measured. **Effort: 0.25 day.**

**Phase 4 exit criterion:**
- 15+ fuzz tests.
- LE staging integration green.
- Master/agent round-trip integration green.
- Real measured coverage ≥ 85% on `go test -cover ./...`.
- README coverage claim matches measured reality.

---

## Phase 5 — Performance & scalability
**Purpose:** Prove the system scales beyond the single-tenant happy path. This phase is also where PostgreSQL lands.

**Effort:** 9-12 engineer-days.
**Risk profile:** MEDIUM-HIGH. PostgreSQL port is new code on a well-defined interface, but it is net-new code.

### 5.1 PostgreSQL as a first-class store
1. **`internal/db/postgres.go`** — implement `core.Store` against `pgx/v5`. Migrations in `internal/db/migrations/*.pgsql.sql` (or a shared `.sql` with dialect shims). Use the existing 12 sub-interfaces as the contract; do not invent new methods. **Effort: 4-5 days.** **DONE (Tier 81):** wired `github.com/jackc/pgx/v5/stdlib`, replaced the hand-rolled no-op test driver, replaced the hand-rolled inline migrate() with a shared embed.FS runner mirroring SQLite. Ported all 26 tables from `0001_init.sql` → `0001_init.pgsql.sql` with Postgres types (TIMESTAMPTZ, BOOLEAN, BIGSERIAL). Added up/down pairs for both 0001 and 0002 pgsql migrations. Both loaders now coexist in the same embed.FS — each skips the dialect files meant for the other backend.
2. **Cross-backend test matrix.** `make test-integration` runs every store test against both SQLite and PostgreSQL. Use build tags or a `DM_DB_DRIVER` env var. **Effort: 1 day.** **DONE (Tier 82):** extracted the end-to-end CRUD flow into `internal/db/store_contract_test.go` (build tag `integration || pgintegration`), a single `runStoreContract(t, store, opts)` function that both backends call with their own placeholder ("?" vs "$1") and raw DB. `sqlite_integration_test.go` now constructs an on-disk DB and calls the shared contract; `postgres_integration_test.go` does the same against a real DSN. Backend-specific assertions (SQLite close+reopen persistence, Postgres pool stats) stay in the wrapper, not the contract. New CI job `test-integration-postgres` spins up a `postgres:17-alpine` service container on every push and runs `-tags pgintegration -race` end-to-end. Added `make test-integration-postgres`, `PostgresDB.DB()` accessor, and a `go vet -tags pgintegration` lint step.

### 5.2 Hot-path performance
3. **Lock-free routing table in ingress.** Replace the `sync.RWMutex` on the routing table with an `atomic.Value` holding an immutable snapshot. Writers build a new snapshot and swap; readers never block. Benchmark before/after under `wrk -c 200 -d 30s`. **Effort: 1 day.** **DONE (Tier 83):** `internal/ingress/router.go` now uses `atomic.Pointer[routesSnapshot]` for reads and a write-only `sync.Mutex` that only serializes concurrent writers — readers never take a lock. Writes clone the current snapshot, apply the mutation, sort, and atomic-Store the replacement. Added `router_bench_test.go`: parallel Match reads drop from 247 ns/op single-threaded to 15 ns/op at 32 cores, and stay at 15 ns/op even with a concurrent writer churning 16 hosts. New `TestRouteTable_LockFree_ConcurrentReadsWrites` drives 8 reader goroutines against a churning writer and asserts zero missed lookups on the stable baseline route. Two legacy callers that reached into unexported `router.mu` / `router.routes` (`health.go`, `metrics.go`) now go through the public `Count()` accessor.
4. **Per-tenant build queue.** Implemented in Phase 3.7; in this phase, add benchmarks and a load-test harness under `tests/loadtest`. **Effort: 0.5 day.** **DONE (Tier 84):** added `internal/build/tenant_queue_bench_test.go` with four microbenchmarks (`Submit_SingleTenant` 801 ns/op, `Submit_ManyTenants` 659 ns/op, `Submit_Parallel` 2163 ns/op, `Submit_SaturatedGlobal` 624 ns/op) guarding the Submit hot path against regressions in the two-phase per-tenant/global acquire pattern. Added a standalone load-test harness at `tests/loadtest/build_queue/main.go` that constructs an in-process `TenantQueue`, drives configurable tenants × jobs with simulated build durations, and reports per-tenant p50/p95/p99/max latency plus a `fairness = max-tenant-p99 / median-tenant-p99` score. Under 4× oversubscription (16 tenants, 4 global slots, 2 per-tenant, 10ms work) all 800 jobs complete with fairness = 1.00x — every tenant sees identical p99 within ~200μs. Exit code 2 if fairness > 2.0x so CI can gate on regressions.

### 5.3 Load testing
5. **`tests/loadtest` already exists in the Makefile (`make loadtest`).** Run it against a production-shaped config and commit a baseline. Alert on regression ≥ 10%. **Effort: 1.5 days.** **DONE (Tier 85):** extended `tests/loadtest/main.go` with rich per-endpoint JSON reports (RPS + p50/p95/p99 in microseconds), a `-save-baseline` flag that captures a run to disk, and a `-baseline` flag that compares against a committed file and exits 2 on a per-endpoint ≥10% regression on EITHER RPS (`current < baseline*(1-threshold)`) OR p95 latency (`current > baseline*(1+threshold)`). Exact-threshold runs pass, endpoints missing from the current run count as regressions, new endpoints are ignored. Added 10 unit tests in `tests/loadtest/main_test.go` pinning the gate semantics (throughput regression, latency regression, at-threshold pass, missing endpoint, extra endpoint, both-axes-regress, round-trip, stable JSON schema). Added Makefile targets `loadtest-check` (30s comparison run, fails on regression) and `loadtest-baseline` (60s capture overwriting the baseline). Committed `tests/loadtest/baselines/http.json` as a real 60s/20-worker capture (99,444 requests, zero errors, ~331 RPS/endpoint, p99 under 90ms) from Windows/Go 1.26.1 seeding the gate — operators regenerate on target CI hardware. Documented the full workflow in `tests/loadtest/baselines/README.md` including regeneration triggers and the hardware-calibration caveat.
6. **Soak test.** 24-hour run at 10% of load-test peak. Verify no goroutine leak, no memory climb, no unbounded audit log growth. **Effort: 1 day** of engineer time + 24h wall clock. **DONE (Tier 86):** built a dedicated soak harness at `tests/soak/main.go` that drives a read-only HTTP workload at ~10% peak (2 workers, round-robin over 5 endpoints) and samples `/metrics/api` at a configurable interval (default 1m for 24h runs, 15s for smoke runs). Harness detects three drift modes with conservative multipliers: goroutine leak (final > 1.5× post-warmup baseline), heap climb (final heap_inuse > 2× baseline), DB bloat (SQLite file > 2× baseline via `-db-file`). Baseline is deliberately chosen from the first sample **after** `warmup-fraction × duration` elapses so GC and cache warmup don't contaminate the reference. Extended `internal/api/middleware/metrics.go` to emit `go_goroutines`, `go_memstats_alloc_bytes`, `go_memstats_heap_inuse_bytes`, `go_memstats_heap_objects`, `go_memstats_sys_bytes`, `go_memstats_next_gc_bytes`, `go_memstats_num_gc`, and `process_uptime_seconds` on the existing unauthenticated `/metrics/api` endpoint so the harness can sample without needing pprof auth. Added `tests/soak/main_test.go` with 6 tests pinning the Prometheus text parser, the skip-labeled-metrics rule, the malformed-line tolerance, byte and duration formatters, and an end-to-end `httptest.NewServer` contract test against the real `/metrics/api` output shape. Smoke-tested end-to-end against a freshly built binary: a 2-minute run drove 190,566 read requests with zero errors, goroutines stayed flat at 27→26, heap oscillated in the 4.9–5.8MiB band (GC healthy at 5,391 cycles), and the harness correctly selected sample 1 (15s elapsed) as the post-warmup baseline over sample 0's 67MiB pre-warmup heap, avoiding a false heap-recovery alarm. New Makefile targets `soak-test` (24h) and `soak-test-short` (5m CI smoke) plus full operator documentation in `tests/soak/README.md` covering flags, drift-gate math, failure-investigation workflow, and the interaction between rate limiting and long-running load drivers.

**Phase 5 exit criterion:**
- PostgreSQL implementation passes the same store contract tests as SQLite.
- Ingress routing table benchmark shows no contention under 200 concurrent connections.
- Soak test at 10% peak load runs for 24 hours without regression.

---

## Phase 6 — Documentation & developer experience
**Purpose:** Make the project runnable and modifiable by someone who is not the author. Correct the drifted claims in README and CLAUDE.md.

**Effort:** 5-7 engineer-days.
**Risk profile:** LOW.

1. **Fix README inaccuracies.** "97% test coverage" → actual measured number. "25+ marketplace templates" → "116". "9 MCP tools" → fix after scale_app is shipped. "TanStack React Query" (CLAUDE.md) → "custom `useApi` hook". Remove or qualify the dev warning ("not yet ready for production") once Phase 1-3 are green. **Effort: 0.5 day.**
2. **OpenAPI regeneration and drift check.** Today `docs/openapi.yaml` is hand-maintained and almost certainly drifted from the 240 registered routes. Write a tool at `cmd/openapi-gen/` that introspects `internal/api/router.go` and emits a spec. Run it in CI and fail on drift. **Effort: 2-3 days.**
3. **ADRs for missing decisions.** Write `docs/adr/0008-encryption-key-strategy.md`, `0009-store-interface-composition.md`, `0010-in-process-event-bus.md`. Each documents why the choice was made, alternatives considered, and under what conditions the decision should be revisited. **Effort: 1 day.**
4. **Upgrade guide.** `docs/upgrade-guide.md` exists (untracked). Complete it with a per-version matrix and a migration checklist. **Effort: 1 day.**
5. **Security audit document.** `docs/security-audit.md` exists (untracked). Complete it with the Phase 1-3 fixes captured as "resolved" findings and an honest list of remaining residual risk. **Effort: 1 day.**
6. **`CLAUDE.md` refresh.** Fix the TanStack drift; accurately describe the `useApi`/`useMutation`/`usePaginatedApi` pattern; remove the `vendor-query` chunk name from any mention. **Effort: 0.25 day.**

**Phase 6 exit criterion:**
- README and CLAUDE.md contain no claim that cannot be verified from HEAD.
- OpenAPI spec is generated from code and CI fails on drift.
- `docs/adr/` covers every non-obvious architectural decision.
- Upgrade and security-audit docs are complete.

---

## Phase 7 — Release preparation
**Purpose:** Move from "we could ship" to "we did ship." This phase is short but it is where the release engineering lives.

**Effort:** 5-6 engineer-days.
**Risk profile:** LOW-MEDIUM. Shipping bugs usually come from process gaps, not code gaps, at this stage.

1. **Version bump.** Adopt the version scheme visible in `STATUS.md` (`v1.4.0` is the current claim; realistic `v0.9.0-rc.1` is probably more honest given Phase 1-6 findings). **Effort: 0.25 day.**
2. **Run `goreleaser release --snapshot --clean`** against HEAD and verify every target platform builds, every SBOM generates, every checksum resolves. **Effort: 0.5 day.**
3. **Smoke-test the release artifacts.** Download the `linux-amd64` tarball into a fresh Ubuntu 24.04 VM, run `./deploymonster init`, run `./deploymonster`, deploy a marketplace app, verify HTTPS cert issues end-to-end. **Effort: 1 day.**
4. **Release notes.** CHANGELOG.md from current `STATUS.md` + Phase 1-6 delta. Group by "breaking", "security", "feature", "fix". **Effort: 0.5 day.**
5. **Dry-run the `get.deploy.monster` installer** against a fresh VPS. The one-line install in the README is a separate project; verify it still works with the new binary. **Effort: 0.5 day.**
6. **Publish to GHCR.** The project uses GHCR (per memory). `docker push ghcr.io/deploy-monster/deploymonster:v1.0.0`. Verify image scan results (Trivy/Grype). **Effort: 0.5 day.**
7. **Announcement checklist.** Website and announcement are separate projects (per memory); coordinate the release date. Security researchers in the security-audit doc get a pre-notification. **Effort: 1-1.5 days** of coordination, not coding.

**Phase 7 exit criterion:**
- Fresh VPS install works end-to-end from a single shell command.
- Release tag is cut, GHCR image is published, checksums and SBOMs are posted.
- CHANGELOG documents every Phase 1-6 change.

---

## Risk assessment and mitigation

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Phase 1 fixes uncover deeper bugs (e.g. the `TestSQLite_Rollback` deadlock is the tip of a connection-pool iceberg) | MEDIUM | HIGH | Budget a 20% buffer on Phase 1 and do not cross into Phase 2 until `go test -race` is clean across 3 consecutive runs. |
| Blue-green and canary strategies (Phase 2.1) require changes to the ingress module that destabilize Phase 1 work | MEDIUM | MEDIUM | Do these in a feature branch with a separate merge after Phase 3 is green. |
| PostgreSQL port (Phase 5.1) introduces subtle bugs (null semantics, collation, tx isolation) | HIGH | MEDIUM | Run the full SQLite test suite against the Postgres store nightly for two weeks before declaring Phase 5 done. |
| SigV4 rework breaks Cloudflare DNS (Phase 1.2) because of shared HTTP client plumbing | LOW | HIGH | Keep SigV4 in a new `internal/aws/sigv4` package. Cloudflare does not use it. |
| "Enforce TLS by default" (Phase 2.2.6) breaks existing installs on upgrade | MEDIUM | MEDIUM | Gate on config version; new installs default on, upgrading installs keep their old setting with a deprecation warning. |
| Per-tenant build queue (Phase 3.3.7) introduces starvation under adversarial load | LOW | MEDIUM | Add fairness guarantee (max N per tenant, max M global, round-robin dequeue). |
| Log streaming (Phase 1.4.11) leaks BBolt space | MEDIUM | LOW | Ring-buffer cap per app (default 10 MB), GC older entries on write. |
| CI regression on 85% coverage gate (Phase 4.3.6) | LOW | LOW | Roll out coverage gate as "warn" for 2 weeks before "fail". |
| Release installer dry-run (Phase 7.5) fails on upgraded OS (Ubuntu 24.04) | LOW | MEDIUM | Cover 20.04 / 22.04 / 24.04 / Debian 12 explicitly. |
| Marketing misalignment between README numbers and reality causes community embarrassment | CERTAIN (already happened) | LOW | Phase 6.1 fix is mandatory before the release announcement. |

---

## Bottom-line estimate summary

| Phase | Scope | Estimate (engineer-days) |
|---|---|---|
| 1 | Critical fixes | 8-11 |
| 2 | Core completion | 14-18 |
| 3 | Hardening | 10-14 |
| 4 | Testing | 7-10 |
| 5 | Performance & Postgres | 9-12 |
| 6 | Docs & DX | 5-7 |
| 7 | Release | 5-6 |
| **Total** | **v1.0.0-ready** | **58-78 engineer-days** |

For a two-engineer team working in parallel with light coordination overhead, 6-8 calendar weeks. For a single engineer, 11-16 weeks.

The team should treat Phase 1 as non-negotiable and Phases 4-7 as the correct place to negotiate scope if the deadline is fixed.

---

*Prepared 2026-04-11 as a companion to `.project/ANALYSIS.md` and `.project/PRODUCTIONREADY.md`. No code was modified during the audit.*
