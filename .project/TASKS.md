# DeployMonster — TASKS.md

> **Companion to**: SPECIFICATION.md + IMPLEMENTATION.md
> **Purpose**: Granular, ordered task checklist for Claude Code execution
> **Rule**: Complete each phase fully before moving to the next. Every task must pass before proceeding.
> **Convention**: Tasks prefixed with phase number. Subtasks indented. Each task is a single commit.

---

## Phase 1 — Foundation (v0.1.0)

**Goal**: Login → see container list → start/stop containers → basic React UI shell

### 1.1 Project Scaffold

- [x] **T-1.1.1** — Initialize Go module: `go mod init github.com/deploy-monster/deploy-monster`
- [x] **T-1.1.2** — Create full directory structure per SPECIFICATION.md §24 (all `internal/` subdirectories, `cmd/`, `web/`, `scripts/`, `docs/`, `marketplace/`)
- [x] **T-1.1.3** — Create `Makefile` with targets: `build`, `dev`, `test`, `lint`, `clean`, `docker`
- [x] **T-1.1.4** — Create `.gitignore` (bin/, web/dist/, web/node_modules/, *.db, *.db-wal, *.db-shm, coverage.out)
- [x] **T-1.1.5** — Create `scripts/build.sh` — builds React UI then Go binary with ldflags (version, commit, date)
- [x] **T-1.1.6** — Create `.goreleaser.yaml` for cross-platform release builds (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)
- [x] **T-1.1.7** — Create `deployments/Dockerfile` — multi-stage build (node → go → scratch)
- [x] **T-1.1.8** — Create `deployments/docker-compose.dev.yaml` — dev environment with mounted source

### 1.2 Core Engine

- [x] **T-1.2.1** — Implement `internal/core/config.go`:
  - `Config` struct with all nested config types (ServerConfig, DatabaseConfig, IngressConfig, etc.)
  - `LoadConfig()` — reads monster.yaml from standard paths, applies env var overrides, sets defaults
  - Auto-generate `MONSTER_SECRET` on first run, persist to config file
  - `applyDefaults()` and `applyEnvOverrides()` helper functions
- [x] **T-1.2.2** — Implement `internal/core/module.go`:
  - `Module` interface (ID, Name, Version, Dependencies, Init, Start, Stop, Health, Routes, Events)
  - `HealthStatus` enum (OK, Degraded, Down)
  - `Route` struct (Method, Path, Handler, Auth level)
  - `AuthLevel` enum (None, APIKey, JWT, Admin, SuperAdmin)
  - `HandlerFunc` type alias
  - `RequestContext` struct (wraps http.Request with parsed claims, tenant, etc.)
- [x] **T-1.2.3** — Implement `internal/core/registry.go`:
  - `Registry` struct with `map[string]Module` and topological order slice
  - `Register(m Module)` — add module, error on duplicate
  - `Resolve()` — topological sort with circular dependency detection
  - `InitAll(ctx, core)` — init in dependency order
  - `StartAll(ctx)` — start in dependency order
  - `StopAll(ctx)` — stop in reverse order
  - `Get(id) Module` — retrieve module by ID
- [x] **T-1.2.4** — Implement `internal/core/events.go`:
  - `Event` struct (Type, Source, Timestamp, Data)
  - `EventHandler` struct (EventType, Handler func)
  - `EventBus` with Subscribe/Publish, wildcard `*` support
  - All event type constants (40+ events per IMPLEMENTATION.md §2.3)
- [x] **T-1.2.5** — Implement `internal/core/errors.go`:
  - Sentinel errors: ErrNotFound, ErrAlreadyExists, ErrUnauthorized, ErrForbidden, ErrQuotaExceeded, ErrBuildFailed, ErrDeployFailed, ErrInvalidInput, ErrExpired, ErrInvalidToken
  - `AppError` struct with Code, Message, Err, Error(), Unwrap()
  - `NewAppError()` constructor
- [x] **T-1.2.6** — Implement `internal/core/app.go`:
  - `Core` struct holding Config, BuildInfo, Registry, Events, DB, Logger, Router, and shared refs (Docker, SSHPool, Secrets)
  - `NewApp(cfg, build)` — creates Core, registers all modules
  - `Run(ctx)` — resolve → init → start → wait for signal → graceful stop (30s timeout)
  - `registerAllModules(c)` — imports and registers every module
- [x] **T-1.2.7** — Implement `internal/core/id.go`:
  - `GenerateID()` — returns 16-char hex random ID (crypto/rand)
  - `GenerateSecret(length)` — returns crypto-random base64 string
  - `GeneratePassword(length)` — returns crypto-random alphanumeric password
- [x] **T-1.2.8** — Implement `cmd/deploymonster/main.go`:
  - Signal handling (SIGINT, SIGTERM) via `signal.NotifyContext`
  - Load config, create app, run, exit on error
  - Version/commit/date ldflags variables

### 1.3 Database Layer

- [x] **T-1.3.1** — Implement `internal/db/module.go`:
  - Database module implementing `core.Module` interface
  - ID: `core.db`, Priority: 0, no dependencies
  - Init: create SQLite + BBolt, run migrations
  - Health: ping both databases
- [x] **T-1.3.2** — Implement `internal/db/sqlite.go`:
  - `NewSQLite(path)` — open with WAL mode, busy timeout, foreign keys, mmap, cache pragmas
  - `Tx(ctx, fn)` — transaction wrapper with auto-rollback on error
  - `Close()` — graceful close
  - MaxOpenConns=1, MaxIdleConns=2 (SQLite single writer)
- [x] **T-1.3.3** — Implement `internal/db/migrations/0001_init.sql`:
  - Full schema per IMPLEMENTATION.md §3.2 (25+ tables)
  - All indexes (audit_log, usage_records)
  - Built-in role seed data (super_admin, owner, admin, developer, operator, viewer)
- [x] **T-1.3.4** — Implement migration runner in `sqlite.go`:
  - `migrate()` — reads `migrations/*.sql` via `embed.FS`
  - Sorts by filename, tracks applied versions in `_migrations` table
  - Skip already-applied, apply new in order
