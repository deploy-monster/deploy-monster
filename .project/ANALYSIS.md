# DeployMonster — Comprehensive Project Analysis

> **Audit date:** 2026-04-11
> **Auditor role:** Senior software architect / production readiness reviewer
> **Repository HEAD:** `b36fba6` — harden: ws DeployHub Shutdown + concurrent-write mu + dead-client eviction (Tier 77)
> **Scope:** Full codebase, documentation, architecture, CI/CD, tests, security posture
> **Method:** Static analysis, build/test execution, spec-vs-code gap analysis, git history review

---

## 1. Executive Summary

DeployMonster is an **ambitious, architecturally sound, well-engineered self-hosted PaaS** built as a single Go binary with an embedded React 19 UI. The project has clearly invested heavily in modularity, event-driven design, and operational hardening. Recent commit history (Tiers 73-77) shows deliberate, surgical work tightening lifecycle correctness — WaitGroup drains, `defer recover`, `stopCtx` plumbing, dead-client eviction in WebSocket hubs. That is the signature of a maintainer who reads their own code under load.

However, **the public narrative overstates the production readiness of the codebase**. The README claims "97% test coverage", "224 REST API endpoints", "25+ marketplace templates", "9 AI-callable MCP tools". Audit findings:

- Test suite **does not compile cleanly** — three untracked test files reference undefined symbols (`RequireAdmin`, `ListCerts`, `goodStorage`), blocking `go test ./...` in those packages.
- **Admin-role middleware is missing entirely.** Tests for `RequireAdmin` / `RequireSuperAdmin` / `RequireOwnerOrAbove` exist; the functions they exercise do not.
- **AWS SigV4 signing is implemented once (for DNS) but not wired into Route53**, and the S3 backup module has a placeholder comment where signing belongs. Private AWS S3 and Route53 will silently fail in production.
- **MCP declares 9 tools but the dispatcher handles 8** — `scale_app` is advertised to LLM callers and returns `"unknown tool"`.
- **Bitbucket webhooks are unsigned** — `VerifySignature` falls through to the `default: return true` branch for any provider not in the switch.
- **Email notifications are not implemented** — no SMTP provider, no interface declaration, nothing.
- **Build log output is discarded** — `deploy/pipeline.go:80` passes `io.Discard` to the builder with a comment "In production, stream to WebSocket/SSE". That comment has lived there since the pipeline landed. Users cannot see why a build failed.
- Marketplace template count is **116, not 25** (README understates) — one of the few places reality beats the marketing.

The project **is not production-ready today**, but the distance between its current state and production is **weeks, not months**, and the distance is dominated by disciplined bug-fix work, not re-architecture. The bones are good. A handful of specific, fixable gaps are presently blocking a "ship it" verdict.

**One-line verdict:** Strong architecture, excellent hardening discipline, marketing overreach, shippable after a focused 4-6 week fix-up sprint.

---

## 2. What This Project Actually Is

| Aspect | Reality |
|---|---|
| **Type** | Self-hosted PaaS. Control plane + worker agents + reverse proxy + build pipeline + marketplace, all in one binary. |
| **Runtime model** | Modular monolith. 20 modules register via `init()` + `core.RegisterModule()` in each module's `module.go`. Topologically sorted on `Dependencies()`. |
| **Deployment model** | Same binary runs as master (full stack) or agent (worker node, `--agent` flag). Bidirectional WebSocket JSON protocol. |
| **Language split** | Go 1.26.1 backend (`internal/`, `cmd/`), React 19.2.4 frontend (`web/`), embedded via `embed.FS` at `internal/api/static/`. |
| **Data plane** | SQLite (`modernc.org/sqlite`, pure Go) + BBolt KV (configs, state, metrics, API keys, webhook secrets). Access exclusively through the `core.Store` interface — 12 sub-repositories composed into one. |
| **Container runtime** | Docker SDK (`github.com/docker/docker v28.5.2`) behind a `core.ContainerRuntime` interface — swappable, testable. |
| **Ingress** | Custom `net/http/httputil` reverse proxy with five load-balancer strategies, Let's Encrypt HTTP-01, in-process routing table. No Traefik or Nginx. |
| **Event system** | In-process pub/sub (`internal/core/events.go`). Bounded async (64-slot semaphore + WaitGroup), wildcard/prefix matching, `{domain}.{action}` naming convention. |
| **Extensibility** | Factory registry (`core.Services`) for DNS, Backup, VPS, Git providers. New provider = one struct implementing the interface. |

---

## 3. Architecture Analysis

### 3.1 Module system (`internal/core/registry.go`)
The module system is the best part of this codebase. Each module implements:

```go
type Module interface {
    ID() string
    Name() string
    Version() string
    Dependencies() []string
    Init(ctx context.Context, core *Core) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Health() HealthStatus
    Routes() []Route
    Events() []EventHandler
}
```

