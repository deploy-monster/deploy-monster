# Architecture Report — DeployMonster

**Date:** 2026-05-01
**Scanner:** Claude Code (sc-recon)
**Scope:** Full codebase (post-security-audit commit a844d6e)

---

## 1. Technology Stack Detection

### Languages
| Language | Approx. % | Notes |
|----------|-----------|-------|
| Go | 56.5% | Primary backend language (Go 1.26.1, toolchain 1.26.2) |
| TypeScript / TSX | 8.2% | React 19 frontend (Vite 8) |
| Markdown | 29.0% | Documentation, specs, tasks |
| YAML | 4.5% | CI/CD, OpenAPI, config |
| CSS | 1.2% | Tailwind CSS v4 |
| SQL | 0.4% | Migrations, schema |
| HTML | 0.1% | Index template |

### Frameworks & Libraries
**Backend:**
- `net/http` (Go 1.22+ `http.ServeMux` with METHOD /path patterns)
- `github.com/docker/docker` v28.5.2 — Docker API client
- `github.com/golang-jwt/jwt/v5` v5.3.1 — JWT
- `github.com/gorilla/websocket` v1.5.3 — WebSocket
- `modernc.org/sqlite` v1.48.2 — Pure Go SQLite
- `go.etcd.io/bbolt` v1.4.3 — BBolt KV
- `github.com/jackc/pgx/v5` v5.9.1 — PostgreSQL driver (future)
- `golang.org/x/crypto` v0.50.0 — Crypto utilities
- `gopkg.in/yaml.v3` v3.0.1 — YAML parsing

**Frontend:**
- React 19.2.5, React Router 7.13.2
- Vite 8.0.5, TypeScript 5.9.3
- Tailwind CSS 4.2.2, tailwind-merge, tailwindcss/vite
- Zustand 5.0.12 — State management
- @xyflow/react 12.10.2 — Topology editor
- dagre 0.8.5 — Graph layout
- lucide-react 1.8.0 — Icons

**Testing:**
- Go: `testing`, stretchr/testify patterns
- Frontend: Vitest 3.2.1, Playwright 1.59.1, Testing Library, jsdom 26.1.0
- Accessibility: @axe-core/playwright 4.11.1

### Databases
- **SQLite** (`modernc.org/sqlite`) — Default relational store
- **BBolt KV** (`go.etcd.io/bbolt`) — 30+ buckets for config, state, metrics, API keys, webhook secrets
- **PostgreSQL** (`github.com/jackc/pgx/v5`) — Planned enterprise support

### Build Tools
- `make` — Go build, dev, test, lint
- `pnpm` — Frontend package manager
- `scripts/build.sh` — Full pipeline: React build → embed copy → Go build with ldflags

---

## 2. Application Type Classification

**Type:** Modular Monolith PaaS (Platform as a Service)

**Indicators:**
- Single binary runs as **master** (full platform) or **agent** (worker node via `--agent` flag)
- 20 auto-registered Go modules via `init()` + `core.RegisterModule()`
- Embedded React SPA served via `embed.FS`
- In-process pub/sub event bus
- Built-in reverse proxy (Ingress Gateway on :80/:443)
- Docker container runtime integration

---

## 3. Entry Points Mapping

### HTTP Routes (API Layer)
All routes registered in `internal/api/router.go` via `http.ServeMux`.

