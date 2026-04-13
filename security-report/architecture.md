# Architecture Map — DeployMonster

## Tech Stack

| Layer | Technology | Version |
|-------|------------|---------|
| **Backend** | Go (modular monolith) | 1.26.1 (toolchain go1.26.2) |
| **Frontend** | React + TypeScript + Vite | React 19.2.4, Vite 8.0.5, TypeScript 5.9.3 |
| **Primary Database** | SQLite (pure Go) | modernc.org/sqlite v1.48.0 |
| **KV Store** | BBolt (embedded) | go.etcd.io/bbolt v1.4.3 |
| **Future DB** | PostgreSQL driver | jackc/pgx/v5 v5.9.1 |
| **Container Runtime** | Docker client | github.com/docker/docker v28.5.2+incompatible |
| **Auth** | JWT + API Keys | golang-jwt/jwt/v5 v5.3.1 |
| **WebSocket** | gorilla/websocket | v1.5.3 |
| **Password Hashing** | bcrypt | golang.org/x/crypto v0.49.0 |
| **Config** | YAML | gopkg.in/yaml.v3 v3.0.1 |
| **Observability** | OpenTelemetry | v1.43.0 |
| **State Management** | Zustand (frontend) | v5.0.12 |
| **UI Framework** | Tailwind CSS + shadcn/ui | v4.2.2 |
| **Routing** | React Router | v7.13.2 |

## Module Architecture (20 modules)

```
cmd/deploymonster/main.go
  └── _ blank imports all 20 modules (auto-register via init())

Modules (dependency-ordered via topological sort on Dependencies()):
├── core.db          — SQLite + BBolt initialization
├── core.auth        — JWT service, password hashing (bcrypt cost 13)
├── core.config      — YAML config + env override (MONSTER_* prefix)
├── core.events      — In-process pub/sub EventBus (exact/prefix/wildcard)
├── api              — http.ServeMux, 70+ handlers, full middleware chain
├── ingress          — Reverse proxy, load balancing, ACME, SSL termination
├── billing          — Stripe integration, usage metering, plans
├── deploy           — Container lifecycle (create/start/stop/remove/restart)
├── build            — Language detection (14 langs), Dockerfile generation
├── topology         — Visual canvas topology composer (@xyflow/react)
├── backup           — Backup orchestration, retention management
├── database         — DB module (coordinates db + core.db)
├── db               — SQLite/PostgreSQL implementations of core.Store
├── discovery        — Agent/master discovery
├── dns              — DNS provider registry (Cloudflare, Route53)
├── enterprise       — Enterprise features, integrations
├── gitsources       — Git provider registry (GitHub, GitLab, Gitea, Bitbucket)
├── marketplace      — App marketplace templates
├── mcp              — Model Context Protocol endpoint
├── notifications    — Multi-channel sender (email/Slack/Discord/Telegram/webhook)
├── resource         — Resource tracking and limits
├── secrets          — Encrypted secret management (AES-256-GCM, key versioning)
├── swarm            — Agent communication, distributed coordination
├── vps              — VPS provisioner registry (Hetzner, DigitalOcean, Vultr)
└── webhooks         — Event webhook delivery (HMAC-signed, retry logic)
```

### Key Interfaces (internal/core/interfaces.go)

- `core.Store` — 12 sub-stores: Tenant, User, App, Deployment, Domain, Project, Role, Audit, Secret, Invite, UsageRecord, Backup
- `core.ContainerRuntime` — Docker operations (CreateAndStart, Stop, Remove, Restart, Logs, Exec, Stats, ImagePull)
- `core.BoltStorer` — BBolt KV (Set, Get, BatchSet, Delete, List, Close) + API key lookup
- `core.EventBus` — pub/sub with exact/prefix/wildcard matching on eventType
- `core.Services` — Factory registry: DNS providers, backup storages, VPS provisioners, git providers
- `core.DNSProvider`, `core.GitProvider`, `core.BackupStorage`, `core.VPSProvisioner` — pluggable provider interfaces

## API Layer Summary

### Router (internal/api/router.go)