- **Topological sort** of `Dependencies()` determines Init order; reverse order determines Stop order, with a 30-second deadline.
- **Auto-registration** via `init()`: importing `_ "github.com/deploy-monster/deploy-monster/internal/auth"` in `cmd/deploymonster/main.go` is all it takes. No manual wiring in `main.go`.
- **Clean dependency injection**: modules receive `*core.Core` in `Init()` with `Store`, `Config`, `EventBus`, `Logger`, `Services`. No globals.

This is as close to "plugin architecture without the plugin tax" as a Go monolith can get. It's also the single most important reason the audit isn't more damning — replacing a module's implementation is a local operation, not a cross-cutting rewrite.

### 3.2 Data access boundary (`internal/core/store.go`)
The `Store` interface composes 12 sub-interfaces:

```
TenantStore, UserStore, AppStore, DeploymentStore, DomainStore,
ProjectStore, RoleStore, AuditStore, SecretStore, InviteStore,
UsageRecordStore, BackupStore
```

`internal/db/sqlite.go` is the only implementation. Modules never touch `*db.SQLiteDB` directly — verified by grep. This is the correct discipline for the "SQLite default, PostgreSQL for enterprise" roadmap item to actually work. The Postgres port will be a fresh `pgStore` struct implementing the same interfaces, not a migration.

**Gap:** PostgreSQL `core.Store` implementation does not exist yet. `README.md` says "PostgreSQL ready"; code says "not started". The spec calls for PostgreSQL in Phase X; the implementation is not in the tree. The interface is ready, the implementation is not.

### 3.3 Event bus (`internal/core/events.go`)
- Synchronous handlers run in the publish goroutine under a `sync.WaitGroup`.
- Async handlers run via `SafeGo` (panic-recovered) through a 64-slot semaphore + `WaitGroup`.
- Matching: exact (`"app.created"`), prefix (`"app.*"`), wildcard (`"*"`).
- Shutdown drains outstanding async handlers before returning.

This is correct bounded concurrency. It is **not** a durable queue — if the master crashes, in-flight events die. That is fine for the event types used (logging, metrics, cache invalidation), **not** fine for the event types the spec mentions for billing reconciliation and audit durability. See §8 technical debt.

### 3.4 API layer (`internal/api/`)
Real work. Middleware chain order:

```
RequestID → GracefulShutdown → GlobalRateLimiter(120/min IP)
  → SecurityHeaders → APIMetrics → APIVersion
  → BodyLimit(10MB, 1MB webhooks) → Timeout(30s)
  → Recovery → RequestLogger → CORS
  → CSRFProtect → IdempotencyMiddleware → AuditLog
  → RequireAuth → TenantRateLimiter(100/min per tenant)
```

Strengths:
- Uses Go 1.22+ `http.ServeMux` `METHOD /path` pattern syntax — no third-party router dependency.
- Three-tier rate limiting (global IP, per-tenant, per-auth-level) is a real defense in depth.
- 240 registered routes across ~115 handlers — close to the README's "224 endpoints" claim.

**Gaps:**
- Admin-role middleware missing (see §5.1). The current layering only has `AuthNone / AuthAPIKey / AuthJWT / AuthAdmin / AuthSuperAdmin` as enum values — the enforcement lives scattered in handlers via `if claims.RoleID == "role_super_admin"` checks rather than declarative middleware. That's a latent security gap waiting for someone to forget one check.
- CSRF protection exists (verified in middleware chain + `web/src/api/client.ts:dm_csrf` cookie), but the tests for it under concurrent session refresh were not found.
- `docs/openapi.yaml` is 57KB. OpenAPI is published. Drift from actual handlers is not verified in CI. Very likely drifted.

### 3.5 Ingress gateway (`internal/ingress/`, 7,579 LOC)
Verified real, not a stub:
- `httputil.ReverseProxy` with per-app routing table, hot-reloadable via events.
- Five LB strategies present and wired: round-robin, least-connections, IP-hash, random, weighted.
- ACME HTTP-01 implementation (`acme/autocert`-style, but hand-rolled). Cert cache on disk.
- Tier 73 hardening: `stopCtx`, `wg.Wait()`, `defer recover()`. Correct.

**Gap (spec vs code):** SPECIFICATION.md calls for DNS-01 challenge support for wildcard certs. **DNS-01 is not implemented.** You cannot issue a `*.example.com` certificate with the current code. For a platform that markets "custom domains for every tenant", this is a gap that will bite on first enterprise pilot.

### 3.6 Master/Agent swarm (`internal/swarm/`, 8,360 LOC)
Tier 76 is very recent (same day) and tightens this module's shutdown semantics: `wg` drain, `defer recover`, `stopCtx`, `closed` flag to prevent double-close. The protocol is a bidirectional WebSocket with JSON message envelopes for `ping/pong/container.*/image.pull/network.create/volume.create/metrics.collect/health.check/config.update`. This is the right shape.

**Gap:** `internal/swarm/client.go:119` — agent port is **hardcoded to `8443`**. If you deploy the master on a non-default port, agents cannot connect. This is a one-line fix that embarrasses the rest of the module.