**Public / Unauthenticated Routes:**
| Method | Path | Handler | Auth | File |
|--------|------|---------|------|------|
| GET | /health | handleHealth | None | router.go:113 |
| GET | /api/v1/health | handleHealth | None | router.go:114 |
| GET | /readyz | handleReadiness | None | router.go:115 |
| GET | /health/detailed | DetailedHealth | None | router.go:118 |
| GET | /api/v1/openapi.json | OpenAPI spec | None | router.go:122 |
| POST | /api/v1/auth/login | Login | Rate-limited | router.go:132 |
| POST | /api/v1/auth/register | Register | Rate-limited | router.go:133 |
| POST | /api/v1/auth/refresh | Refresh | Rate-limited (5/min) | router.go:134 |
| POST | /api/v1/auth/logout | Logout | None | router.go:135 |
| POST | /hooks/v1/{webhookID} | Webhook Receiver | HMAC sig | router.go:148 |
| GET | /api/v1/marketplace | List templates | None (ETag) | router.go:585 |
| GET | /api/v1/marketplace/{slug} | Get template | None (ETag) | router.go:586 |
| GET | /api/v1/billing/plans | List plans | None | router.go:555 |
| GET | /api/v1/databases/engines | List DB engines | None | router.go:487 |
| GET | /api/v1/environments/presets | List presets | None | router.go:391 |

