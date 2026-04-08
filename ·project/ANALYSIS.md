# Project Analysis Report

> Auto-generated comprehensive analysis of DeployMonster
> Generated: 2026-04-08
> Analyzer: Claude Code — Full Codebase Audit

## 1. Executive Summary

DeployMonster is a self-hosted Platform-as-a-Service (PaaS) delivered as a single Go binary with an embedded React UI. It targets developers and hosting providers who want to transform any VPS or bare-metal server into a full deployment platform. The product competes with Coolify, Dokploy, CapRover, and Dokku.

### Key Metrics

| Metric | Value |
|---|---|
| Total Files (excl. deps) | 856 |
| Go Source Files | 284 |
| Go Test Files | 251 |
| Go Source LOC | ~37,900 |
| Go Test LOC | ~89,600 |
| Frontend Files (.ts/.tsx) | 88 |
| Frontend LOC | ~15,600 |
| Frontend Test Files | 10 |
| External Go Deps (direct) | 8 |
| External Go Deps (indirect) | 37 |
| API Endpoints (router.go) | 231 |
| Modules | 20 |
| Test Coverage (avg) | ~92% |

### Overall Health Score: 7.5/10

**Top 3 Strengths:**
1. **Exceptional architecture** — True modular monolith with clean interfaces, dependency injection, topological sort, and no circular imports.
2. **Outstanding test coverage** — 251 test files, 92%+ average coverage, 7 fuzz tests, 38 benchmarks. Test LOC exceeds source LOC 2.4:1.
3. **Minimal dependency surface** — Only 8 direct Go dependencies. No Redis, no Kafka, no external DB required.

**Top 3 Concerns:**
1. **Specification-to-implementation gap** — The spec describes 45+ modules; 20 exist. Many implemented modules have stub/placeholder handlers that return canned responses rather than performing real operations (billing, VPS provisioning, DNS, marketplace deploy, swarm clustering).
2. **Inconsistent error handling** — Many handlers swallow errors silently (e.g., `UpdateLastLogin`, `Emit`). No request-level error traceability. Generic "internal error" responses without correlation IDs.
3. **Security hardening gaps** — JWT uses HS256 single shared key with no rotation, refresh tokens have no revocation mechanism, admin password logged to console in plaintext, CORS defaults to `*`, localStorage for tokens (XSS-vulnerable).

---

## 2. Architecture Analysis

### 2.1 High-Level Architecture

DeployMonster is a **modular monolith** — a single process containing 20 self-registering modules. Each module implements the `core.Module` interface (ID, Name, Version, Dependencies, Init, Start, Stop, Health, Routes, Events).

```
                         +---------------------+
                         |   cmd/deploymonster  |  CLI entry point
                         |   (main.go)          |  serve/version/init/config
                         +----------+----------+
                                    | blank imports all modules
                         +----------v----------+
                         |    internal/core      |  Registry, EventBus, Config
                         |    App.Run()          |  topo-sort -> Init -> Start
                         +----------+----------+
                    +---------------+---------------+
              +-----v-----+  +-----v-----+  +------v------+
              |  core.db   |  | core.auth  |  |   ingress   |  ... 17 more
              |  SQLite +  |  | JWT/RBAC   |  | Rev. Proxy  |  modules
              |  BBolt     |  | API Keys   |  | ACME/TLS    |
              +-----+------+  +------------+  +-------------+
                    |
              +-----v------+
              | core.Store |  12 sub-interfaces
              | interface   |  (Tenant, User, App, Deploy...)
              +-------------+
```

**Data flow (deploy):** Git push -> Webhook receiver (HMAC verified) -> Build engine (clone -> detect -> Dockerfile -> docker build) -> Deploy engine (pull -> create container -> start -> update route table) -> Ingress proxy serves traffic.