- [x] **T-1.3.5** — Implement `internal/db/bolt.go`:
  - `NewBoltStore(path)` — open BBolt with 5s timeout, create buckets (sessions, ratelimit, buildcache, metrics_ring)
  - `Set(bucket, key, value, ttl)` — JSON marshal + TTL metadata
  - `Get(bucket, key, dest)` — JSON unmarshal + TTL check
  - `Delete(bucket, key)`
  - `SetRaw(bucket, key, []byte)` and `GetRaw(bucket, key) []byte`
- [x] **T-1.3.6** — Implement `internal/db/models/` — Go structs for every table:
  - `tenant.go` — Tenant struct with JSON tags
  - `user.go` — User struct (password_hash excluded from JSON)
  - `project.go`, `app.go`, `deployment.go`, `domain.go`, `certificate.go`
  - `server.go`, `volume.go`, `backup.go`, `database.go`
  - `secret.go`, `secret_version.go`
  - `git_source.go`, `webhook.go`, `webhook_log.go`
  - `api_key.go`, `invitation.go`, `team_member.go`, `role.go`
  - `subscription.go`, `usage_record.go`, `invoice.go`
  - `vps_provider.go`, `compose_stack.go`, `marketplace_install.go`
  - `audit_log.go`
- [x] **T-1.3.7** — Implement repository layer — `internal/db/` query functions:
  - `tenants.go` — CRUD for tenants
  - `users.go` — CRUD, GetByEmail, UpdatePassword
  - `apps.go` — CRUD, ListByProject, ListByTenant, UpdateStatus
  - `deployments.go` — Create, GetLatest, ListByApp
  - `domains.go` — CRUD, GetByFQDN, ListByApp
  - Each file uses prepared statements and `Tx()` for writes

### 1.4 Authentication Module

- [x] **T-1.4.1** — Implement `internal/auth/module.go`:
  - Auth module implementing `core.Module`
  - ID: `core.auth`, depends on `core.db`
  - Init: create JWTService, load RBAC config
  - Routes: POST /auth/login, POST /auth/register, POST /auth/refresh, DELETE /auth/logout
- [x] **T-1.4.2** — Implement `internal/auth/jwt.go`:
  - `Claims` struct (UserID, TenantID, RoleID, Email + RegisteredClaims)
  - `TokenPair` struct (access, refresh, expires_in, type)
  - `GenerateTokenPair()` — HS256 signed, 15min access + 7d refresh
  - `ValidateAccessToken()` — parse + validate
  - `ValidateRefreshToken()` — parse + validate
- [x] **T-1.4.3** — Implement `internal/auth/password.go`:
  - `HashPassword(password)` — bcrypt hash (cost 12)
  - `VerifyPassword(hash, password)` — bcrypt compare
  - Password strength validation (min 8 chars, configurable)
- [x] **T-1.4.4** — Implement `internal/auth/middleware.go`:
  - `RequireAuth(level)` — middleware that extracts JWT from Authorization header or API key from X-API-Key
  - `ClaimsFromContext(ctx)` — extract claims from context
  - `RequirePermission(permission)` — middleware that checks RBAC permission
- [x] **T-1.4.5** — Implement `internal/auth/rbac.go`:
  - `HasPermission(ctx, permission)` — checks user's role permissions with wildcard matching
  - `LoadRole(roleID)` — load role from DB with in-memory cache (5 min TTL)
  - Permission constants: `PermAppView`, `PermAppDeploy`, `PermAppDelete`, `PermMemberManage`, etc.
- [x] **T-1.4.6** — Implement `internal/auth/apikey.go`:
  - `GenerateAPIKey()` — creates `dm_` prefixed key, returns key + hash
  - `ValidateAPIKey(key)` — hash + lookup in DB
  - `CreateAPIKey(userID, tenantID, name, scopes, expiry)` — store in DB
- [x] **T-1.4.7** — Implement first-run setup:
  - If no users exist in DB, create super admin user
  - Prompt for email/password on first startup (or use env vars MONSTER_ADMIN_EMAIL, MONSTER_ADMIN_PASSWORD)
  - Create default tenant "Platform"
  - Print login URL to stdout

### 1.5 REST API Skeleton

- [x] **T-1.5.1** — Implement `internal/api/module.go`:
  - API module implementing `core.Module`
  - ID: `api`, depends on `core.db`, `core.auth`
  - Init: create router, register routes
  - Start: start HTTP server on configured port (8443)
- [x] **T-1.5.2** — Implement `internal/api/router.go`:
  - Router setup with Go 1.22+ ServeMux pattern matching
  - `registerRoutes()` — all endpoints grouped by resource
  - `registerModuleRoutes()` — collect routes from all modules
  - CORS middleware (configurable origins)
  - Request logging middleware (slog)
  - Recovery middleware (panic → 500)
- [x] **T-1.5.3** — Implement `internal/api/helpers.go`:
  - `writeJSON(w, status, data)` — JSON response helper
  - `writeError(w, status, message)` — error response helper
  - `parseJSON(r, dest)` — parse request body into struct
  - `parsePagination(r)` — extract page, per_page, sort, order from query
  - `PaginatedResponse` struct
  - `realIP(r)` — extract real IP from X-Forwarded-For/X-Real-IP
- [x] **T-1.5.4** — Implement `internal/api/handlers/auth.go`:
  - `POST /api/v1/auth/login` — email + password → JWT pair
  - `POST /api/v1/auth/register` — create user + tenant (if registration open)
  - `POST /api/v1/auth/refresh` — refresh token → new JWT pair
  - `DELETE /api/v1/auth/logout` — invalidate refresh token
  - All handlers with proper error responses and audit logging
- [x] **T-1.5.5** — Implement `internal/api/handlers/apps.go` (basic):
  - `GET /api/v1/apps` — list apps for current tenant (paginated)
  - `POST /api/v1/apps` — create app (name, type, source_type)
  - `GET /api/v1/apps/:id` — get app detail
  - `DELETE /api/v1/apps/:id` — delete app
  - `POST /api/v1/apps/:id/restart` — restart container
  - `POST /api/v1/apps/:id/stop` — stop container
  - `POST /api/v1/apps/:id/start` — start container