### 3.7 Frontend (`web/`, ~11,064 LOC)
React 19 + Vite 8 + TypeScript 5.9 with strict mode. Verified:
- `tsconfig.app.json` has `"strict": true`, `"noUnusedLocals": true`, `"noUnusedParameters": true`, `"noUncheckedSideEffectImports": true`, `"noFallthroughCasesInSwitch": true`.
- Zero `any` types found.
- Zero `@ts-ignore` found.
- 21 lazy-loaded pages in `web/src/pages/`.
- 44 components, 4 Zustand stores (`auth`, `theme`, `toastStore`, `topologyStore`), 6 custom hooks (`useApi`, `useMutation`, `usePaginatedApi`, `useWebSocket`, `useEventSource`, `useDebouncedValue`).
- 9 Playwright E2E test suites (`web/e2e/`).

**Contradiction with CLAUDE.md:** CLAUDE.md claims "TanStack React Query 5" is in use. It **is not**. `web/package.json` has no `@tanstack/react-query`. The codebase uses a custom `useApi` hook and `vendor-query` is just a chunk name leftover. Documentation drift, not a bug, but a reader-trust issue.

**Gaps in `web/src/api/client.ts`:**
- No per-request timeout (fetches can hang indefinitely on an unresponsive master).
- No retry with backoff on 5xx.
- `tryRefresh()` has no attempt cap — a dead refresh token can trigger an infinite refresh storm on a single failing request.

### 3.8 Docker container management (`internal/deploy/`)
Solid. The module owns `docker.go` (250+ lines wrapping the Docker SDK), `strategies/strategy.go` (recreate + rolling), `graceful.go` (241 lines SIGTERM → SIGKILL with grace), `rollback.go` (176 lines automatic on failed deploy), `autorestart.go`, `autorollback.go`.

**Gaps:**
- `pipeline.go:80` passes `io.Discard` to the builder — **build logs are thrown away**. This is the single most user-hostile bug in the tree.
- Rolling deploy strategy (`strategies/strategy.go`) does not pass a SIGTERM grace period to `runtime.Stop()` on the old replicas. Zero-downtime claim is not honored for apps that need drain time.
- Blue-green and canary strategies exist in the spec; only recreate and rolling are implemented.

---

## 4. Code Quality Analysis

### 4.1 Compilation & test run (measured at HEAD `b36fba6`)
```
go build ./...       → PASS (clean)
go vet ./...         → 3 undefined-symbol errors (test files only)
go test ./...        → FAIL (summary below)
```

Test run result across all 38 packages:
- **34 packages PASS**
- **3 packages FAIL with build errors** (`internal/api/middleware`, `internal/backup`, `internal/ingress`) — untracked test files reference symbols that do not exist.
- **1 package has a failing test** (`internal/dns/providers` → `TestRoute53_SignsRequests`) — the test asserts the Route53 request is SigV4-signed; the code never calls the signer.
- **1 package hangs in deadlock** (`internal/db` → `TestSQLite_Rollback` at `sqlite_test.go:240`) — a `QueryRow` against a rolled-back database never returns; goroutine dump shows `database/sql.(*DB).conn` blocked in `select` for the entire timeout window. This is not a slow test. It is a stuck test.

The three build failures are all in **untracked test files** (git status shows `??` markers). Someone wrote tests for implementations that were never committed:

| File | Undefined symbol | Likely meaning |
|---|---|---|
| `internal/api/middleware/admin_test.go:23` | `RequireAdmin` | Admin-role middleware was planned, never written |
| `internal/ingress/cert_monitor_test.go:21` | `cs.ListCerts` | `CertStore.ListCerts` method planned, never written |
| `internal/backup/backup_coverage_test.go:367` | `goodStorage` | Test helper removed in a refactor, test file not updated |

Production blocker implications:
- `internal/api/middleware/admin_test.go` is **the most load-bearing one** because it implies a deliberate decision to add role-gated middleware that was abandoned mid-flight. Every admin-only endpoint relies on handler-local `if` checks. One forgotten check = privilege escalation.
- `internal/db/sqlite_test.go:240` is the second-most load-bearing. The test is titled `TestSQLite_Rollback` and exercises `migrations/0001_init.down.sql`. The rollback does **not** drop the `tenants` table (the test prints the error message "expected tenants table to be dropped after rollback"), **and** the next query deadlocks, meaning the rollback also leaves a stuck connection behind. Schema rollback is not exercisable at HEAD.
- `TestRoute53_SignsRequests` is the smoking-gun evidence for the SigV4-not-wired gap in §5.4. Someone committed the test alongside `sigv4.go`. The integration was never done.

### 4.2 Test coverage (README claim: 97%)
Measured reality:
- Test file count: 194 `_test.go` files.
- Test LOC: 46,960.
- Source LOC: 27,051.
- **Test-to-source ratio: 1.74 : 1** (objectively high for Go).

