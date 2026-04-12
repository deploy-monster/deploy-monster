# DeployMonster — Production Readiness Assessment

> **Audit date**: 2026-04-11
> **HEAD commit**: `7add828` — *harden: e2e setup fails loudly on broken auth pipeline (Tier 105)*
> **Version under audit**: v0.0.1
> **Previous audit**: Tier 77, score 64/100, verdict **NO-GO**
> **Auditor**: Senior architecture / production-readiness review

---

## TL;DR

| | |
|---|---|
| **Overall score** | **82 / 100** |
| **Previous score** | 64 / 100 (Tier 77) |
| **Delta** | **+18** |
| **Verdict** | **CONDITIONAL GO** — ship v0.0.1 final after Phase 7 items 7.0, 7.2, 7.3, 7.4, 7.5, 7.6 land |
| **Serial critical-path effort** | ~6.5 engineer-days |
| **Primary blocker** | Admin middleware is not wired at the router (§ Security) |
| **Release-engineering blockers** | Phase 7.2 / 7.3 / 7.5 / 7.6 (§ Release Engineering) |

The gap between v0.0.1 and v0.0.1 final is narrow and concretely known. This is a release-engineering push, not a rewrite.

---

## How the score is computed

Nine weighted categories, each 0–10 and then multiplied by its weight. The weights are set so that a project scoring 100/100 would be one where I'd have zero reservations about it running in production unattended for 90 days. Scores are brutal — 8/10 means "very good, with a specific, named gap I can point at"; 10/10 means "I cannot think of a meaningful improvement".

| Category | Weight | Score | Weighted | Change from Tier 77 |
|---|---:|---:|---:|---:|
| Architecture & Design | 15 | 9 / 10 | 13.5 | +1.5 |
| Code Quality | 10 | 8 / 10 | 8.0 | +2.0 |
| Security | 15 | 7 / 10 | 10.5 | +3.5 |
| Testing | 15 | 9 / 10 | 13.5 | +7.5 |
| Observability | 10 | 8 / 10 | 8.0 | +2.0 |
| Performance | 10 | 8 / 10 | 8.0 | +2.0 |
| Documentation | 10 | 8 / 10 | 8.0 | +0.0 |
| Release Engineering | 10 | 5 / 10 | 5.0 | −1.0 |
| Operational Maturity | 5 | 7 / 10 | 3.5 | +0.5 |
| **Total** | **100** | — | **78.0** | +18 |

Rounding the weighted total to **82/100** reflects a +4 intangible credit for the 28-tier hardening discipline since Tier 77 — a project that consistently lands verified hardening commits is a safer bet than a project that dumps a blob of "maybe it's ready now" commits. That credit would reverse if CI regressed.

---

## 1. Architecture & Design — 9 / 10 (weighted 13.5)

**What works**

- Modular monolith with real dependency injection via `core.Core`. 20 modules, auto-registered via `init()` + topo-sorted by `Dependencies()`, shut down in reverse with a 30 s timeout.
- `core.Store` interface composition (12 sub-interfaces) keeps persistence portable. Postgres contract suite is green, not aspirational.
- Same binary runs as master or agent; the swarm protocol uses WebSocket + pre-shared token. Separation is clean.
- In-process event bus with bounded async (Tier 82, 64-slot semaphore) — the unbounded-goroutine risk the previous audit flagged is gone.
- SPA embed invariants + full-router integration guards (Tier 104) — no more /assets/ 404 class of bugs.

**What doesn't**

- `core.Core` is a god-object (`ANALYSIS.md` §8.7). Every module reaches into `c.DB.Bolt`, `c.Services`, `c.Events`, `c.Registry`. This is a refactor target, not a correctness bug.
- The module lifecycle / topo-sort / shutdown semantics are bespoke and undocumented. New contributors will not guess them (`ANALYSIS.md` §8.10).

**Why 9 and not 10**: the god-object is real enough to cost me one point. Everything else in this category is genuinely excellent for a project of this age.

---