- [x] **T-1.5.6** — Implement `internal/api/spa.go`:
  - Serve React SPA from `embed.FS`
  - Fallback to index.html for SPA routes (any non-API, non-WS path)
  - Proper Content-Type for static assets (js, css, svg, etc.)
- [x] **T-1.5.7** — Implement `internal/api/middleware/ratelimit.go`:
  - Token bucket rate limiter using BBolt
  - 100 req/min per IP for API endpoints
  - Configurable per-route overrides
  - `Retry-After` header on 429 response
- [x] **T-1.5.8** — Implement health endpoint:
  - `GET /health` — returns `{"status":"ok","version":"...","modules":{...}}`
  - Checks all module health statuses
  - Returns 503 if any critical module is down

### 1.6 Docker Integration (Basic)

- [x] **T-1.6.1** — Implement `internal/deploy/module.go`:
  - Deploy module implementing `core.Module`
  - ID: `deploy`, depends on `core.db`
  - Init: create Docker client, verify connection
  - Set `core.Docker` reference
- [x] **T-1.6.2** — Implement `internal/deploy/docker.go`:
  - `NewDockerManager(socketPath)` — create Docker client with API version negotiation
  - `CreateAndStartContainer(ctx, opts)` — pull image + create + start with labels and resource limits
  - `StopContainer(ctx, id, timeout)` — graceful stop
  - `RemoveContainer(ctx, id, force)` — remove with optional force
  - `RestartContainer(ctx, id)` — stop + start
  - `ListContainers(ctx, filters)` — list with label filters
  - `GetContainerLogs(ctx, id, tail, follow)` — log reader
  - `InspectContainer(ctx, id)` — full container info
  - `ContainerStats(ctx, id)` — resource usage stats
- [x] **T-1.6.3** — Implement `internal/deploy/deployer.go`:
  - `DeployImage(ctx, app, imageRef)` — deploy from Docker image directly
  - Creates container with monster.* labels
  - Connects to monster-network
  - Applies resource limits from tenant plan
  - Creates deployment record in DB
  - Emits `EventAppDeployed` event
- [x] **T-1.6.4** — Implement Docker network management:
  - `EnsureNetwork(ctx, name)` — create `monster-network` bridge if not exists
  - Auto-connect new containers to monster-network
  - Network cleanup on container removal

### 1.7 React UI Shell

- [x] **T-1.7.1** — Initialize React project:
  - `cd web && npm create vite@latest . -- --template react-ts`
  - Install core deps: `react@19 react-dom@19 react-router@7 @tanstack/react-query@5 zustand@5`
  - Install UI deps: `tailwindcss@4 @tailwindcss/vite lucide-react`
  - Install shadcn/ui: `npx shadcn@latest init` (New York style, slate base, CSS variables)
  - Add shadcn components: button, input, label, card, dialog, dropdown-menu, table, badge, toast, separator, tabs, avatar, skeleton, sheet, command, sonner
- [x] **T-1.7.2** — Setup Tailwind CSS 4.1:
  - Configure `@tailwindcss/vite` plugin in `vite.config.ts`
  - Create CSS theme with DeployMonster brand colors (Monster Green + Monster Purple)
  - Dark/light mode via class strategy with system preference detection
  - Persist theme choice in localStorage
- [x] **T-1.7.3** — Implement base layout:
  - `AppLayout.tsx` — sidebar + topbar + main content area
  - Sidebar: logo, navigation links (grouped by section), user avatar at bottom, theme toggle
  - Topbar: breadcrumbs, search (cmdk trigger), notifications bell, user dropdown
  - Responsive: sidebar collapses to icons on mobile, hamburger menu
- [x] **T-1.7.4** — Implement authentication pages:
  - `Login.tsx` — email/password form, "Remember me", SSO buttons, link to register
  - `Register.tsx` — name/email/password form (respects registration mode)
  - Auth store (Zustand): accessToken, user, tenant, login/logout actions
  - API client with automatic token refresh and 401 redirect
- [x] **T-1.7.5** — Implement Dashboard page:
  - App count card, container count card, server status card
  - Recent deployments list (last 5)
  - Quick action buttons: "Deploy New App", "Add Database"
  - Placeholder for charts (to be filled in metrics phase)
- [x] **T-1.7.6** — Implement Applications list page:
  - Table/card view toggle
  - Columns: name, type, status (badge), domain, last deployed, actions
  - Status badges: 🟢 Running, 🔴 Stopped, 🟡 Deploying, 🔵 Building
  - Actions dropdown: restart, stop, start, delete
  - "Deploy New App" button → placeholder deploy wizard
- [x] **T-1.7.7** — Implement App Detail page (basic):
  - Header: app name, status badge, domain link, action buttons
  - Tabs structure: Overview, Deployments, Logs, Environment, Settings
  - Overview tab: basic info, last deployment, container ID
  - Deployments tab: version history table
  - Settings tab: rename, delete (danger zone)
- [x] **T-1.7.8** — Implement API client layer:
  - `web/src/api/client.ts` — axios or fetch wrapper with base URL, auth header, error handling
  - `web/src/api/auth.ts` — login, register, refresh, logout
  - `web/src/api/apps.ts` — CRUD, restart, stop, start
  - TanStack Query hooks: `useApps()`, `useApp(id)`, `useCreateApp()`, etc.

### 1.8 Phase 1 Testing & Validation

- [x] **T-1.8.1** — Unit tests for `internal/core/` — module registry resolve, event bus pub/sub, config loading
- [x] **T-1.8.2** — Unit tests for `internal/auth/` — JWT generation/validation, password hash/verify, RBAC permission check
- [x] **T-1.8.3** — Unit tests for `internal/db/` — migration runner, SQLite CRUD operations, BBolt set/get with TTL
- [x] **T-1.8.4** — Integration test: full app bootstrap → login → list containers → stop/start
- [x] **T-1.8.5** — Build test: `make build` produces single binary, `make docker` produces image
- [x] **T-1.8.6** — Manual smoke test: run binary, open UI, login as admin, see container list

---

## Phase 2 — Ingress & SSL (v0.2.0)

**Goal**: Deploy container with monster.* labels → auto-discovered → routed with domain → SSL cert issued

### 2.1 Ingress Gateway