- Go 1.22+ `http.ServeMux` with `METHOD /path` pattern syntax
- Entry point: `Router.Handler()` wraps all routes in global middleware chain

### Middleware Chain (per-request order)

```
RequestID → GracefulShutdown → GlobalRateLimit → SecurityHeaders → APIMetrics
→ APIVersion → BodyLimit(10MB) → Timeout(30s) → Recovery → RequestLogger
→ CORS → CSRFProtect → IdempotencyMiddleware → AuditLog
```

### Route Categories

- **Public**: `/api/v1/auth/*` (login, register, refresh), `/hooks/*` (webhooks), `/api/v1/health`
- **Protected** (RequireAuth + TenantRateLimiter): all `/api/v1/*` beyond public ones
- **SPA fallback**: all non-API routes serve embedded React app
- **WebSocket**: `/api/v1/ws` (server-sent events / agent communication)

### API Handlers (internal/api/handlers/)

70+ handlers covering: apps, deployments, domains, projects, teams, secrets, backups, builds, logs, metrics, topology, billing, admin, agents, notifications, webhooks, git sources, certificates, settings

## Authentication / Authorization

### Auth Mechanisms

1. **JWT Bearer Token** — `Authorization: Bearer <token>`
   - Algorithm: HS256
   - Access token expiry: 15 minutes
   - Refresh token expiry: 7 days
   - Claims: UserID, TenantID, RoleID, Email, JTI
   - Storage: httpOnly cookie (`dm_access`) + Authorization header
   - Revocation: JTI stored in BBolt, previousKeys with RotationGracePeriod (1h)

2. **API Key** — `X-API-Key: dm_<prefix><secret>`
   - Format: `dm_` prefix + 12 hex chars (15 chars total, upgraded from 8)
   - Storage: SHA-256 hash in BBolt (per-tenant isolation)
   - Lookup: prefix-based retrieval via BoltStorer.GetAPIKeyByPrefix()

3. **Cookie Auth** — `dm_access` httpOnly cookie (SameSite=Strict)
   - Same validation as JWT bearer

### Auth Levels

- `AuthNone` — public endpoints
- `AuthAPIKey` — API key only
- `AuthJWT` — JWT required
- `AuthAdmin` — tenant admin role
- `AuthSuperAdmin` — platform admin

### RBAC

- Roles stored with permissions JSON array
- Wildcard `"*"` grants all permissions
- Permission checks via `auth.CheckPermission()`

### Password Security

- bcrypt cost: 13 (upgraded from 12)
- Argon2id available for key derivation

## Data Layer Summary

### SQLite (default)

- Driver: `modernc.org/sqlite` (pure Go, no CGO)
- File: configured via `MONSTER_DB_PATH` or `database.path` in monster.yaml
- Mode: WAL enabled for concurrent reads
- Snapshot backup via `VACUUM INTO` (DBSnapshotter interface)

### BBolt (embedded KV)

- 30+ buckets for: config, state, metrics, API keys, webhook secrets, JWT revocation, rate limiters, sessions
- Used for: API key lookup, webhook HMAC secrets, JWT JTI revocation list, idempotency keys
- TTL support for time-limited entries

### Data Access

- All data access through `core.Store` interface — never use concrete DB types directly
- `core.Database` wraps both `*sql.DB` (SQLite) and `BoltStorer` (BBolt)
- PostgreSQL support planned via alternate `Store` implementation (pgx driver already in go.mod)

## External Integrations

### Git Providers (internal/gitsources/providers/)

- GitHub (`/internal/gitsources/providers/github.go`)
- GitLab (`/internal/gitsources/providers/gitlab.go`)
- Gitea (`/internal/gitsources/providers/gitea.go`)
- Bitbucket (`/internal/gitsources/providers/bitbucket.go`)
- Interface: `core.GitProvider` — ListRepos, ListBranches, GetRepoInfo, CreateWebhook, DeleteWebhook

### DNS Providers (internal/dns/providers/)

- Cloudflare (`/internal/dns/providers/cloudflare.go`)
- Route53 (planned)
- Interface: `core.DNSProvider` — CreateRecord, UpdateRecord, DeleteRecord, Verify