The 97% **function coverage** number from `go test -cover` is not achievable cleanly because of the vet failures above. The coverage number is credible **for the packages that compile**, but the aggregate claim cannot be verified until the three broken test files are fixed. Several packages have "coverage-boost" and "tier_hardening" test files whose explicit purpose is to push coverage numbers — useful for regression but a signal that the number was optimized for rather than earned. I'd trust 85-90% real coverage on modules that matter (`core`, `auth`, `db`, `ingress`, `deploy`), lower on modules that add integration depth (`swarm`, `backup`, `vps`).

### 4.3 Linting (`golangci-lint`)
Makefile target exists (`make lint`) but not run in this audit — `golangci-lint` is not installed in-tree. CI runs it; the last green CI run is trusted.

### 4.4 Formatting
`gofmt -s` pass verified indirectly via `go build` success — `gofmt` violations don't block compilation. Not exhaustively rechecked.

### 4.5 Idiom and style
Positives observed while reading:
- Consistent `context.Context` as first argument everywhere.
- Consistent `log/slog` with `"module"` key.
- Error wrapping with `fmt.Errorf("context: %w", err)`.
- Table-driven tests with `t.Run` subtests — verified in `internal/deploy/`, `internal/core/`, `internal/auth/`.
- No global state. DI via `core.Core`.
- Interfaces defined where consumed (not in `types` packages).

Negatives observed:
- Untracked test files that don't compile (see §4.1).
- Occasional "would go here" comments in production paths (Route53, S3).
- Occasional feature-parity gaps between what the spec advertises and what ships (DNS-01, blue-green, email notifications, MCP scale_app).
- Some modules have multiple "coverage" test files (`gitsources_coverage_test.go`, `mcp_coverage_test.go`, `resource_coverage_test.go`, etc.) that suggest test files were generated or backfilled after the fact to hit a target number rather than driving design.

---

## 5. Spec vs Implementation Gap Analysis

Cross-referenced `·project/SPECIFICATION.md` (4,597 lines), `·project/IMPLEMENTATION.md`, and `·project/TASKS.md` (all 251 tasks marked `[x]`) against actual code. Results:

### 5.1 Security & Auth
| Spec says | Code status | File:line |
|---|---|---|
| RBAC with 6 built-in roles + custom roles | **Partial** — role records exist, middleware enforcement missing | `internal/api/middleware/admin_test.go` references non-existent `RequireAdmin` |
| JWT HS256 with rotation | **Real** | `internal/auth/jwt.go:1-159` |
| Password bcrypt cost 12 | **Real** | `internal/auth/password.go` |
| TOTP 2FA | **Real** but recovery codes stored plaintext | `internal/auth/totp.go` |
| OAuth SSO (Google, GitHub) | **Real** but no PKCE | `internal/auth/oauth.go:1-189` |
| API keys | **Real** — SHA-256 hashed with `dm_` prefix | `internal/auth/apikey.go:1-42` |
| Secrets vault AES-256-GCM + Argon2id | **Real** but master key in config file/env var (no HSM), hardcoded salt | `internal/secrets/vault.go:1-81` |

**Score: 6/10.** The cryptography is correct; the operational gaps are real. The biggest single risk is the missing admin middleware — every handler that should check `role_super_admin` does so locally, which is a time bomb.

### 5.2 Deploy pipeline
| Spec says | Code status |
|---|---|
| Git clone → language detect → Dockerfile gen → build → deploy | **Real** end-to-end |
| 14 language detectors (Next.js, Vite, Nuxt, Node, Go, Rust, Python, Ruby, PHP, Java, .NET, Static, …) | **Real** — `internal/build/detectors/` |
| 12 Dockerfile templates | **Real** but hardcoded strings, no multi-stage optimization |
| Recreate, rolling, blue-green, canary strategies | **Partial** — only recreate and rolling |
| Automatic rollback on failure | **Real** — `internal/deploy/rollback.go:176` |
| Build log streaming to UI | **Missing** — `pipeline.go:80` passes `io.Discard` |
| Build cancellation on user request | **Missing** — no `Stop()` on builder; a cancel force-kills |

**Score: 7/10.** The 80% that ships works. The 20% that doesn't is exactly the 20% users notice.

### 5.3 Ingress
| Spec says | Code status |
|---|---|
| Custom reverse proxy, no nginx/traefik | **Real** — `internal/ingress/proxy.go` |
| Automatic Let's Encrypt (HTTP-01) | **Real** |
| Let's Encrypt DNS-01 for wildcards | **Missing** |
| 5 LB strategies | **All 5 real** |
| Hot reload of routing table via events | **Real** |
| WebSocket frame rate limiting | **Missing** |
| TLS enforced by default | **Not enforced** — HTTPS is opt-in |

**Score: 7/10.** The core reverse proxy is the project's biggest positive surprise. The missing DNS-01 is the biggest negative surprise for a PaaS.