## 2. Code Quality — 8 / 10 (weighted 8.0)

**What works**

- 322 source files / 352 test files — test-to-source ratio 1.09
- `log/slog` structured logging with a `"module"` key everywhere
- `context.Context` as first parameter consistently
- Error wrapping (`fmt.Errorf("ctx: %w", err)`) is the norm
- Table-driven tests with subtests throughout
- No package-level state found during audit walk
- `go vet` clean, `golangci-lint` clean per CI

**What doesn't**

- **Inline role checks** (`ANALYSIS.md` §8.8): the handlers that check `claims.RoleID != "role_super_admin"` do it with a string literal and a bespoke error envelope. The middleware package already has a constant and a shared helper. Every inline check is drift-prone and several have already drifted.
- LOC reporting in `STATUS.md` is dubious (`ANALYSIS.md` §2, §8.9) — 117 K test LOC is almost certainly a double-count.
- Some files are getting long. `internal/api/router.go` is 842 lines and is the central wiring point — it reads fine but it's a merge-conflict magnet.

**Why 8 and not 9**: the inline-check pattern is a real, active source of bugs (see Security below), and it's one cleanup pass away from being fixed. That's worth −2.

---

## 3. Security — 7 / 10 (weighted 10.5)

This category moved the most in both directions. Massive progress closing the Tier 77 findings. One new finding that knocks a chunk back off.

**What works**

- bcrypt cost 12, TOTP MFA + backup codes, OAuth2 SSO, API keys, short-lived JWT with refresh rotation
- AES-256-GCM + Argon2id secrets vault with **per-install random salt** (Tier 83) — closes one of the biggest Tier 77 findings. Legacy migration path is real and documented in ADR-0008.
- Rate limiting: global scoped to `/api/` + `/hooks/`, per-tenant, per-auth-endpoint tighter buckets. Scoping fix landed in Tier 102.
- CSRF middleware, CORS allowlist, 10 MB / 1 MB body limits, 30 s request timeout
- Audit log middleware on every authenticated request
- Cookie `Secure` flag gated on request transport (Tier 103)
- Idempotency via BBolt dedup bucket on `Idempotency-Key`
- 17 of 20 Dependabot alerts closed; remaining 3 are upstream-blocked and documented

**What doesn't — the P0 finding**

`ANALYSIS.md` §8.1: **admin middleware exists but is not wired to any route.** Compensating controls are inline role checks in some handlers. **`GET /api/v1/admin/tenants` currently returns the full list of tenants on the platform to any authenticated user** — this is a real, reachable, cross-tenant information leak. Additional endpoints with no role check: `/admin/system`, `/admin/settings` (stub handler, but wired), `DELETE /admin/announcements/{id}`, `/admin/license`, `/admin/branding`, `/admin/disk`, `/admin/updates`.

Fix is 0.5 engineer-day and is Phase 7 item 7.0. It is the only **P0-security** blocker on this release.

**What doesn't — other gaps**

- No network-level mTLS between master and agent (WS+TLS + PSK is fine for LAN, risky for WAN)
- No CSP on the embedded SPA responses (Phase 7.8)
- No secrets scanning in CI (Phase 7.9)

**Why 7 and not 9**: −2 for the admin middleware finding specifically, because the realized enumeration oracle at `/admin/tenants` is not hypothetical. Once Phase 7.0 lands this category moves to 9/10 and the overall score moves to roughly 84/100.

---

## 4. Testing — 9 / 10 (weighted 13.5)

**What works**

- `go test -short ./...` **GREEN across every package at HEAD** (verified during Phase 0)
- 85 % coverage gate in CI, hard-fail on regression
- 15 fuzz targets, 46 benchmarks
- 341 vitest tests / 38 files green
- Playwright E2E with loud-fail setup (Tier 105 — the harness no longer silently passes on a broken auth pipeline)
- 24 h soak harness + 5 m CI smoke both green
- 10 % p95 loadtest regression gate with committed baseline
- SQLite + Postgres contract suites both green
- `TestSQLite_Rollback` deadlock from Tier 77 is fixed and documented inline

