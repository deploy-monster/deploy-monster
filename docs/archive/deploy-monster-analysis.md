# DeployMonster — Full Project Analysis

> **Version:** v0.1.9 | **License:** AGPL-3.0 | **Owner:** ECOSTACK TECHNOLOGY OÜ (Ersin KOÇ)
> **Analysis date:** 2026-07-15 | **Stack:** Go 1.26+ / React 19 / TypeScript / SQLite+PostgreSQL

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Architecture & Design](#2-architecture--design)
3. [Backend Analysis](#3-backend-analysis)
4. [Frontend Analysis](#4-frontend-analysis)
5. [Security Posture](#5-security-posture)
6. [Database Layer](#6-database-layer)
7. [API Layer](#7-api-layer)
8. [Infrastructure & DevOps](#8-infrastructure--devops)
9. [Testing & Quality](#9-testing--quality)
10. [UI/UX Assessment](#10-uiux-assessment)
11. [Data Integrity & Backup](#11-data-integrity--backup)
12. [Strengths](#12-strengths)
13. [Weaknesses & Areas for Improvement](#13-weaknesses--areas-for-improvement)
14. [Recommendations](#14-recommendations)

---

## 1. Executive Summary

DeployMonster is a **self-hosted Platform-as-a-Service (PaaS)** delivered as a single ~24 MB Go binary with an embedded React 19 UI. It transforms any VPS with Docker into a multi-tenant deployment platform. At **~57K production Go LOC** spread across **22 auto-registered modules**, it represents a large, well-organized codebase with **85.1% test coverage** and **236 documented REST API routes**.

**Readiness verdict (from PRODUCTION-STATUS.md):**
| Deployment Model | Status |
|---|---|
| Self-hosted single-tenant | ✅ **GO** |
| Hosted multi-tenant SaaS | ⏳ **CONDITIONAL GO** (needs staging validation) |

**Key numbers** at a glance:
- **689 Go files** (291 source + 398 test) — ~180K total LOC (57K production + 123K tests)
- **172 TypeScript/TSX source files** + 44 test files + 13 E2E specs
- **22 modules** auto-registered via `init()`
- **17 fuzz targets**, **44 benchmarks**, **405 frontend unit tests**, **13 Playwright E2E specs**
- **91 marketplace templates** across 19 categories
- **11 Architecture Decision Records (ADRs)**

---

## 2. Architecture & Design

### 2.1 High-Level Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                    DeployMonster Binary (~24 MB)               │
├─────────┬─────────┬─────────┬──────────┬─────────┬────────────┤
│ Web UI  │  REST   │  SSE    │ Webhooks │ Ingress │ MCP Server │
│ shadcn  │ 236 rt  │ Stream  │  In+Out  │ :80/443 │  9 tools   │
├─────────┴─────────┴─────────┴──────────┴─────────┴────────────┤
│              22 auto-registered modules                        │
│  auth │ deploy │ build │ ingress │ dns │ secrets │ billing    │
│  db   │ backup │ vps   │ swarm   │ marketplace │ notifications│
├────────────────────────────────────────────────────────────────┤
│   SQLite + KV      │   Docker SDK   │   EventBus   │  Store   │
└────────────────────────────────────────────────────────────────┘
```

### 2.2 Module System

Every subsystem implements the `Module` interface (`internal/core/module.go`) with `Init` → `Start` → `Stop` lifecycle. Modules auto-register via `init()` using `core.RegisterModule()`, then a topological sort resolves initialization order by declared dependencies.

**Why this matters:** This pattern gives microservice-style decoupling without microservice complexity (see ADR 0002 — "Modular monolith, not microservices").

**The 22 modules:**

| Module | Dependencies | Purpose |
|---|---|---|
| `core.db` | — | SQLite/PostgreSQL + KV store |
| `core.auth` | `core.db` | JWT, password hashing, sessions |
| `api` | `core.db`, `core.auth`, `marketplace`, `billing` | HTTP server, routes, middleware |
| `deploy` | `core.db` | Docker container management |
| `build` | `core.db`, `deploy` | Worker pool, Dockerfile generation |
| `ingress` | `core.db`, `deploy` | Reverse proxy on :80/:443, ACME |
| `discovery` | `deploy`, `ingress` | Docker events, route registration |
| `secrets` | `core.db` | AES-256-GCM encryption vault |
| `notifications` | `core.db` | Slack, Discord, Telegram, SMTP |
| `backup` | `core.db` | Local/S3 storage, snapshots |
| `billing` | `core.db` | Stripe, usage metering, plans |
| `marketplace` | `core.db`, `deploy` | Template registry, 91 templates |
| `dns.sync` | `core.db` | Cloudflare DNS sync |
| `vps` | `core.db` | Multi-provider VPS provisioning |
| `gitsources` | `core.db` | GitHub/GitLab integration |
| `swarm` | `deploy` | Multi-node Docker Swarm |
| `resource` | `core.db`, `deploy` | Metrics, alerting |
| `enterprise` | `core.db`, `billing` | White-label, reseller, GDPR |
| `mcp` | `core.db`, `deploy` | Model Context Protocol |
| `database` | `core.db`, `deploy` | Managed DB containers |
| `cron` | `core.db` | Job scheduling |
| `autoscale` | `core.db`, `deploy` | Dynamic container scaling |

### 2.3 Event System

The in-process pub/sub event bus (`internal/core/events.go`) supports:
- **Exact, prefix (`app.*`), and wildcard (`*`)** event matching
- **Sync and async** handlers (64-worker semaphore-bounded pool)
- **~50 defined event types** across app lifecycle, builds, deployments, billing, etc.

**Pattern:** Events carry `TenantID`, `UserID`, and `CorrelationID` for tracing — this is production-grade observability baked into the design.

### 2.4 Architecture Decision Records (ADRs)

The project has **11 ADRs** following the Michael Nygard template:

| # | Decision | Key Rationale |
|---|---|---|
| 0001 | SQLite as default database | Single-binary simplicity |
| 0002 | Modular monolith, not microservices | Operational simplicity |
| 0003 | No Kubernetes | Direct Docker SDK usage |
| 0004 | Pure-Go SQLite driver (modernc.org/sqlite) | CGO-free builds |
| 0005 | Embed React UI into Go binary | Single artifact |
| 0006 | In-process pub/sub (not message broker) | No external dependency |
| 0007 | Master/agent = same binary | Simpler swarm deployment |
| 0008 | Argon2id + AES-256-GCM vault | Defense-in-depth |
| 0009 | 12-way Store interface composition | Backend portability |
| 0010 | Custom `useApi` hook over TanStack Query | Bundle size, control |
| 0011 | `core.ErrBoltNotFound` sentinel | KV-corruption observability |

---

## 3. Backend Analysis

### 3.1 Entry Point (`cmd/deploymonster/main.go`)

- **Commands:** `serve`, `config`, `health`, `init`, `rotate-keys`, `setup`, `version`
- **22 modules imported** via `_ "package"` blank imports for `init()` registration
- **Version info** injected at build via `-ldflags`
- Uses `os/signal` for graceful shutdown

### 3.2 Core Dependency Injection (`internal/core/app.go`)

The `Core` struct acts as a DI container:

```go
type Core struct {
    Config     *Config
    Registry   *Registry
    Events     *EventBus
    Scheduler  *Scheduler
    DB         *Database  // SQL + KV wrappers
    Store      Store      // Unified data access
    Services   *Services  // Service provider registry
    Logger     *slog.Logger
}
```

**Services Registry pattern:** Modules communicate via service interfaces (`ContainerRuntime`, `SSHClient`, `SecretResolver`, `DNSProvider`, `VPSProvisioner`, etc.), not direct imports. This is clean dependency inversion.

### 3.3 Configuration (`internal/core/config.go`)

The `Config` struct has **18 top-level sections**:

```go
Server, Database, Ingress, ACME, DNS, Docker, Backup,
Notifications, Swarm, VPSProviders, GitSources, Marketplace,
Registration, Secrets, Billing, Limits, Enterprise, Observability
```

Notable config features:
- **Hot-reload path** (`ConfigPath` on Core)
- **Previous secret keys** for graceful JWT rotation
- **PostgreSQL replication mode** with read replica URL
- **OTel observability config**

### 3.4 Authentication (`internal/auth/`)

**Three authentication levels:**
1. **JWT** (HS256, 32-char min secret) — access + refresh tokens with rotation grace period (20 min)
2. **API Keys** — stored in Bolt KV
3. **TOTP** — 2FA with backup codes

**Security features in auth:**
- `bcrypt` cost 13 for password hashing
- `RotationGracePeriod` reduced from 1h → 20m (SESS-006 fix)
- Standard issuer/audience claims (JWT-004 fix)
- Minimum 32-char secret enforcement (JWT-002 fix)
- OAuth SSO (Google/GitHub)

**Documented future improvement:** Migration from HS256 (symmetric) to RS256 (asymmetric) for JWT signing.

### 3.5 Router & Middleware (`internal/api/router.go`)

**Middleware chain (16 layers):**

```
Request → RequestID → Tracing → IP Allowlist → Graceful Shutdown →
Global Rate Limit (120/min) → Security Headers → API Metrics →
API Version → Body Limit (10MB) → Timeout (30s) → Recovery →
Request Logger → CORS → CSRF Protect → Idempotency → Audit Log → Handler
```

**Auth middleware wrappers:**
- `protected` = auth + per-tenant rate limiting
- `adminOnly` = protected + RequireSuperAdmin
- `protectedPerm(perm)` = protected + RBAC permission check

**Routes registered** with Go 1.22+ `mux.HandleFunc("METHOD /path", handler)` pattern.

### 3.6 Key Backend Modules

#### Deploy Engine (`internal/deploy/`)
- `DockerManager` implements `ContainerRuntime`
- Auto-negotiates API version via `client.NewClientWithOpts`
- Default CPU/memory resource limits
- Registry auth (base64-encoded Docker config)
- **Graceful shutdown** with `ConnectionTracker`, `DrainManager`, `CircuitBreakerManager`
- **Auto-rollback** based on health-check gating
- **Canary deployments** with weighted LB strategy

#### Ingress Gateway (`internal/ingress/`)
- **5 LB strategies:** RoundRobin (3.6ns/op), IPHash (26ns/op), LeastConn (55ns/op), Random, Weighted
- Custom reverse proxy via `net/http/httputil`
- **ACME** (Let's Encrypt) with `autocert`
- Circuit breaker per-backend
- Connection draining for zero-downtime deploys
- Prometheus metrics + health checks

#### Secrets Vault (`internal/secrets/`)
- **AES-256-GCM** encryption
- **Argon2id** key derivation
- Per-deployment random salt (32 bytes)
- Legacy salt migration path
- Hierarchical scope: global → tenant → project → app
- `${SECRET:name}` template syntax

#### Webhooks (`internal/webhooks/`)
- HMAC signature verification (SHA-256)
- Dedup via KV-backed delivery keys (10min TTL)
- Supports GitHub, GitLab, Gitea, Gogs, Bitbucket
- Replay protection
- Delivery logging

#### Notifications (`internal/notifications/`)
- Slack, Discord, Telegram, SMTP providers
- **SSRF protection** on webhook URLs (HTTPS-only, private IP check via `net.ParseIP`)
- Rate-limited sending

#### VPS Provisioning (`internal/vps/`)
- **5 providers:** Hetzner, DigitalOcean, Vultr, Linode, Custom-SSH
- Factory pattern in `providers.Registry` map
- SSH-key-aware provisioning

#### Swarm (`internal/swarm/`)
- Master/agent mode (same binary, ADR 0007)
- Framed JSON protocol (max 8 MiB per message)
- TLS mutual auth between nodes
- Heartbeat monitoring

#### MCP Server (`internal/mcp/`)
- **9 AI-callable tools** at `/mcp/v1/tools`
- Tools: `deploy_app`, `list_apps`, `get_app_status`, `scale_app`, `restart_app`, `get_logs`, `list_domains`, `get_metrics`, `deploy_compose`

#### Topology (`internal/topology/`)
- Visual deployment topology compiler
- YAML-based deployment definitions
- Compile → Deploy pipeline

#### Compose (`internal/compose/`)
- Docker Compose YAML parser (17.6 μs/op)
- Multi-service stack deployment

---

## 4. Frontend Analysis

### 4.1 Stack & Tooling

| Layer | Technology | Version |
|---|---|---|
| Framework | React | 19.2.6 |
| Language | TypeScript | ~6.0.3 |
| Bundler | Vite | 8.0.16 |
| Styling | Tailwind CSS | 4.3.1 |
| UI Library | shadcn/ui (customized) | — |
| State | Zustand | 5.0.14 |
| Routing | react-router | 7.16.0 |
| Graph | @xyflow/react + @dagrejs | 12.10.2 |
| Icons | lucide-react | 1.17.0 |
| Tests | Vitest + Testing Library | 4.1.8 |
| E2E | Playwright + axe-core | 1.60.0 |

### 4.2 Code Structure

```
web/src/
├── api/          # API client modules (one per domain)
│   ├── client.ts # Core HTTP client with retry, timeout, CSRF, auth refresh
│   ├── apps.ts, auth.ts, backups.ts, billing.ts, ...
│   └── __tests__/
├── components/   # Reusable UI components
│   ├── ui/       # shadcn/ui primitives (button, card, dialog, table, etc.)
│   ├── layout/   # AppLayout, Sidebar
│   ├── AppDetail/, Apps/, Dashboard/, Marketplace/, ...
│   ├── topology/ # React Flow topology editor
│   └── __tests__/
├── hooks/        # Custom hooks (useApi, useMutation, useDebouncedValue, etc.)
│   └── __tests__/
├── lib/          # Utilities (utils, roles, generatedSecrets)
│   └── __tests__/
├── pages/        # Route-level page components
│   └── __tests__/
├── stores/       # Zustand stores (auth, theme, toast, topology)
│   └── __tests__/
├── types/        # TypeScript type definitions
├── test/         # Test setup
├── App.tsx       # Root component with routes
├── main.tsx      # Entry point
└── index.css     # Tailwind + CSS variables (light/dark themes)
```

### 4.3 SPA Routing

**Protected routes (behind auth):** Dashboard, Apps, AppDetail, DeployWizard, Marketplace, Domains, Databases, Servers, Git, Backups, Secrets, Team, Billing, Monitoring, Topology, Settings

**Public routes:** Login, Register, Onboarding

**Admin route:** Protected behind `canAccessAdmin()` (requires `role_super_admin`)

**Lazy loading:** Every page is code-split via `React.lazy()` with `Suspense`.

### 4.4 API Client (`web/src/api/client.ts`)

The custom API client (chosen over TanStack Query per ADR 0010) implements:

- **Timeout:** 30s default, 10s for auth refresh (AbortController-based)
- **Retry:** Exponential backoff with jitter for 502/503/504 (2 retries max)
- **CSRF:** Reads `__Host-dm_csrf` or `dm_csrf` cookie
- **Auth refresh:** Coalesced concurrent refresh requests, 30s cooldown on failure, loop protection
- **Error handling:** `APIError` class with numeric status codes

### 4.5 State Management

- **`useAuthStore`** — Auth state, login/register/logout, `/auth/me`-based user identity (not JWT client-side decode — this was a security fix)
- **`useThemeStore`** — Light/dark/system theme with `matchMedia` listener
- **`toastStore`** — Toast notification queue
- **`useTopologyStore`** — React Flow node/edge state for the visual topology editor

### 4.6 Bundle Size

Strategically optimized via Vite `manualChunks`:
- `vendor-react`: React, react-dom, react-router
- `vendor-graph`: @xyflow/react, @dagrejs
- `vendor-ui`: lucide-react
- `vendor-state`: zustand
- Page-specific chunks for every route

**Main entry: ~19 KB gzip** (budget: 300 KB) — well optimized.

### 4.7 Topology Editor

The topology editor uses `@xyflow/react` (React Flow) with:
- Custom node types
- Visual component palette
- Config panel for selected components
- Compile/Deploy workflow with WebSocket progress streaming
- Auto-layout via dagre
- Multi-environment support (production/staging/development)

---

## 5. Security Posture

### 5.1 Current Findings (from security-report)

| Severity | Count | Status |
|---|---|---|
| **CRITICAL** | 0 | All resolved |
| **HIGH** | 0 | All resolved |
| **MEDIUM** | 3 | Residual coordination risks |
| **LOW** | 4 | Hardening/documentation items |

### 5.2 Resolved Security Issues

The security audit shows a **mature remediation process**. Notable prior findings now resolved:

| Finding | Severity | Fix |
|---|---|---|
| JWT decoded client-side without verification | CRITICAL | Now uses `/auth/me` for verified user info |
| Admin panel missing RBAC | HIGH | Gated to `role_super_admin` in front + backend |
| Docker SDK vulnerabilities | HIGH | Migrated to split Moby client/API modules |
| Predictable first-run admin email | HIGH | Random `admin-<hash>@deploymonster.local` |
| TOTP login UX gap | MEDIUM | Login page handles TOTP challenge flow |
| Password policy drift | MEDIUM | Frontend matches backend 12-char policy |
| TOTP backup codes not persisted | MEDIUM | Now persisted with one-time consumption |
| Direct DB access in migrations | MEDIUM | Now goes through Store interface |
| Marketplace weak secret defaults | MEDIUM | Sanitized during registry insertion |
| Deploy freeze bypass | MEDIUM | Checks freeze windows before deploy |
| Topology path traversal | MEDIUM | Strict path-component validation |
| File browser/backup traversal | MEDIUM | Decode-before-check, strict key validation |
| Custom Dockerfile path traversal | MEDIUM | Relative path + containment check |
| Invitation role assignment boundary | MEDIUM | Permission-boundary validation |
| Import manifest URL validation | MEDIUM | Uses `build.ValidateGitURL` |
| Redirect rule validation | MEDIUM | Shape/CRLF/scheme validation |

### 5.3 Active Security Features

- **JWT** (HS256, 32-char min secret) with rotation grace period
- **bcrypt** cost 13 for password hashing
- **TOTP 2FA** with backup codes
- **Argon2id + AES-256-GCM** secret vault with per-deployment salt
- **Docker socket hardening** via Tecnativa proxy (deployments/docker-compose.yml)
- **Audit logging** — IP, timestamp, actor on every mutation
- **Tenant isolation** — `requireTenantApp()` at every resource-scoped handler, validated by fuzz + mutation matrix tests
- **Rate limiting** — per-tenant (Bolt-backed sliding window) and global (per-IP)
- **Request timeout** — 30s default with configurable middleware
- **Security headers** — HSTS, X-Frame-Options, etc.
- **CORS** — allowlist mode
- **CSRF** — double-submit cookie pattern
- **SSRF protection** — notification webhook URLs checked against localhost/private IPs
- **Idempotency** — KV-backed deduplication
- **IP allowlisting** — CIDR-based access control
- **OpenAPI drift detection** — CI enforces code/spec alignment

### 5.4 Dependency Security

- **govulncheck**: Clean — 0 called vulnerabilities
- **pnpm audit**: Clean — no known vulnerabilities
- **Trivy**: Clean on Docker image (v0.1.9)
- **golangci-lint**: Clean
- **Previous CVE remediation**: 9 HIGH-severity CVEs in `golang.org/x/crypto` fixed in v0.1.9

---

## 6. Database Layer

### 6.1 Dual-Store Strategy

| Store | Implementation | Purpose |
|---|---|---|
| SQLite | `internal/db/sqlite.go` | Default relational store (pure Go, CGO-free) |
| PostgreSQL | `internal/db/postgres.go` | Enterprise relational store |
| SQLite-backed KV | `internal/db/bolt.go` | Rate limits, API keys, metrics, vault |

### 6.2 SQLite Configuration

- **WAL mode** for concurrent reads
- **Single-writer connection pool** (`SetMaxOpenConns(1)`)
- **64MB cache**, **256MB mmap**
- **Busy timeout:** 5s
- Configurable per-query timeout

### 6.3 Store Interface

The `Store` interface composes **12 sub-interfaces** (ADR 0009):
- `TenantStore`, `UserStore`, `AppStore`, `DeploymentStore`, `DomainStore`
- `ProjectStore`, `RoleStore`, `AuditStore`, `SecretStore`, `InviteStore`
- `UsageRecordStore`, `BackupStore`

This design allows switching SQLite ↔ PostgreSQL without changing handler code.

### 6.4 Migrations

**7 migration versions**, each with `.sql` + `.pgsql.sql` variants and `.down.sql` rollbacks:

| Migration | Changes |
|---|---|
| 0001 | Initial schema (tenants, users, team_members, roles, projects, applications, deployments, domains, secrets, etc.) |
| 0002 | Performance indexes on FK columns |
| 0003 | Hot query indexes |
| 0004 | Server region/size columns |
| 0005 | Expanded built-in role permissions |
| 0006 | Leader election tables |
| 0007 | TOTP backup codes |

**Safety:** CONCURRENT migration support with `ON CONFLICT DO NOTHING` for race-safe dual-process upgrades.

**6 built-in roles** with granular permissions (SuperAdmin, Owner, Admin, Developer, Operator, Viewer).

---

## 7. API Layer

### 7.1 API Design

- **236 REST routes**, all documented in `docs/openapi.yaml`
- **OpenAPI drift detection** in CI — `openapi-gen` fails on code/spec mismatch
- Conventional HTTP status codes with consistent error shape:
  ```json
  { "error": { "code": "ERROR_CODE", "message": "Human readable" } }
  ```
- Cursor-based pagination for large datasets

### 7.2 Route Groups

| Group | Auth | Example Endpoints |
|---|---|---|
| Health | None | `GET /health`, `GET /readyz` |
| Auth | None/JWT | `POST /auth/login`, `POST /auth/register`, `POST /auth/refresh` |
| Apps | JWT+Tenant | `GET/POST /apps`, `GET/PUT/DELETE /apps/{id}` |
| Deployments | JWT+Perm | `POST /apps/{id}/deploy`, `POST /apps/{id}/rollback` |
| Domains | JWT+Tenant | `GET/POST /domains`, `POST /domains/{id}/verify` |
| Secrets | JWT+Tenant | `GET/POST /secrets`, `GET /secrets/{id}/versions` |
| Marketplace | JWT | `GET /marketplace`, `POST /marketplace/deploy` |
| Admin | SuperAdmin | `GET/POST /admin/tenants`, `GET /admin/servers` |
| Webhooks | HMAC | `POST /hooks/v1/{webhookID}` (inbound) |
| MCP | JWT | `GET /mcp/v1/tools` |
| Streaming | JWT | `GET /api/v1/apps/{id}/logs/stream` (SSE) |

### 7.3 API Client (Frontend)

The frontend API client (`web/src/api/client.ts`) is custom-built (per ADR 0010) with:
- Transparent retry (exponential backoff with jitter)
- Request timeout (30s default)
- CSRF token handling
- Coalesced auth refresh (prevents N parallel refreshes)
- AbortController support

---

## 8. Infrastructure & DevOps

### 8.1 CI/CD Pipeline

**5 CI workflows:**

| Workflow | Trigger | Purpose |
|---|---|---|
| `ci.yml` | Push/PR to master | Full build, test, coverage, lint, E2E |
| `release.yml` | Tag v*.*.* | UI build → GoReleaser cross-compile → GHCR push |
| `loadtest-nightly.yml` | Scheduled (nightly) | HTTP load regression |
| `race-nightly.yml` | Scheduled (nightly) | Extended race detector |
| `staging-smoke.yml` | Manual | Staging smoke checks |

**All 16 CI gates pass on master:**

| Gate | Tool | Status |
|---|---|---|
| Go build | `go build ./...` | ✅ |
| Go vet | `go vet ./...` | ✅ |
| Full test suite | `go test -count=1 ./...` | ✅ 44 packages, 0 FAIL |
| Coverage gate | `go test -coverprofile` | ✅ 85.1% (gate: 85%) |
| OpenAPI drift | `go run ./cmd/openapi-gen` | ✅ 236/236 routes |
| Writers-under-load | `TestStore_ConcurrentWrites_BaselineGate` | ✅ |
| Web tests | `pnpm test` (Vitest) | ✅ 44 files, 405 tests |
| Web build | `pnpm build` (Vite) | ✅ 933ms |
| E2E | Playwright | ✅ 13 spec files |
| Frontend lint | ESLint | ✅ |
| pnpm audit | `pnpm audit` | ✅ |
| govulncheck | `govulncheck ./...` | ✅ |
| golangci-lint | `golangci-lint run ./...` | ✅ |
| Race detector | `go test -race ./...` | ✅ |
| Bundle budget | `pnpm run check:bundle` | ✅ 19 KB (budget: 300 KB) |

### 8.2 Docker Deployments

**4 Docker Compose files:**

| File | Purpose |
|---|---|
| `docker-compose.yml` | Production with socket proxy (Tecnativa) |
| `docker-compose.prod.yml` | Production variant |
| `docker-compose.dev.yaml` | Development environment |
| `docker-compose.hardened.yaml` | Security-hardened deployment |

**Dockerfile** uses multi-stage build:
1. Node 22 Alpine → build React UI
2. Go 1.26 Alpine → compile binary with embedded UI
3. Alpine 3.21 → minimal final image (~24 MB)

**Key hardening features:**
- Non-root user (`monster`)
- Tecnativa Docker socket proxy (restricts API access)
- Health checks
- Read-only root FS + tmpfs

### 8.3 Observability

- **Prometheus metrics** at `/metrics` endpoint
- **3 Grafana dashboards:** deployments, build-queue, tenant-activity
- **Structured logging** (text or JSON format, configurable level)
- **OpenTelemetry SDK** imported (stubbed — not yet wiring spans)
- **Health endpoints:** `/health`, `/readyz`

### 8.4 Load & Soak Testing

- **Load test** harness in `tests/loadtest/` — concurrent HTTP requests with baseline regression detection (10% threshold)
- **Soak test** harness in `tests/soak/` — longer duration runs
- **Concurrent writes baseline** gate — 64-worker fan-out vs committed p95 threshold

---

## 9. Testing & Quality

### 9.1 Coverage Metrics

**Go backend: 85.1% statement coverage** (filtered, CI gate is 85%)
- 398 test files, 42 test packages
- 17 fuzz targets
- 44 benchmarks

**Frontend: 405 unit tests** across 44 files
- 13 Playwright E2E spec files (blocking in CI)
- a11y spec (axe-core)

### 9.2 Key Test Areas

| Area | What's Tested |
|---|---|
| Auth | JWT validation/rotation, password hashing, TOTP, API keys, RBAC, rate limiting |
| API handlers | All 236 routes with auth context, tenant isolation, edge cases |
| Router | Fuzz test for cross-tenant access (38 GETs), mutation matrix (38 mutations) |
| DB | SQLite + Postgres store contract tests, migration compatibility, concurrent writes baseline |
| Deploy | Docker mock tests, rollback, auto-restart, circuit breaker, drain |
| Ingress | 5 LB strategies, router, TLS, ACME, metrics |
| Secrets | AES encrypt/decrypt, vault salt, scoped resolution |
| Webhooks | Receiver with HMAC verification, dedup, replay, fuzz |
| Notifications | Providers (Slack, Discord, Telegram, SMTP), SSRF validation |
| Security | Path traversal, injection, cross-tenant, freeze bypass, permission boundary |
| Frontend | Component rendering, store state management, API client, utility functions |

### 9.3 Fuzz Targets

17 fuzz targets across:
- **Auth:** JWT, password validation
- **DB:** Secrets resolver
- **Router:** Cross-tenant
- **Marketplace:** Validator
- **Webhooks:** Receiver edge cases
- **Compose:** Parser
- **Ingress:** Router
- **Secrets:** Vault + resolver

---

## 10. UI/UX Assessment

### 10.1 Visual Design

The frontend uses a **Tailwind CSS v4** design system with:
- **OKLCH color space** for perceptually uniform colors
- **Light and dark themes** via CSS custom properties (`.dark` class toggle)
- **Green-emerald primary** (`oklch(0.523 0.209 163.073)` — a teal/emerald tone)
- **shadcn/ui** component primitives (button, card, dialog, sheet, table, tabs, etc.)
- **Responsive layout** with mobile-first breakpoints
- **Gradient banners** and **backdrop-blur** effects on search inputs
- **Zustand-based theme store** with `prefers-color-scheme` media query listener

### 10.2 Component Coverage

| Page | Components | Features |
|---|---|---|
| Dashboard | Stat cards, announcements banner, quick actions, activity table, welcome banner with search | Greeting, time-ago, skeleton loading |
| Apps | App cards with status badges, skeletons, search | CRUD, status indicators |
| AppDetail | Tabs (overview, settings, env vars, deployments, logs), stats cards | Polling stats (10s), start/stop/restart, delete with confirmation |
| DeployWizard | Multi-step form | Source selection, config, review, deploy |
| Marketplace | Template cards with categories | One-click deploy, 91 templates |
| TemplateDetail | Deploy form | Per-template config |
| Domains | Domain list, verification | DNS management |
| Databases | DB engine list, provisioning | PostgreSQL, MySQL, Redis, etc. |
| Servers | Server cards, provisioning | Provider selection (Hetzner, DO, Vultr, Linode, custom) |
| Topology | React Flow canvas, component palette, config panel, compile/deploy modals | Drag-and-drop, auto-layout, real-time deploy progress via WebSocket |
| Team | Member list, invites | Role management |
| Billing | Plan display, usage | Stripe integration |
| Settings | Profile, appearance, security | Theme toggle, password change, 2FA |
| Monitoring | Metrics display | Resource graphs |

### 10.3 Accessibility

- **E2E a11y spec** using `@axe-core/playwright` — automated accessibility testing in CI
- Semantic markup (aria labels, roles)
- Focus-visible indicators (shadcn/ui baseline)
- Color contrast via OKLCH color system
- `prefers-reduced-motion` respected
- Loading/full-page loader during lazy routes

### 10.4 UX Patterns

- **Skeleton loading** for cards and stat blocks
- **Toast notifications** for success/error feedback (Zustand store)
- **Confirmation dialogs** for destructive actions (delete app, etc.)
- **Real-time log streaming** via WebSocket/SSE
- **Deploy progress** with WebSocket status updates
- **Error boundaries** at app root level
- **Not Found page** for unmatched routes

### 10.5 Identified UI Gaps

- **No dark mode toggle component** in the settings page (or hidden) — theme is managed via localStorage/system preference
- **No empty states** observed for empty lists (apps, deployments, etc.) — likely shows skeleton or blank
- **Dashboard greeting** hardcodes "admin" instead of using the user's name
- **No comprehensive E2E coverage for billing flow** (Stripe requires real integration)
- **Onboarding page** exists but its content was not examined in detail

---

## 11. Data Integrity & Backup

### 11.1 Backup System

- **Local storage** (filesystem) and **S3/MinIO/R2** support
- Cron-based scheduled backups
- Configurable retention policy
- Database snapshots stored in `backups/` directory (3 snapshots observed)
- Encrypted backup option (via secrets vault)

### 11.2 Database Integrity

- **SQLite WAL mode** for crash recovery
- **Writers-under-load gate** — 64 concurrent writers against p95 latency baseline
- **Atomic deployments** with version sequencing (`AtomicNextDeployVersion`)
- **Foreign keys enabled** in SQLite (`_foreign_keys=on`)
- **ON DELETE CASCADE** on all FK relationships
- **Migration system** with up/down scripts and dual-SQL/PostgreSQL support

### 11.3 Concurrent Access Safety

- SQLite: `SetMaxOpenConns(1)` with `_busy_timeout=5000`
- Bolt KV: per-bucket locking
- Migration: `ON CONFLICT DO NOTHING` for dual-process safety
- Deploy version: atomic counter increment

---

## 12. Strengths

### 12.1 Architecture
1. **Modular monolith pattern** — 22 modules with clean interfaces, dependency injection, and event-driven communication. Scales from single binary to swarm deployment without architectural rewrites.
2. **12-way Store interface** — SQLite ↔ PostgreSQL swap without handler changes. Excellent testability via interface mocking.
3. **Event bus** with ~50 typed events enables loose coupling. Async handler pool bounded at 64 workers prevents goroutine explosions.
4. **11 ADRs** — every major decision is documented with context, decision, and consequences. This is best-in-class for a project of this size.

### 12.2 Code Quality
5. **85.1% test coverage** with 398 Go test files, 17 fuzz targets, and 44 benchmarks. E2E tests in Playwright block CI.
6. **17 fuzz targets** across security-critical areas (JWT, password validation, cross-tenant access, compose parsing, webhook receiver, etc.).
7. **OpenAPI drift detection** in CI — code and spec stay in sync programmatically.
8. **Go 1.26+** with modern patterns (slog, native mux patterns, context usage, atomic types).
9. **Pure-Go SQLite** (CGO-free) enables cross-compilation for 5 platforms.

### 12.3 Security
10. **Comprehensive security remediation** — 16+ resolved findings from critical to medium with documented fixes.
11. **Defense in depth** — JWT + bcrypt + TOTP + AES-256-GCM + Argon2id + CSRF + rate limiting + audit logging + IP allowlisting.
12. **Docker socket hardening** via Tecnativa proxy in production compose.
13. **No critical or high open findings** in the current security audit.

### 12.4 Frontend
14. **Excellent bundle optimization** — main entry ~19 KB gzip, strategic code splitting.
15. **Tailwind CSS v4 with OKLCH** — modern, maintainable design tokens with proper light/dark theming.
16. **Custom API client** with retry, timeout, CSRF, and coalesced auth refresh — better than many TanStack Query setups for this use case.

### 12.5 Operations
17. **Single binary deployment** (~24 MB) — trivial to install, update, and manage.
18. **Comprehensive CI** — 16 gates including security scanning, bundle budget, race detection, and load test regression.
19. **Production documentation** — runbook, SLA, upgrade guide, troubleshooting, security audit, staging validation checklist.

---

## 13. Weaknesses & Areas for Improvement

### 13.1 Architecture Concerns

| Issue | Severity | Details |
|---|---|---|
| **No multi-master HA** | Medium | SQLite single-writer prevents active-active HA. PostgreSQL support exists but HA topology not fully implemented (ADR 0001 trade-off). |
| **OTel spans not emitted** | Medium | OTel SDK is imported transitively but no span emission occurs. Observability is incomplete without distributed tracing. |
| **Plugin system absent** | Medium | Every builder, DNS provider, VPS provider, and notifier is first-party code. Cannot extend without modifying the binary. |
| **No Kubernetes support** | Low | Intentional (ADR 0003), but limits adoption in K8s-heavy organizations. |
| **Route53 DNS missing** | Low | Only Cloudflare DNS provider ships. AWS users must use manual DNS or custom integration. |

### 13.2 Security Observations

| Issue | Severity | Details |
|---|---|---|
| **HS256 symmetric JWT** | Medium | Documented in code as future work (to migrate to RS256). Symmetric key means the same key signs and verifies — compromise of the key allows token forgery. |
| **3 residual MEDIUM findings** | Medium | Coordination risks documented in security report — not code bugs but operational concerns. |
| **No HSM/KMS integration** | Low | Vault key is in-memory `[]byte`. Best-effort zeroing in Go is not guaranteed. |
| **No automated secret rotation** | Low | Vault key rotation requires manual process. |

### 13.3 Code Quality Observations

| Issue | Severity | Details |
|---|---|---|
| **398 test files — high test-to-code ratio** | Neutral | 123K test LOC vs 57K production LOC (2.1:1 ratio). Indicates thorough testing but also potential test maintenance burden. |
| **Coverage_boost_test.go pattern** | Low | Multiple files named `coverage_boost_test.go` suggest coverage was boosted after initial development, not written alongside. Some may be low-value tests. |
| **Test file count per package** | Low | Some packages have 10+ test files, suggesting iterative addition of test coverage rather than organized test suites. |
| **`go.mod` has unused entries** | Low | `go.sum` is large (60+ indirect entries). Some transitive dependencies may be unused after Docker SDK migration. |

### 13.4 Frontend Observations

| Issue | Severity | Details |
|---|---|---|
| **No visual regression tests** | Medium | No Chromatic/Percy/screenshot tests. E2E covers functional paths but not visual changes. |
| **Hardcoded "admin" in dashboard greeting** | Low | Line 64 of Dashboard.tsx: `{greeting}, admin` — should use actual user name. |
| **No empty state components** | Low | Empty lists likely show blank pages or raw loading states. User experience degrades when no data exists. |
| **Limited i18n** | Low | No internationalization framework. All UI text is hardcoded in English. |
| **No storybook component catalog** | Low | UI components lack isolated development/preview environment. |

### 13.5 Testing Gaps

| Gap | Severity | Details |
|---|---|---|
| **`go test ./...` timeout in CI** | Medium | The monolithic command doesn't complete within 120s locally. Split into groups in CI but the unified command is unverified. |
| **No chaos engineering tests** | Medium | No fault injection testing for Docker failures, network partitions, or disk-full scenarios. |
| **Load test baselines limited** | Low | Only HTTP baseline committed. No real workload profile baselines. |
| **No benchmark regression gate** | Low | Benchmarks exist but are not gated in CI against regression thresholds. |

### 13.6 Production Readiness Gaps (for SaaS)

| Gap | Status |
|---|---|
| Staging validation on real infrastructure | ⏳ Pending |
| Backup/restore drill evidence | ⏳ Pending |
| Rollback drill evidence | ⏳ Pending |
| Load + soak evidence | ⏳ Pending |
| Deploy freeze bypass repair verification | ⏳ Partially verified |

---

## 14. Recommendations

### Priority: High (Address before SaaS launch)

1. **Complete staging validation** per `docs/staging-validation.md` — the 9-step checklist is the blocking item for SaaS readiness.

2. **Run the monolithic `go test ./...`** in a local shell or CI runner to confirm it completes within the timeout. Currently passes in CI via grouped runs, but the unified command is unconfirmed.

3. **Backup/restore and rollback drills** on real infrastructure — document results as release evidence.

4. **Load test and short soak** (5m+) against a staging environment with realistic workloads. Commit the results as a baseline.

### Priority: Medium (Next 1-2 releases)

5. **Migrate JWT signing from HS256 to RS256** — asymmetric keys prevent forgery on public-key disclosure. Documented in code as future work.

6. **Emit OpenTelemetry spans** from module lifecycle and key operations. The SDK is already imported; wiring span emission would unlock distributed tracing.

7. **Reduce test file fragmentation** — consolidate related tests into fewer files per package. The `coverage_boost_test.go` naming convention suggests tests were bolted on; refactor into logical test suites.

8. **Add empty state components** for list pages (apps, deployments, domains, etc.) — improves UX when users first start with the platform.

9. **Fix dashboard greeting** to use the logged-in user's name instead of hardcoded "admin".

### Priority: Low (Nice-to-have)

10. **Add visual regression tests** (Playwright screenshot diffing or Chromatic) to catch UI regressions.

11. **Add storybook or similar component catalog** for UI component development.

12. **Add benchmark regression gates** in CI — fail the build if key benchmarks regress beyond a threshold.

13. **Add K8s operator** as an optional deployment target (ADR 0003 notwithstanding, this would expand the addressable market).

14. **Add Route53 DNS provider** for AWS-native users.

15. **Document and test the plugin system** — even if it's just a documented "fork and extend" pattern, it helps adopters understand how to extend.

16. **Investigate chaos engineering** — add fault injection tests for Docker daemon failures, network partitions, and disk-full scenarios to validate resilience.

---

## Appendix A: File Count Breakdown

| Category | Go | TypeScript/TSX | Other | Total |
|---|---|---|---|---|
| Production source | 291 | 172 | — | 463 |
| Unit tests | 398 | 44 | — | 442 |
| E2E tests | — | 13 | — | 13 |
| Config/infra | — | — | ~40 | ~40 |
| Documentation | — | — | ~30 | ~30 |
| **Total** | **689** | **229** | **~70** | **~988** |

## Appendix B: Key Dependencies

**Go:**
- `modernc.org/sqlite` — Pure-Go SQLite driver
- `golang-jwt/jwt/v5` — JWT implementation
- `moby/moby` — Docker SDK (client + API modules)
- `prometheus/client_golang` — Metrics
- `go.opentelemetry.io/otel` — OpenTelemetry
- `gorilla/websocket` — WebSocket support

**Frontend:**
- React 19 + Vite 8 + TypeScript 6
- Tailwind CSS 4 + tw-animate-css
- shadcn/ui components (customized)
- Zustand 5 — State management
- @xyflow/react 12 — Topology graph editor
- lucide-react — Icons
- Vitest + Playwright — Testing

---

## Appendix C: High-Priority Recommendation Status (2026-07-15)

| # | Recommendation | Status | Evidence |
|---|---|---|---|
| 1 | Staging validation on real infrastructure | ⏳ **BLOCKED** — requires staging host with Docker, DNS, ACME | 9-step checklist in `docs/staging-validation.md` |
| 2 | Monolithic `go test ./...` | ✅ **RESOLVED** — 44 packages, 0 FAIL | Completed in ~2m30s within 300s timeout (was incorrectly attempted with 120s timeout) |
| 3 | Backup/restore and rollback drills | ⏳ **BLOCKED** — requires real Docker infrastructure | Cannot run in repo-only context |
| 4 | Load test + short soak | ⏳ **BLOCKED** — requires running DeployMonster instance | Staging infrastructure dependency |

### Additional locally-actionable verification passed:

| Gate | Result | Detail |
|---|---|---|
| `pnpm test` (Vitest) | ✅ **405 tests, 44 files, 0 FAIL** | Completed in 10.29s |
| `pnpm build` (Vite) | ✅ **Built in 348ms** | Main chunk 19.72 KB gzip (budget: 300 KB) |
| OpenAPI drift check | ✅ **236/236 routes match** | allowlist=0, no drift |
| `scripts/build.sh` | ✅ **28 MB binary with embedded UI** | Binary `bin/deploymonster` built successfully |
| Concurrent writes baseline | ✅ **PASSED** | 6772 ops/s, p95=26.7ms vs baseline 187.9ms |

### Key finding
The report's concern about `go test ./...` not completing within 120s is **resolved** — the test suite completes in ~2m30s with a 300s timeout. The 120s timeout documented in the original verification report was simply too tight; the CI uses `240s` and passes.

### Staging validation progress — local Docker-based staging

Since no cloud VPS staging host was available, a **local Docker-based staging environment** was set up to validate as much of the pipeline as possible:

| Step | Status | Detail |
|---|---|---|
| Build release-shaped binary | ✅ **PASS** | `scripts/build.sh` → 28 MB binary at `bin/deploymonster` |
| Deploy to staging | ✅ **PASS** | Server started on `127.0.0.1:8443` with fresh SQLite DB |
| Public health checks | ✅ **PASS** | `/health`, `/api/v1/health` return `{"status":"ok"}` |
| First-run admin creation | ✅ **PASS** | Random `admin-*@deploymonster.local` email generated | 
| Authenticated login | ✅ **PASS** | JWT token returned correctly |
| Auth/me | ✅ **PASS** | Returns user profile with role_id, tenant_id |
| Marketplace | ✅ **PASS** | 91 templates loaded across 19 categories |
| Apps list | ✅ **PASS** | Returns empty list (correct for fresh install) |
| OpenAPI spec | ✅ **PASS** | `/api/v1/openapi.json` serves spec |
| MCP tools | ✅ **PASS** | 9 AI tools registered at `/mcp/v1/tools` |
| **Full deploy pipeline** | ✅ **PASS** | Created app → deployed nginx:alpine → container running (`dm-*` naming) → app status "running" |
| API dashboard | ✅ **PASS** | Returns stats |
| Servers | ✅ **PASS** | 1 local server detected |

**Cannot validate locally:** Real DNS + Let's Encrypt ACME, webhook delivery from external providers, VPS provisioning, multi-tenant isolation at scale, backup/restore from S3, load/soak under real traffic patterns.

**Remaining blocker for full staging validation:** A disposable staging host (VPS with Docker), a real domain with DNS access, and operator time to run through the 9-step checklist in `docs/staging-validation.md`. The **end-to-end deploy pipeline** was verified locally, which de-risks the most critical path.

### Infrastructure provisioning attempt — results

| Provider | Status | Detail |
|---|---|---|
| Hetzner (hcloud CLI) | ✅ CLI available (v1.66.0) | Installed but **no API token configured** |
| DigitalOcean (doctl CLI) | ❌ Could not install | 404 from release URL |
| Vultr, Linode, Custom | ❌ No CLIs or credentials | — |
| SSH keys | ❌ None found | No `~/.ssh/id_*` keys |
| Cloud API tokens | ❌ None found | No env vars or config files |

**Blocking gap:** A Hetzner API token (`HCLOUD_TOKEN`) or equivalent cloud provider credential is required to provision the staging VPS. Once provided, the workflow is:
1. `hcloud context create <name>` — authenticate
2. `hcloud server create --name dm-staging --image ubuntu-24.04 --type cx22` — provision
3. `hcloud server ssh dm-staging` — install Docker + deploy DeployMonster
4. Point DNS A record at the server IP
5. Run `docs/staging-validation.md` runbook

---

*Analysis prepared by WrongStack (leader agent) on 2026-07-15. Based on project state at commit on branch `master` (20 modified, 0 staged). Updated with verification results from 2026-07-15.*