- [x] **T-2.1.1** — Implement `internal/ingress/module.go`:
  - Ingress module, ID: `ingress`, depends on `core.db`, `discovery`
  - Init: create route table, ACME manager
  - Start: launch HTTP (:80) and HTTPS (:443) servers
  - Stop: graceful shutdown with drain
- [x] **T-2.1.2** — Implement `internal/ingress/proxy.go`:
  - HTTP handler (:80): ACME HTTP-01 challenge handler + redirect all other to HTTPS
  - HTTPS handler (:443): route matching → middleware chain → reverse proxy
  - `createProxy(target)` — `httputil.ReverseProxy` with connection pooling
  - Proper error pages (502 Bad Gateway, 504 Gateway Timeout)
  - X-Forwarded-For, X-Real-IP, X-Forwarded-Proto header injection
- [x] **T-2.1.3** — Implement `internal/ingress/router.go`:
  - `RouteTable` with RWMutex for thread-safe reads
  - `Match(host, path, method)` — exact host > wildcard > longest path prefix
  - `Upsert(entry)` — add/update route, sort by priority
  - `Remove(host, pathPrefix)` — remove route
  - `matchHost(pattern, host)` — exact + `*.example.com` wildcard matching
- [x] **T-2.1.4** — Implement `internal/ingress/tls.go`:
  - TLS config with `GetCertificate` callback for dynamic cert loading
  - Cert cache in memory (map[domain]*tls.Certificate)
  - Fallback to self-signed cert for unknown domains
  - TLS 1.2 minimum, prefer TLS 1.3

### 2.2 SSL / ACME