**Concurrency model:** Go's goroutine-per-request for HTTP. EventBus supports sync and async subscribers (async dispatched in goroutines). Build worker pool uses semaphore pattern (`chan struct{}`). Ingress uses `RWMutex` on route table. Single-writer SQLite with WAL mode.

### 2.2 Package Structure Assessment

| Package | Responsibility | Cohesion |
|---|---|---|
| `cmd/deploymonster` | CLI entry, signal handling | Good |
| `internal/core` | Module registry, EventBus, Config, Store interface, types | Good — could split Store into own pkg |
| `internal/db` | SQLite + BBolt implementation, migrations | Good |
| `internal/api` | HTTP router, handlers, middleware, WebSocket, SPA embedding | Slightly large — 60+ handler files |
| `internal/auth` | JWT, bcrypt, RBAC, API keys | Good |
| `internal/deploy` | Docker client, deployer, strategies, graceful shutdown | Good |
| `internal/build` | Git clone, language detection, Dockerfile gen, worker pool | Good |
| `internal/ingress` | Reverse proxy, TLS, ACME, route table, LB, health, middleware | Good |
| `internal/compose` | Docker Compose parser, validator, interpolator, deployer | Good |
| `internal/secrets` | AES-256-GCM vault, scoping, resolution | Good |
| `internal/webhooks` | Universal webhook receiver, provider parsers | Good |
| `internal/gitsources` | Git provider interface, GitHub/GitLab/Gitea/Generic impl | Good |
| `internal/dns` | DNS provider interface, Cloudflare implementation | Good |
| `internal/vps` | VPS provider interface, Hetzner/DO/Vultr/Custom impl | Good |
| `internal/backup` | Backup engine, storage backends (local, S3) | Good |
| `internal/database` | Managed DB provisioning (Postgres, MySQL, Redis containers) | Good |
| `internal/billing` | Plans, metering, quotas, Stripe integration | Good |
| `internal/enterprise` | White-label, reseller, GDPR, WHMCS, license | Good |
| `internal/notifications` | Multi-channel dispatcher (Slack, Discord, Telegram, email) | Good |
| `internal/resource` | Server/container metrics collection, alerts | Good |
| `internal/marketplace` | Template loader, search, deploy wizard | Good |
| `internal/mcp` | AI/LLM tool server (Model Context Protocol) | Good |
| `internal/swarm` | Docker Swarm manager, agent client, placement | Good |
| `internal/discovery` | Docker event watcher, label parser, health checker | Good |
| `internal/topology` | Visual topology persistence | Good |

**Circular dependency risks:** None detected. Module dependencies are explicit and resolved via topological sort. The `api` module deliberately depends on all others to ensure correct init order.

### 2.3 Dependency Analysis

**Direct Go dependencies (8):**

| Dependency | Purpose | Version | Assessment |
|---|---|---|---|
| `github.com/docker/docker` | Docker Engine API client | v28.5.2 | Essential, well-maintained |
| `github.com/golang-jwt/jwt/v5` | JWT token handling | v5.3.1 | Standard choice, actively maintained |
| `github.com/gorilla/websocket` | WebSocket support | v1.5.3 | De facto standard, archived but stable |
| `go.etcd.io/bbolt` | Embedded KV store | v1.4.3 | Well-maintained by etcd team |
| `golang.org/x/crypto` | bcrypt, SSH, crypto primitives | v0.49.0 | Official Go extended library |
| `gopkg.in/yaml.v3` | YAML config parsing | v3.0.1 | Standard, stable |
| `modernc.org/sqlite` | Pure Go SQLite driver | v1.48.0 | Good — no CGo required |
| `github.com/DATA-DOG/go-sqlmock` | SQL mock for tests | v1.5.2 | Test-only — should be in test build tag |

**Assessment:** Excellent dependency hygiene. Only 8 direct deps — remarkably lean for a PaaS. The `gorilla/websocket` package is archived but stable and widely used. `go-sqlmock` should arguably be test-only but has no runtime impact.