**Protected Routes (JWT Bearer + Tenant Rate Limit):**
| Method | Path | Handler | File |
|--------|------|---------|------|
| GET | /api/v1/auth/me | GetCurrentUser | router.go:139 |
| PATCH | /api/v1/auth/me | UpdateProfile | router.go:140 |
| POST | /api/v1/auth/change-password | ChangePassword | router.go:141 |
| GET | /api/v1/auth/sessions | ListSessions | router.go:142 |
| POST | /api/v1/auth/logout-all | LogoutAll | router.go:143 |
| GET | /api/v1/dashboard/stats | Stats | router.go:158 |
| GET/POST | /api/v1/apps | List/Create | router.go:162-163 |
| GET/POST/PATCH/DELETE | /api/v1/apps/{id} | CRUD | router.go:167-169 |
| POST | /api/v1/apps/{id}/restart | Restart | router.go:170 |
| POST | /api/v1/apps/{id}/stop | Stop | router.go:171 |
| POST | /api/v1/apps/{id}/start | Start | router.go:172 |
| POST | /api/v1/apps/{id}/deploy | TriggerDeploy | router.go:175 |
| POST | /api/v1/apps/{id}/suspend | Suspend | router.go:179 |
| POST | /api/v1/apps/{id}/resume | Resume | router.go:180 |
| POST | /api/v1/apps/{id}/transfer | TransferApp | **adminOnly** | router.go:182 |
| GET | /api/v1/apps/{id}/metrics/export | Export | router.go:186 |
| POST | /api/v1/apps/{id}/rename | Rename | router.go:190 |
| GET/PUT | /api/v1/apps/{id}/gpu | GPU config | router.go:195-196 |
| POST/DELETE | /api/v1/apps/{id}/pin | Pin/Unpin | router.go:200-201 |
| POST | /api/v1/apps/{id}/save-template | SaveTemplate | router.go:205 |
| POST | /api/v1/apps/{id}/rollback-to-commit | RollbackToCommit | router.go:209 |
| GET/POST | /api/v1/apps/{id}/snapshots | Snapshots | router.go:213-214 |
| POST | /api/v1/apps/{id}/webhooks/{logId}/replay | Replay | router.go:218 |
| POST | /api/v1/apps/{id}/clone | Clone | router.go:222 |
| POST | /api/v1/apps/bulk | BulkExecute | router.go:224 |
| PUT | /api/v1/apps/{id}/restart-policy | UpdateRestartPolicy | router.go:228 |
| GET/PUT | /api/v1/apps/{id}/labels | Labels | router.go:232-233 |
| GET | /api/v1/apps/{id}/disk | AppDisk | router.go:237 |
| POST | /api/v1/apps/{id}/webhooks/test | TestDeliver | router.go:241 |
| GET/PUT | /api/v1/apps/{id}/ports | Ports | router.go:245-246 |
| GET/PUT | /api/v1/apps/{id}/healthcheck | HealthCheck | router.go:250-251 |
| POST/GET | /api/v1/apps/{id}/commands | Run/History | router.go:255-256 |
| GET/PUT | /api/v1/apps/{id}/log-retention | LogRetention | router.go:260-261 |
| GET/PUT | /api/v1/apps/{id}/middleware | AppMiddleware | router.go:265-266 |
| GET | /api/v1/apps/{id}/restarts | RestartHistory | router.go:270 |
| POST | /api/v1/apps/{id}/webhooks/rotate | Rotate | router.go:274 |
| POST | /api/v1/apps/{id}/deploy/preview | Preview | router.go:278 |
| GET | /api/v1/apps/{id}/deployments/diff | Diff | router.go:280 |
| GET | /api/v1/apps/{id}/builds/latest/log | BuildLog | router.go:284-285 |
| GET/PUT | /api/v1/apps/{id}/maintenance | Maintenance | router.go:289-290 |
| GET/POST/DELETE | /api/v1/apps/{id}/redirects | Redirects | router.go:294-297 |
| GET/PUT | /api/v1/apps/{id}/error-pages | ErrorPages | router.go:301-302 |
| GET/PUT | /api/v1/apps/{id}/sticky-sessions | StickySessions | router.go:306-307 |
| GET/PUT | /api/v1/apps/{id}/autoscale | Autoscale | router.go:312-313 |
| GET/PUT | /api/v1/apps/{id}/response-headers | ResponseHeaders | router.go:317-318 |
| GET | /api/v1/apps/{id}/containers/history | ContainerHistory | router.go:322 |
| GET/PUT | /api/v1/apps/{id}/deploy-notifications | DeployNotify | router.go:326-327 |
| GET/PUT | /api/v1/apps/{id}/basic-auth | BasicAuth | router.go:332-333 |
| GET | /api/v1/apps/{id}/processes | Top | router.go:337 |
| GET | /api/v1/apps/{id}/webhooks/logs | WebhookLogs | router.go:341 |
| GET/POST/DELETE | /api/v1/apps/{id}/cron | CronJobs | router.go:346-348 |
| GET | /api/v1/apps/{id}/logs/download | LogDownload | router.go:352 |
| POST | /api/v1/apps/{id}/rollback | Rollback | router.go:356 |
| GET | /api/v1/apps/{id}/versions | Versions | router.go:357 |
| GET | /api/v1/apps/{id}/files | FileBrowser | router.go:366 |
| GET/POST | /api/v1/apps/{id}/stats | AppStats | router.go:370 |
| GET | /api/v1/servers/stats | ServerStats | router.go:371 |
| POST | /api/v1/apps/{id}/scale | Scale | router.go:373 |
| GET/PUT | /api/v1/apps/{id}/resources | Resources | router.go:377-378 |
| GET | /api/v1/apps/{id}/dependencies | DependencyGraph | router.go:382 |
| GET | /api/v1/apps/{id}/metrics | AppMetrics | router.go:386 |
| GET | /api/v1/servers/{id}/metrics | ServerMetrics | router.go:387 |
| POST | /api/v1/projects/{id}/environment | ApplyPreset | router.go:392 |
| GET/POST | /api/v1/networks | List/Connect | router.go:396-397 |
| POST/GET | /api/v1/apps/{id}/env/import-export | EnvImportExport | router.go:401-402 |
| GET/POST/DELETE | /api/v1/dns/records | DNS Records | router.go:407-409 |
| GET | /api/v1/domains/ssl-check | SSLCheck | router.go:413 |
| GET | /api/v1/agents | ListAgents | router.go:417 |
| GET | /api/v1/agents/{id} | GetAgent | router.go:418 |
| GET | /api/v1/apps/{id}/logs | GetLogs | router.go:422 |
| POST | /api/v1/domains/{id}/verify | VerifyDomain | router.go:426 |
| POST | /api/v1/domains/verify-batch | BatchVerify | router.go:427 |
| GET/POST | /api/v1/certificates | List/Upload | router.go:431-432 |
| POST | /api/v1/certificates/wildcard | RequestWildcard | router.go:436 |
| GET | /api/v1/images/tags | ImageTags | router.go:440 |
| GET | /api/v1/images/dangling | DanglingImages | router.go:441 |
| DELETE | /api/v1/images/prune | PruneImages | router.go:443 |
| GET/POST | /api/v1/volumes | List/Create | router.go:447-448 |
| GET/POST/DELETE | /api/v1/projects | Projects CRUD | router.go:452-455 |
| GET/PUT | /api/v1/apps/{id}/env | EnvVars | router.go:459-460 |
| GET/POST | /api/v1/registries | Registries | router.go:464-465 |
| GET/POST/DELETE | /api/v1/domains | Domains | router.go:469-471 |
| POST | /api/v1/apps/{id}/exec | ContainerExec | router.go:475 |
| GET | /api/v1/team/roles | ListRoles | router.go:479 |
| GET/POST | /api/v1/team/invites | Invites | router.go:481-482 |
| GET | /api/v1/team/audit-log | AuditLog | router.go:483 |
| POST | /api/v1/databases | CreateDatabase | router.go:488 |
| GET/POST | /api/v1/backups | List/Create | router.go:493-494 |
| GET | /api/v1/backups/{key}/download | DownloadBackup | router.go:495 |
| GET | /api/v1/servers/providers | ListProviders | router.go:499 |
| GET | /api/v1/servers/providers/{provider}/regions | ListRegions | router.go:500 |
| GET | /api/v1/servers/providers/{provider}/sizes | ListSizes | router.go:501 |
| POST | /api/v1/servers/provision | ProvisionServer | router.go:502 |
| POST | /api/v1/servers/test-ssh | TestSSH | router.go:504 |
| GET/DELETE | /api/v1/build/cache | CacheStats/Clear | router.go:508-509 |
| GET/PATCH | /api/v1/tenant/settings | TenantSettings | router.go:513-514 |
| GET | /api/v1/storage/usage | StorageUsage | router.go:518 |
| GET | /api/v1/git/providers | ListGitProviders | router.go:522 |
| GET | /api/v1/git/{provider}/repos | ListRepos | router.go:523 |
| GET | /api/v1/git/{provider}/repos/{repo}/branches | ListBranches | router.go:524 |
| POST | /api/v1/stacks | DeployStack | router.go:529 |
| POST | /api/v1/stacks/validate | ValidateStack | router.go:530 |
| GET/POST | /api/v1/secrets | Secrets CRUD | router.go:550-551 |
| GET | /api/v1/billing/usage | GetUsage | router.go:556 |
| GET | /api/v1/billing/usage/history | UsageHistory | router.go:558 |
| POST | /api/v1/webhooks/stripe | StripeWebhook | Stripe sig | router.go:566 |
| POST | /api/v1/marketplace/deploy | DeployTemplate | router.go:589 |
| POST/GET | /api/v1/topology | Save/Load | router.go:594-595 |
| POST | /api/v1/topology/compile | Compile | router.go:596 |
| POST | /api/v1/topology/validate | Validate | router.go:597 |
| POST | /api/v1/topology/deploy | DeployTopology | router.go:598 |
| GET/POST/DELETE | /api/v1/webhooks/outbound | OutboundWebhooks | router.go:603-605 |
| GET/POST/DELETE | /api/v1/deploy/freeze | DeployFreeze | router.go:609-611 |
| POST | /api/v1/apps/env/compare | EnvCompare | router.go:615 |
| POST | /api/v1/notifications/test | TestNotification | router.go:619 |
| GET/POST | /api/v1/apps/{id}/terminal | TerminalStream/Send | router.go:623-624 |
| GET | /api/v1/deploy/approvals | ListPending | router.go:628 |
| POST | /api/v1/deploy/approvals/{id}/approve | Approve | router.go:629 |
| POST | /api/v1/deploy/approvals/{id}/reject | Reject | router.go:630 |
| GET | /api/v1/search | Search | router.go:634 |
| GET | /api/v1/activity | ActivityFeed | router.go:638 |
| GET/POST | /api/v1/ssh-keys | List/Generate | router.go:642-643 |
| GET/POST | /api/v1/mcp/v1/tools | MCP List/Call | router.go:647-648 |
| GET | /api/v1/apps/{id}/logs/stream | StreamLogs (SSE) | router.go:653 |
| GET | /api/v1/events/stream | StreamEvents (SSE) | router.go:654 |
| GET | /api/v1/topology/deploy/{projectId}/progress | DeployProgress (WS) | router.go:658-665 |