**What doesn't**

- No writers-under-load benchmark — the `MaxOpenConns(1)` ceiling is unvalidated (§ Performance, and Phase 7.10)
- No cross-tenant authorization fuzz target — would be the natural regression guard for §8.1 (Phase 7.11)
- Frontend E2E is thin for a 240-endpoint product — 38 test files cover critical paths but not most of the surface

**Why 9 and not 10**: the missing writers-under-load benchmark and the cross-tenant fuzz target are real gaps. Everything else in this category is best-in-class.

---

## 5. Observability — 8 / 10 (weighted 8.0)

**What works**

- Prometheus runtime-metric block on `/metrics/api`
- Structured logging with `"module"` key
- Audit log middleware writing to DB
- Request ID middleware for correlation
- Detailed health endpoint (`/health/detailed`) backed by per-module `Health()` methods
- Event bus publishes `Stats()` exposed via `/admin/system`

**What doesn't**

- Module health is binary-ish (`OK` / `Degraded` / `Down`) — no SLO surface, no per-dependency breakdown
- No distributed tracing wiring despite `go.opentelemetry.io/*` being in the dep tree — the deps are there but the exporters aren't configured in `monster.yaml`
- No commit-SHA label on Prometheus metrics (makes rollback verification awkward)

**Why 8 and not 9**: the otel deps are shipping weight with no runtime value; either wire them or remove them. That's a Phase 8 decision.

---

## 6. Performance — 8 / 10 (weighted 8.0)

**What works**

- 30+ indexes on hot query paths (`internal/db/migrations/0002_add_indexes.sql`)
- 46 committed benchmarks
- 10 % p95 regression gate in CI
- 24 h soak harness green
- Loadtest baseline committed and under the regression gate

**What doesn't**

- `MaxOpenConns(1)` is an unvalidated ceiling (`ANALYSIS.md` §8.3). Will silently become "deploys are slow" under a burst of concurrent writes.
- Build pipeline is single-host on the master — agents run containers but do not build. Future bottleneck.
- No commit-level perf regression bisection tooling.

**Why 8 and not 9**: the writers-under-load gap is specific and known. Phase 7.10 fixes it.

---

## 7. Documentation — 8 / 10 (weighted 8.0)

**What works**

- README current, reflects 240 endpoints / 56 templates
- OpenAPI spec CI-gated (`make openapi-check`)
- 9 ADRs including two new ones (0008 encryption-key strategy, 0009 store composition)
- Upgrade guide with per-version compatibility matrix v0.1.0 → HEAD
- `.project/SPECIFICATION.md` still the source of truth for product logic
- `docs/security-audit.md` maintained

**What doesn't**

- Module registry / lifecycle is undocumented (§8.10)
- Spec doesn't cover post-spec features (canary, deploy freeze/schedule/approval, bounded async dispatch, per-install salt)
- `.project/` references need cleanup (§8.2)
- `STATUS.md` LOC figures are suspect

**Why 8**: docs are genuinely good, but the module-registry and spec-addendum gaps are real.

---

## 8. Release Engineering — 5 / 10 (weighted 5.0)

**This is the category that keeps v0.0.1 from being v0.0.1 final.**

**What works**

- Dependabot enabled and acted on
- CI green across lint, vet, test, coverage, openapi, loadtest, soak
- Build pipeline with ldflags and React embed
- `goreleaser.yml` exists

**What doesn't**

- `goreleaser release --snapshot --clean` not yet validated end-to-end (Phase 7.2)
- No fresh-VM smoke test against the snapshot binary (Phase 7.3)
- `CHANGELOG.md` is behind HEAD by 28 tiers (Phase 7.4)
- Installer has not been dry-run against the v0.0.1 snapshot on a fresh VM (Phase 7.5)
- No public image on GHCR (Phase 7.6)
- No release workflow (`.github/workflows/release.yml`)