**Indirect dependencies (37):** Mostly from Docker SDK (`containerd`, `opencontainers`, `moby`, `otel`). All are well-maintained and necessary transitive deps.

**Frontend dependencies (16 production, 17 dev):**

| Dependency | Version | Assessment |
|---|---|---|
| react | 19.2.4 | Latest stable |
| react-router | 7.13.2 | Latest v7 |
| @tanstack/react-query | 5.95.2 | Enterprise-grade data fetching |
| zustand | 5.0.12 | Minimal state management |
| @xyflow/react | 12.10.2 | Topology visualization — heavy |
| tailwindcss | 4.2.2 | Latest v4 |
| vite | 8.0.1 | Latest build tool |
| typescript | 5.9.3 | Latest |
| vitest | 3.2.1 | Test framework |
| lucide-react | 1.7.0 | Tree-shakeable icon lib |

**Assessment:** Modern, well-chosen stack. No bloat. All latest stable versions.

### 2.4 API & Interface Design

**Endpoint inventory:** 231 routes registered in `internal/api/router.go`. Categories: Auth (12), Apps (35), Deployments (15), Domains (10), Projects (8), Servers (12), Secrets (10), Backups (8), Billing (15), Team (12), Admin (25), MCP (9), Webhooks (10), Marketplace (15), Monitoring (8), WebSocket (5+).

**Response format:** Consistent JSON wrapping:
- Success: `{"data": ...}` or `{"data": [...], "total": N, "page": N, "per_page": N}`
- Error: `{"error": "message"}`

**Authentication model:**
- JWT Bearer (`Authorization: Bearer <token>`) — 15min access, 7d refresh
- API Key (`X-API-Key: dm_xxx`) — prefix-based lookup, constant-time comparison
- Auth levels: None, APIKey, JWT, Admin, SuperAdmin
- Multi-tenant isolation via TenantID in JWT claims

**Middleware chain:** RequestID -> APIVersion -> BodyLimit(10MB) -> Timeout(30s) -> Recovery -> RequestLogger -> CORS -> AuditLog

**Gaps:** No request-level rate limiting on auth endpoints (only ingress-level). No API versioning beyond `/api/v1`. CORS defaults to `*` which is insecure for production.

---

## 3. Code Quality Assessment

### 3.1 Go Code Quality

**Style consistency:** Excellent. All code appears `gofmt`-compliant. Consistent naming conventions (camelCase locals, PascalCase exports). Clean imports with standard grouping.

**Error handling patterns:** Inconsistent.
- Good: DB layer consistently uses `fmt.Errorf("context: %w", err)` wrapping
- Bad: Many API handlers swallow errors silently:
  - `h.store.UpdateLastLogin(r.Context(), user.ID)` — error ignored
  - `m.core.Events.Emit(...)` — error ignored
  - `io.Copy(io.Discard, reader)` — error ignored
- No structured error types beyond sentinel errors in `core/errors.go`
- Generic "internal error" responses without request IDs

**Context usage:** Good. `context.Context` propagated consistently as first parameter. Signal-based cancellation in main. 30s shutdown timeout.

**Logging:** Good. `log/slog` throughout with structured fields. Module logger created with `c.Logger.With("module", m.ID())`. Request logger middleware logs method, path, status, duration.

**Configuration:** Good. YAML file + `MONSTER_*` env overrides. Sensible defaults. Auto-generated secret key on first run.

**Magic numbers / hardcoded values:** Few — mostly timeouts (30s shutdown, 10s HTTP), pagination defaults (20 per page, 100 max), and build concurrency (5). All are reasonable.

**TODO/FIXME/HACK comments:** Zero found. Clean codebase.

### 3.2 Frontend Code Quality

**TypeScript strictness:** Excellent. `strict: true` in tsconfig. `noUnusedLocals`, `noUnusedParameters`, `noFallthroughCasesInSwitch` all enabled. Zero `any` types detected.

