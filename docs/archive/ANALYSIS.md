# Project Analysis Report

> Auto-generated comprehensive analysis of **DeployMonster**
> Generated: **2026-04-16**
> Analyzer: Claude Code — Full Codebase Audit
> Method: 4 parallel discovery agents + verification runs on a fresh `go test` execution.

---

## 1. Executive Summary

DeployMonster is a **self-hosted, single-binary PaaS** (Platform as a Service) — a Go 1.26+ modular monolith with an embedded React 19 SPA. It targets self-hosted users who want Heroku/Render-style "git-to-deploy" UX on their own VPS: Git webhooks trigger builds (14 language detectors, 12 Dockerfile templates), containers are orchestrated via the Docker SDK, and a custom reverse proxy provides TLS via Let's Encrypt autocert. The same binary runs as **master** (full platform) or **agent** (worker node) via the `--agent` flag and a WebSocket control channel. An event bus with topologically-ordered module lifecycle and a `core.Store` repository abstraction make the system cleanly extensible.

**Key metrics**

| Metric | Value |
|---|---|
| Go source files (non-test) | 278 |
| Go test files | 312 (of 593 total `.go` files) |
| Go production LOC | **46,028** |
| Go test LOC | **12,631** (ratio 0.27 test:prod) |
| Frontend TS/TSX files | **123** (22,338 LOC) |
| Registered backend modules | **20** (+ 4 shared internal libs) |
| API routes wired in `router.go` | **205** |
| API routes in `docs/openapi.yaml` | ~88 (**~144-route drift**, allowlisted) |
| Direct Go dependencies | 8 |
| Direct frontend dependencies | ~25 (React 19.2, Vite 8, Zustand 5, Tailwind 4) |
| Unit/vitest test files (web) | 36 |
| E2E spec files (Playwright) | 11 |
| `http.StatusNotImplemented` stubs in handlers | **26** |
| Currently failing Go tests | **4** (verified today) |

**Overall health score: 7.2 / 10**

The codebase is genuinely well-engineered: cleanly decomposed modules, interface-driven DI, strict TypeScript, robust CI, thoughtful supply-chain hardening (SHA-pinned scanners), and an explicit graceful-shutdown path. But multiple claims in README, `PRODUCTION-READY.md`, and `TASKS.md` do not match the code:

- **VERSION file says `v0.1.2`** (2026-04-14) while README headlines `v0.1.6`.
- **4 Go tests currently fail** on a clean run today (discovery, ingress ×2, swarm).
- **1 critical + 12 high** security findings live in `security-report/SECURITY-REPORT.md` and are unremediated.
- Marketplace template count, API endpoint count, and provider coverage are overstated.

**Top 3 strengths**
1. Clean modular architecture — 20 modules with topologically-sorted lifecycle; no leaky SQLite types outside `internal/db/`.
2. Test discipline — 85% coverage CI gate genuinely enforced; baseline-gated load/writer tests; OpenAPI drift detector.
3. Supply-chain hygiene — gitleaks and Trivy pinned by SHA (post March-2026 tj-actions compromise), SBOMs generated, multi-platform GoReleaser builds.

**Top 3 concerns**
1. **Claim drift.** `PRODUCTION-READY.md` scores itself 100/100 while 4 tests fail today and 13 security findings are open. This is the single biggest credibility issue.
2. **Incomplete providers.** DNS: 1/3 claimed (Cloudflare only). VPS: ~4/6 (Linode stub, AWS absent). Managed DB: 4/5 (MongoDB is a type enum with zero implementation).
3. **Known race conditions.** Security report enumerates 7 races including RACE-001 (duplicate deployments under concurrent triggers) and RACE-002 (non-atomic `GetNextDeployVersion`). Partial mitigation exists (`AtomicNextDeployVersion`) but not all call sites use it.

---

## 2. Architecture Analysis

### 2.1 High-level architecture

**Pattern:** Event-driven modular monolith with a distributed *data plane* (agents). One Go binary, two modes:

- **Master** (`deploymonster` or `deploymonster serve`): boots 20 modules, serves API + UI, owns the DB/KV, runs the event bus.
- **Agent** (`deploymonster serve --agent`): short-lived, no DB/API, connects via WebSocket to the master at `/api/v1/agent/ws` using `MONSTER_JOIN_TOKEN`.

**Data flow (master, happy path for deploy):**

```
Git provider webhook  →  /hooks/v1/{id}  (HMAC verified, BBolt secret lookup)
                       ↓
             webhook module decodes payload
                       ↓
                 EventBus.Emit("app.deploy.requested")
                       ↓
 build module (sync sub to event, enqueues in per-tenant queue)
                       ↓
 language detect (14) → Dockerfile synth (12 templates) → `docker build`
                       ↓
                 EventBus.Emit("build.completed")
                       ↓
 deploy module creates container via Docker SDK  →  container.started
                       ↓
 ingress module (reverse proxy + ACME autocert) routes domain → container
```

