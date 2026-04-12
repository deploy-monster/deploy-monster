# DeployMonster — Project Analysis

> **Audit date**: 2026-04-11
> **HEAD commit**: `7add828` — *harden: e2e setup fails loudly on broken auth pipeline (Tier 105)*
> **Version**: v0.0.1
> **Auditor**: Senior architecture / production-readiness review
> **Scope**: Full read-only audit of source, tests, docs, and build surface

---

## 0. How to read this document

This analysis was produced by walking the code at HEAD, not by trusting `STATUS.md` or `ROADMAP.md`. Every claim below is backed by a file path (and usually a line number) so you can re-verify. Where the existing project docs are out of date or contradict the code, this audit sides with the code and flags the drift.

A previous audit was produced at **Tier 77**, scoring 64/100 with a NO-GO verdict. Since then 28 hardening tiers (Tier 78 → Tier 105) have landed. This analysis re-verifies every finding from that audit and is clear about what moved, what regressed, and what was never fixed.

---

## 1. Project at a glance

| | |
|---|---|
| Domain | Self-hosted PaaS, single-binary modular monolith with embedded React UI |
| Language | Go 1.26.1 (toolchain go1.26.2) + TypeScript 5.9 / React 19.2.4 |
| Architecture | 20 auto-registered modules, in-process EventBus, store-interface composition, same-binary master/agent |
| Data | SQLite (`modernc.org/sqlite`, pure Go) primary, BBolt KV for runtime state, PostgreSQL contract suite |
| Runtime target | Single static binary (~24 MB), embedded UI via `embed.FS` |
| Web | React 19 + Vite 8 + Zustand 5 + React Router 7 + Tailwind 4 + shadcn/ui patterns |
| Repo | github.com/deploy-monster/deploy-monster |
| Commits | 308 total, every one in 2026 — young repository, high churn |
| Branch | `master` (clean working tree at audit time) |

---

## 2. Metrics (measured)

| Metric | Value | Source |
|---|---|---|
| Go source files (non-test) | 322 | `find internal -name '*.go' -not -name '*_test.go'` |
| Go test files | 352 | `find internal -name '*_test.go'` |
| Test-to-source ratio | 1.09 | derived |
| API routes | 240 | `STATUS.md` / router.go handler wiring |
| Modules | 20 | `internal/*/module.go` + `cmd/deploymonster/main.go` blank imports |
| Marketplace templates | 56 | `STATUS.md`, verified by `internal/marketplace` directory listing |
| CI coverage gate | 85 % | `.github/workflows/ci.yml` |
| Fuzz targets | 15 | `STATUS.md` / `make bench` |
| Benchmarks | 46 | `STATUS.md` / `make bench` |
| Direct Go deps | 10 | `go.mod` lines 7–17 |
| Indirect Go deps | 42 | `go.mod` lines 19–62 |
| React runtime deps | 13 | `web/package.json` |
| Vitest suites | 341 tests / 38 files | `STATUS.md` |
| Remaining Dependabot alerts | 3 (upstream-blocked, documented) | `docs/security-audit.md` |
| Binary size (stripped) | ~24 MB | build pipeline |

> **Reality check on LOC**: `STATUS.md` claims ~188 K total and ~50 K Go source. The test-to-source file ratio above (1.09) is healthy for a project of this age; the LOC ratio in `STATUS.md` (117 K test vs 50 K source) is suspicious and almost certainly double-counts generated testdata or fuzz corpora. It does not invalidate the test coverage number — the coverage gate is measured by Go's own tooling in CI — but any narrative that says "we have 117 K lines of hand-written tests" should be viewed skeptically.

---

## 3. Repository layout