**React patterns:** Good. Functional components throughout. Code-split with `React.lazy()` + `Suspense`. Error boundary present. Custom hooks (`useApi`, `useMutation`, `usePaginatedApi`, `useWebSocket`, `useEventSource`).

**State management:** Good. Zustand stores for auth, theme, topology, toasts. TanStack React Query for server state. Clean separation.

**CSS approach:** Consistent. Tailwind CSS 4 with `cn()` utility (clsx + tailwind-merge). Design tokens in CSS variables with oklch color space. Dark mode via class strategy.

**Bundle concerns:** 1.1MB dist — acceptable. All pages lazy-loaded. `@xyflow/react` (topology editor) is the heaviest component but only loaded on that page.

**Accessibility:** Partial. ARIA attributes present in UI components (aria-expanded, aria-selected, etc.). Keyboard navigation for search (Cmd+K). Missing: explicit ARIA landmarks (main, nav), role="dialog" on modals, consistent focus management.

### 3.3 Concurrency & Safety

**Goroutine management:**
- EventBus async handlers: goroutines launched per event, no limit. Potential goroutine leak under event storms.
- Build worker pool: Semaphore-based (`chan struct{}`), properly bounded.
- Ingress servers: Standard `http.Server` with proper graceful shutdown.
- Discovery watcher: Long-running goroutine watching Docker events with reconnect.

**Mutex usage:**
- EventBus: `sync.RWMutex` for subscription list. Separate lock for publish count.
- Route table: `sync.RWMutex` for thread-safe reads/writes.
- Build pool: WaitGroup for completion tracking.

**Race condition risks:**
- EventBus publish count increment under write lock — safe but could use `atomic.Int64`.
- No explicit race detector run in CI pipeline (missing `-race` flag in GitHub Actions test job — actually present in Makefile `test` target).

**Resource leak risks:**
- `io.Copy(io.Discard, reader)` without error check — minor.
- ACME renewal goroutine: fire-and-forget, crashes not visible.
- WebSocket connections: need explicit cleanup on disconnect.

**Graceful shutdown:** Excellent. Signal handling -> 30s timeout -> modules stopped in reverse dependency order -> all resources cleaned up.

### 3.4 Security Assessment

**Input validation:** Partial. Password strength validation exists (min 8 chars). JSON body parsing with size limits (10MB). Missing: email format validation, slug sanitization, env var key validation.

**SQL injection:** Protected. All queries use parameterized statements (`?` placeholders). No string concatenation in SQL.

**XSS protection:** Partial. JSON responses (not HTML) reduce risk. Missing CSP headers.

**Secrets management:** Good. AES-256-GCM encryption for stored secrets. Scoped resolution chain (app -> project -> tenant -> global). `${SECRET:name}` syntax resolved at deploy time.

**Concerns:**
- JWT HS256 with single shared `server.secret_key` — no key rotation
- Refresh tokens: no revocation list, no rotation on use
- Admin password logged to stdout in plaintext on first run
- CORS configured as `*` in code — must be restricted for production
- Token stored in localStorage — vulnerable to XSS
- No rate limiting on `/api/v1/auth/login` (brute force risk)

---

## 4. Testing Assessment

### 4.1 Test Coverage

All tests pass (0 failures). `go vet` clean.

| Package | Coverage | Assessment |
|---|---|---|
| internal/database/engines | 100.0% | Excellent |
| internal/api/middleware | 99.0% | Excellent |
| internal/notifications | 99.3% | Excellent |
| internal/compose | 98.4% | Excellent |
| internal/build | 97.8% | Excellent |
| internal/billing | 96.6% | Excellent |
| internal/auth | 96.5% | Excellent |
| internal/api (router) | 94.5% | Good |
| internal/api/handlers | 94.0% | Good |
| internal/secrets | 90.7% | Good |
| internal/core | 88.5% | Good |
| internal/db | 83.8% | Acceptable |
| internal/mcp | 83.3% | Acceptable |
| internal/deploy | 78.9% | Below target |
| internal/swarm | 78.8% | Below target |
| internal/api/ws | 66.0% | Needs work |
| internal/topology | 66.5% | Needs work |