**Concurrency model:**
- Request goroutines per incoming HTTP request (Go stdlib).
- Event bus has a bounded async pool (**default 64** concurrent handlers) guarded by a semaphore — see `internal/core/events.go`.
- Background workers (e.g. scheduler ticks, TLS renewals, ACME orders, agent heartbeat) launched via a `SafeGo` wrapper (`internal/core/safego.go`) which recovers panics and names the goroutine for debugging.
- All start with cancellable context derived from the app context; `core.App.Run` calls `StopAll` with a 30-second timeout on shutdown.

### 2.2 Package structure

**Registered modules** (`internal/*/module.go` → `core.RegisterModule(factory)` in each package's `init()`):

| Module | Responsibility |
|---|---|
| `core.db` / `database` | SQLite/Postgres via `core.Store`; BBolt KV |
| `core.auth` / `auth` | bcrypt, JWT HS256, API-key prefix lookup, RBAC, SSO |
| `api` | HTTP router + middleware chain, OpenAPI surface |
| `deploy` | Docker SDK lifecycle (CreateAndStart, Restart, Logs, Exec, Stats, ImagePull) |
| `build` | Per-tenant queue, 14 detectors, 12 Dockerfile templates |
| `ingress` | Reverse proxy + ACME autocert + 4 (+1 stub) LB strategies |
| `backup` | Cron schedules, local + S3/R2 storage |
| `billing` | Stripe integration, metered usage |
| `dns` | Pluggable providers (only Cloudflare shipped) |
| `discovery` | Health-check sweep (TCP + HTTP) |
| `enterprise` | White-label, reseller, HA |
| `gitsources` | GitHub / GitLab / Gitea / Bitbucket adapters |
| `marketplace` | 56 one-click templates (not 150+ as claimed) |
| `mcp` | Model Context Protocol server with 9 tools |
| `notifications` | Email, Slack, Discord, Telegram, Webhook |
| `resource` | Per-tenant quota enforcement + `/metrics` (Prometheus) |
| `secrets` | AES-256-GCM vault with versioning, `${SECRET:name}` resolver |
| `swarm` | Master↔agent WebSocket control plane |
| `vps` | Provisioning adapters (Hetzner, DO, Vultr complete; Linode stub; AWS absent) |

**Shared libs** (no `module.go`, used by modules): `internal/awsauth/`, `internal/compose/`, `internal/topology/`, `internal/webhooks/`.

**Cohesion assessment.** Responsibilities are mostly single-purpose. Two mild overlaps:
- `internal/db/` (SQLite/Postgres concrete types) vs `internal/database/` (managed DB engines like Postgres-for-user-apps). The naming collision is unfortunate — a new reader will waste 10 minutes disambiguating.
- `internal/deploy/` contains both Docker low-level calls and deployment orchestration; could split into `docker/` and `deploy/orchestrator/`.

**Circular dependency risk.** None found. Module `Dependencies()` declarations form a DAG (checked in `internal/core/registry.go`'s topological sort — a cycle would refuse to boot).

**`internal/` vs `pkg/`.** There is no `pkg/`. Everything is in `internal/` (appropriate — this is a product, not a library).

### 2.3 Dependency analysis

**Direct Go dependencies (`go.mod`):**

| Dep | Version | Purpose | Notes |
|---|---|---|---|
| `github.com/docker/docker` | v28.5.2 | Docker SDK | Unavoidable. DEP-006/007 CVEs flagged but not exploitable (no AuthZ plugins). |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | JWT sign/verify | Current. Algorithm explicitly pinned to HS256. |
| `github.com/gorilla/websocket` | v1.5.3 | WS (master↔agent) | Maintained fork. |
| `github.com/jackc/pgx/v5` | v5.9.1 | Postgres driver | Optional (only loaded if `database.driver=postgres`). |
| `github.com/mattn/go-isatty` | v0.0.21 | TTY detection for setup wizard | Minor. |
| `go.etcd.io/bbolt` | v1.4.3 | Embedded KV | Used for API-key prefix lookup, webhook secrets, idempotency keys, rate-limit buckets. |
| `golang.org/x/crypto` | v0.50.0 | bcrypt, HMAC | Standard. |
| `gopkg.in/yaml.v3` | v3.0.1 | Config parser | Standard. |
| `modernc.org/sqlite` | v1.48.2 | Pure-Go SQLite | No CGO — key property for distribution (static binary, scratch Docker image). |

Plus `github.com/DATA-DOG/go-sqlmock` v1.5.2 (test-only).

No redundancy (one YAML parser, one JSON via stdlib, one crypto lib). No supply-chain surprises. 63 transitive deps — OpenTelemetry auto-SDK (`v1.2.1`) dominates that count; it's loaded but unused in the current build (no exporter wired).

**Frontend dependencies (`web/package.json`):**

| Package | Version | Notes |
|---|---|---|
| `react` / `react-dom` | ^19.2.5 | Latest. StrictMode on. |
| `react-router` | ^7.13.2 | v7 data-router API. |
| `vite` | ^8.0.5 | Latest. Manual vendor chunk splitting. |
| `typescript` | ~5.9.3 | Strict mode on (see §3.2). |
| `tailwindcss` + `@tailwindcss/vite` | ^4.2.2 | No PostCSS config needed. |
| `zustand` | ^5.0.12 | Sole state-mgmt lib. |
| `@xyflow/react` | ^12.10.2 | Topology canvas. |
| `dagre` | ^0.8.5 | DAG layout. |
| `lucide-react` | ^1.8.0 | Icons. |
| `vitest` + `@testing-library/react` | v3.2.1 / v16.3.0 | Unit tests. |
| `@playwright/test` | v1.59.1 | E2E tests. |

Missing dependencies that would be conventional for this app's size:
- No `react-hook-form` / `zod` / `yup` → form validation is imperative `useState` + hand-rolled checks. Fine today (4 forms), risky as forms grow.
- No data-fetching lib (TanStack Query / SWR) → all data-fetching state goes through a custom `useApi` hook family around `web/src/api/client.ts`. The client is actually good (timeouts, retry w/ jitter, refresh coalescing — see §3.2), but caching, stale-while-revalidate, and request dedup are all bespoke.
- No accessibility test lib (`jest-axe`, `axe-playwright`).

### 2.4 API & interface design

**Surface.** `internal/api/router.go` wires **205** routes using Go 1.22+ `http.ServeMux` `METHOD /path` syntax. A separate OpenAPI spec (`docs/openapi.yaml`) describes **~88** routes — the gap is tracked explicitly via `docs/openapi-drift-allowlist.txt` and `make openapi-check` fails on *unallowlisted* drift, which is a good pattern, but it also means ~144 endpoints are knowingly undocumented to external consumers.

**Middleware chain** (`internal/api/middleware/*`, applied in order by `middleware.Chain`):

```
RequestID → GracefulShutdown → GlobalRateLimiter(120/min/IP, /api/* + /hooks/*)
  → SecurityHeaders → APIMetrics → APIVersion → BodyLimit(10MB; 1MB on /hooks)
  → Timeout(30s) → Recovery → RequestLogger → CORS → CSRFProtect
  → IdempotencyMiddleware → AuditLog
```

**Auth levels** (`internal/core/module.go:52`): `AuthNone (0)`, `AuthAPIKey (1)`, `AuthJWT (2)`, `AuthAdmin (3)`, `AuthSuperAdmin (4)`. Standard `/api/v1/*` is `AuthJWT` (Bearer) — API keys (`X-API-Key` prefixed, BBolt-indexed by first 8 chars) are accepted but JWT is canonical.

**JWT:** HS256, minimum 32-char secret enforced at boot (panics if misconfigured, `internal/auth/jwt.go:54`). Access tokens = 15 min, refresh = 7 days, rotation with 20-min grace. **Access tokens are not revocable** — SESS-001 in security report.

**Webhook ingestion:** `POST /hooks/v1/{webhookID}` with HMAC signature verification; body limit is tightened to 1 MB for this prefix.

**WebSocket:** only `GET /api/v1/agent/ws` (bidirectional master↔agent channel, `internal/swarm/module.go`).

**Response consistency.** A single JSON envelope convention is followed by most handlers (`writeJSON`, `writePaginatedJSON`). Error shape is consistent: `{"error": {"code": "...", "message": "..."}}`. Good.

---

## 3. Code Quality Assessment

### 3.1 Go code quality

| Check | Result |
|---|---|
| `gofmt -l .` | ✅ 0 unformatted files |
| `go vet ./...` | ✅ 0 warnings |
| `go test -short -count=1 ./...` (today) | ❌ **4 failures** across 3 packages |
| slog call sites | 741 (vs 2 legacy `log.*`) — effectively 100% structured logging |

**Error handling.** Spot-check of `internal/auth/module.go:51,91,110,116,121,129` and `internal/db/*.go` shows consistent `fmt.Errorf("…: %w", err)` wrapping and meaningful sentinel errors (`core.ErrNotFound`, `auth.ErrInvalidToken`, `auth.ErrExpiredToken`, `db.ErrConflict`). `internal/deploy/` is mostly fine; a few call sites drop context (no `%w`), around Docker SDK boundaries.

**Context propagation.** Context is threaded everywhere — all `Store` methods, Docker calls (`*Context` variants), and DB ops (`QueryRowContext`, `ExecContext`). Graceful shutdown propagates through `ctx.Done()` to every background worker.

**TODO/FIXME inventory.** Grep of `internal/` and `cmd/` for `TODO|FIXME|XXX|HACK|unimplemented` yields a manageable list — the most significant being:

- 26 × `http.StatusNotImplemented` returns in `internal/api/handlers/` (mostly billing-plan branches and enterprise stubs).
- A handful of `// TODO: finalize once …` in `internal/enterprise/` and `internal/vps/providers/linode.go` (the Linode stub).

**Magic numbers and hard-coded values** are not a pervasive problem; the project has a `core.Config` pipeline with env-variable overrides (`MONSTER_*` prefix) and YAML config.

### 3.2 Frontend code quality

**TypeScript.** `tsconfig.app.json` enforces `strict: true` + `noUnusedLocals` + `noUnusedParameters` + `noFallthroughCasesInSwitch`. Grep across `web/src/**/*.{ts,tsx}` finds **zero `: any`** and **zero `@ts-ignore` / `@ts-nocheck`** — this is unusually disciplined.

**React patterns.** Functional components throughout. Routes are lazy-loaded (`React.lazy` + `<Suspense>`). `<ErrorBoundary>` wraps the router. StrictMode enabled in `main.tsx`. Zustand stores use `persist()` for theme/auth state. Explicit `AbortSignal` on all fetch calls with per-call timeout.

**React 19 usage.** SPA only — `use()`, `useActionState`, and Server Components are not used (appropriate; there is no RSC runtime here). Forms use raw `useState` + inline validation, which is workable at current scale but will age poorly.

**API client (`web/src/api/client.ts`).** Genuinely well-written:
- 30-second per-request timeout, 10 seconds on refresh calls.
- Retry with exponential backoff + jitter on gateway errors (502/503/504 only, never 500).
- Single in-flight refresh coalesces concurrent 401s; 30-second cooldown after refresh failure.
- Reads CSRF token from `__Host-dm_csrf` cookie (the `__Host-` prefix is a good security-hardening choice).

**Accessibility.** Reasonable baseline: `aria-label` on icon buttons, `role="dialog"` + `aria-modal` on dialogs, `role="status"` on loaders, and keyboard support inherited from shadcn/ui primitives. No axe-core or Playwright accessibility scans in CI.

**Bundle.** Vite manual chunk splitting for vendors. Route-level code split. No bundle-size budget enforcement in CI. Lucide is tree-shaken. No image assets to speak of (icons only).

### 3.3 Concurrency & safety

**Goroutines.** ~186 `go func()` occurrences across `internal/`. Almost all go through `SafeGo` (panic recovery + naming). The async event pool is **semaphore-bounded at 64** — critical safeguard against noisy-event-storm goroutine explosion. Graceful drain via `sync.WaitGroup.Wait()` on shutdown.

**Mutex patterns.** `core.EventBus` uses `sync.RWMutex` for the subscription list (read-heavy). Metrics counters re-lock on each increment (minor — could be batched under one critical section). `core.App.configMu` guards hot-reloadable config fields.

**Race conditions — documented and live.** `security-report/SECURITY-REPORT.md` enumerates 7 races. Two are HIGH:
- **RACE-001:** Deployment trigger race. A burst of webhook-driven triggers for the same app can allocate duplicate deployments because the "is a deploy already running?" check is not atomic with the insert. `internal/api/handlers/deploy_trigger.go:62`.
- **RACE-002:** `GetNextDeployVersion` is a read-modify-write outside a transaction. A new `AtomicNextDeployVersion` method exists in the `DeploymentStore` interface and in `internal/db/deployments.go`, but the handler at `internal/api/handlers/deployments.go:115` still calls the non-atomic variant.

Four medium-severity races (rate limiter TOCTOU, BBolt metrics, connection tracking, idempotency cache) round out the list. Remediations are scoped but not shipped.

### 3.4 Security assessment (summary; full details in `security-report/`)

| Category | Severity | Count |
|---|---|---|
| Critical (AUTHZ-001) | 🔴 | 1 |
| High | 🟠 | 12 |
| Medium | 🟡 | 21 |
| Low | 🟢 | 13 |

**Critical — AUTHZ-001** (`internal/api/handlers/domains.go:83–141`): domain creation does not verify that the authenticated tenant actually owns the target `AppID`. Effect: cross-tenant domain hijacking against any running app.

**High-severity highlights:**
- **CORS-001** — `Access-Control-Allow-Origin: *` wildcard with `Allow-Credentials`. `internal/api/middleware/middleware.go:112`. Explicitly confirmed by commit `a72550d "fix: eliminate CORS headaches - always allow all origins"` from recent history. That commit trades security for ergonomics.
- **CORS-002** — Empty-origin bypass in the WebSocket upgrader (`internal/api/ws/deploy.go:107`).
- **AUTHZ-002 / AUTHZ-003** — Port and health-check endpoints do not perform tenant-scope checks.
- **AUTH-001** — JWT *refresh* handler does not explicitly verify the `alg` header (`internal/auth/jwt.go:208`). HS256 is pinned elsewhere, but the refresh path is a `jwt.Parse` that trusts header-declared algorithm if misused.
- **SESS-001** — Access tokens cannot be revoked; a stolen access token is valid until expiry.
- **RACE-001 / RACE-002** — see §3.3.

**Safe areas.** Parameterized SQL everywhere (spot-check of `internal/db/apps.go` and 4 other files). `exec.Command` uses slice args, never shell interpolation (`internal/build/builder.go:340,349,393`). Path traversal guarded by `ValidateVolumePaths`. XSS protected by React JSX auto-escaping + a comprehensive CSP. Password hashing is bcrypt cost 13. Secrets encrypted AES-256-GCM with versioning.

**Secrets in source.** Grep for `password|secret|api[_-]?key|token = "…"` in `internal/` / `cmd/` returns 0 genuine hardcoded secrets (only test fixtures and docker-compose demo creds labelled as such).

---

## 4. Testing Assessment

### 4.1 Coverage

Fresh run of `go test -short -count=1 ./...` today:

- **27 of 31 packages PASS**, **4 packages FAIL** (discovery, ingress, swarm; swarm also flaky at 9.8 s).
- Median coverage: **82–90%** — 18/31 packages ≥ 85%.
- Packages below 75%: `cmd/openapi-gen` (54%), `internal/marketplace` (**69.1%**), `internal/webhooks` (77.1%), `internal/db` (78.9%), `internal/auth` (78.7%).
- Zero-test packages: `cmd/deploymonster` (exercised via integration), `internal/db/models`.

**The 85% CI coverage gate is enforced** (see `.github/workflows/ci.yml` `test` job), and it's computed across aggregated coverage — marketplace at 69% doesn't break CI because the aggregate clears 85%.

**Verified failing tests today:**

| Package | Test | Symptom |
|---|---|---|
| `internal/discovery` | `TestHealthChecker_CheckAll_TCPUnhealthy` | `health_test.go:307`: closed port returns **healthy** |
| `internal/ingress` | `TestReverseProxy_ServeHTTP_BackendConnectionError` | Expected `502`, got `404` (`coverage_boost_test.go:327`) |
| `internal/ingress` | `TestReverseProxy_CircuitBreaker_RecordsFailure` | Expected `502`, got `404`; no circuit-breaker stat recorded (`proxy_test.go:257`) |
| `internal/swarm` | `TestAgentClient_Dial_DefaultPort` | Error `"master rejected connection: HTTP 200"` — expected a port-related message (`swarm_coverage_test.go:1320`) |

These are **real bugs**, not flakes — each reflects an incorrect code path, not a timing issue.

### 4.2 Test infrastructure

- **Table-driven tests with `t.Run` subtests** are the norm.
- Mocks implement interfaces with optional function fields + call-tracking booleans (pattern documented in `internal/deploy/mock_test.go`).
- **13 benchmark test files** (fewer than typical for a 46 kLOC codebase but adequate — benchmarks focus on hot paths: bolt ops, event bus, JWT, bcrypt).
- **No fuzz tests found** (`grep -rln 'func Fuzz' --include='*_test.go'` returns 0). This contradicts the `PRODUCTION-READY.md` and `STATUS.md` claims of "15 fuzz targets"; those were either never committed, later removed, or miscounted.
- **Integration tests** gated by `-tags integration` and `-tags pgintegration`. The Postgres conformance suite (`make test-integration-postgres`) requires `TEST_POSTGRES_DSN`.
- **Load/soak framework**: `tests/loadtest/` (HTTP baseline, 10% regression gate) and `tests/soak/` (24 h soak harness + 5-min smoke).
- **Baseline-gated tests** are committed as JSON artifacts (`tests/loadtest/baselines/http.json`, `internal/db/testdata/concurrent_writes_baseline.json`). `make db-gate` compares against committed p95 — currently `continue-on-error` in CI because the baseline was captured on dev hardware (16-core Ryzen) vs a 2-vCPU GH Actions runner.
- **E2E tests**: 11 Playwright specs under `web/e2e/` with `continue-on-error: true` in CI. Recent git log (last 9 commits) is almost entirely E2E stabilization work, confirming the suite is brittle in its current state.

---

## 5. Specification vs Implementation Gap Analysis ⚠️

This is the most important section. The project ships docs (README, `PRODUCTION-READY.md`, `TASKS.md`) claiming feature completeness. The audit evaluates those claims against the code.

### 5.1 Feature completion matrix

| Planned feature | Source of claim | Actual implementation | Status | Evidence |
|---|---|---|---|---|
| Git-to-deploy (4 providers) | README | GitHub, GitLab, Gitea, Bitbucket | ✅ | `internal/gitsources/providers/` |
| 14 language detectors | README | 14 present | ✅ | `internal/build/detect/*.go` |
| 12 Dockerfile templates | README | 12 present; Scala/Kotlin/Haskell/Elixir/Clojure detectors have **no** matching template | ⚠️ | `internal/build/dockerfiles/` |
| 5 load-balancer strategies | README | 4 real (round-robin, least-conn, IP-hash, random) + 1 skeletal weighted | ⚠️ | `internal/ingress/lb/weighted.go` |
| **150+ marketplace templates** | README, SPEC, `PRODUCTION-READY.md` | **56 unique templates** (≈132 Slug entries, many duplicates) | ❌ | `internal/marketplace/builtins*.go` |
| Managed DBs: PG, MySQL, MariaDB, Redis, MongoDB | README | PG, MySQL, Redis implemented; MariaDB = MySQL binary; **MongoDB: type enum only** | ⚠️ | `internal/database/engines/*.go` |
| Backup: S3 / MinIO / R2 | README | Generic S3-compat client works for all three; no MinIO-specific code | ⚠️ | `internal/backup/storage/` |
| DNS: Cloudflare, Route53, RFC2136 | SPEC | **Cloudflare only** | ❌ | `internal/dns/providers/cloudflare.go` (sole file) |
| VPS: Hetzner, DO, Vultr, Linode, AWS, SSH | SPEC | Hetzner, DO, Vultr, Custom-SSH complete; Linode ≈ 50-LOC stub; **AWS absent** | ⚠️ | `internal/vps/providers/` |
| Auth + RBAC + JWT + API keys + SSO | SPEC | All present | ✅ | `internal/auth/` |
| Secrets vault AES-256-GCM | SPEC | Fully implemented, versioned | ✅ | `internal/secrets/` |
| Notifications: email/Slack/Discord/Telegram/webhook | README | All 5 | ✅ | `internal/notifications/` |
| Webhooks inbound HMAC + outbound | SPEC | Both present | ✅ | `internal/webhooks/` + handlers |
| Billing: Stripe, metered | SPEC | Present | ✅ | `internal/billing/` |
| Enterprise: white-label, reseller, HA | SPEC | Scaffolded; HA/Litestream story thin | ⚠️ | `internal/enterprise/` |
| MCP server with 9 tools | SPEC | 9 tools implemented | ✅ | `internal/mcp/server.go` |
| Master/agent mode | SPEC | Master solid; agent dial test currently failing | ⚠️ | `internal/swarm/` |
| Resource metrics + `/metrics` | SPEC | Present | ✅ | `internal/resource/`, `/api/v1/servers/*/metrics` |
| **240 API endpoints** | README | **205 wired, ~88 in OpenAPI** | ❌ | `internal/api/router.go` vs `docs/openapi.yaml` |
| **222 handlers, zero placeholders** | `PRODUCTION-READY.md` | ≈185 handlers, **26 `StatusNotImplemented` stubs** | ❌ | grep |

### 5.2 Architectural deviations

1. **CORS hardening reversed.** A recent commit (`a72550d`, "fix: eliminate CORS headaches - always allow all origins") explicitly weakened the originally-designed CORS policy. Likely a dev-loop convenience that leaked to main. Ship-blocker under the original threat model.
2. **Marketplace dedup missed.** `internal/marketplace/builtins_100.go` has ~80 template definitions but many are near-duplicates, producing only 56 unique slugs in practice. The "100" in the filename is aspirational.
3. **OpenAPI drift normalized.** The drift allowlist (`docs/openapi-drift-allowlist.txt`) captures a ~144-route gap as acceptable. That's pragmatic engineering but misrepresents the externally-visible API surface.

### 5.3 Task completion assessment

`.project/TASKS.md` claims **251 / 251 complete** across 15 phases. Spot-check of 15 diverse tasks:

- 13 / 15 verifiably complete in code.
- 2 partial: "20+ marketplace apps" (claim was 20+, delivery 56 — passes); "Linode provider" (checked ✅ in TASKS but the file is a 50-LOC stub — fails).

Projecting across the full list, **actual completion is likely ~90–92%**, not 100%. The task ledger is *close* but contains ⚠️-level approvals that deserve "partial" status.

### 5.4 Scope creep

Code that exists but is not in the original SPECIFICATION.md:
- `internal/topology/` — React Flow DAG editor. Recent feature; adds real value.
- `internal/awsauth/` — AWS Cognito adapter (brief spec mention, heavily tested).
- Multiple "Tier 69 / 73 / 100 / 102" hardening passes enumerated throughout comments and CHANGELOG. These are not scope creep so much as undocumented iteration — but they cause inconsistency between SPEC and reality.
- The OpenAPI drift tool and writers-under-load gate — operational tooling added during the stabilization phase.

None of this is "bad" scope creep. It's just un-specified.

### 5.5 Missing critical components (prioritized)

| Priority | Gap | Impact |
|---|---|---|
| P0 | Fix 4 failing tests (ingress proxy ×2, discovery TCP health, swarm agent dial) | Claims of "production-ready" fall apart otherwise |
| P0 | AUTHZ-001 domain-hijacking fix | Cross-tenant compromise today |
| P0 | CORS-001 strict origin policy | Credential-bearing cross-origin reads today |
| P1 | RACE-001 / RACE-002 remediation at call sites | Duplicate-deployment incidents |
| P1 | MongoDB engine impl OR remove from marketing | Truth-in-advertising |
| P1 | Route53 DNS provider OR remove claim | Truth-in-advertising |
| P2 | Linode provider real impl | Secondary VPS support |
| P2 | AWS VPS provider | Tertiary |
| P2 | Weighted LB finished | Canary deploys feature |
| P2 | Marketplace: get to 100+ verified templates | Close the 150-vs-56 gap |
| P3 | Stabilize E2E suite and remove `continue-on-error` | CI signal quality |
| P3 | db-gate baseline on GH Actions runners; flip to blocking | Perf regression signal |

---

## 6. Performance & Scalability

### 6.1 Performance patterns

- **Connection pooling**: SQLite/Postgres drivers' own pools; shared HTTP client in `internal/core/httpclient.go`; single Docker SDK client per app lifetime.
- **Streaming**: list handlers use `writePaginatedJSON` with cursor pagination (good).
- **No `sync.Pool`** (probably fine at current traffic profile).
- **Integer-overflow warnings (gosec G115)** in `internal/ingress/lb/balancer.go:102`, `internal/resource/host_other.go:31`, `internal/deploy/docker.go` — theoretical rather than exploitable at realistic container/backend counts.

### 6.2 Scalability

- **Horizontal scale path** is the master/agent model. Agents are stateless executors over WebSocket; the master is the single point of truth.
- State: master holds SQLite/Postgres + BBolt. SQLite single-writer remains the bottleneck under heavy concurrent writes — this is exactly what `make db-gate` watches for. The committed baseline was captured on a 16-core dev machine, so the current "gate" is not representative of production topology. Litestream-based HA is mentioned in `internal/enterprise/` but is not a concrete, tested story.
- **Back-pressure**: rate limiting at global (120/min/IP) and per-tenant (100/min) tiers. Per-tenant build queue prevents a noisy tenant from starving the platform. Event bus has a bounded async pool.
- **Sticky sessions**: not required — stateless JWT.

---

## 7. Developer Experience

### 7.1 Onboarding
Clone → `make build` works if Go 1.26 and `pnpm` are installed. `scripts/build.sh` runs the full `pnpm install && pnpm run build → cp → go build` pipeline. `make dev` is `go run` (no React HMR — `cd web && pnpm run dev` must be run separately). The README's one-line curl install is appropriate for end-users, not for contributors.

### 7.2 Documentation quality
- README is glossy but **overstates features** (see §5).
- `docs/openapi.yaml` is an OpenAPI 3.0.3 spec — covers 88 of 205 routes. `docs/examples/api-quickstart.md` has curl recipes.
- `CLAUDE.md` is current and useful.
- `.project/SPECIFICATION.md`, `.project/IMPLEMENTATION.md`, `.project/TASKS.md` exist but claim 100% completion against reality that's ≈90%.
- No ADRs (Architecture Decision Records). Good candidates: "Why SQLite not Postgres by default", "Why custom API client not TanStack Query", "Why embedded UI not separately-served", "Why a modular monolith not microservices".

### 7.3 Build & deploy
- `make build-all` cross-compiles 5 platforms. Static binaries (CGO off).
- Production Dockerfile is **scratch** image + CA certs + tzdata + binary → minimal attack surface. Non-root (65534:65534). Health check against `/api/v1/health`. SBOM labels.
- GoReleaser produces tar.gz + zip archives, SPDX SBOMs, SHA256 checksums, GHCR image.
- CI pipeline (`.github/workflows/ci.yml`): Go test w/ race + coverage gate, Vitest + `tsc --noEmit`, gitleaks (SHA-pinned), golangci-lint-less (`go vet` + build), 4-branch integration test matrix, Postgres integration, Playwright E2E (continue-on-error), multi-arch build matrix, Docker build on push, GoReleaser on tag, post-release Trivy scan (SHA-pinned).
- Release flow is mature for a pre-1.0 project.

---

## 8. Technical Debt Inventory

### 🔴 Critical — blocks "production ready"
1. **4 failing Go tests** (discovery, ingress×2, swarm). `go test ./...` is red today. `internal/discovery/health.go` health-check logic; `internal/ingress/proxy.go` error-path wiring; `internal/swarm/agent.go` dial-error message shape. **Est. 8–12 h to fix + reland.**
2. **AUTHZ-001** domain-hijacking. Add `requireTenantApp(h.store, claims, req.AppID)` to `internal/api/handlers/domains.go:83`. **1–2 h.**
3. **CORS-001** — revert the wildcard. Implement origin allowlist (read from `core.Config.Server.CORSOrigins`, reject empty origin where credentials are enabled). **2–4 h.**
4. **Version/CHANGELOG drift.** `VERSION` file says `v0.1.2` (2026-04-14); README and `.project/STATUS.md` say `v0.1.6`. A tag/release was never cut for 0.1.3–0.1.6. **30 min to reconcile.**

### 🟡 Important — should fix before v1.0
5. **RACE-001/002** — route all callers to `AtomicNextDeployVersion`; make deploy-trigger insert use `INSERT … WHERE NOT EXISTS` or a named lock. `internal/api/handlers/deployments.go:115`, `deploy_trigger.go:62`.
6. **AUTH-001** — pin `alg` check on refresh-token parse.
7. **SESS-001** — access-token revocation (denylist in BBolt keyed on JTI, TTL = access-token expiry).
8. **AUTHZ-002/003** — tenant-scope checks on `ports.go:44`, `healthcheck.go:47`.
9. **MongoDB engine** — implement or remove from README/SPEC.
10. **Route53 DNS** — implement or remove from SPEC.
11. **Linode provider** — complete or remove from provider list.
12. **Marketplace 150-vs-56 claim** — stop claiming 150; add the missing ~94 templates over time, or publicly re-baseline.
13. **26 `StatusNotImplemented` handlers** — finish or hide them behind a feature flag.
14. **E2E `continue-on-error: true`** — stabilize the suite and flip to blocking.
15. **`db-gate` baseline** on GH Actions runners; remove `continue-on-error`.

### 🟢 Minor — nice to fix
16. Split `internal/db/` vs `internal/database/` or rename one (naming is confusing).
17. Remove unused OpenTelemetry auto-SDK transitive if no exporter is wired.
18. Move two legacy `log.*` call sites to `slog`.
19. Consider `react-hook-form` + `zod` as forms grow.
20. Add `axe-playwright` to CI for basic accessibility checks.
21. Document ADRs for 4–6 load-bearing decisions (see §7.2).
22. `internal/marketplace/` coverage 69% → 85%.

---

## 9. Metrics Summary Table

| Metric | Value |
|---|---|
| Total Go files (all) | 593 |
| Go source files (non-test) | 278 |
| Total Go LOC | 46,028 |
| Go test files | 312 |
| Go test LOC | 12,631 |
| Median test coverage | 82–90% (aggregate ≥ 85% gate-enforced) |
| Frontend TS/TSX files | 123 |
| Frontend LOC | 22,338 |
| Frontend unit-test files | 36 |
| Frontend E2E spec files | 11 |
| External Go deps (direct) | 8 (+ 63 transitive) |
| External frontend deps (direct) | ~25 |
| Registered backend modules | 20 |
| API endpoints wired | 205 |
| API endpoints in OpenAPI | ~88 (drift allowlisted) |
| `StatusNotImplemented` handler stubs | 26 |
| Open critical/high security findings | 1 crit + 12 high |
| Currently failing Go tests | 4 |
| Spec feature completion (measured) | ≈ 85–90% |
| Task completion (measured) | ≈ 90–92% (claimed 100%) |
| Production-readiness (measured) | ≈ 68 / 100 (see PRODUCTIONREADY.md) |
| **Overall health score** | **7.2 / 10** |

---

## 10. Conclusion

DeployMonster is a **very capable pre-1.0 PaaS** with solid bones. The architecture is clean, the Go code is high quality by the objective metrics (formatting, vet, error wrapping, structured logging, coverage), the frontend is strictly typed and well-organized, and the CI/CD pipeline is more mature than most projects this size.

The honest gap is between what the repository *says* and what the repository *is*. `PRODUCTION-READY.md` claims 100/100; 4 tests fail today and 13 security findings are live. README claims 240 endpoints and 150 marketplace templates; reality is 205 and 56. Several provider claims are aspirational.

Close those gaps — either by finishing the work or by pruning the claims — and this project is genuinely production-ready for its stated scope. The roadmap in `.project/ROADMAP.md` lays out how; the verdict in `.project/PRODUCTIONREADY.md` is blunt about what needs to happen before a 1.0 release.