```
deploy-monster/
├── cmd/
│   └── deploymonster/main.go          # entrypoint; blank-imports every module
├── internal/
│   ├── api/                           # HTTP router, middleware, handlers (~240 routes)
│   │   ├── router.go                  # central wiring (842 lines)
│   │   ├── middleware/                # auth, rate limit, CORS, CSRF, idempotency, audit, admin
│   │   ├── handlers/                  # ~80 handler files, each scoped to a resource
│   │   └── static/                    # embedded React build
│   ├── auth/                          # JWT, bcrypt, TOTP, OAuth SSO, API keys
│   ├── billing/                       # Stripe-style abstraction
│   ├── build/                         # 14 language detectors, 12 Dockerfile templates
│   ├── core/                          # Module, Store, EventBus interfaces; DI container
│   ├── db/                            # SQLite driver, migrations, store implementations
│   ├── deploy/                        # deploy pipeline (recreate + rolling)
│   ├── domains/                       # DNS + ACME integration
│   ├── events/                        # in-process pub/sub
│   ├── ingress/                       # reverse proxy, ACME, load balancer
│   ├── marketplace/                   # template registry
│   ├── monitoring/                    # runtime metrics + Prometheus exporter
│   ├── secrets/                       # AES-256-GCM + Argon2id vault
│   ├── swarm/                         # master/agent WS protocol
│   └── ...                            # 20 modules total
├── web/                               # React 19 + Vite 8
├── docs/
│   ├── openapi.yaml                   # CI-gated drift check
│   ├── security-audit.md
│   └── adr/                           # 9 ADRs
├── scripts/
└── .project/                          # project docs + audit output (ASCII dot)
```

The `.project/` directory now holds both the original spec/implementation/tasks docs and this audit's output. `CLAUDE.md` references it correctly.

---

## 4. Architecture

### 4.1 Backend: modular monolith

Every module lives under `internal/<name>/` and implements `core.Module`:

```go
type Module interface {
    ID() string
    Name() string
    Version() string
    Dependencies() []string
    Init(ctx context.Context, c *Core) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Health() Health
    Routes() []Route
    Events() []EventHandler
}
```

Registration is via `init()` + `core.RegisterModule()`, and `cmd/deploymonster/main.go` blank-imports all 20. Dependency order is resolved by topological sort on `Dependencies()`; shutdown runs in reverse with a 30 s timeout. This is the right shape for a single-binary PaaS — it gives you clean seams without distributed-system overhead.

**Strengths**
- Real dependency injection via `core.Core` — no singletons, no package-level state
- Module boundaries are enforced by Go's package system, not just convention
- Same binary runs as master or agent (`--agent` flag); reduces operational surface
- Graceful shutdown is actually implemented (not just `os.Exit`)

**Weaknesses**
- The module registry is bespoke (it is not a generic DI framework); new contributors will need to read `internal/core/module.go` and `internal/core/registry.go` before they can add a module
- `core.Core` has grown large — it exposes `Store`, `DB`, `Config`, `Events`, `Logger`, `Services`, `Build`, `Registry`. Passing it as a god-object is convenient during wiring but couples modules to more than they need
- Module-level health is binary-ish (`HealthOK` / `HealthDegraded` / `HealthDown`) — no SLO surface, no per-dependency breakdown

### 4.2 Store interface composition

`internal/core/store.go` defines `Store` as a composition of 12 sub-interfaces: `TenantStore`, `UserStore`, `AppStore`, `DeploymentStore`, `DomainStore`, `ProjectStore`, `RoleStore`, `AuditStore`, `SecretStore`, `InviteStore`, `UsageRecordStore`, `BackupStore`. Anyone consuming persistence goes through the interface — `*db.SQLiteDB` is never referenced outside the driver package. ADR-0009 documents this as a deliberate choice to unblock a Postgres backend.

The Postgres backend now has a contract suite (`STATUS.md` says both SQLite and Postgres contract suites are green), which means the interface is actually portable, not aspirationally portable. That is a very meaningful upgrade from the Tier 77 state.

### 4.3 Event bus

In-process pub/sub at `internal/core/events.go`:

- `Subscribe(type, handler)` — synchronous, caller blocks on fan-out
- `SubscribeAsync(type, handler)` — bounded async: a 64-slot semaphore (Tier 82) prevents the runaway goroutine explosion that was on the previous audit's risk list
- Matching: exact (`app.created`), prefix (`app.*`), wildcard (`*`)
- Naming convention: `{domain}.{action}` — enforced by lint, not by a central registry

This is fine for a single-node PaaS. For master/agent fan-out across nodes, agents communicate via the WebSocket swarm protocol, not the event bus, which is the right separation.

### 4.4 API layer

Go 1.22+ `http.ServeMux` with method-scoped pattern syntax (`POST /api/v1/apps/{id}/deploy`). Middleware chain is built in `Router.buildChain` (router.go ~74–94):

```
RequestID → APIVersion → BodyLimit(10MB) → Timeout(30s) → Recovery →
RequestLogger → CORS → CSRFProtect → IdempotencyMiddleware → AuditLog
```

Per-route wrappers add authentication (`authMiddleware`) and per-tenant rate limiting (`tenantRL.Middleware`), combined into `protected` at router.go:100–102:

```go
protected := func(next http.Handler) http.Handler {
    return authMiddleware(tenantRL.Middleware(next))
}
```

**Auth levels** (in theory): AuthNone, AuthAPIKey, AuthJWT, AuthAdmin, AuthSuperAdmin.
**Auth levels** (in practice): AuthNone, AuthJWT-or-APIKey. Admin/SuperAdmin is not enforced via middleware at the router level — see §8.1 for the critical finding.

**Auth primitives**: JWT HS256 access (15 min) + refresh (7 day), claims carry `UserID`, `TenantID`, `RoleID`, `Email`. API keys go through a separate header path. Both land in `auth.ClaimsFromContext(r.Context())`.

**Webhooks**: `POST /hooks/v1/{webhookID}` — HMAC-verified, 1 MB body limit (tighter than the 10 MB global), separate rate limit bucket. The global rate limiter was scoped to `/api/` and `/hooks/` in Tier 102 (`fe08133`) which removed an earlier risk of rate-limiting `/metrics/api` and the embedded SPA assets.

### 4.5 Database layer

- **SQLite** via `modernc.org/sqlite` — pure Go, no CGO, single file. Migration 0001 seeds roles; migration 0002 adds 30+ indexes on hot paths.
- **BBolt KV** (`go.etcd.io/bbolt`) — 30+ buckets for config, state, metrics, API keys, webhook secrets. Used anywhere the data model is naturally key-value and doesn't need joins.
- **PostgreSQL** via `jackc/pgx/v5` — Tier 81 introduced the contract suite, and `STATUS.md` claims both backends pass it.
- **Connection pool**: `MaxOpenConns(1)` on SQLite by design to serialize writes and sidestep the "SQLITE_BUSY" cliff; the previous rollback-test deadlock (Tier 77 finding) is now fixed (`internal/db/sqlite_test.go:220–294` closes the rows handle and comments the reason at line 236).

**Risk**: `MaxOpenConns(1)` is a silent contention ceiling. Under heavy concurrent writes, the pool becomes a global serialization point that will not show up as an error — just as latency. Benchmarks exist (46 targets) but I did not see a dedicated "writers-under-load" benchmark. Flag for Phase 7 / Phase 8 work.

### 4.6 Deploy pipeline

Webhook → git clone → detect (14 language detectors under `internal/build/detect/`) → build (12 Dockerfile templates) → deploy (recreate or rolling). The build step is still single-host on the master; agents run containers but do not build. This is fine at v0.0.1 scale but is a future bottleneck.

### 4.7 Frontend

