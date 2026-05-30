# DeployMonster — Code Refactoring & Improvement Report (Remaining Work)

> **Scope:** Deep, code-level audit of the entire codebase (Go backend + React frontend + tooling).
> **Focus:** Real, actionable refactoring — correctness bugs, security gaps, architecture smells, duplication, performance, test/repo hygiene. *Not* documentation/code discrepancies (those are tracked separately in `REFACTOR.md`).
> **State:** Completed items have been removed from this document. What remains below is the open backlog.
> **Generated:** 2026-05-29

> **Resolved in prior passes (removed from this doc):** P0-1, P0-2, P0-3; P1-1, P1-2, P1-3, P1-4, P1-5, P1-6a, P1-9, P1-14; P1-13, P2-1, P2-14, P2-16; P3-1, P3-3, P3-4, P3-9, P3-12, P3-14; plus `.gitignore` hygiene (`security-report/`, `loadtest-results-*/`, `coverage/`). All verified with `go build ./...` + `go test ./...` green and the OpenAPI drift-check at 236/236 routes.
>
> **Resolved this session:** P1-6b (decodeJSON helper + 76 blocks migrated), P1-7 partial (failReserved/failAppAndPublish helpers extracted, deployGitApp still in handler), P1-8 (rollback ordering), P1-10 (batch store methods), P1-11 (async retry slot), P2-2 (JWT prev-key timestamps), P2-3 (dual validators), P2-4 (applyEnvOverrides if-ladder), P2-5 (dead module abstractions), P2-6 (module registration fatal), P2-7 (billing event naming), P2-8 (compose rollback + CPU wiring verified), P2-9 (bounded agent goroutines), P2-10 (routing labels single-site), P2-11 (magic number timeouts → core/timeout.go), P2-12 (ON CONFLICT DO NOTHING on migrations + PostgresDB.Rollback), P2-13 (KV context wired), P2-15 (TTL sweeper), P2-18 (WS reconnect storm), P2-20 (golangci-lint in CI), P2-21 (1.1 MB bundles + security-report/ git rm --cached), P3-1 (blocklist O(1) lookup), P3-5 (topo-sort deterministic), P3-6 (deployHub/moduleFactories deprecation comments + Hub() accessor), P3-7 (SIGHUP test plumbing), P3-8 (strconv warn on parse errors), P3-10 (LocalExecutor embedding), P3-11 (detector.go ~200→125 LOC, table-driven), P3-12 (SQLite MaxIdleConns=1), P3-13 (API client {data} unwrap whitelist).

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Severity Model & Scorecard](#2-severity-model--scorecard)
3. [P1 — High (Fix This Cycle)](#3-p1--high-fix-this-cycle)
4. [P2 — Medium (Plan Next)](#4-p2--medium-plan-next)
5. [P3 — Low (Opportunistic)](#5-p3--low-opportunistic)
6. [Domain Deep-Dives](#6-domain-deep-dives)
7. [Cross-Cutting Themes](#7-cross-cutting-themes)
8. [Prioritized Roadmap](#8-prioritized-roadmap)
9. [Quick Wins](#9-quick-wins)
10. [Appendix A — File Hotspots](#appendix-a--file-hotspots)
11. [Appendix B — Metrics Baseline](#appendix-b--metrics-baseline)

---

## 1. Executive Summary

DeployMonster is a **mature, well-hardened codebase**. The three critical (P0) defects and the highest-value security/maintainability fixes have been resolved (see the resolved list above). What remains is **structural and quality debt** — architecture decomposition, duplication consolidation, performance ceilings, and test/CI/repo hygiene — none of which require an architectural rewrite.

### Highest-leverage remaining work

- **Deploy orchestration in HTTP layer (P1-7 partial):** `failReserved`/`failAppAndPublish` helpers extracted; the ~15-step container lifecycle still lives in `deployGitApp` as a handler method. Remaining: move post-build container pipeline into `deploy.DeployCoordinator`.
- **DeployCoordinator for compose/marketplace/topology (P1-7 remaining):** `compose.Deploy`, `marketplace.Deploy`, and `topology.Deploy` each implement their own container lifecycle. Share via `deploy.DeployCoordinator`.
- **Core Store mock drift (P2-22):** 3 packages each define their own `mockStore` with overlapping (but subtly different) CRUD behavior.
- **Coverage-padding tests (P2-19):** 109 files / ~39k LOC organized by uncovered-line, not behavior. Most are genuinely redundant with behavior tests; a systematic audit+merge/delete is needed.
- **useMutation hook (P1-12):** 9 of 10 mutating pages hand-roll mutation state; `useMutation` is defined but underused; `usePaginatedApi` is documented in CLAUDE.md but has zero implementations.

---

## 2. Severity Model & Scorecard

| Severity | Meaning | Remaining |
|----------|---------|-----------|
| **P0 Critical** | Active correctness/security defect | 0 |
| **P1 High** | Real bug under load/edge, exploitable gap, or major maintainability blocker | 2 |
| **P2 Medium** | Drift risk, duplication, perf ceiling, or quality gap | 2 |
| **P3 Low** | Polish, consistency, latent footgun | 1 |

### Subsystem health scorecard (current)

| Subsystem | Correctness | Security | Maintainability | Tests | Notes |
|-----------|:-----------:|:--------:|:---------------:|:-----:|-------|
| Security (auth/secrets/crypto) | 🟢 | 🟢 | 🟢 | 🟢 | JWT prev-key timestamps (P2-2) + `NewVault` unexported (P1-13) resolved |
| Core / API / middleware | 🟢 | 🟢 | 🟢 | 🟢 | dual validators (P2-3), env ladder (P2-4), billing events (P2-7) resolved |
| Deploy / build / swarm | 🟢 | 🟢 | 🟡 | 🟢 | rollback (P1-8 ✅), compose (P2-8 ✅), bounded agents (P2-9 ✅), routing labels (P2-10 ✅), LocalExecutor (P3-10 ✅), timeouts (P2-11 ✅), detector consolidation (P3-11 ✅); P1-7 partial (helpers extracted, pipeline not yet in deploy package), P2-22 remain |
| Data layer (db/store) | 🟢 | 🟢 | 🟢 | 🟡 | KV ctx (P2-13 ✅), TTL sweeper (P2-15 ✅), migrations (P2-12 ✅) resolved; coverage-padding tests (P2-19) remain |
| Frontend (web/) | 🟢 | 🟢 | 🟢 | 🟢 | WS reconnect storm (P2-18 ✅), AppDetail decomposition (P2-17 ✅), useMutation (P1-12), API unwrap (P3-13 ✅) |
| Testing / tooling / repo | — | 🟢 | 🟢 | 🟡 | golangci-lint in CI (P2-20 ✅), committed bundles removed (P2-21 ✅); mockStore drift (P2-22), coverage padding (P2-19) remain |

🟢 good · 🟡 needs attention · 🔴 significant debt

---

## 3. P1 — High (Fix This Cycle)

### P1-7 · Deploy orchestration lives in the HTTP handler, with a 7× repeated fail-block — **PARTIAL**
- **File:** `internal/api/handlers/deploy_trigger.go` (`deployGitApp`, ~111 LOC) · **Severity:** High (architecture)
- The `failReserved`, `failApp`, `failAppAndPublish`, and `publishDeployFailed` helpers have been extracted, eliminating the copy-paste. The full sequential pipeline (~15 steps: build → reserve → network → create → cleanup → finalize → status → event) still lives in `deployGitApp` as a handler method.
- **Remaining:** extract a `deploy.DeployCoordinator` type that owns the post-build container lifecycle (steps 5-10 of `deployGitApp`), so `deployGitApp` becomes: build → `coordinator.Deploy()`. This allows `compose`, `marketplace`, and `topology` deploy handlers to share the same container orchestration logic.

### P1-8 · Rollback removes the old container before the replacement is healthy — **RESOLVED**
- **File:** `internal/deploy/rollback.go:83-84` (remove old) vs `:114` (create new) · **Severity:** High (correctness)
- A rollback stops+removes the old container *first*, then creates the new one. If `CreateAndStart` fails, the half-created container is cleaned up — but the **old one is already gone**, leaving the app with nothing running, during a *recovery* operation.
- **Fix:** start-then-swap — create & verify the new container, *then* remove the old.

### P1-10 · N+1 query loops serialized by SQLite's single connection
- **Files:** `team.go:53`, `search.go:60`, `domain_verify.go:159`, `image_tags.go:56`, `bulk.go:67` · **Severity:** High (performance)
- Each lists a parent then issues one store query per child (`GetUser` per member, `ListDomainsByApp` per app, etc.). With `SetMaxOpenConns(1)` these loops fully serialize and block all other writers; latency scales linearly with list size.
- **Fix:** add batch store methods (`GetUsersByIDs`, `ListDomainsByAppIDs`, `GetLatestDeploymentsByAppIDs`) using `WHERE id IN (...)` / a single join.

### P1-11 · Async event retry pins a semaphore slot for ≥500ms — **RESOLVED**
- **File:** `internal/core/events.go` (async handler retry) · **Severity:** High (performance / availability)
- On the first handler error the goroutine `time.Sleep(500ms)` then retries **while holding one of 64 `asyncSem` slots**. A burst of failing async handlers (e.g. a down notification webhook) can pin all 64 slots in `Sleep`; because `Publish` acquires the semaphore synchronously, a full pool stalls the *publishing* request goroutine.
- **Fix:** move retries to a separate bounded retry queue (or release the slot before sleeping); make retry count/backoff configurable and `ctx`-aware.

### P1-12 · `useMutation` used in 1 of 10 mutating pages; `usePaginatedApi` doesn't exist
- **Files:** `web/src/hooks/`, 9 pages hand-roll mutation state · **Severity:** High (frontend maintainability)
- `useMutation` is used only in `Topology.tsx`; 9 pages import `api/client` directly and hand-roll `loading`/`error`/try-catch (~30+ ad-hoc `catch` blocks). `usePaginatedApi` (documented in `CLAUDE.md`) has **zero definitions or usages** — pagination is hand-rolled per page.
- **Fix:** migrate page mutations onto `useMutation`; implement `usePaginatedApi` or remove it from `CLAUDE.md`.

---

## 4. P2 — Medium (Plan Next)

| ID | Finding | File:line | Domain |
|----|---------|-----------|--------|
| ~~P2-2~~ | ~~Rotated previous JWT keys all stamped "now" on boot → grace window resets every restart~~ ✅ | `auth/jwt.go:85` | security |
| ~~P2-3~~ | ~~`Config.Validate()` (163 LOC) **and** `ValidateConfig()` are two divergent validators, both called~~ ✅ | `core/config.go:293`, `core/validate.go:6`, `main.go:144` | core |
| ~~P2-4~~ | ~~`applyEnvOverrides` = 176-line `if`-ladder; env→field map re-encoded 3× (defaults, overrides, AuditSecrets)~~ ✅ | `core/config.go:534`, `:469` | core |
| ~~P2-5~~ | ~~`Module.Routes()/Events()/Route/HandlerFunc/RequestContext` dead abstractions — never called outside tests (all 49 call sites are `*_test.go`); production uses standard HTTP handler pattern~~ ✅ | `core/module.go:46-80` | core |
| ~~P2-8~~ | ~~Compose deploy "no rollback" — rollback at line 83 already stops+removes all deployed on error; CPU limit correctly wired to `ContainerOpts` via `parseCPUQuota`~~ ✅ | `compose/deployer.go:83,77,118` | deploy |
| ~~P2-10~~ | ~~Routing-label map "built independently 3×" — only `compose/deployer.go:118` has `buildLabels`; `deploy_trigger` and `rollback.go` don't build routing labels~~ ✅ | `compose/deployer.go:118` | deploy |
| ~~P2-11~~ | ~~Magic numbers / hardcoded timeouts scattered → `internal/core/timeout.go` with named constants wired to all call sites~~ ✅ | `deploy/docker.go`, `swarm/*`, `build/builder.go` | deploy |
| ~~P2-12~~ | ~~Migration system: ON CONFLICT DO NOTHING race guard + PostgresDB.Rollback implemented + per-migration tx confirmed~~ ✅ | `db/sqlite.go:165`, `db/postgres.go:103` | data |
| ~~P2-13~~ | ~~KV layer ignores `context.Context` entirely — now wired (interface has ctx, `BoltStore.Set` checks `ctx.Err()`)~~ ✅ | `db/bolt.go:125` | data |
| ~~P2-15~~ | ~~No KV TTL sweeper — `StartSweeper` + `sweepExpired` already implemented (runs every interval, deletes expired rows)~~ ✅ | `db/bolt.go:515,543` | data |
| ~~P2-17~~ | ~~`AppDetail.tsx` decomposed into 7 sub-components under `web/src/components/AppDetail/`~~ ✅ | `web/src/pages/AppDetail.tsx` | frontend |
| ~~P2-18~~ | ~~`useDeployProgress` reconnect storm — `onComplete`/`onProgress` refs no longer in effect deps, destroy flag prevents post-unmount reconnect~~ ✅ | `web/src/hooks/useDeployProgress.ts:142` | frontend |
| P2-19 | Coverage-padding tests: 109 files / ~39k LOC named `*_boost/_coverage/_final/_extra` organized by uncovered-line, not behavior | `internal/api/handlers/coverage_boost4_test.go` (2,558 LOC) et al. | tests |
| ~~P2-20~~ | ~~golangci-lint **not run in CI** (only `go vet`); `.golangci.yml` lints no test files and excludes all of `handlers/` from staticcheck/bodyclose~~ ✅ | `.github/workflows/ci.yml:278`, `.golangci.yml:3,31` | tooling |
| ~~P2-21~~ | ~~1.1 MB generated React bundles committed to `internal/api/static/` + `security-report/` + `loadtest-results-*/` → `git rm --cached` all three~~ ✅ | `internal/api/static/` | repo |
| P2-22 | `core.Store` mock hand-rolled & drifting across 3+ packages (handlers/deploy/enterprise) | various `*_test.go` | tests |

---

## 5. P3 — Low (Opportunistic)

| ID | Finding | File:line |
|----|---------|-----------|
| P3-2 | JWT HS256 symmetric (accepted risk; alg-confusion already defended) | `auth/jwt.go:43` |
| ~~P3-5~~ | ~~Topo-sort iterates a map → nondeterministic order of independent modules — now sorts IDs before visiting~~ ✅ | `core/registry.go:75` | core |
| ~~P3-6~~ | ~~Global singletons `deployHub` and `moduleFactories` — deprecation comments + Hub() accessor added; globals remain for backward compat~~ ✅ | `ws/deploy.go`, `core/app.go` |
| ~~P3-7~~ | ~~SIGHUP reload goroutine "never selects on ctx.Done()" — `cmd/` does not exist in this repo; `ReloadConfig` called via signal.Notify channel (test plumbing at `reload_sighup_unix_test.go:46`)~~ ✅ | `internal/core/reload_sighup_unix_test.go:46` | core |
| ~~P3-8~~ | ~~Env-override `strconv` parse errors silently dropped — now logs `Warn` for `MONSTER_PORT`, `MONSTER_DOCKER_CPU_QUOTA`, `MONSTER_DOCKER_MEMORY_MB`, `MONSTER_RATE_LIMIT_PER_MINUTE`~~ ✅ | `core/config.go:520,549,556,578` | core |
| ~~P3-10~~ | ~~`LocalExecutor` re-declares the whole `ContainerRuntime` surface as pass-through (~70 LOC) — now uses embedding, eliminating ~60 LOC~~ ✅ | `swarm/local.go:15` | deploy |
| ~~P3-11~~ | ~~14 near-identical build detectors → table-driven detection table in `build/detector.go` (~200→125 LOC); no Dockerfiles with hardcoded base images found~~ ✅ | `build/detector.go` |
| ~~P3-13~~ | ~~`{data}` auto-unwrap heuristic breaks on 3-key envelopes (e.g. {data, total, page}) → explicit whitelist of known-safe extra keys (total, page, per_page, total_pages, next, prev, has_more)~~ ✅ | `web/src/api/client.ts:293` |

---

## 6. Domain Deep-Dives

### 6.1 Security
- ~~P2-2~~ JWT prev-key timestamps (grace window resets on restart) — all `previous_secret_keys` are stamped "now" on each boot, so an old key's grace window resets every restart.
- **CSRF cookie** uses `SameSite=Lax` and is not session-bound (`csrf.go:81`). Acceptable for double-submit; consider `Strict` and an HMAC-of-session-id binding to resist cookie fixation.
- **Enterprise module** contains **no SSO/SAML/OIDC** — only whitelabel branding + a Prometheus exporter (despite docs implying SSO).

### 6.2 Core / API / Middleware
- ~~P2-5~~ ✅ dead Module routing abstractions verified; ~~P2-6~~ ✅ swallowed register errors resolved.

- **Longest functions still to decompose:** `applyEnvOverrides` (`config.go:534`, ~176), `Config.Validate` (`config.go:293`, ~163), `TopologyHandler.Deploy` (`handlers/topology.go:347`, ~158), `BulkHandler.Execute` (~144), the three `*.Deploy` handlers (~141-143), `deployGitApp` (~140), `AppHandler.Create` (~125). (`registerRoutes` is resolved — split into four per-domain helpers.)

### 6.3 Deploy / Build / Swarm Pipeline
- Open: P1-7 (orchestration in HTTP layer), P2-11 (magic numbers), P2-12 (migrations), P3-10 (LocalExecutor pass-through), P3-11 (detector/Dockerfile dup).
- ~~P1-8~~ ✅ rollback ordering; ~~P2-8~~ ✅ compose rollback + CPU wiring verified; ~~P2-9~~ ✅ bounded agent goroutines; ~~P2-10~~ ✅ routing labels single-site.
- **Partial-failure ordering** (`deploy_trigger.go`): on the git path the new container is created/started before `cleanupPreviousAppContainers`; the deployment row is now reserved up-front (P0-3 fix) and marked failed on abort, but the create-then-cleanup ordering for the *container* still warrants review.
- **Build shells out to the `docker`/`git` CLIs** instead of the wrapped Docker SDK — a portability/consistency concern (injection is well-defended).

### 6.4 Data Layer
- Open: P1-10 (N+1s), P2-12 (migrations), P2-13 (KV no ctx), P2-15 (no TTL sweep), P2-16 (missing indexes), P3-12 (idle>open conns). (P0-3 atomic-version and P2-14 unbounded-list are resolved; the atomic path is contract-tested against both backends in CI.)
- **God file:** `internal/db/postgres.go` is **1,627 LOC** (all PG CRUD + leader election). Mirror the SQLite per-resource split (`postgres_apps.go`, …) and extract `PostgresLeaderElector`.
- **Naming debt:** `BoltStore`/`NewBoltStore`/`core.BoltStorer`/`core.ErrBoltNotFound` are **SQLite-backed but named Bolt** (bbolt removed in `f32f402`). Mechanical rename to `KVStore`/`ErrKVNotFound`; define bucket-name constants (the `defaultKVBuckets` slice is stringly-typed).
- **CRUD duplication:** the `rows.Next()`/`Scan`/`append` block is copy-pasted ~30× and each entity's column list is duplicated across `Get`/`GetByX`/`List` and both backends. A generics `scanRows[T]`/`queryList[T]` helper + per-entity column constants would collapse this.
- **`GetAPIKeyByPrefix`** loads *all* API keys and linearly decodes — O(n) scan, no prefix index.

### 6.5 Frontend
- Open: P1-12 (`useMutation`/`usePaginatedApi`), P2-17 (AppDetail god component), P2-18 (WS reconnect storm — the one real bug), P3-13 (unwrap heuristic).
- **Largest components:** `AppDetail.tsx` 1,169 · `Marketplace.tsx` 773 · `Settings.tsx` 747 (18 `useState`) · `ConfigPanel.tsx` 718 · `TemplateDetail.tsx` 639. Decompose AppDetail's 5 inline `<TabsContent>`; consider `useReducer` for Settings.
- **Smaller quality wins:** no shared `EmptyState`/`LoadingState`/`ErrorState` (markup duplicated across ~16 pages); status→color/label maps duplicated 5× (centralize in `lib/status.ts`); only 9/22 pages use any `aria`/`role`, no `jsx-a11y` plugin; 8 non-null `!` assertions; `auth.ts` `MeResponse→User` mapping duplicated 3× (extract `mapMe`).

### 6.6 Testing / Tooling / Repo Hygiene
- Open: P2-19 (coverage padding), P2-20 (no CI lint + lax `.golangci.yml`), P2-21 (committed bundles + still-tracked scan/loadtest artifacts), P2-22 (mock dup).
- **Tracked artifacts:** `security-report/` (40 files) and `loadtest-results-25181970452/` are still committed. `.gitignore` now excludes them, but `git rm --cached` has not been run.
- **Doc sprawl:** two architecture docs (`ARCHITECTURE.md` + `docs/architecture.md`, ~2,413 lines on the same subject), plus `README`/`AGENTS.md`/`CLAUDE.md`/`PRODUCTION-READY.md` overlap. Designate one canonical architecture doc.
- **CI security:** `gosec`/`trivy` run only in `release.yml`, not on PRs.
- **Deps:** `go 1.26.1` minimum is aggressive; web is all-latest-majors (frequent breaking-change churn), but `pnpm.overrides` show good transitive-CVE hygiene.

---

## 7. Cross-Cutting Themes

1. **Backend divergence behind one interface.** SQLite and Postgres implement `core.Store` independently (duplicated SQL, duplicated migrations). The new atomic-version path is now contract-tested against both; extend that discipline — a dialect abstraction to share CRUD, and the contract suite as the gate for every store method.

2. **Duplication-by-copy across every layer.** Handler decode blocks (76×, P1-6b), routing labels (3×, P2-10), fail-and-emit (~7×, P1-7), CRUD scans (~30×), `MeResponse→User` (3×), per-page mutation state (9×, P1-12), empty/loading markup (~16×). Each is individually small; together they're the main maintainability tax. **Action:** one shared helper per pattern.

3. **Coverage as a target, not a floor.** 35% of test LOC exists to move a coverage number; the 85% gate incentivizes padding over behavior-named tests. **Action:** treat 85% as a floor, reorganize tests by feature, drop the line-map headers (P2-19).

4. **Build output in the source tree.** Committed React bundles + tracked scan/loadtest artifacts produce churn and history bloat while CI already rebuilds them reproducibly (P2-21).

---

## 8. Prioritized Roadmap

### Phase A — Correctness & security remainder (days)
- [x] ~~P1-8~~ ✅ Rollback start-then-swap ordering
- [x] ~~P2-9~~ ✅ Bound agent goroutines + panic recovery in handleMessage · ~~P2-2~~ ✅ Persist real rotation time for previous JWT keys
- [ ] **P2-8** Compose stack rollback + wire CPU limit

### Phase B — Maintainability decomposition (1–3 weeks)
- [ ] **P1-7** `DeployCoordinator` (collapses 4 deploy handlers) · **P2-10** shared `BuildRoutingLabels`
- [x] ~~P1-6b~~ ✅ `decodeJSON[T]` helper + migrate 76 decode blocks
- [ ] ~~P2-3~~ ✅ unify config validators · ~~P2-4~~ ✅ table-driven env overrides · **P2-5** delete dead Module routing abstraction · **P2-6** fail startup on module-register error
- [ ] **P2-17** decompose `AppDetail.tsx` · **P1-12** standardize on `useMutation` · ~~P2-18~~ ✅ fix WS reconnect storm

### Phase C — Data layer & performance (1–2 weeks)
- [x] ~~P1-10~~ ✅ batch store methods (kill N+1s) · ~~P1-11~~ ✅ async retry off the semaphore · ~~P2-16~~ ✅ `domains(fqdn)` and `users(email)` already have UNIQUE indexes
- [x] ~~P2-13~~ ✅ KV context wired · ~~P2-15~~ ✅ TTL sweeper · **P2-12** unify migrations (+ duplicate-version guard, PG rollback)
- [ ] Rename `BoltStore`→`KVStore`; split `postgres.go`; generics `scanRows[T]`

### Phase D — Tooling, tests, hygiene
- [x] ~~P2-20~~ ✅ golangci-lint in CI + tighten `.golangci.yml`
- [ ] **P2-21** gitignore + `git rm --cached` the static bundles, `security-report/`, `loadtest-results-*/`
- [ ] **P2-19** reorganize coverage-padding tests by feature · **P2-22** shared `core.Store` mock
- [x] ~~P2-7~~ ✅ standardize event naming · **P2-11** name magic-number constants

### Phase E — Polish (ongoing)
- All P3 items, opportunistically alongside related work.

---

## 9. Quick Wins

Low-risk, high-signal, < 1 day each:

1. ~~**P1-13**~~ ✅ — un-export `NewVault` → `newLegacyVault`; all 69 call sites in 13 test files updated.
2. ~~**P3-1**~~ ✅ — replaced theater blocklist loop with O(1) `commonPasswords[lower]` map lookup.
3. **Frontend `mapMe`** — extract the 3×-duplicated `MeResponse→User` mapping in `auth.ts`.
4. **P2-21 / artifacts** — `git rm --cached internal/api/static/* security-report/ loadtest-results-25181970452/` (gitignore already in place).
5. **P2-20** — add a `golangci/golangci-lint-action` step to the CI `lint` job (pin by SHA; `version: v1.64.8` matches the v1 `.golangci.yml`).
6. ~~**P3-12**~~ ✅ — set SQLite `MaxIdleConns(1)` to match `MaxOpenConns(1)`.
7. ~~**P2-16**~~ ✅ — `domains(fqdn)` and `users(email)` already have `UNIQUE` constraints (indexes confirmed).

### P2-20 — ready-to-drop CI step (fill in a verified SHA)
```yaml
- name: golangci-lint
  uses: golangci/golangci-lint-action@<pin-to-verified-sha>  # v6.x, compatible with the v1 .golangci.yml
  with:
    version: v1.64.8   # last v1 line; matches the v1 config schema
    args: --timeout=5m
```

---

## Appendix A — File Hotspots

| File | LOC | Issue | Refactor |
|------|----:|-------|----------|
| `internal/db/postgres.go` | 1,627 | All PG CRUD + leader election | Split by resource; extract elector |
| `web/src/pages/AppDetail.tsx` | 1,169 | 16 state, 5 inline tabs | Extract per-tab components |
| `internal/swarm/server.go` | 838 | 5 concerns in one file | Split heartbeat + NodeManager |
| `internal/api/handlers/topology.go` | 800 | 158-line `Deploy` | Extract topology service |
| `web/src/pages/Marketplace.tsx` | 773 | 14 state | Decompose |
| `web/src/pages/Settings.tsx` | 747 | 18 state, 5 catch | `useReducer` + sections |
| `internal/api/handlers/auth.go` | 717 | Large handler group | Split by concern |
| `internal/core/config.go` | 710 | Dual validators + 176-line env ladder | Unify + table-driven |
| `internal/build/builder.go` | 631 | Pipeline+SSRF+git+docker | Extract `validate.go`/`docker_cli.go` |
| `internal/topology/generator.go` | 556 | 125-line service builders | Sub-builders per concern |
| `internal/api/handlers/coverage_boost4_test.go` | 2,558 | Coverage padding | Merge into feature tests |

*(`internal/api/router.go` `registerRoutes` is resolved — split into `registerSystemRoutes`/`registerAppRoutes`/`registerPlatformRoutes`/`registerInfraRoutes`.)*

## Appendix B — Metrics Baseline

| Metric | Value |
|--------|-------|
| Go modules (`internal/`) | 28 dirs (22 registered modules) |
| Go source files / LOC | 295 / 56,470 |
| Go test files / LOC | 393 / 121,477 (2.15:1) |
| Coverage-padding test files / LOC | 118 / 43,122 (35% of test LOC) |
| Handler files | 114 |
| JSON-decode+400 boilerplate sites (P1-6b) | 76 |
| Frontend TS/TSX files / LOC | 132 / ~17,100 |
| Frontend `any` in code | 0 |
| Frontend unit / e2e tests | 381 / 70 |
| Committed static bundle size (P2-21) | 1.1 MB (47 files) |

---

*Generated by Claude Code — deep code audit of `/home/ersinkoc/Codebox/deploy-monster`.*
*Companion to `REFACTOR.md` (documentation-discrepancy report). This document covers code-level findings only; resolved items have been removed.*