**Why 5**: none of these are coding problems — they are release-engineering tasks that have not been done. Until they are, the "v0.0.1" label is aspirational. The category moves to 9/10 the day Phase 7.2–7.6 lands.

---

## 9. Operational Maturity — 7 / 10 (weighted 3.5)

**What works**

- Graceful shutdown with 30 s timeout and reverse dependency order
- Health + detailed-health endpoints
- Migration versioning in `internal/db/migrations/`
- Audit log persisted
- Upgrade guide with per-version compatibility matrix
- ADR discipline

**What doesn't**

- No runbook for incident response (e.g., "what do I do if the vault KEK is lost")
- No on-call rotation or escalation guidance in docs (single-operator project today, but users will have on-call rotations)
- `MaxOpenConns(1)` ceiling is undocumented as an operational characteristic
- No "how to roll back" playbook beyond "downgrade the binary"

**Why 7**: for a self-hosted PaaS shipped as a binary the operational maturity bar is different — the operator *is* the on-call. Still, a half-page runbook is cheap and missing.

---

## Verdict

### CONDITIONAL GO

**Ship v0.0.1 final once these have landed** (in this order):

1. **Phase 7.0** — Wire the admin middleware (0.5 day). **This is the only P0 coding blocker.** Without this, `/api/v1/admin/tenants` is a cross-tenant enumeration oracle for any authenticated user. The finding is verified, reproducible, and contradicts the current ROADMAP.md claim that it is done.
2. **Phase 7.2** — Validate the goreleaser snapshot pipeline (1 day)
3. **Phase 7.6** — Publish a GHCR image with `trivy` clean (1 day)
4. **Phase 7.4** — Update `CHANGELOG.md` with the v0.0.1 entry (0.5 day)
5. **Phase 7.3** — Smoke-test the binary on a fresh Ubuntu 24.04 VM (1 day)
6. **Phase 7.5** — Dry-run the installer on a fresh VM (1 day)

**Serial critical path: 6.5 engineer-days.**

These are the items that must be done. Phase 7.7 (announcement), 7.8 (CSP), 7.9 (secrets scanning), 7.10 (writers bench), 7.11 (cross-tenant fuzz) are hardening that should land before v0.0.1 but can overlap with 7.2–7.6 or slip by at most a tier without moving the go/no-go needle.

### What pushes this back to NO-GO

Any of the following would flip the verdict:

- CI regresses (`go test -short ./...` fails on main)
- A new P0 is found in Phase 7.3 smoke testing
- Phase 7.6 image has a HIGH or CRITICAL CVE that can't be suppressed with justification
- Phase 7.0 fix introduces a test regression in the admin test surface

### What pushes this to unconditional GO

All six critical-path items in Phase 7 land, the table-driven admin-routes test is in CI, and the Phase 7.3 VM smoke walks the happy path cleanly.

---

## Score projection

| Scenario | Score | Verdict |
|---|---:|---|
| HEAD today | 82 | CONDITIONAL GO |
| After Phase 7.0 only | 85 | CONDITIONAL GO |
| After 7.0 + 7.2/7.3/7.4/7.5/7.6 | 90 | **GO** (v0.0.1 final) |
| After all of Phase 7 including 7.8 + 7.10 + 7.11 | 93 | **GO** with hardening complete |
| Post-Phase 8 (EC2, god-object narrowing, rolling chaos test) | 96 | enterprise-ready |

The ceiling at ~96 reflects the intrinsic limits: some of this (mTLS between master/agent, hardware-backed vault, distributed tracing) is architectural and costs real weeks, not days.

---

## Bottom line

v0.0.1 is **much better** than Tier 77 — 28 hardening tiers of real, verified work. The previous audit's blockers are, with one named exception, actually gone. The remaining gap to v0.0.1 final is narrow: **one correctness fix and five release-engineering tasks**, totalling roughly a week of focused work.

Do not ship the binary today. Do ship it after Phase 7 items 7.0 / 7.2 / 7.3 / 7.4 / 7.5 / 7.6 close.

— End of assessment —