**Admin Routes (`adminOnly` middleware = SuperAdmin required):**
Registered via `internal/api/routes_admin.go`.
All `/api/v1/admin/*` routes require SuperAdmin role. Verified by CI test in `router_test.go`.

**Ingress Gateway Routes (port 80/443):**
| Method | Path | Handler | File |
|--------|------|---------|------|
| GET | /health | healthHandler | ingress/module.go:218 |
| GET | /ready | readyHandler | ingress/module.go:219 |
| GET | /live | liveHandler | ingress/module.go:220 |
| GET | /metrics | PrometheusHandler | ingress/module.go:223 |
| * | / | HTTPS redirect or reverse proxy | ingress/module.go:234 |

**WebSocket Endpoints:**
- `GET /api/v1/apps/{id}/terminal` — Container terminal (router.go:623-624)
- `GET /api/v1/topology/deploy/{projectId}/progress` — Deploy progress hub (router.go:658)

**SSE Endpoints:**
- `GET /api/v1/apps/{id}/logs/stream` — Log streaming (router.go:653)
- `GET /api/v1/events/stream` — Event streaming (router.go:654)

**CLI Entry Points:**
- `cmd/deploymonster/main.go` — Main binary, `--agent` flag for agent mode
- `make` targets: build, dev, test, lint, docker, etc.