- [x] **T-2.2.1** — Implement `internal/ingress/acme.go`:
  - `ACMEManager` using `github.com/go-acme/lego/v4`
  - `ObtainCertificate(domain)` — request cert via HTTP-01 challenge
  - `RenewCertificate(domain)` — renew before expiry (30 days)
  - Store certs encrypted in SQLite (ssl_certs table)
  - `GetCertificate(hello *tls.ClientHelloInfo)` — TLS callback, load from cache/DB
  - Auto-renewal goroutine: check all certs daily, renew expiring ones
  - Support staging (Let's Encrypt staging for testing)

### 2.3 Service Discovery

- [x] **T-2.3.1** — Implement `internal/discovery/module.go`:
  - Discovery module, ID: `discovery`, depends on `deploy`
  - Init: create watcher, label parser, health checker
  - Start: begin watching Docker events
- [x] **T-2.3.2** — Implement `internal/discovery/watcher.go`:
  - Watch Docker event stream filtered by `monster.enable=true` label
  - Handle: container.start → parse labels → register route
  - Handle: container.stop/die → deregister route
  - Handle: health_status events → update backend health
  - Reconnect on event stream error
- [x] **T-2.3.3** — Implement `internal/discovery/labels.go`:
  - `ParseLabels(labels map[string]string)` — extract monster.* labels into structured config
  - Parse router rules: `monster.http.routers.{name}.rule`
  - Parse service port: `monster.http.services.{name}.loadbalancer.server.port`
  - Parse middleware: `monster.http.routers.{name}.middlewares`
  - Parse TLS: `monster.http.routers.{name}.tls`
- [x] **T-2.3.4** — Implement `internal/discovery/health.go`:
  - Periodic health checks for all registered backends
  - HTTP health check (GET path, expect 2xx)
  - TCP health check (connection test)
  - Remove unhealthy backends from LB pool
  - Re-add when healthy again

### 2.4 Load Balancer

- [x] **T-2.4.1** — Implement `internal/ingress/lb/balancer.go`:
  - `Strategy` interface with `Next(backends, request) string`
  - Factory: `NewStrategy(name) Strategy`
- [x] **T-2.4.2** — Implement `roundrobin.go` — atomic counter
- [x] **T-2.4.3** — Implement `leastconn.go` — connection counting with mutex
- [x] **T-2.4.4** — Implement `iphash.go` — FNV-32a hash of client IP
- [x] **T-2.4.5** — Implement `weighted.go` — weighted round-robin for canary

### 2.5 Ingress Middleware

- [x] **T-2.5.1** — Implement `internal/ingress/middleware/ratelimit.go` — per-route rate limiting
- [x] **T-2.5.2** — Implement `internal/ingress/middleware/cors.go` — configurable CORS headers
- [x] **T-2.5.3** — Implement `internal/ingress/middleware/compress.go` — gzip response compression
- [x] **T-2.5.4** — Implement `internal/ingress/middleware/headers.go` — custom header add/remove

### 2.6 Domain Management

- [x] **T-2.6.1** — Implement `internal/api/handlers/domains.go`:
  - `POST /api/v1/domains` — add domain to app, trigger SSL cert request
  - `GET /api/v1/domains` — list domains with SSL status
  - `DELETE /api/v1/domains/:id` — remove domain, cleanup cert
  - `POST /api/v1/domains/:id/verify` — DNS verification check
- [x] **T-2.6.2** — Implement auto-subdomain generation:
  - When app created → auto-assign `{app-name}.deploy.monster` (or configured suffix)
  - Store in domains table with type=auto

### 2.7 Phase 2 UI Updates

- [x] **T-2.7.1** — Domain management page in React:
  - List domains with SSL status badge (🟢 Active, 🟡 Pending, 🔴 Expired)
  - Add custom domain form with DNS instructions
  - Domain verification button
- [x] **T-2.7.2** — App detail: add Domains tab showing linked domains + SSL status

### 2.8 Phase 2 Testing

- [x] **T-2.8.1** — Unit tests: route matching (exact, wildcard, path prefix), label parsing
- [x] **T-2.8.2** — Unit tests: LB strategies (round-robin distribution, IP hash consistency)
- [x] **T-2.8.3** — Integration test: deploy labeled container → auto-route → HTTP request returns response
- [x] **T-2.8.4** — Integration test: add domain → SSL cert issued → HTTPS works

---

## Phase 3 — Build & Deploy (v0.3.0)

**Goal**: Push to GitHub → webhook → auto-build → deploy → live with domain

### 3.1 Build Engine

- [x] **T-3.1.1** — Implement `internal/build/module.go` — Build module, depends on `deploy`
- [x] **T-3.1.2** — Implement `internal/build/detector.go` — project type detection (14 types, file-based checks)
- [x] **T-3.1.3** — Implement `internal/build/templates/` — embedded Dockerfile templates for: nodejs, nextjs, vite, nuxt, go, python, rust, php, java, dotnet, ruby, static
- [x] **T-3.1.4** — Implement `internal/build/generator.go` — `GenerateDockerfile(projType, dir)` — select and render template
- [x] **T-3.1.5** — Implement `internal/build/builder.go`:
  - `Build(ctx, opts, logWriter)` — full pipeline: clone → detect → generate → docker build → tag
  - Stream build output to logWriter (for WebSocket streaming)
  - Build timeout enforcement (configurable, default 30 min)
  - Cleanup work directory after build
- [x] **T-3.1.6** — Implement `internal/build/git.go`:
  - `GitClone(ctx, url, branch, token, sshKey)` — clone with depth=1
  - Token injection into HTTPS URLs
  - SSH key-based clone support
  - Extract commit SHA, message, author after clone
- [x] **T-3.1.7** — Implement build worker pool:
  - Configurable max concurrent builds (default: 5)
  - Queue excess builds
  - Build status tracking (queued → building → completed/failed)

### 3.2 Git Source Manager

- [x] **T-3.2.1** — Implement `internal/gitsources/module.go` — Git source module
- [x] **T-3.2.2** — Implement `internal/gitsources/providers/provider.go` — GitProvider interface:
  - `ListRepos(ctx, page, perPage)` — list user's repositories
  - `ListBranches(ctx, repoFullName)` — list branches
  - `GetRepoInfo(ctx, repoFullName)` — repo metadata
  - `CreateWebhook(ctx, repoFullName, url, secret, events)` — auto-register webhook
  - `DeleteWebhook(ctx, repoFullName, webhookID)` — cleanup
- [x] **T-3.2.3** — Implement `internal/gitsources/providers/github.go` — GitHub API v3 integration
- [x] **T-3.2.4** — Implement `internal/gitsources/providers/gitlab.go` — GitLab API v4 integration
- [x] **T-3.2.5** — Implement `internal/gitsources/providers/gitea.go` — Gitea API integration
- [x] **T-3.2.6** — Implement `internal/gitsources/providers/generic.go` — generic git clone (SSH/HTTPS, no API)
- [x] **T-3.2.7** — Implement `internal/gitsources/oauth.go` — OAuth2 flow handler (GitHub, GitLab, Bitbucket callbacks)
- [x] **T-3.2.8** — Implement Git Source API endpoints:
  - `GET/POST/DELETE /api/v1/git/sources`
  - `GET /api/v1/git/sources/:id/repos` — list repositories
  - `GET /api/v1/git/sources/:id/repos/:repo/branches`
  - `GET /auth/callback/:provider` — OAuth callback

### 3.3 Webhook System

- [x] **T-3.3.1** — Implement `internal/webhooks/module.go` — Webhook module
- [x] **T-3.3.2** — Implement `internal/webhooks/receiver.go`:
  - `POST /hooks/v1/{webhookID}` — universal receiver
  - Signature verification (HMAC-SHA256 for GitHub/Gitea/Gogs, token for GitLab)
  - Provider auto-detection from headers
  - Branch filtering, event filtering
  - Dispatch to build pipeline (channel-based, async)
- [x] **T-3.3.3** — Implement `internal/webhooks/parsers/`:
  - `github.go` — parse X-GitHub-Event push/tag/release payloads
  - `gitlab.go` — parse X-Gitlab-Event payloads
  - `gitea.go` — parse X-Gitea-Event payloads
  - `bitbucket.go` — parse X-Event-Key payloads
  - `generic.go` — generic JSON payload parser
- [x] **T-3.3.4** — Implement webhook auto-registration:
  - When app created with git source → auto-create webhook via provider API
  - Store webhook secret in secrets vault
  - Webhook logs: record every delivery (received_at, processed_at, status)
- [x] **T-3.3.5** — Implement webhook redeliver API: `POST /api/v1/webhooks/:id/redeliver/:log_id`

### 3.4 Deploy Strategies

- [x] **T-3.4.1** — Implement deploy recreate strategy — stop old → start new (later merged into `internal/deploy`)
- [x] **T-3.4.2** — Implement deploy rolling strategy — gradual replacement (later merged into `internal/deploy`)
- [x] **T-3.4.3** — Implement `internal/deploy/rollback.go`:
  - Store last 10 deployment versions
  - `Rollback(ctx, appID, version)` — redeploy specific version's image
  - Rollback creates a new deployment record (for audit trail)

### 3.5 Environment Variables

- [x] **T-3.5.1** — Implement env var management API:
  - `GET /api/v1/apps/:id/env` — get env vars (secrets masked)
  - `PUT /api/v1/apps/:id/env` — update env vars (encrypted at rest)
  - Env var inheritance: project-level inherited by all apps
  - `${SECRET:name}` syntax supported (resolved at deploy time)

### 3.6 Phase 3 UI Updates

- [x] **T-3.6.1** — Deploy Wizard (multi-step):
  - Step 1: Choose source (Git / Docker Image / Marketplace)
  - Step 2: If Git → connect provider (OAuth) → select repo + branch
  - Step 2: If Image → enter image reference
  - Step 3: Configure env vars, port
  - Step 4: Domain selection (auto or custom)
  - Step 5: Review → Deploy (live build log)
- [x] **T-3.6.2** — Git Sources page: connected providers, add new (OAuth flow), repo browser
- [x] **T-3.6.3** — Build log viewer: real-time WebSocket streaming, ANSI color support
- [x] **T-3.6.4** — App detail: Deployments tab with version history, commit info, rollback button
- [x] **T-3.6.5** — App detail: Environment tab with key-value editor + Monaco editor for bulk edit
- [x] **T-3.6.6** — Webhooks page: list per app, delivery logs, redeliver button

### 3.7 WebSocket Endpoints

- [x] **T-3.7.1** — Implement `internal/api/ws/logs.go` — real-time container log streaming
- [x] **T-3.7.2** — Implement `internal/api/ws/exec.go` — container exec terminal (xterm.js ↔ WebSocket ↔ docker exec)
- [x] **T-3.7.3** — Implement `internal/api/ws/events.go` — global event stream (deploy status, alerts)
- [x] **T-3.7.4** — Implement build log streaming: WebSocket endpoint that streams build output in real-time

---

## Phase 4 — Docker Compose & Image Deploy (v0.4.0)

**Goal**: Upload docker-compose.yml → deploy multi-service stack → manage per-service

- [x] **T-4.1** — Implement `internal/compose/parser.go` — parse Compose v2/v3 YAML into Go structs
- [x] **T-4.2** — Implement `internal/compose/validator.go` — validate parsed compose (images exist, ports valid, no conflicts)
- [x] **T-4.3** — Implement `internal/compose/interpolate.go` — `${VAR:-default}` variable interpolation, .env file support
- [x] **T-4.4** — Implement `internal/compose/deployer.go` — ordered multi-service deploy (respect depends_on), network creation, volume creation, label injection
- [x] **T-4.5** — Implement `internal/compose/converter.go` — compose service → DeployMonster Application model
- [x] **T-4.6** — Implement Compose API: `POST/GET/PATCH/DELETE /api/v1/stacks`, `POST /stacks/:id/redeploy`, `POST /stacks/:id/services/:svc/scale`
- [x] **T-4.7** — Implement Docker Image direct deploy: `POST /api/v1/apps` with `source_type=image` — pull any image, deploy with config
- [x] **T-4.8** — UI: Compose Stacks page, stack detail with service list, per-service logs/restart/scale
- [x] **T-4.9** — UI: Docker Image deploy flow in Deploy Wizard (enter image → configure → deploy)

---

## Phase 5 — Resource & Monitoring (v0.5.0)

- [x] **T-5.1** — Implement `internal/resource/collector.go` — server metrics (CPU, RAM, disk, network) via /proc parsing
- [x] **T-5.2** — Implement container metrics collection via Docker stats API
- [x] **T-5.3** — Implement HTTP/ingress metrics (request count, latency percentiles, error rate, status codes)
- [x] **T-5.4** — Implement metrics storage: 1-second ring buffer → 1-minute → 1-hour → 1-day rollups in SQLite
- [x] **T-5.5** — Implement `internal/resource/alerts.go` — threshold-based alerts with duration, severity, notification dispatch
- [x] **T-5.6** — Implement `internal/notifications/` — email (SMTP), Slack (webhook), Discord (webhook), Telegram (bot API)
- [x] **T-5.7** — Implement metrics API: `GET /api/v1/servers/:id/metrics`, `GET /api/v1/apps/:id/metrics` with time range query
- [x] **T-5.8** — UI: Server dashboard (CPU/RAM/disk gauges + history charts via Recharts)
- [x] **T-5.9** — UI: App metrics tab (response time, request rate, error rate, resource usage vs limits)
- [x] **T-5.10** — UI: Alert center in dashboard (active alerts, history, acknowledge)

---

## Phase 6 — Database & Backup (v0.6.0)

- [x] **T-6.1** — Implement `internal/database/provisioner.go` — managed DB lifecycle (create volume → deploy container → health check → register)
- [x] **T-6.2** — Implement `internal/database/engines/postgres.go` — PostgreSQL 14-17 provisioning with connection string generation
- [x] **T-6.3** — Implement `internal/database/engines/mysql.go` — MySQL 8.x provisioning
- [x] **T-6.4** — Implement `internal/database/engines/redis.go` — Redis 7.x provisioning
- [x] **T-6.5** — Implement `internal/backup/volume.go` — Docker volume tar + gzip snapshot
- [x] **T-6.6** — Implement `internal/backup/database.go` — pg_dump / mysqldump / redis BGSAVE wrappers
- [x] **T-6.7** — Local filesystem backup target (removed in dead-code sweep; local backups handled directly in `internal/backup`)
- [x] **T-6.8** — Implement `internal/backup/storage/s3.go` — S3/S3-compatible upload (MinIO, R2, Backblaze)
- [x] **T-6.9** — Implement `internal/backup/scheduler.go` — cron-based backup scheduling with retention policy
- [x] **T-6.10** — Implement `internal/backup/encryption.go` — AES-256-GCM backup encryption
- [x] **T-6.11** — Implement backup/database API endpoints
- [x] **T-6.12** — UI: Database management page (create, connection string, backups, logs)
- [x] **T-6.13** — UI: Backup management page (list, create, restore, storage targets config)

---

## Phase 7 — Secret Vault & Registration (v0.7.0)

- [x] **T-7.1** — Implement `internal/secrets/vault.go` — AES-256-GCM encrypt/decrypt with Argon2id key derivation
- [x] **T-7.2** — Implement `internal/secrets/store.go` — CRUD with versioning (last 10 versions per secret)
- [x] **T-7.3** — Implement `internal/secrets/resolver.go` — `${SECRET:name}` resolution with scope chain
- [x] **T-7.4** — Implement `internal/secrets/scoping.go` — global → tenant → project → app scope resolution
- [x] **T-7.5** — Implement secret rotation: change value → auto-redeploy affected containers
- [x] **T-7.6** — Implement .env import/export: `POST /api/v1/secrets/import`, `GET /api/v1/secrets/export`
- [x] **T-7.7** — Implement secret masking in all log outputs (build logs, deploy logs, API responses)
- [x] **T-7.8** — Implement registration modes: open, invite_only, approval, disabled
- [x] **T-7.9** — Implement invite system: create invite → email → accept with token → create user
- [x] **T-7.10** — Implement approval queue: register → pending → admin approve/reject
- [x] **T-7.11** — Implement SSO/OAuth login: Google, GitHub, GitLab providers
- [x] **T-7.12** — Implement 2FA: TOTP setup, verification, backup codes
- [x] **T-7.13** — Implement onboarding wizard (first login flow — 5 steps)
- [x] **T-7.14** — UI: Secrets management page (list, create, edit, versions, diff view, bulk import)
- [x] **T-7.15** — UI: Registration settings page (admin panel — mode selector, invite management, approval queue)
- [x] **T-7.16** — UI: Auth pages (forgot password, reset password, accept invite, pending approval, 2FA)

---

## Phase 8 — VPS Providers & Remote Servers (v0.8.0)

- [x] **T-8.1** — Implement `internal/vps/providers/provider.go` — VPS Provider interface
- [x] **T-8.2** — Implement `internal/vps/providers/hetzner.go` — Hetzner Cloud full API (create, list, delete, resize, snapshot)
- [x] **T-8.3** — Implement `internal/vps/providers/digitalocean.go` — DigitalOcean API
- [x] **T-8.4** — Implement `internal/vps/providers/vultr.go` — Vultr API
- [x] **T-8.5** — Implement `internal/vps/providers/custom.go` — existing server via SSH (no provisioning API)
- [x] **T-8.6** — Implement `internal/vps/bootstrap.go` — cloud-init template, SSH bootstrap (install Docker + agent)
- [x] **T-8.7** — Implement `internal/vps/ssh.go` — SSH connection pool using `golang.org/x/crypto/ssh`
- [x] **T-8.8** — Implement server provisioning flow: provider API → wait boot → SSH bootstrap → join Swarm → register
- [x] **T-8.9** — Implement VPS API endpoints per SPECIFICATION.md §18.2
- [x] **T-8.10** — UI: VPS Providers page (connect, provision, connect existing, cost overview)
- [x] **T-8.11** — UI: Server provision wizard (provider → region → size → name → create)

---

## Phase 9 — DNS & Topology (v0.9.0)

- [x] **T-9.1** — Implement `internal/dns/providers/cloudflare.go` — Cloudflare API v4 (create/update/delete records, proxy toggle)
- [x] **T-9.2** — Implement `internal/dns/sync.go` — DNS sync queue: domain change → create/update DNS record → verify propagation
- [x] **T-9.3** — Implement auto-subdomain: app created → auto DNS A record
- [x] **T-9.4** — Implement wildcard DNS setup for auto-subdomains
- [x] **T-9.5** — UI: Topology View (React Flow):
  - Sidebar palette with draggable node types (App, DB, Cache, Volume, Domain, Globe)
  - Canvas with connection drawing between nodes
  - Node click → properties panel (logs, metrics, settings)
  - Connection dialog (auto env var injection)
  - Status indicators (colored borders/badges)
  - Minimap, zoom, auto-layout
  - Import from docker-compose.yml
  - Export as YAML/SVG

---

## Phase 10 — Marketplace (v0.10.0)

- [x] **T-10.1** — Implement `internal/marketplace/loader.go` — parse template YAML manifests (embedded + external)
- [x] **T-10.2** — Implement `internal/marketplace/wizard.go` — JSON Schema → config form data
- [x] **T-10.3** — Implement `internal/marketplace/deployer.go` — template config → compose YAML → deploy stack
- [x] **T-10.4** — Implement `internal/marketplace/search.go` — full-text search index over template names, descriptions, tags
- [x] **T-10.5** — Implement `internal/marketplace/registry.go` — community template sync from GitHub
- [x] **T-10.6** — Create 20+ initial marketplace templates: WordPress, Ghost, Strapi, Plausible, Umami, MinIO, Gitea, n8n, Uptime Kuma, Vaultwarden, Nextcloud, Metabase, PostgreSQL, MySQL, Redis, MongoDB, Ollama, Open WebUI, Code-Server, Meilisearch
- [x] **T-10.7** — Implement Marketplace API endpoints per SPECIFICATION.md §18.2
- [x] **T-10.8** — UI: Marketplace page (category grid, search, filter, template detail, one-click deploy wizard)

---

## Phase 11 — Team Management & RBAC (v0.11.0)

- [x] **T-11.1** — Implement Team Management API: invite, bulk invite, remove member, change role, activity log
- [x] **T-11.2** — Implement custom role CRUD: create role with granular permissions, assign to members
- [x] **T-11.3** — Implement project-level member overrides: per-project role assignment
- [x] **T-11.4** — Implement team activity feed: who deployed what, when, from which commit
- [x] **T-11.5** — Implement session management: active sessions list, force logout
- [x] **T-11.6** — Implement 2FA enforcement toggle (admin forces all members to enable 2FA)
- [x] **T-11.7** — UI: Team Admin Panel (/team/*) — full panel with members, roles, projects, billing
- [x] **T-11.8** — UI: Team Members page (list, invite, role change, remove, activity, 2FA status)
- [x] **T-11.9** — UI: Custom Roles page (create with permission checkboxes, assign)
- [x] **T-11.10** — UI: Panel switcher (top-left dropdown: Super Admin / Team / Customer)

---

## Phase 12 — Multi-Node & Swarm (v0.12.0)

- [x] **T-12.1** — Implement `internal/swarm/manager.go` — Docker Swarm init, join token generation
- [x] **T-12.2** — Implement `internal/swarm/agent.go` — agent mode (`--agent` flag), metrics reporting
- [x] **T-12.3** — Implement `internal/swarm/placement.go` — node labels, placement constraints
- [x] **T-12.4** — Implement `internal/swarm/network.go` — overlay network management
- [x] **T-12.5** — Implement multi-node deploy: deploy as Swarm service with replicas
- [x] **T-12.6** — UI: Cluster topology map (visual node layout with container placement)

---

## Phase 13 — Billing & Pay-Per-Usage (v0.13.0)

- [x] **T-13.1** — Implement `internal/billing/plans.go` — plan definitions, limit enforcement
- [x] **T-13.2** — Implement `internal/billing/metering.go` — 60-second Docker stats collection → hourly rollup
- [x] **T-13.3** — Implement `internal/billing/quotas.go` — Docker cgroups enforcement, soft/hard limits
- [x] **T-13.4** — Implement `internal/billing/invoicing.go` — monthly invoice generation with line items
- [x] **T-13.5** — Implement `internal/billing/stripe.go` — Stripe subscriptions, metered billing, customer portal, webhooks
- [x] **T-13.6** — Implement billing API endpoints per SPECIFICATION.md §18.2
- [x] **T-13.7** — UI: Customer billing page (plan, usage bars, invoices, upgrade/downgrade, payment methods)
- [x] **T-13.8** — UI: Admin billing settings (plans CRUD, pricing, Stripe config, revenue dashboard)
- [x] **T-13.9** — UI: Public pricing page

---

## Phase 14 — Enterprise & White-Label (v0.14.0)

- [x] **T-14.1** — Implement `internal/enterprise/whitelabel.go` — branding engine (logo, colors, domain, emails, copyright)
- [x] **T-14.2** — Implement white-label in React: dynamic logo, colors from API, custom title, hide "Powered by"
- [x] **T-14.3** — Implement `internal/enterprise/reseller.go` — reseller CRUD, wholesale pricing, customer isolation
- [x] **T-14.4** — Implement `internal/enterprise/provisioning.go` — Enterprise tenant provisioning API
- [x] **T-14.5** — Implement `internal/enterprise/compliance.go` — GDPR tools (data export, right to erasure)
- [x] **T-14.6** — Implement `internal/enterprise/ha.go` — Litestream SQLite replication config
- [x] **T-14.7** — Implement `internal/enterprise/license.go` — license key validation
- [x] **T-14.8** — Implement `internal/enterprise/integrations/whmcs.go` — WHMCS provisioning bridge
- [x] **T-14.9** — Implement `internal/enterprise/integrations/prometheus.go` — `/metrics` OpenMetrics endpoint
- [x] **T-14.10** — UI: Admin branding settings page (logo upload, color picker, domain config)
- [x] **T-14.11** — UI: Reseller management (admin panel)

---

## Phase 15 — Polish & Launch (v1.0.0)

### 15.1 MCP Server

- [x] **T-15.1.1** — Implement `internal/mcp/server.go` — MCP protocol handler
- [x] **T-15.1.2** — Implement MCP tools: deploy_app, list_apps, get_app_status, scale_app, view_logs, create_database, add_domain, create_backup, provision_server, marketplace_deploy
- [x] **T-15.1.3** — Implement MCP resources: monster://apps, monster://servers, monster://metrics, monster://logs

### 15.2 CLI Tool

- [x] **T-15.2.1** — Implement CLI commands per SPECIFICATION.md §22 using `cobra` or stdlib `flag`
- [x] **T-15.2.2** — CLI subcommands: `serve`, `deploy`, `apps`, `stack`, `image`, `marketplace`, `git`, `vps`, `db`, `domain`, `backup`, `cluster`, `config`
- [x] **T-15.2.3** — CLI interactive mode: `deploymonster deploy .` auto-detects project, prompts for config

### 15.3 Audit & Logging

- [x] **T-15.3.1** — Ensure every state-changing API endpoint has audit logging
- [x] **T-15.3.2** — Implement audit log API: `GET /api/v1/admin/audit-log` with search, filter, export
- [x] **T-15.3.3** — UI: Audit log page (admin panel — searchable, filterable table)

### 15.4 Documentation

- [x] **T-15.4.1** — Write `README.md` with quick start, features, screenshots
- [x] **T-15.4.2** — Write `docs/getting-started.md` — install → first deploy in 5 minutes
- [x] **T-15.4.3** — Write `docs/architecture.md` — high-level architecture overview
- [x] **T-15.4.4** — Write `docs/api-reference.md` — auto-generated from endpoint map
- [x] **T-15.4.5** — Write `docs/deployment-guide.md` — production deployment best practices

### 15.5 Quality & Performance

- [x] **T-15.5.1** — Run `golangci-lint` — fix all warnings
- [x] **T-15.5.2** — Achieve >70% test coverage on critical paths (auth, deploy, ingress, billing)
- [x] **T-15.5.3** — Load test: 100 concurrent requests, p95 < 100ms
- [x] **T-15.5.4** — Load test: proxy 1000 req/s with < 5ms overhead
- [x] **T-15.5.5** — Binary size check: < 50 MB
- [x] **T-15.5.6** — Memory test: idle < 100 MB, 50 containers < 500 MB
- [x] **T-15.5.7** — Startup time check: cold start < 3 seconds

### 15.6 Release

- [x] **T-15.6.1** — Create `scripts/install.sh` — curl | bash installer
- [x] **T-15.6.2** — GitHub Actions CI: test → lint → build → release
- [x] **T-15.6.3** — Create GitHub Release with changelog, binaries, Docker image
- [x] **T-15.6.4** — ~~Push Docker image to Docker Hub~~ → GHCR via GoReleaser (configured)
- [x] **T-15.6.5** — ~~Deploy docs site~~ → Out of scope (separate project)
- [x] **T-15.6.6** — ~~Public announcement~~ → Out of scope (separate project)

---

## Task Summary

| Phase | Tasks | Focus |
|-------|-------|-------|
| Phase 1 | ~50 | Foundation: core, DB, auth, API, Docker, React shell |
| Phase 2 | ~20 | Ingress: reverse proxy, SSL, discovery, LB |
| Phase 3 | ~25 | Build: git sources, webhooks, build pipeline, deploy strategies |
| Phase 4 | ~9 | Compose: YAML parser, multi-service deploy, image deploy |
| Phase 5 | ~10 | Monitoring: metrics collection, alerts, notification channels |
| Phase 6 | ~13 | Data: managed DBs, backup engine, S3 storage |
| Phase 7 | ~16 | Security: secret vault, registration modes, SSO, 2FA |
| Phase 8 | ~11 | Infrastructure: VPS providers, SSH bootstrap, remote servers |
| Phase 9 | ~5 | Network: DNS sync, topology canvas |
| Phase 10 | ~8 | Marketplace: templates, wizard, deploy, community sync |
| Phase 11 | ~10 | Teams: RBAC, custom roles, team panel, activity |
| Phase 12 | ~6 | Cluster: Swarm, agent mode, multi-node |
| Phase 13 | ~9 | Billing: plans, metering, Stripe, invoices |
| Phase 14 | ~11 | Enterprise: white-label, reseller, WHMCS, GDPR |
| Phase 15 | ~20 | Launch: MCP, CLI, audit, docs, testing, release |
| **Total** | **~223** | |

---

*Every task maps to a section in SPECIFICATION.md and a pattern in IMPLEMENTATION.md. When executing a task, reference both documents. Each task should result in a working, testable increment.*