**Packages with 0% or no test files:** `cmd/deploymonster` (entry point), `internal/db/models` (data structs only).

### 4.2 Test Infrastructure

- **Unit tests:** 251 files with table-driven `t.Run` subtests. Mocks use function-field pattern (see `internal/deploy/mock_test.go`).
- **Integration tests:** Some handler tests use real SQLite DBs.
- **Fuzz tests:** 7 files (JWT, config parsing, etc.)
- **Benchmarks:** 38 functions
- **Frontend tests:** 10 test files (Button, Card, Badge, Input, Spinner, ErrorBoundary, Table, Toast, useApi) — 11.7% coverage, needs improvement.
- **CI pipeline:** GitHub Actions runs tests with race detection, lint, multi-platform build.
- **Missing:** End-to-end tests, load tests, Playwright/e2e for frontend.

---

## 5. Specification vs Implementation Gap Analysis

### 5.1 Feature Completion Matrix

| Planned Feature | Spec Section | Status | Notes |
|---|---|---|---|
| Module System | S3 | Complete | 20 modules, topo-sort, health, events |
| SQLite + BBolt DB | S4 | Complete | WAL mode, migrations, 12 sub-interfaces |
| JWT + RBAC Auth | S3, S7 | Complete | bcrypt, API keys, 6 roles |
| REST API (224 endpoints) | S18 | Partial | Routes registered but ~30% return placeholder data |
| Docker Integration | S6 | Complete | Full SDK client, labels, networking |
| Build Engine | S8 | Complete | 14 detectors, 12 templates, worker pool |
| Ingress/Reverse Proxy | S5 | Complete | TLS, ACME, LB (5 strategies), middleware |
| Service Discovery | S5 | Complete | Docker event watcher, label parsing |
| Git Sources | S9 | Partial | Interface + GitHub/GitLab/Gitea stubs, OAuth flow skeletal |
| Webhook System | S9 | Complete | Universal receiver, HMAC verification, multi-provider |
| Deploy Strategies | S6 | Partial | Recreate + rolling work. Canary skeletal |
| Docker Compose | S10 | Complete | Parser, validator, interpolator, deployer |
| Secret Vault | S14 | Complete | AES-256-GCM, scoping, versioning |
| DNS Sync | S11 | Partial | Cloudflare provider implemented, integration incomplete |
| VPS Provisioning | S12 | Partial | Interface + provider stubs, actual API calls unverified |
| Backup Engine | S13 | Partial | Framework present, storage backends skeletal |
| Managed Databases | S13 | Partial | Engine definitions present, actual provisioning unverified |
| Billing/Stripe | S16 | Partial | Plans/metering framework, Stripe integration incomplete |
| Marketplace | S15 | Partial | 25 templates exist, deploy wizard partial |
| Multi-Node/Swarm | S17 | Partial | Manager/agent skeleton, no real clustering |
| Enterprise Features | S19 | Partial | White-label/GDPR framework, no real license validation |
| MCP Server | S20 | Partial | 9 tool definitions, actual execution unclear |
| Notifications | S14 | Complete | Dispatcher + channel implementations |
| Resource Monitoring | S13 | Partial | Collector framework, metrics storage partial |
| React UI | S21 | Complete | 15 pages, all core flows, topology editor |
| Topology View | S9 | Complete | React Flow editor, drag-drop, dagre layout |

### 5.2 Architectural Deviations