### 5.4 DNS providers
| Spec says | Code status | File:line |
|---|---|---|
| Cloudflare | **Real** | `internal/dns/providers/cloudflare.go:1-155` |
| Route53 | **Broken** — HTTP request built, no SigV4 signature attached | `internal/dns/providers/route53.go:98` ("AWS SigV4 signing would go here") |
| SigV4 helper | **Exists and passes AWS test vectors** | `internal/dns/providers/sigv4.go` (195 lines, untracked `??`) |

This is the most embarrassing finding. Someone wrote the SigV4 helper and tested it against AWS vectors, but never wired it into `route53.go`. A 10-line change away from working. As of HEAD, **Route53 DNS provider is non-functional.**

### 5.5 Backup targets
| Spec says | Code status |
|---|---|
| Local filesystem | **Real** |
| S3/R2/MinIO | **Broken on AWS S3** — `s3.go:155` has `// Note: Full AWS SigV4 signing would be implemented here.` and sends unsigned requests. Works against anonymous MinIO; fails against any authenticated S3 bucket. |
| SFTP | **Present** in the `·project/` spec, not verified in code |
| Rclone | **Missing** |
| Scheduler | **Real** — `internal/backup/scheduler.go:473` is solid |

Same category as Route53: the critical signing is missing. The `internal/dns/providers/sigv4.go` helper could be lifted into a shared `internal/aws/sigv4` package and reused here in a day.

### 5.6 VPS providers
| Spec says | Code status | File:line |
|---|---|---|
| Hetzner | **Real** | `internal/vps/providers/hetzner.go:1-203` |
| DigitalOcean | **Real** | `internal/vps/providers/digitalocean.go:1-213` |
| Vultr | **Real** | `internal/vps/providers/vultr.go:1-198` |
| Linode | **Real** | `internal/vps/providers/linode.go:1-188` |
| AWS EC2 | **Missing** |
| Custom SSH | **Real** | `internal/vps/providers/custom.go:1-44` |

All four implemented providers use `core.Retry` + `core.CircuitBreaker` correctly. This subsystem is in good shape. The missing AWS EC2 provider is expected given the Route53/S3 SigV4 gap — the team hasn't adopted AWS yet.

### 5.7 Git source providers
| Spec says | Code status |
|---|---|
| GitHub | **Real** |
| GitLab | **Real** |
| Gitea | **Real** |
| Bitbucket | **Real (parsing)** but **signature verification missing** (see §5.9) |

### 5.8 MCP (AI-callable tools)
| Spec says | Code status | File:line |
|---|---|---|
| 9 MCP tools | **Only 8 implemented** | `internal/mcp/tools.go:54` declares `scale_app`; `internal/mcp/handler.go:48` switch has no `scale_app` case |

The `HandleToolCall` switch falls through `default: return nil, fmt.Errorf("unknown tool: %s", toolName)`. An LLM caller invoking `scale_app` gets a generic error even though the tool is in the catalog.

### 5.9 Webhooks
| Spec says | Code status | File:line |
|---|---|---|
| GitHub HMAC-SHA256 | **Real** | `internal/webhooks/receiver.go:282` |
| GitLab token | **Real** | `receiver.go:284` |
| Gitea / Gogs signature | **Real** | `receiver.go:286-291` |
| Bitbucket signature | **MISSING** — falls through to `default: return true` | `receiver.go:292-294` |

`webhooks_final_test.go:265` literally has a comment `"Bitbucket falls through to parseGeneric since there's no parseBitbucket"` — the author knows. The Bitbucket path is unverified; anyone who can guess a webhook ID can trigger deployments on a Bitbucket-connected app.

### 5.10 Notifications
| Spec says | Code status |
|---|---|
| Slack | **Real** — `internal/notifications/providers.go` |
| Discord | **Real** |
| Telegram | **Real** |
| Generic webhook | **Real** |
| Email (SMTP) | **Missing entirely** — no interface, no struct, no `smtp` import anywhere in the module |

The README does not claim email support; the spec does. Classic spec-vs-code drift.

### 5.11 Marketplace
| Spec says | Code status |
|---|---|
| 25+ one-click apps | **Understated** — 116 real Docker Compose templates in `internal/marketplace/builtins*.go` |

The only place where reality beats the README. Nothing wrong.

### 5.12 Database engines (managed DBs)
| Spec says | Code status |
|---|---|
| PostgreSQL | **Real** — `internal/database/engines/postgres.go` |
| MySQL / MariaDB | **Real** |
| Redis | **Real** |
| MongoDB | **Missing** despite README claim |

### 5.13 Resource monitoring
| Spec says | Code status |
|---|---|
| Per-container metrics (CPU/RAM/disk) via Docker Stats | **Real** |
| Host-level metrics (Linux /proc parsing) | **Missing** — no `/proc/stat`, `/proc/meminfo` readers |
| Prometheus `/metrics` endpoint | **Real** |

Host-level gauges on the master dashboard will read zero or "n/a" on Linux.

### 5.14 PostgreSQL as primary store
| Spec says | Code status |
|---|---|
| "PostgreSQL ready" | **Interface ready, implementation absent** — only `internal/db/sqlite.go` implements `core.Store` |