---

## 4. Data Flow Map

### Sources
- **HTTP request body** — JSON payloads up to 10MB (1MB for webhooks)
- **Query parameters** — Used in List, Search, Filter handlers
- **URL path segments** — `{id}`, `{webhookID}`, `{projectId}`, `{ruleId}`
- **Headers** — `Authorization: Bearer <JWT>`, `X-API-Key`, `Content-Type`
- **Cookies** — Session tracking (CSRF token)
- **WebSocket messages** — Terminal input, deploy progress
- **File uploads** — App imports, certificates, backups, images
- **Environment variables** — `MONSTER_*` prefixed config overrides
- **Config file** — `monster.yaml`
- **Database reads** — SQLite/BBolt store queries
- **Docker events** — Container lifecycle events
- **Webhook payloads** — External Git/webhook POSTs with HMAC signatures

### Processing
- **Validation** — Custom handler validators per domain
- **Sanitization** — `middleware.BodyLimit`, `middleware.Recovery`, `middleware.SecurityHeaders`
- **Auth** — `middleware.RequireAuth` (JWT validation), `middleware.RequireSuperAdmin`
- **Rate limiting** — Global per-IP (`/api/`, `/hooks/`), per-tenant (100/min), per-auth-type (login 120/min, register 120/min, refresh 5/min)
- **CSRF** — `middleware.CSRFProtect` on all routes
- **CORS** — `middleware.CORS` with configurable origins
- **Audit** — `middleware.AuditLog` records all requests
- **Idempotency** — `middleware.IdempotencyMiddleware` using BBolt