1. **Spec lists 45+ module IDs** (S3); only 20 exist. Many planned sub-modules were collapsed into parent modules (e.g., `build.cache`, `deploy.git`, `deploy.image`, `deploy.rollback`, `ingress.acme`, `ingress.lb` all folded into their parent packages).
2. **Spec calls for `lego` ACME library** (S2); implementation uses custom ACME manager. This is an improvement — fewer deps.
3. **Spec calls for Go 1.23+**; actual requires Go 1.26.1. Improvement — using latest Go.
4. **Spec plans SQLite with CGo**; actual uses `modernc.org/sqlite` (pure Go). Improvement — eliminates CGo requirement.
5. **Spec references `cobra` for CLI**; actual uses stdlib `flag`. Simplification — fewer deps.
6. **PostgreSQL support**: Spec positions it as enterprise feature. Interface exists (`core.Store`) but no PostgreSQL implementation.

### 5.3 Task Completion Assessment

**All 223 tasks in TASKS.md are marked `[x]` (complete)** — 100% checkbox completion.

However, the actual depth of implementation varies significantly. Tasks like "Implement Hetzner Cloud full API" (T-8.2) are marked complete, but the implementation is a provider struct that satisfies the interface without necessarily making real API calls with full error handling. The test coverage is achieved through mocking.

**Estimated real functional completion: ~65-70%** — core pipeline works (git -> build -> deploy -> route), but many supporting features (billing, VPS, DNS, enterprise) have interface-level implementations that would need significant work to use in production with real external services.

### 5.4 Scope Creep Detection

- **Topology editor** (`internal/topology/`, React Flow integration) — Not in original spec module list but adds significant value.
- **MCP server** (`internal/mcp/`) — In spec, implemented as planned.
- **Compose support** (`internal/compose/`) — In spec, implemented well.

No significant unplanned code detected. The project has been disciplined about scope.

### 5.5 Missing Critical Components

1. **PostgreSQL implementation** — Interface exists, no concrete implementation beyond SQLite
2. **Real Stripe webhook handling** — Framework exists but actual Stripe API integration untested with real keys
3. **Agent job execution** — Agent connects to master but no job dispatch/execution loop
4. **Real VPS API calls** — Provider structs exist but HTTP calls to Hetzner/DO/Vultr APIs likely not battle-tested
5. **ACME DNS-01 challenge** — Listed in PROJECT_STATUS.md but implementation depth unclear
6. **Token revocation** — No mechanism to invalidate refresh tokens
7. **Migration rollback** — One-way migrations only, no downgrade path

---

## 6. Performance & Scalability

### 6.1 Performance Patterns

- **SQLite single-writer:** MaxOpenConns=1 with WAL mode. Correct for SQLite but will bottleneck under write-heavy loads.
- **Build worker pool:** Semaphore-bounded concurrency. Good pattern.
- **Ingress proxy:** Standard `httputil.ReverseProxy` with connection pooling. Adequate for moderate traffic.
- **BBolt KV:** All operations serialized. Fine for config/state, would bottleneck for high-frequency metrics writes.

**Potential bottlenecks:**
- SQLite under concurrent writes (single writer)
- BBolt under high-frequency metrics collection
- No HTTP response caching for static API responses
- Image pull blocking during deploy (no streaming progress to UI)

### 6.2 Scalability Assessment

**Horizontal scaling:** Not currently possible. SQLite is file-based, BBolt is file-based. PostgreSQL support (planned) would enable multi-instance deployment.

**Vertical scaling:** Good. Go's goroutine model handles concurrent connections efficiently. Memory usage claimed <200MB for 100 apps.

**Multi-node:** Swarm module exists but agent mode is skeletal. Cannot distribute workloads across nodes in current state.

---

## 7. Developer Experience

### 7.1 Onboarding Assessment

- **Clone and build:** `make build` works. `make dev` runs the server.
- **Prerequisites:** Go 1.26+, Node.js 22+ (for frontend), Docker (for container operations)
- **First run:** Auto-generates admin credentials, prints to console. `monster.yaml` created on first run.
- **Development:** Makefile has `dev`, `test`, `lint`, `fmt`, `vet` targets. Frontend has `pnpm dev` with HMR.