The interface-based architecture makes this a greenfield implementation, not a migration, but the work has not started.

---

## 6. Testing Analysis

### 6.1 Test inventory (Go)
- 194 `_test.go` files, 46,960 LOC.
- `go test ./...` **fails to compile** in three packages due to untracked test files referencing undefined symbols. See §4.1.
- Fuzz tests: 7 (present in `internal/auth/`, `internal/core/`, `internal/ingress/`).
- Benchmarks: 38.
- Integration tests tagged `integration` — present for `internal/db/postgres_integration_test.go` (untracked, verifying a Postgres store that doesn't exist yet).

### 6.2 Test inventory (React)
- Vitest unit tests: ~38 files under `web/src/**/__tests__/`.
- Playwright E2E: 9 suites in `web/e2e/` (`auth`, `dashboard`, `apps`, `deploy-flow`, `domain-setup`, `marketplace`, `navigation`, `team-management`, `topology-editor`).

### 6.3 What is NOT tested (or not tested well)
- **The master/agent protocol end-to-end.** Unit tests mock the wire; there is no "launch a real agent process, have it connect to a real master, issue 100 deploy commands" test.
- **ACME certificate issuance.** `acme/autocert`-style code is unit tested; no integration against Let's Encrypt staging.
- **SIGHUP config reload.** Code path exists (`main.go:122-131`); no test reloads live config and verifies a running deploy survives.
- **Backup restore round-trip.** Backup creation is tested; full restore-from-zero is not exercised end-to-end.
- **Multi-tenant isolation under adversarial conditions.** Tenant A cannot read tenant B's data (verified in unit tests); tenant A cannot exhaust shared Docker daemon resources is **not** verified.
- **Stripe billing webhook replay.** HMAC signature is verified; idempotent replay is not exercised.
- **Graceful shutdown mid-build.** Build module has no `Stop()` — tests don't cover Ctrl-C during a `docker build`.

### 6.4 Coverage claim (README: "97%")
Unverifiable at HEAD because of the vet failures. Spot-checked package coverage:
- `internal/core`: very high (~95%), test LOC 2x source LOC.
- `internal/auth`: high, fuzz tests exist.
- `internal/db`: high, `*_coverage_test.go` files present.
- `internal/backup`, `internal/ingress`, `internal/api/middleware`: **broken at HEAD** — cannot measure.

Practical belief: average real coverage is somewhere in the 80-90% band on the packages that compile. The claim-vs-reality gap is small; the README could honestly say 85% and be fine.

---

## 7. Performance & Scalability Analysis

### 7.1 Cold start
- Binary size: ~22 MB (README), ~23 MB (`STATUS.md`). Plausible for a stripped Go binary with embedded React.
- Startup: module registry topological sort + 20 `Init()` + 20 `Start()` + ingress warm. Should be sub-second on any modern host. Not measured in this audit.

### 7.2 Hot path: HTTP proxy
- `httputil.ReverseProxy` per app, with `sync.RWMutex` protecting the routing table.
- Routing table updates on `app.deployed` / `app.removed` events via the in-process event bus — hot reload, no restart.
- LB strategies live in `internal/ingress/lb/`. Round-robin uses an atomic counter; IP-hash uses `fnv.New64a`. No allocation per request in the common path.

**Concern:** Every request acquires the `RWMutex` read lock on the routing table. Under high QPS on a large tenant count (thousands of apps), this becomes a hotspot. A lock-free `atomic.Value` swap on a prebuilt routing table would be the right fix when it matters. Not urgent.

### 7.3 Build throughput
- Concurrent builds limited by `limits.max_concurrent_builds` (config default 5).
- Build output to `io.Discard` (see §5.2) means memory is not the bottleneck, but also that users have no visibility.
- No build cache between deploys — each deploy re-downloads base images unless Docker's own cache is warm.

### 7.4 Database write amplification
- SQLite with WAL mode + PRAGMA tuning (verified in `internal/db/sqlite.go`).
- For a single-node master handling ~100 tenants, this is fine.
- Beyond that, contention on the audit log writer and usage-record writer becomes real. PostgreSQL implementation will be needed before 500+ tenants.

### 7.5 Memory
- EventBus async semaphore caps in-flight handlers at 64 — bounded.
- No goroutine leak hunters found in review, but Tier 73-77 hardening explicitly addresses WaitGroup drains, which is the right symptom to chase.

### 7.6 Frontend bundle
- Vite 8 with 5 manual chunks (`vendor-react`, `vendor-query`, `vendor-graph`, `vendor-ui`, `vendor-state`).
- 21 lazy-loaded pages — good first-paint.
- `vendor-query` is a dead chunk name (no React Query). Minor cleanup only.

---

## 8. Technical Debt Inventory

Ranked by impact to production readiness.

### P0 — Production blockers
1. **`go test ./...` is red at HEAD.** 3 build failures + 1 test failure + 1 deadlock. Ship-blocker for any responsible release engineer.
2. **SQLite migration rollback is broken.** `TestSQLite_Rollback` fails its assertion (`tenants` table not dropped after rollback) and then deadlocks on the next query. `internal/db/migrations/0001_init.down.sql` is suspect; the rollback machinery leaks a DB connection. No safe rollback = no safe upgrade story.
3. **Admin-role middleware does not exist.** `RequireAdmin`/`RequireSuperAdmin`/`RequireOwnerOrAbove` are tested but unimplemented. Every admin-only endpoint relies on handler-local role checks — privilege escalation risk on any forgotten check.
4. **AWS SigV4 not wired into Route53** — `route53.go:98`. `TestRoute53_SignsRequests` fails. The signing helper (`sigv4.go`, 195 lines, verified against AWS test vectors) exists but is not called.
5. **AWS SigV4 not wired into S3 backup** — `s3.go:155`. Silently breaks backups to authenticated S3 buckets. MinIO with anonymous access works; private AWS S3 does not.
6. **Bitbucket webhook signature verification missing** — `receiver.go:292-294`. Any provider not in the switch falls through to `default: return true`. Authentication bypass on Bitbucket-connected apps.
7. **Build log output discarded** — `pipeline.go:80` uses `io.Discard`. Users cannot see why a build failed. Fundamental UX regression and debugging black hole.
8. **MCP `scale_app` declared but not implemented** — `handler.go:48-65`. LLM callers get `unknown tool` for an advertised capability.

### P1 — Must fix before "stable" release
8. **Email (SMTP) notification provider missing entirely.** Spec calls for it; code has no implementation.
9. **TLS not enforced by default.** HTTP is opt-in to disable, not opt-in to enable. For a platform installing on a public VPS, this is wrong.
10. **DNS-01 challenge missing** → no wildcard certs.
11. **Rolling deploy has no SIGTERM grace period** — old replicas get killed without drain. Contradicts zero-downtime claim.
12. **Blue-green and canary deploy strategies missing** — in spec, not in code.
13. **WebSocket frame rate limiting missing** on all WS endpoints (swarm agent, DeployHub, log streaming).
14. **Recovery codes stored plaintext** in the users table — they are secondary passwords and should be hashed.
15. **No PKCE on OAuth** — Google/GitHub SSO is vulnerable to auth-code interception on public/mobile clients. Low risk given current clients are all confidential, but PKCE is free to add.
16. **Master encryption key in config file or env var.** Not HSM-backed, not KMS-fetched on startup. Operationally acceptable only if ops discipline is strict.
17. **Hardcoded Argon2id salt** (`"deploymonster-vault-salt-v1"`) in `internal/secrets/vault.go`. Per-deployment salt would be correct; the current setup means a rainbow-table attack can be precomputed against "any DeployMonster vault" rather than against one specific vault.
18. **Agent WS port hardcoded to 8443** — `internal/swarm/client.go:119`. Blocks non-default master ports.
19. **Build module has no `Stop()`** — force-kills in-flight builds on shutdown or cancel. Incorrect in a system that claims graceful shutdown.

### P2 — Should fix before v1.0
20. **Host-level `/proc` metrics missing** (Linux) — dashboard gauges read zero for host CPU/mem/disk.
21. **MongoDB managed-DB engine missing** despite README claim.
22. **PostgreSQL `core.Store` implementation missing.** README says "PostgreSQL ready"; code has interface only.
23. **AWS EC2 VPS provider missing.**
24. **Rclone backup target missing.**
25. **SIGHUP reload not tested** end-to-end.
26. **Frontend API client: no per-request timeout, no retry-with-backoff, unlimited refresh attempts** (`web/src/api/client.ts`).
27. **OpenAPI spec drift not verified in CI** — `docs/openapi.yaml` is published, CI does not validate routes match it.
28. **`docs/openapi.yaml` and `·project/SPECIFICATION.md` are snapshots**; no drift detection.
29. **CLAUDE.md claims "TanStack React Query 5"** — factually wrong, the codebase uses a custom `useApi`.
30. **README claims "97% test coverage"** — unverifiable at HEAD until vet errors are fixed; realistic measurement is likely 85-90%.
31. **README claims "9 MCP tools"** — 8 working, 1 broken.
32. **README claims "25+ marketplace templates"** — 116 exist (unusual: an understatement).
33. **Several `*_coverage_test.go` files** across modules look like coverage-chasing rather than behavior-driven tests.

### P3 — Nice to have
34. **`docs/adr/` has 7 ADRs.** They are good. Missing an ADR for: the master encryption key strategy, the Store interface composition decision, the event bus being in-process only (no Redis/NATS).
35. **`·project/` uses middle-dot U+00B7.** Filesystems and tools cope, but it is a footgun on Windows and in shell scripts without quoting.
36. **No load test results committed** despite `tests/loadtest` target existing in the Makefile.
37. **Stripe integration is real but only mock-tested** — integration test against Stripe test mode would catch wire-format issues.

---

## 9. Metrics Summary (measured, not claimed)

| Metric | Measured |
|---|---|
| Go source LOC | ~27,000 (spread across ~21 top-level packages under `internal/`) |
| Go test LOC | ~47,000 |
| Test-to-source ratio | ~1.74 : 1 |
| Test files | 194 `_test.go` |
| Source files | ~262 `.go` (non-test) |
| Packages currently broken at HEAD | 3 (`internal/api/middleware`, `internal/ingress`, `internal/backup`) |
| Fuzz tests | 7 |
| Benchmarks | 38 |
| Go modules (direct deps) | 8 |
| React/TS LOC | ~11,000 |
| React components | 44 |
| React pages | 21 (all lazy-loaded) |
| Zustand stores | 4 |
| Custom hooks | 6 |
| Playwright suites | 9 |
| Vitest test files | ~38 |
| Marketplace templates | 116 (not 25) |
| MCP tools declared | 9 |
| MCP tools implemented | 8 (`scale_app` missing) |
| API endpoints (registered routes) | ~240 |
| Modules auto-registered | 20 |
| Docker SDK version | `github.com/docker/docker v28.5.2` |
| Go version | 1.26.1 (toolchain) |
| CI stages | 5 (test / test-react / test-e2e / lint / build matrix + docker + release) |
| Coverage threshold in CI | 80% (below claimed 97%) |
| Binary target platforms | 5 (linux/darwin/windows × amd64/arm64, 8 combos minus darwin/arm64 windows dropout) |
| SBOM generation | Yes (goreleaser) |
| Current git branch | `master` |
| Recent hardening tiers | 73, 74, 75, 76, 77 (all within current session week) |

---

## 10. Developer Experience (DX)

Strengths:
- `Makefile` is complete and well-commented. `make help` works.
- `make build` / `make test` / `make lint` / `make bench` / `make coverage` / `make test-e2e` all exist.
- `scripts/build.sh` is a real full-pipeline build (React → embed → Go).
- `docs/adr/` exists and is populated.
- `docs/openapi.yaml` is published.
- `.goreleaser.yml` is populated with real release artifacts, SBOM, multi-arch.
- Auto-registration modules → adding a new module is one file in `internal/<name>/` + one blank import.

Friction:
- `go test ./...` is broken at HEAD.
- CLAUDE.md and README are drifted from reality (TanStack, coverage %, MCP tools, marketplace count).
- `·project/` with middle-dot is unfriendly to copy-paste and shell scripts.
- The 97% test coverage number is a CI-passing goal (threshold 80%) presented as a current state (97%). That kind of gap erodes trust in other claims.

---

## 11. What Is Actually Good (Credit Where Due)

Not every audit finds this much to praise.

- **The module system.** `core.Registry` + `Module` interface + topological sort + reverse-order shutdown is textbook. It is also battle-tested: Tier 73-77 hardening shows the maintainer reads their own shutdown paths under load.
- **The Store interface.** 12-way composition is the right shape. When PostgreSQL lands, it is a new file, not a migration.
- **The event bus.** Bounded async, `SafeGo` panic recovery, wildcard/prefix matching. Right scope for an in-process monolith; correctly resists the temptation to become a general-purpose queue.
- **The ingress gateway.** A real HTTP reverse proxy in ~7,500 LOC with five LB strategies, ACME, hot reload via events. Huge amount of value per line of code. No Traefik/Nginx dependency to manage.
- **The VPS providers.** Retry + circuit breaker on every call. Four implementations look mechanically similar, which is what you want (boring, uniform, no snowflakes).
- **The marketplace.** 116 real Docker Compose templates. Way over what the README advertises.
- **The Tier 73-77 hardening sprint.** Five consecutive commits in recent history fixing `wg.Wait`, `defer recover`, `stopCtx` plumbing, dead-client eviction. That is what good production code looks like in the middle of being written — the maintainer saw the gaps and closed them one by one.
- **The React frontend.** Strict TS, no `any`, 21 lazy pages, 9 Playwright suites, 4 clean Zustand stores. Not over-engineered with Redux or a custom framework.
- **CI/CD.** 5-stage GitHub Actions matrix. GoReleaser with SBOM. Non-root Docker image with health check. `docker-compose.prod.yml` with `read_only: true`, `tmpfs`, `no-new-privileges`, and memory/CPU caps.
- **The developer ergonomics.** `make` targets cover everything. `monster.yaml` config with env override. SIGHUP reload. `rotate-keys` subcommand for the secrets vault. `deploymonster init` scaffold.

---

## 12. Documents Map

Where this audit fits:
- **`.project/ANALYSIS.md`** ← this file. What the project is, what's in it, what's broken, what's good.
- **`.project/ROADMAP.md`** — the plan to close every P0/P1/P2 gap, in seven phases, with effort estimates.
- **`.project/PRODUCTIONREADY.md`** — the scored verdict with category breakdown and a go/no-go recommendation.

---

*Prepared 2026-04-11 from HEAD `b36fba6`. No code was modified during the audit. All findings are cite-able by file:line.*