### Sinks
- **Database queries** — SQLite via `core.Store` interface (SQL, parameterized)
- **BBolt KV** — Config, state, metrics, API keys, webhook secrets
- **Docker API** — Container CRUD, exec, logs, stats via `github.com/docker/docker`
- **File system** — Migrations, backups, credentials file, SQLite DB, embedded static assets
- **HTTP responses** — JSON API responses, SSE streams, WebSocket messages, embedded SPA
- **Outbound HTTP** — Webhook deliveries, ACME challenges, Git clone, VPS provider APIs, DNS provider APIs
- **Logging** — Structured `log/slog` with module key
- **Notifications** — Email, Slack, webhook notifications

---

## 5. Trust Boundaries

### Authentication
- **JWT Bearer** — `middleware.RequireAuth` validates HS256 tokens (15min access, 7day refresh)
- **API Key** — `X-API-Key` header, validated against BBolt store
- **SuperAdmin** — `middleware.RequireSuperAdmin` stacks on protected
- **Webhook HMAC** — `webhooks.NewReceiver` verifies SHA-256 signatures
- **Stripe webhook** — Signature verification with shared secret

### Rate Limiting
- **Global** — 120 req/min per IP on `/api/` and `/hooks/` prefixes (`globalRL`)
- **Tenant** — 100 req/min per tenant (`tenantRL`)
- **Auth-specific** — Login/register 120/min, refresh 5/min
- **Bypass** — Static SPA assets (`/`, `/assets/*`) are excluded from global rate limit

### Input Validation
- **Body limit** — 10MB global, 1MB webhooks
- **Request timeout** — 30s
- **CORS origin whitelist** — Configurable in `monster.yaml`
- **CSRF protection** — `middleware.CSRFProtect`
- **Idempotency keys** — BBolt-backed deduplication

### Authorization
- **Role-based** — `role_super_admin`, tenant-scoped roles
- **Cross-tenant protection** — Middleware enforces tenant isolation
- **Admin route protection** — CI test asserts 403 on all `/api/v1/admin/*` with developer token

---

## 6. External Integrations