React 19.2.4 (new) + Vite 8 + TypeScript 5.9 + Tailwind 4 + Zustand 5. No TanStack React Query — data-fetch state lives in a custom `useApi` hook family (`web/src/api/client.ts`, `web/src/hooks/`). This is a deliberate choice and it keeps the bundle small, but it means the cache-invalidation story is whatever the hook does today; regressions here are easy to miss.

Embed is via `embed.FS`. Tier 104 added a "SPA embed invariants + full-router integration guards" commit (`744f684`) which materially reduced the risk of the /assets/ 404 class of bugs.

---

## 5. Security posture

### 5.1 What's in place

- **Password hashing**: bcrypt with cost 12 (measured in `internal/auth/password.go`)
- **JWT**: HS256, 15 min access / 7 day refresh, separate refresh rotation table, revocation list
- **MFA**: TOTP (`internal/auth/totp.go`) with backup codes
- **SSO**: OAuth2 handlers for Google, GitHub, generic OIDC
- **Secrets vault**: AES-256-GCM + Argon2id, per-install random salt (Tier 83 fix for the previous audit's hard-coded-salt finding). Legacy migration path is in place for installs predating the fix — see `docs/adr/0008-encryption-key-strategy.md`.
- **Rate limiting**: global (scoped to `/api/`, `/hooks/`), per-tenant (100 req/min default), per-auth-endpoint tighter buckets (login 5/min, register 3/min, refresh 5/min)
- **CSRF**: `middleware.CSRFProtect` in the global chain
- **CORS**: origins-allowlist from config
- **Audit log**: middleware writes every authenticated request to `audit_log` table
- **Body limits**: 10 MB global, 1 MB webhook
- **Request timeout**: 30 s
- **Idempotency**: `Idempotency-Key` header → BBolt dedup bucket
- **Cookies**: `Secure` flag gated on request transport (Tier 103, `091f6c4`)
- **Prometheus exposure**: runtime-metric block on `/metrics/api`, no sensitive labels I could spot

### 5.2 What is NOT in place

The biggest finding of this re-audit is §8.1 below: **admin middleware functions exist but are not wired to any route**. The compensating controls are inline role checks in some (not all) admin handlers, which is brittle and has already let several endpoints slip through.

Other gaps:

- **No network-level mTLS between master and agent** — the swarm protocol relies on a pre-shared token over WebSocket+TLS. That is fine for LAN clusters, risky for WAN
- **No signed-commit enforcement on the binary release pipeline** — see Phase 7.6 in the roadmap
- **No secrets scanning in CI** — there is `make openapi-check` but no `gitleaks` / `trufflehog`
- **No CSP header** that I can find on the embedded SPA responses (Tier 104 added embed invariants but I did not see CSP)

### 5.3 Dependabot delta

- Starting: 20 alerts open at Tier 77
- Now: 3 open at HEAD, all upstream-blocked and accepted in `docs/security-audit.md`
- 17 closed via targeted upgrades (otel 1.42→1.43, vite 8.0.3→8.0.8, lodash pin, vite@7 transitive pin)

This is real progress and was one of the three hard blockers on the previous audit.

---

## 6. Test surface

### 6.1 Measured state at HEAD

- `go test -short ./...` — **GREEN across every package** (verified during Phase 0 discovery)
- `go vet ./...` — clean
- `go build ./cmd/deploymonster` — clean
- Coverage gate: 85 % in CI; drops below → hard fail
- 15 fuzz targets, 46 benchmarks
- Integration: SQLite + Postgres contract suites both green
- React: 341 vitest tests / 38 files pass
- E2E: Playwright suite (`web/tests/e2e/`), Tier 105 added a loud-fail setup guard (`7add828`) so a broken auth pipeline no longer silently short-circuits the harness
- Soak harness: 24 h soak runner + 5 m CI smoke
- Loadtest: committed baseline + 10 % p95 regression gate

### 6.2 Delta from Tier 77 audit

The previous audit listed the test suite as red (`TestSQLite_Rollback` deadlocking, several handler tests flaking). Verified fixed:

- `internal/db/sqlite_test.go:220–294` — TestSQLite_Rollback now explicitly closes the rows handle and documents *why* at line 236: `MaxOpenConns(1), so leaking a cursor here would deadlock`
- Handler flakes: the table-driven tests under `internal/api/handlers/` all pass in `go test -short`
- Race-flagged runs pass in CI

### 6.3 What's still thin

- **Writers-under-load benchmark**: no dedicated target that exercises `MaxOpenConns(1)` at meaningful concurrency
- **Rolling-deploy chaos test**: `internal/deploy/rolling.go` has unit coverage but I did not see a test that kills a container mid-rollout
- **Cross-tenant fuzz**: the multi-tenant isolation story would benefit from a dedicated fuzz target that tries to reach a different tenant's resource via every route. Ties directly to §8.1.
- **Frontend E2E depth**: 38 test files is thin for a 240-endpoint product. The critical-path flows are covered, but most of the endpoint surface is not exercised through the UI.

---

## 7. Tooling & developer experience

### 7.1 Build

- `make build` — `scripts/build.sh` runs React → embed copy → Go build with ldflags (version, commit, date)
- `make dev` — `go run` with hot reload via vite proxy
- `make test` — race detection + coverage
- `make test-short` — skip integration, runs in ~seconds
- `make lint` — golangci-lint
- `make bench` — benchmarks + fuzz
- `make openapi-check` — drift check against `docs/openapi.yaml` (CI-gated)

### 7.2 CI

- `.github/workflows/ci.yml` — lint, vet, test, coverage ≥85 % gate, openapi drift, loadtest regression gate, soak smoke
- Dependabot enabled and acted on (17 alerts closed)
- No release workflow committed yet — `goreleaser` config exists but `snapshot --clean` pipeline validation is still pending (Phase 7.2)

### 7.3 Docs

- `README.md` — current, reflects 240 endpoints / 56 templates
- `docs/openapi.yaml` — CI-gated
- `docs/security-audit.md` — maintained
- `docs/adr/` — 9 ADRs including the two new ones (0008 encryption-key strategy, 0009 store composition)
- `docs/upgrade.md` — per-version matrix v0.1.0 → HEAD
- `.project/SPECIFICATION.md` — full product spec (still the source of truth for business logic)
- `.project/IMPLEMENTATION.md` — code-level patterns
- `.project/TASKS.md` — 251-task ordered checklist, 100 % complete at v0.0.1

**Drift**: `.project/STATUS.md` and `.project/ROADMAP.md` both declare Phase 6 done and Phase 7 largely done. The code supports most of these claims but the **admin middleware wiring claim is false** — see §8.1.

---

## 8. Findings

Findings are ranked P0 (must fix before cutting v0.0.1 final) / P1 (must fix before scale) / P2 (should fix) / P3 (tech debt). Every finding is backed by a file path so you can re-verify.

### 8.1 P0 — Admin middleware exists but is not wired; compensating controls are incomplete

**This is the single most important finding in this audit.** The previous roadmap claimed this was DONE; the code says otherwise.

**Evidence**:

1. `internal/api/middleware/admin.go` defines `RequireAdmin` (line 44), `RequireSuperAdmin` (line 54), `RequireOwnerOrAbove` (line 65). All three wrap a private `requireRole()` helper that returns 401 on missing claims and 403 on role mismatch. Unit tests in `admin_test.go` (14 tests) verify the behavior correctly.
2. **Grep for any use in `internal/api/router.go`**: zero matches. I ran this explicitly.
3. Every `/api/v1/admin/*` route is wired via `protected(...)` only (router.go:688–757), which is `authMiddleware ∘ tenantRL.Middleware` (router.go:100–102). That gives you authenticated-user + per-tenant rate-limit. **It does not check role.**
4. Compensating controls: some handlers do `if claims == nil || claims.RoleID != "role_super_admin"` inline. Verified present in:
   - `admin_apikeys.go:59, 139` (List, Generate/Revoke)
   - `platform_stats.go:24` (Overview)
   - `db_backup.go:26, 71` (Backup, Status)
   - `migrations.go:22` (Status)
   - `tenant_ratelimit.go:41, 63` (Get, Update)
   - `announcements.go:57` (Create only — Dismiss does NOT check)
   - `transfer.go:29` (TransferApp)
5. **Missing role checks in these admin handlers** (verified by grep):
   - `admin.go:22–57` — `SystemInfo` (GET /admin/system) — leaks version, commit, Go runtime info, module registry, event stats, memory, goroutine count to **any authenticated user**
   - `admin.go:60–72` — `UpdateSettings` (PATCH /admin/settings) — handler is a stub that doesn't persist anything yet, but it's wired and wide open. The instant someone plugs persistence into line 67 this becomes an RCE-adjacent surface
   - `admin.go:76–89` — `ListTenants` (GET /admin/tenants) — **any authenticated user can enumerate every tenant on the platform**. This is a real cross-tenant information leak, not a hypothetical one
   - `announcements.go:99` — `Dismiss` (DELETE /admin/announcements/{id}) — no role check, any authed user can dismiss
   - `license.go` — Get + Activate — no `claims` references in file; any authed user
   - `branding.go` — Update — no `claims` references in file; any authed user
   - `disk_usage.go` — SystemDisk — no `claims` references in file; any authed user
   - The self-update handler wired at router.go:752 — no `claims` references in the underlying handler file; any authed user

**Impact**:
- **High, realized**: `/admin/tenants` is a cross-tenant enumeration oracle for any logged-in user. This alone is a v0.0.1 blocker.
- **Medium, latent**: the stub handlers (`UpdateSettings`, `license.Activate`, `branding.Update`) are ticking bombs — the moment someone wires persistence or side effects they become privilege-escalation primitives.
- **Systemic**: the inline-check pattern is fragile. Every new admin endpoint needs the author to remember to copy the pattern. Several endpoints already forgot. There will be more.

**Fix shape**:
1. Wrap the middleware at the router level. At router.go the admin block (roughly lines 688–757) should read `protected(middleware.RequireSuperAdmin(http.HandlerFunc(...)))` (or `RequireAdmin` / `RequireOwnerOrAbove` per endpoint).
2. Once the middleware is wired, **remove** the inline `claims.RoleID != "role_super_admin"` checks from the handlers — keep a single source of truth for authorization, not defense-in-depth that's actually defense-in-duplication.
3. Add a router-level test that asserts every `/api/v1/admin/*` route, when called with a `role_developer` token, returns 403.
4. Update `ROADMAP.md` line 22 — the claim that this is DONE is objectively false at HEAD.

**Effort**: 0.5 engineer-day including the table-driven test.

---

### 8.2 P1 — `.project` cleanup is a drift generator

The `.project/` directory holds both original project docs and audit output, but references in `CLAUDE.md` and `STATUS.md` were still split or stale.

**Impact**: Medium. Any new contributor might look for the wrong path, and stale references break after consolidation.

**Fix**: update all stale `.project/` references to the canonical path in Phase 7 cleanup. Not urgent for v0.0.1 final but must be done before v2 work starts.

---

### 8.3 P1 — `MaxOpenConns(1)` is an unvalidated throughput ceiling

SQLite pool is serialized to 1 writer. Benchmarks exist (46 targets per `STATUS.md`) but I did not find a writers-under-load benchmark that exercises the ceiling. Under a burst of concurrent deploys, this will look like "deploys are slow" rather than "the DB is pegged".

**Fix**: Add a benchmark target `BenchmarkStore_ConcurrentWrites_64Workers` that hammers the `DeploymentStore.Create` path and asserts p95 latency stays under the committed baseline. One engineer-day. If the ceiling turns out to be real, Phase 8 becomes "move runtime state off SQLite onto BBolt or onto Postgres."

---

### 8.4 P1 — No CSP on the embedded SPA responses

Tier 104 landed SPA embed invariants but I did not see a `Content-Security-Policy` header on SPA responses. The UI is embedded and server-rendered as a single bundle; the CSP can be strict without affecting dev velocity. Phase 4 hardening item that did not make it in.

**Fix**: Add CSP in `middleware.SPAHeaders` (or equivalent) with `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'`. Verify the Tailwind 4 build doesn't need any extra exceptions. 0.5 engineer-day.

---

### 8.5 P1 — AWS EC2 VPS provider is still missing

The previous audit flagged this and it remains the only feature-level gap. Hetzner, DigitalOcean, and Linode providers exist under `internal/vps/providers/`; EC2 does not. The marketing page and docs promise "any VPS", and the plugin interface supports it, but the EC2 implementation is a TODO.

**Effort**: 3–5 engineer-days for a first-class provider (auth, region selection, AMI picker, security group, SSH key injection, teardown). Can be deferred past v0.0.1 if EC2 is removed from the marketing copy.

---

### 8.6 P1 — Phase 7 release engineering is only half-done

From `STATUS.md` outstanding list:

| Item | State | Notes |
|---|---|---|
| 7.2 goreleaser snapshot pipeline | pending | config exists, full pipeline not validated |
| 7.3 fresh Ubuntu 24.04 VM smoke | pending | |
| 7.4 CHANGELOG from Phase 1–7 delta | pending | |
| 7.5 Installer dry-run | pending | |
| 7.6 GHCR image push + scan | pending | no public image yet |
| 7.7 announcement coordination | pending (non-code) | |

These are not code bugs — they are release-engineering tasks. But "v0.0.1" is a fiction until they are done; what exists today is "master branch at Tier 105 that happens to pass CI". Do not ship a binary to users until Phase 7 closes.

**Effort**: 4–6 engineer-days total.

---

### 8.7 P2 — `core.Core` is a god-object

Passing the whole `*core.Core` into every module's `Init` makes wiring cheap and tests hard. Modules routinely reach into `c.DB.Bolt`, `c.Services.Container`, `c.Events`, `c.Registry` — any of which could be an explicit dependency.

**Fix**: Start narrowing `Init` signatures module-by-module as a refactor pass in Phase 8. Do not attempt during v0.0.1 stabilization.

---

### 8.8 P2 — Inline role checks in handlers are the wrong pattern

Even the handlers that correctly check `claims.RoleID != "role_super_admin"` are doing it with a string literal and a bespoke error envelope. The middleware package already has `RoleSuperAdmin` as a constant and `writeErrorJSON` as a shared helper. Every inline check is drift-prone.

**Fix**: Once §8.1 lands, delete the inline checks. This is a cleanup pass after the middleware wiring; 1 engineer-day to touch ~10 handler files and the accompanying tests.

---

### 8.9 P2 — STATUS.md claims contradict the code in one place

`STATUS.md` says "~117 K Go test LOC" — almost certainly double-counts generated testdata or fuzz corpora, as noted in §2. Not a correctness issue but it erodes trust in the document. A dedicated LOC metric script (run in CI) would keep the number honest.

---

### 8.10 P3 — Module registry is bespoke and undocumented

`internal/core/registry.go` and `internal/core/module.go` together encode the full lifecycle contract, dependency-order sort, and shutdown semantics. New contributors will not guess their way through this. An ADR or a `docs/modules.md` would pay for itself after two onboarding cycles.

---

### 8.11 P3 — No secrets scanning in CI

`openapi-check` is gated; `gitleaks` / `trufflehog` is not. Low probability, high blast radius if a token ever slips. Add one; it's an hour of setup.

---

## 9. Spec-vs-code gap

Walked `.project/SPECIFICATION.md` against the code. The spec is large and well-organized; the code implements almost all of it. The only feature-level gap is **AWS EC2 VPS provider** (§8.5). Every other spec bullet I spot-checked maps to a real module, route, or handler.

The reverse direction is more interesting: the code has **more** than the spec describes in a few places —

- Canary deployments and commit rollback (handlers present in router.go:195–203)
- Deploy freeze, deploy schedule, deploy approval (handlers under `internal/api/handlers/deploy_*`)
- Bounded async event dispatch (Tier 82) — not mentioned in the spec
- Per-install random salt for the secrets vault (ADR-0008) — the spec predates the design

These are drift in the *good* direction: the code has gone past the spec because the audit surface pulled them in. The spec should be updated (or an addendum added) so that new contributors don't discover them by reading code.

---

## 10. Delta from the Tier 77 audit

This is the most important section for anyone who read the previous audit.

| Previous finding | State at HEAD |
|---|---|
| Test suite red (TestSQLite_Rollback deadlock) | **FIXED** (Tier 79) — `internal/db/sqlite_test.go:236` closes rows + documents reason |
| Hard-coded vault salt | **FIXED** (Tier 83) + legacy migration path (ADR-0008) |
| 20 Dependabot alerts | **FIXED** — down to 3, all upstream-blocked |
| Event bus unbounded async goroutines | **FIXED** (Tier 82) — 64-slot semaphore |
| Cookie Secure flag always-on breaking dev | **FIXED** (Tier 103) — gated on request transport |
| Global rate limiter hitting `/metrics/api` + SPA | **FIXED** (Tier 102) — scoped to `/api/`, `/hooks/` |
| SPA /assets/ 404 on stale embed | **FIXED** (Tier 102, Tier 104) |
| E2E harness silently passing on broken auth | **FIXED** (Tier 105) — fails loudly |
| Missing Postgres backend | **FIXED** — pgx/v5 pulled in (Tier 81), contract suite green |
| OpenAPI drift | **FIXED** — `make openapi-check` in CI |
| Loadtest regression | **FIXED** — 10 % p95 gate committed |
| 24 h soak | **FIXED** — harness exists, both 24 h and 5 m smoke green |
| Admin middleware missing | **PARTIAL** — middleware *exists* but is **not wired** (§8.1). Net state is *worse* than before because the roadmap now claims it is done |
| AWS EC2 VPS provider missing | **OPEN** (§8.5) |
| Phase 7 release engineering | **PARTIAL** — 7.1/7.4 done, 7.2/7.3/7.5/7.6/7.7 pending (§8.6) |

**Net**: 12 of 14 previous findings fixed, 1 partial, 1 new critical finding surfaced by this audit (the admin middleware wiring claim).

---

## 11. Conclusion

DeployMonster at Tier 105 is a substantially different product from Tier 77. The test suite is green, the dependency risk is contained, the vault is fixed, the event bus is bounded, the SPA embed is invariant-checked, and the Postgres backend is real and tested. **Most of the previous audit's blockers are actually gone.**

The remaining blockers are narrow:

1. **Admin middleware wiring** (§8.1) — real, verified, hits you in `/admin/tenants` today. **0.5 engineer-day.**
2. **Phase 7 release engineering** (§8.6) — required to ship a binary. **4–6 engineer-days.**
3. **AWS EC2 provider** (§8.5) — required only if the marketing copy keeps it. **3–5 engineer-days.**
4. **CSP header** (§8.4), **writers-under-load benchmark** (§8.3), **inline-check cleanup** (§8.8) — hardening. **3 engineer-days.**

Everything else in §8 is tech debt that can be booked for Phase 8 without blocking v0.0.1.

v0.0.1 is **not** ready to ship. It is **close**. See `PRODUCTIONREADY.md` for the numeric scorecard and go/no-go verdict, and `ROADMAP.md` for the sequenced remediation plan.