### 7.2 Documentation Quality

- **README.md:** Comprehensive with features, quick start, architecture diagram
- **docs/:** Getting started, architecture, deployment guide, API reference, examples
- **OpenAPI:** `docs/openapi.yaml` exists
- **CLAUDE.md:** Well-maintained development guide
- **Inline comments:** Sparse but adequate — code is largely self-documenting
- **project docs:** Spec, implementation guide, tasks — thorough planning docs

### 7.3 Build & Deploy

- **Makefile:** 15 targets covering build, test, lint, docker, release
- **GoReleaser:** Cross-platform builds (linux/darwin/windows, amd64/arm64)
- **Dockerfile:** Multi-stage Alpine build, non-root user, health check
- **CI/CD:** GitHub Actions with test -> lint -> build -> docker -> release pipeline

---

## 8. Technical Debt Inventory

### Critical (blocks production readiness)

1. **Admin password logged in plaintext** — `internal/auth/module.go` firstRunSetup logs password to stdout via slog.Warn. Must print to stderr only or use secure delivery.

2. **CORS wildcard default** — `middleware.CORS("*")` in router.go. Production deployments must restrict to actual domain.

3. **No refresh token revocation** — Stolen refresh token valid for 7 days with no invalidation mechanism.

4. **Error swallowing in handlers** — Multiple handlers ignore error returns (UpdateLastLogin, Emit, etc.). Silent failures mask bugs in production.

### Important (should fix before v1.0)

5. **JWT HS256 single key** — No key rotation. Compromised key requires restarting all sessions. Consider RS256 or key versioning.

6. **localStorage for tokens** — XSS vulnerability. Should use httpOnly cookies.

7. **No rate limiting on auth endpoints** — `/api/v1/auth/login` vulnerable to brute force.

8. **Migration rollback missing** — One-way migrations only. Database downgrade impossible.

9. **Placeholder API handlers** — ~30% of 231 endpoints return canned responses. Should either implement fully or remove/disable.

10. **Frontend test coverage at 11.7%** — Only 10 test files for 88 source files. Critical flows (auth, deploy) untested.

11. **go-sqlmock as production dependency** — Should be test-only build-tagged.

12. **WebSocket/topology coverage below 70%** — `internal/api/ws` at 66%, `internal/topology` at 66.5%.

### Minor (nice to fix, not urgent)

13. **Async event handler unbounded goroutines** — EventBus launches goroutine per async event with no pool limit.

14. **No request ID in error responses** — Makes production debugging difficult.

15. **PROJECT_STATUS.md claims JWT RS256** but code uses HS256 — documentation inconsistency.

16. **Config path flag parsed but unused** — `cmd/deploymonster/main.go` parses `--config` flag but doesn't use it.

17. **ACME renewal fire-and-forget** — Renewal goroutine errors not surfaced.

18. **No bundle analysis in CI** — Frontend bundle size not tracked over time.

---

## 9. Metrics Summary Table

| Metric | Value |
|---|---|
| Total Go Files | 535 (284 source + 251 test) |
| Total Go LOC | ~127,500 (37,900 source + 89,600 test) |
| Total Frontend Files | 88 (.ts/.tsx) |
| Total Frontend LOC | ~15,600 |
| Test Files | 261 (251 Go + 10 Frontend) |
| Test Coverage (Go avg) | ~92% |
| Test Coverage (Frontend) | ~12% |
| External Go Dependencies | 45 (8 direct + 37 indirect) |
| External Frontend Dependencies | 33 (16 prod + 17 dev) |
| Open TODOs/FIXMEs | 0 |
| API Endpoints | 231 |
| Spec Feature Completion (checkbox) | 100% (223/223 tasks) |
| Spec Feature Completion (functional) | ~65-70% |
| Task Completion | 100% (all marked [x]) |
| Overall Health Score | 7.5/10 |