### Notification Channels (internal/notifications/)

- Email (SMTP)
- Slack
- Discord
- Telegram
- Webhook (generic outbound)
- Interface: `core.NotificationSender.Send()`

### Billing (internal/billing/)

- Stripe integration (`stripe.go`, `stripe_webhook.go`)
- Webhook endpoint: `POST /hooks/v1/stripe` (HMAC verified)
- Usage metering for per-plan limits
- Plans: free, starter, pro, enterprise

### VPS Providers (internal/vps/)

- Hetzner, DigitalOcean, Vultr (interface defined, actual implementations in enterprise)
- Interface: `core.VPSProvisioner` — ListRegions, ListSizes, Create, Delete, Status

### Container Registry

- GHCR (GitHub Container Registry) only — no Docker Hub
- Image pull via Docker client

### Webhooks (internal/webhooks/)

- Event webhooks: `POST /api/v1/webhooks` + `POST /hooks/v1/{webhookID}`
- Delivery: HMAC-SHA256 signed requests, retry with exponential backoff
- Stripe webhooks: `POST /hooks/v1/stripe` with signature verification
- Inbound receiver: `POST /hooks/v1/{webhookID}` with HMAC verification

### Secrets (internal/secrets/)

- Vault-style encryption: AES-256-GCM
- Master key from `MONSTER_SECRET` config
- Key rotation via `rotate-keys` CLI command
- Per-secret salt, versioned values
- Scope: global, tenant, project, app

## Frontend Tech Stack (web/)

### Core

- **React 19.2.4** + TypeScript 5.9.3
- **Vite 8.0.5** build tool
- **React Router v7.13.2** (lazy-loaded pages)
- **Tailwind CSS 4.2.2** + `@tailwindcss/vite`
- **Zustand 5.0.12** — state management (5 stores)
- **@xyflow/react 12.10.2** — topology canvas

### UI

- shadcn/ui patterns (class-variance-authority, clsx, tailwind-merge)
- Lucide React 1.7.0 icons
- tw-animate-css animations

### API Client (web/src/api/client.ts)

- Base URL: `/api/v1`
- 30s timeout (10s for refresh)
- Retry: exponential backoff on 502/503/504
- CSRF token on mutating requests
- Refresh token coalescing (30s cooldown after failure)
- Auto token refresh on 401

### Hooks (web/src/hooks/)

- `useApi<T>(path)` — GET requests
- `useMutation<TInput, TOutput>(method, path)` — POST/PUT/DELETE
- `usePaginatedApi<T>(path, perPage)` — paginated lists

### Pages (web/src/pages/)

Lazy-loaded via React Router — dashboard, apps, deployments, projects, topology, settings, admin, auth

### Testing

- Vitest 3.2.1 (unit/integration)
- Playwright 1.59.1 (E2E, `pnpm test:e2e`)

## Deployment Model

### Binary

- Single Go binary (`deploymonster`) with embedded React SPA
- Embedding via `go:embed` — built React copied to `internal/api/static/`
- Binary runs as **master** (full platform) or **agent** (worker node) via `--agent` flag

### Agent Communication

- WebSocket-based (gorilla/websocket)
- Agent connects to master via `--master` URL + `--token` join token
- Server ID derived from hostname

### Docker

- Multi-stage Dockerfile (alpine → scratch)
- Production image: scratch base, CA certs, tzdata, pre-chowned data dir
- Docker socket mount: `/var/run/docker.sock` (required for container management)
- Ports: 8443 (API), 80 (HTTP), 443 (HTTPS)

### ACME/Let's Encrypt

- HTTP-01 challenge via ingress module
- Wildcard support planned
- Staging + production endpoints

## Config

- `monster.yaml` — YAML configuration file
- Environment variable overrides: `MONSTER_*` prefix (e.g., `MONSTER_SECRET`, `MONSTER_DB_PATH`)
- Key sections: server, database, ingress, acme, dns, docker, backup, notifications, swarm, vps_providers, git_sources, marketplace, registration, sso, secrets, billing, limits, enterprise