| Service | Type | Config Location | Notes |
|---------|------|-----------------|-------|
| Docker Engine | Container runtime | `core.Services.Container` | Local socket or TCP |
| SQLite | Primary DB | `monster.yaml` Database.Path | File-based |
| BBolt | KV store | Alongside SQLite | `core.DB.Bolt` |
| PostgreSQL | Enterprise DB | `monster.yaml` | Via `core.Store` interface |
| ACME (Let's Encrypt) | TLS certificates | `monster.yaml` ACME section | HTTP-01 challenge |
| DNS Providers | DNS management | `internal/dns/providers/` | Pluggable |
| VPS Providers | Server provisioning | `internal/vps/providers/` | Pluggable |
| Git Providers | Source code | `internal/gitsources/providers/` | Pluggable |
| Backup Storage | Backups | `core.Services.BackupStorage()` | Pluggable |
| SMTP | Email notifications | `monster.yaml` Notifications | Configurable |
| Stripe | Billing | `internal/billing/` | Webhook signature verified |
| Prometheus | Metrics | `internal/ingress/` + `/metrics` | Protected with auth |

---

## 7. Authentication Architecture

**Pattern:** JWT (primary) + API Key (secondary) + Webhook HMAC

- **JWT Library:** `github.com/golang-jwt/jwt/v5` v5.3.1
- **Algorithm:** HS256
- **Access token lifetime:** 15 minutes
- **Refresh token lifetime:** 7 days
- **Claims:** UserID, TenantID, RoleID, Email
- **Key rotation:** Supported via `PreviousSecretKeys` in `JWTService`
- **Storage:** JWT tokens are stateless; refresh tokens tracked in BBolt/session store
- **Password hashing:** `golang.org/x/crypto/bcrypt` (inferred from `HashPassword` function)
- **First-run setup:** Auto-creates super admin from `MONSTER_ADMIN_EMAIL`/`MONSTER_ADMIN_PASSWORD` env vars; writes auto-generated creds to `.credentials` file with 0600 perms
- **MFA:** Not implemented
- **Account lockout:** Not implemented
- **Session management:** `ListSessions`, `LogoutAll` endpoints present

---

## 8. File Structure Analysis

### Sensitive Paths
| Path | Purpose | Risk Level |
|------|---------|------------|
| `/api/v1/admin/*` | Admin operations | High — SuperAdmin only |
| `/metrics` | Prometheus metrics | Medium — Info disclosure if unprotected |
| `/debug/pprof/*` | Go profiling | High — Only when `EnablePprof` true, auth-protected |
| `/health/detailed` | Detailed health | Low — May leak component status |
| `/.credentials` | Auto-generated admin creds | Critical — Written to DB dir with 0600 |
| `/api/v1/apps/{id}/exec` | Container exec | High — Command execution |
| `/api/v1/apps/{id}/terminal` | Terminal WebSocket | High — Interactive shell access |
| `/api/v1/apps/{id}/files` | File browser | Medium — Filesystem access |
| `/hooks/v1/{webhookID}` | Public webhook endpoint | Medium — Must verify HMAC |
| `/api/v1/webhooks/stripe` | Stripe webhook | Medium — Signature verified |

### Configuration Files
- `monster.yaml` — Main server config
- `.env` / `.env.*` — Environment overrides (if present)
- `go.mod` / `go.sum` — Go dependencies
- `web/package.json` — Frontend dependencies
- `docs/openapi.yaml` — API specification

### Deployment Files
- `Dockerfile` / `docker-compose.yml`
- `.github/workflows/` — CI/CD pipelines
- `scripts/build.sh` — Build script
- `internal/api/static/` — Embedded React build output

---

## 9. Detected Security Controls

| Control | Status | Location |
|---------|--------|----------|
| **Rate Limiting** | Yes | Global, tenant, auth-specific in router.go |
| **CORS** | Yes | Configurable origins, HTTPS-aware |
| **CSRF Protection** | Yes | `middleware.CSRFProtect` |
| **Security Headers** | Yes | `middleware.SecurityHeaders` |
| **Body Limit** | Yes | 10MB global, 1MB webhooks |
| **Request Timeout** | Yes | 30s |
| **Request ID** | Yes | `middleware.RequestID` |
| **Audit Logging** | Yes | `middleware.AuditLog` |
| **Recovery/Panic** | Yes | `middleware.Recovery` |
| **Input Validation** | Partial | Per-handler, no global schema validator |
| **Output Encoding** | Partial | JSON responses; HTML via React (safe by default) |
| **CSP** | Not detected | No explicit CSP header in middleware chain |
| **WAF** | Not detected | No WAF references |
| **Encryption at Rest** | Partial | SQLite file perms depend on OS; secrets module encrypts sensitive values |
| **mTLS** | Not detected | Not implemented |
| **HSTS** | Yes | Set on HTTP→HTTPS redirects (ingress/module.go:243) |
| **TLS Min Version** | Yes | TLS 1.2 (ingress/module.go:263) |
| **Open Redirect Protection** | Yes | `isValidRedirectHost` validates Host header (ingress/module.go:272) |
| **Idempotency** | Yes | BBolt-backed idempotency middleware |
| **Prometheus Auth** | Yes | `/metrics` protected with JWT |
| **pprof Auth** | Yes | Only when `EnablePprof` true, auth-protected |

---

## 10. Language Detection Summary

```markdown
## Detected Languages
- Go (56.5% of codebase) → activates sc-lang-go
- TypeScript (8.2% of codebase) → activates sc-lang-typescript
- YAML (4.5% of codebase, config/CI only) → no dedicated scanner
- CSS (1.2% of codebase) → no dedicated scanner
- SQL (0.4% of codebase, migrations only) → no dedicated scanner
- Markdown (29.0% of codebase, documentation) → no dedicated scanner
```

**Phase 2 language scanners to activate:** `sc-lang-go`, `sc-lang-typescript`
