# DeployMonster Architecture Map

## Tech Stack Summary

| Layer | Technology | Version |
|-------|-----------|---------|
| **Backend** | Go | 1.26.1 |
| **Database** | SQLite (modernc.org/sqlite) | 1.48.2 |
| **KV Store** | BBolt (embedded KV) | 1.4.3 |
| **Auth** | JWT (golang-jwt/jwt/v5) | 5.3.1 |
| **Password Hashing** | bcrypt (golang.org/x/crypto) | — |
| **Encryption** | AES-256-GCM + Argon2id | — |
| **WebSocket** | gorilla/websocket | 1.5.3 |
| **Container Runtime** | Docker SDK | 28.5.2 |
| **Frontend** | React | 19.2.5 |
| **Frontend Build** | Vite | 8.0.5 |
| **Routing** | React Router | 7.13.2 |
| **State Management** | Zustand | 5.0.12 |
| **Styling** | Tailwind CSS | 4.2.2 |
| **HTTP Framework** | Go 1.22+ http.ServeMux | — |
| **Config** | YAML (gopkg.in/yaml.v3) | 3.0.1 |

---

## Module Structure (20 Modules)

### Core Modules

| Module | ID | Security-Relevant Responsibilities |
|--------|----|-----------------------------------|
| **core.db** | `core.db` | Database initialization, SQLite/BBolt wiring, backup |
| **core.auth** | `core.auth` | JWT service, password hashing, first-run admin setup, API key bcrypt hashing |
| **api** | `api` | HTTP server, all REST routes, middleware chain, SPA serving |
| **secrets** | `secrets` | AES-256-GCM vault, Argon2id KDF, per-deployment salt, secret versioning, key rotation |

### Infrastructure Modules

| Module | ID | Security-Relevant Responsibilities |
|--------|----|-----------------------------------|
| **deploy** | `deploy` | Docker container lifecycle, resource quotas, volume path validation, orphan cleanup |
| **swarm** | `swarm` | Master-agent WebSocket comms, agent authentication via join token, heartbeat, bidirectional messaging |
| **backup** | `backup` | Encrypted backup storage, scheduled backups, retention enforcement |
| **dns** | `dns` | DNS record management (Cloudflare, manual), domain verification |
| **ingress** | `ingress` | HTTP/HTTPS serving, ACME Let's Encrypt certificates, HTTP-01/DNS-01 challenges |
| **vps** | `vps` | VPS provisioning (Hetzner, DigitalOcean, Vultr, Linode), SSH access |

### Application Modules

| Module | ID | Security-Relevant Responsibilities |
|--------|----|-----------------------------------|
| **notifications** | `notifications` | Multi-channel notifications (email, Slack, Discord, Telegram, webhook) |
| **billing** | `billing` | Stripe integration, subscription management, webhook signature verification |
| **marketplace** | `marketplace` | Template registry, one-click deploys |
| **gitsources** | `gitsources` | Git provider integration (GitHub, GitLab, Gitea), webhook creation |
| **webhooks** | `webhooks` | Inbound webhook verification (GitHub/GitLab/Gitea/Bitbucket HMAC), outbound delivery, replay |
| **enterprise** | `enterprise` | License validation, feature gates |
| **discovery** | `discovery` | Service discovery |
| **resource** | `resource` | Resource limit management |
| **compose** | `compose` | Docker Compose stack deployment |
| **mcp** | `mcp` | MCP protocol endpoint for AI tool integration |
| **awsauth** | `awsauth` | AWS authentication integration |

---

## Entry Points

### Main Entry Point
**File:** `cmd/deploymonster/main.go`

```
Commands:
  serve/start  → runServe()      — starts full master server
  version      → runVersion()     — prints version info
  config       → runConfigCheck() — validates config
  init         → runInit()        — creates monster.yaml
  rotate-keys  → runRotateKeys()  — re-encrypts all secrets
  setup        → runSetup()       — interactive setup wizard
  help         → printUsage()
```

### Agent Mode Entry Point
**Function:** `runAgent()` — Same binary, `--agent` flag switches to worker mode. Connects to master via WebSocket using join token authentication.

CLI flags:
- `--agent` — run in agent mode
- `--master` — master server URL (or `MONSTER_MASTER_URL`)
- `--token` — join token for agent auth (or `MONSTER_JOIN_TOKEN`)
- `--master-port` — fallback port (default 8443)

### API Entry Point
**Route registration:** `internal/api/router.go` → `registerRoutes()`

All routes registered on `http.ServeMux` using Go 1.22+ `METHOD /path` syntax.

### Webhook Entry Point
**Route:** `POST /hooks/v1/{webhookID}`
**Handler:** `internal/webhooks/receiver.go` → `HandleWebhook()`
- No JWT auth — signature-verified only
- 1MB body limit (vs global 10MB)

### Agent WebSocket Entry Point
**Route:** `GET /api/v1/agent/ws`
**Handler:** `internal/swarm/server.go` → `AgentServer.HandleConnect()`
- Token auth via `X-Agent-Token` header or query param
- Upgrades to raw TCP via HTTP hijack

### CLI Entry Point
**File:** `cmd/deploymonster/main.go`

---

## Security Boundaries

### Authentication (AuthN)

#### JWT Tokens
- **Algorithm:** HS256 (explicitly enforced via `WithValidMethods`)
- **Access token TTL:** 15 minutes
- **Refresh token TTL:** 7 days
- **Key rotation grace period:** 20 minutes (reduced from 1 hour — SESS-006 fix)
- **Minimum secret length:** 32 characters enforced at startup
- **Token revocation:** JTI stored in BBolt with same TTL as token
- **Claims:** UserID, TenantID, RoleID, Email

#### API Keys
- **Prefix:** `dm_` (enforced in middleware)
- **Format:** `dm_` + 64 hex chars (32 bytes random)
- **Hashing:** bcrypt with cost 13
- **Storage:** Prefix lookup (first 12 chars) in BBolt, full hash comparison via bcrypt
- **Expiry:** Optional, checked at auth time
- **SC-017 FINDING:** API keys stored with SHA-256 prefix lookup then bcrypt comparison — security report notes CRYPTO-001 fix applied

#### Multi-Factor Auth
- TOTP enabled flag on User model (`TOTPEnabled bool`)
- TOTP verification logic not observed in current handler scan

#### Auth Priority Order (RequireAuth middleware)
1. JWT from `Authorization: Bearer <token>` header
2. JWT from `dm_access` httpOnly cookie
3. API key from `X-API-Key` header (prefix `dm_` required)

### Authorization (AuthZ)

#### RBAC System (`internal/auth/rbac.go`)
Permission constants defined:
```
app.view, app.create, app.deploy, app.delete, app.restart, app.stop,
app.logs, app.metrics, app.env.edit,
project.view, project.create, project.delete,
member.view, member.invite, member.remove, member.manage,
secret.view, secret.create, secret.delete,
domain.view, domain.manage,
billing.view, billing.manage,
db.view, db.manage,
* (admin all)
```

Built-in roles: super_admin, owner, admin, developer, operator, viewer, billing

#### Role-Based Routes
- `protected` middleware: auth + tenant rate limiting
- `adminOnly` middleware: `protected` + `RequireSuperAdmin`
- All `/api/v1/admin/*` routes wrapped with `adminOnly`

### Encryption

#### Secrets Vault (`internal/secrets/vault.go`)
- **Cipher:** AES-256-GCM
- **KDF:** Argon2id (1 iteration, 64MB memory, 4 parallelism, 32-byte output)
- **Salt:** Per-deployment (32 bytes), persisted in BBolt bucket `vault`, key `salt`
- **Legacy migration:** Automatic on first boot if pre-salt secrets exist
- **Key rotation:** `RotateEncryptionKey()` re-encrypts all versions with new master secret

#### JWT Secrets
- **Minimum 32 characters** enforced at service creation (JWT-002 fix)
- Previous keys kept for 20-minute grace period for in-flight tokens
- Keys purged automatically after grace period expires via `purgeExpiredPreviousKeys()`

#### Password Hashing
- bcrypt with cost 13
- API keys also use cost 13 for equivalent offline-attack economics

### Input Validation

#### Volume Path Validation (`ContainerOpts.ValidateVolumePaths`)
- Blocks `..` traversal before AND after path cleaning
- Requires absolute paths
- Blocks Docker socket mounts (`/var/run/docker.sock`, `/run/docker.sock`, `/var/run/docker`) unless `AllowDockerSocket=true`
- Blocks root directory mounts
- Rejects null bytes in paths

#### Body Size Limits
- Global: 10MB
- Webhooks: 1MB

#### Rate Limiting
- Global: 120 req/min per IP (configurable via `server.rate_limit_per_minute`)
- Auth endpoints: 120 req/min per IP for login/register
- Token refresh: 5 req/min per IP
- Tenant rate limiting: 100 req/min per tenant (applied after auth)
- Per-IP limits only apply to `/api/` and `/hooks/` prefixes (SPA static assets exempt)

### Middleware Chain (applied in order — last runs first)

```
RequestID → GracefulShutdown → GlobalRateLimit → SecurityHeaders →
APIMetrics → APIVersion → BodyLimit(10MB) → Timeout(30s) →
Recovery → RequestLogger → CORS → CSRFProtect →
Idempotency → AuditLog → RequireAuth → [handler]
```

#### Middleware Details
| Middleware | File | Purpose |
|------------|------|---------|
| `RequestID` | `middleware/middleware.go` | Generates/sets `X-Request-ID` header |
| `SecurityHeaders` | `middleware/middleware.go` | HSTS, X-Content-Type-Options, X-Frame-Options, CSP |
| `CSRFProtect` | `middleware/middleware.go` | Token-based CSRF protection |
| `IdempotencyMiddleware` | `middleware/middleware.go` | BBolt-backed idempotency key store |
| `AuditLog` | `middleware/middleware.go` | Logs all authenticated requests to AuditStore |
| `GlobalRateLimiter` | `middleware/middleware.go` | Per-IP rate limiting |
| `TenantRateLimiter` | `middleware/middleware.go` | Per-tenant rate limiting |
| `RequireAuth` | `middleware/middleware.go` | JWT/API key validation + revocation check |
| `RequireSuperAdmin` | `middleware/middleware.go` | Admin-only route guard |

### Network Security

#### TLS/ACME
- Let's Encrypt support via HTTP-01 or DNS-01 challenges
- Automatic certificate management via ingress module
- `force_https` redirect option

#### CORS
- Public mode (`*`): wildcard origin, no credentials
- Allowlist mode: specific origins echoed, credentials allowed
- Vary: Origin always set for cache correctness
- `Access-Control-Allow-Headers`: Content-Type, Authorization, X-API-Key, X-Request-ID
- `Access-Control-Expose-Headers`: X-Request-ID, X-DeployMonster-Version, X-API-Version
- `Access-Control-Max-Age`: 86400

#### Webhook Signature Verification
| Provider | Header | Method |
|----------|--------|--------|
| GitHub | `X-Hub-Signature-256` | HMAC-SHA256 |
| GitLab | `X-Gitlab-Token` | Plain token comparison |
| Gitea/Gogs | `X-Gitea-Signature` / `X-Gogs-Signature` | HMAC-SHA256 |
| Bitbucket Server | `X-Hub-Signature` | HMAC-SHA256 |
| Bitbucket Cloud | None | URL-based secret only |
| Generic | None | URL-based secret only |

#### Agent Authentication (Master-Agent)
- Join token: constant-time comparison via `subtle.ConstantTimeCompare`
- Token passed via `X-Agent-Token` header (preferred) or query param
- Heartbeat: 30s interval, 90s death threshold
- All agent messages are JSON over raw TCP (WebSocket-style hijack)

---

## Data Flows

### Authentication Flow
```
User → POST /api/v1/auth/login
     → AuthHandler.Login
     → bcrypt password verification against stored hash
     → JWTService.GenerateTokenPair (access + refresh)
     → Returns access_token (15min), refresh_token (7days)
     → Client stores in memory / httpOnly cookie
```

### API Request Flow (Authenticated)
```
Request → Middleware Chain
       → RequireAuth (JWT or API key validation)
       → TenantRateLimiter
       → Handler
       → AuditLog (writes to AuditStore)
       → Response
```

### Webhook Flow
```
Git Provider → POST /hooks/v1/{webhookID}
            → BodyLimit(1MB)
            → Lookup webhook secret from BBolt
            → Verify signature (provider-specific HMAC)
            → Parse payload (GitHub/GitLab/Gitea/Bitbucket)
            → Emit core.EventWebhookReceived
            → 200 OK immediately (async processing)
```

### Deploy Pipeline Flow
```
Webhook received
  → Parse payload, emit webhook.received event
  → Build module clones Git repo
  → Build module detects language, creates Dockerfile
  → Deploy module pulls image, creates container
  → Container started with env vars, volumes, labels
  → Events: build.started, build.completed, app.deployed, container.started
```

### Agent Connection Flow
```
Agent → GET /api/v1/agent/ws
      → Token verification (X-Agent-Token, constant-time compare)
      → HTTP hijack to raw TCP
      → Send AgentInfo JSON
      → Enter bidirectional message loop
      → Master tracks via heartbeat (30s ping, 90s death)
      → Agent disconnect → agent.disconnected event
```

### Secret Resolution Flow
```
Container start / env var interpolation
  → SecretResolver.Resolve(scope, name)
  → Check exact scope first (tenant/project/app), then fallback to global
  → Lookup secret metadata in SQLite
  → Get latest version from SQLite
  → Decrypt value_enc using AES-256-GCM + Argon2id-derived key
  → Return plaintext (never persisted decrypted)
```

### Master-Agent Communication
```
Master ──ping (30s)──→ Agent
     ←──pong──────────
     ←──metrics───────
     ←──container_event───
     ←──server_status────
     ←──log_stream───────

Agent ──any message──→ Master (updates lastSeen heartbeat)
```

---

## Trust Boundaries

```
┌─────────────────────────────────────────────────────────┐
│                    UNTRUSTED                             │
│  Git Providers (webhooks)    │  Browsers (SPA)         │
│  - Signature verified         │  - CORS enforced         │
│  - Body size limited (1MB)    │  - CSRF protected        │
│  - Token in URL (BB Cloud)    │  - Auth via JWT/API key  │
└────────────┬───────────────────┴─────────────────────────┘
             │ Webhook / HTTP
             ▼
┌─────────────────────────────────────────────────────────┐
│              DMZ (API Edge)                              │
│  Middleware chain: RequestID, RateLimit, SecurityHeaders,│
│  BodyLimit, Timeout, Recovery, CORS, CSRF, AuditLog    │
│  - 10MB body limit (1MB for webhooks)                   │
│  - 30s request timeout                                 │
│  - 120 req/min per IP global rate limit                 │
└────────────┬────────────────────────────────────────────┘
             │ AuthN passed (JWT / API key)
             ▼
┌─────────────────────────────────────────────────────────┐
│           TENANT ISOLATION LAYER                        │
│  - TenantID in JWT claims                              │
│  - Per-tenant rate limiting (100 req/min)               │
│  - RBAC permission checks                              │
│  - Audit logging with tenant context                   │
└────────────┬────────────────────────────────────────────┘
             │ AuthZ passed
             ▼
┌─────────────────────────────────────────────────────────┐
│              APPLICATION LAYER                         │
│  Handlers access core.Store (tenant-scoped queries)    │
│  Handlers access core.Services.Container               │
│  Secrets resolved via SecretResolver (decrypted in-app) │
└────────────┬────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────┐
│              DATA LAYER                                 │
│  SQLite: App data, Deployments, Users, Secrets (enc)  │
│  BBolt:  API keys (bcrypt hash), Sessions, Webhooks  │
│          Revoked tokens, Config, Metrics              │
│  Docker: Containers (tenant-isolated via labels)      │
└─────────────────────────────────────────────────────────┘
```

### Master-Agent Trust
```
┌──────────────┐    WebSocket (TCP hijack)    ┌──────────────┐
│    Agent     │ ←──────────────────────────→ │    Master    │
│  (untrusted  │   Token auth on connect     │  (trusts     │
│   network)   │   Heartbeat keepalive       │   agents via │
│              │   JSON bidirectional msgs   │   token)     │
└──────────────┘                              └──────────────┘
```

### External Integrations Trust Boundary
```
┌────────────────┐                    ┌────────────────┐
│  GitHub/GitLab │ webhook signatures  │ DeployMonster  │
│  Bitbucket     │ ──────────────────→ │    receives    │
│  Gitea         │ HMAC verified       │               │
└────────────────┘                    └────────────────┘

┌────────────────┐                    ┌────────────────┐
│   Stripe       │ webhook signatures  │ DeployMonster  │
│                │ ──────────────────→ │    receives    │
└────────────────┘                    └────────────────┘

┌────────────────┐                    ┌────────────────┐
│  Cloudflare    │ API token           │ DeployMonster  │
│                │ ──────────────────→ │    manages     │
└────────────────┘                    │  DNS records   │
                                      └────────────────┘

┌────────────────┐                    ┌────────────────┐
│  SMTP Server   │ SMTP credentials    │ DeployMonster  │
│                │ ──────────────────→ │    sends       │
└────────────────┘                    │  email         │
                                      └────────────────┘
```

---

## Security-Critical Code Locations

| Concern | File | Key Functions/Types |
|---------|------|---------------------|
| JWT validation | `internal/auth/jwt.go` | `ValidateAccessToken`, `ValidateRefreshToken`, `WithValidMethods(HS256)` |
| JWT key rotation | `internal/auth/jwt.go` | `AddPreviousKey`, `purgeExpiredPreviousKeys`, `RotationGracePeriod=20min` |
| JWT token revocation | `internal/auth/jwt.go` | `RevokeAccessToken`, `IsAccessTokenRevoked`, `AccessTokenRevocation` |
| API key hashing | `internal/auth/apikey.go` | `HashAPIKey`, `VerifyAPIKey`, `apiKeyBcryptCost=13` |
| Webhook signature | `internal/webhooks/receiver.go` | `VerifySignature`, `VerifyGitHubSignature`, `VerifyGitLabToken` |
| Password hashing | `internal/auth/password.go` | `HashPassword`, `VerifyPassword` |
| Secret encryption | `internal/secrets/vault.go` | `Vault.Encrypt`, `Vault.Decrypt`, `NewVaultWithSalt`, `argon2.IDKey` |
| Secret key rotation | `internal/secrets/module.go` | `RotateEncryptionKey`, `migrateLegacyVault` |
| Volume path validation | `internal/core/interfaces.go` | `ContainerOpts.ValidateVolumePaths`, `dangerousPaths` |
| Agent auth | `internal/swarm/server.go` | `HandleConnect`, `subtle.ConstantTimeCompare` |
| Auth middleware | `internal/api/middleware/middleware.go` | `RequireAuth` (JWT/API key/Cookie), `RequireSuperAdmin` |
| RBAC | `internal/auth/rbac.go` | Permission constants, `Role.HasPermission` |
| Rate limiting | `internal/api/middleware/middleware.go` | `GlobalRateLimiter`, `TenantRateLimiter`, `AuthRateLimiter` |
| Session context | `internal/auth/auth.go` | `ClaimsFromContext`, `ContextWithClaims` |
| Config secrets audit | `internal/core/config.go` | `AuditSecrets` |
| Container resource limits | `internal/deploy/docker.go` | `ContainerOpts.ApplyResourceDefaults` |
| Graceful shutdown | `internal/api/router.go` | `GracefulShutdown.StartDraining`, `InFlight` tracking |
| Event bus | `internal/core/events.go` | `EventBus.Publish`, `Subscribe`, `SubscribeAsync` |

---

## Configuration Security-Relevant Settings

```yaml
server:
  secret_key: ""           # REQUIRED, min 32 chars, auto-generated on first run
  cors_origins: ""        # Comma-separated allowlist or "*"
  rate_limit_per_minute: 120
  enable_pprof: false     # Debug endpoints, auth-protected

database:
  driver: sqlite          # or postgres (enterprise)
  path: deploymonster.db

docker:
  host: unix:///var/run/docker.sock
  default_cpu_quota: 0    # Per-container CPU limit (optional)
  default_memory_mb: 0    # Per-container memory limit (optional)

secrets:
  encryption_key: ""      # Defaults to server.secret_key

backup:
  schedule: "02:00"
  retention_days: 30
  encryption: true

ingress:
  http_port: 80
  https_port: 443
  enable_https: true
  force_https: false

acme:
  email: ""
  staging: false
  provider: http-01        # or dns-01

registration:
  mode: open              # open, invite_only, approval, disabled

swarm:
  enabled: false
  join_token: ""          # Used for agent authentication

notifications:
  # email_smtp: ""
  # slack_webhook: ""
  # discord_webhook: ""
  # telegram_token: ""

billing:
  enabled: false
  # stripe_secret_key: ""
  # stripe_webhook_key: ""
```

---

## Event System Security Considerations

The event bus (`internal/core/events.go`) is **in-process only**. Security boundaries are enforced at the API edge.

| Event Type | Triggered By | Security Relevance |
|------------|--------------|-------------------|
| `webhook.received` | Inbound webhook | Entry point for untrusted input from Git providers |
| `system.started` | Master boot | Audit event |
| `system.stopping` | Master shutdown | Audit event |
| `system.config_reloaded` | SIGHUP reload | Operational event |
| `app.created` | App creation | Audit event |
| `app.deployed` | Successful deploy | Audit event |
| `deploy.failed` | Failed deploy | Triggers auto-rollback |
| `deploy.finished` | Deploy completed | Audit event |
| `build.started` | Build triggered | Audit event |
| `build.completed` | Build success | Audit event |
| `build.failed` | Build failure | Audit event |
| `agent.connected` | Agent connect | Trust boundary event |
| `agent.disconnected` | Agent disconnect | Trust boundary event |
| `agent.metrics` | Agent metrics report | Operational monitoring |
| `outbound.sent` | Webhook delivery | Audit event |
| `outbound.failed` | Webhook failure | Retry/retry logic |
| `notification.sent` | Notification sent | Audit event |
| `secret.created` | Secret created | Audit event |

---

## Key Security Findings (from scan)

1. **JWT Rotation Grace Period Reduced** — SESS-006 fix reduced from 1 hour to 20 minutes
2. **API Key Bcrypt Cost 13** — API keys and passwords have equivalent offline-attack economics
3. **Per-Deployment Salt for Vault** — Each install has unique salt even with same master secret
4. **Volume Path Validation** — Blocks `..`, Docker socket, root mounts, null bytes
5. **Webhook Body Limit 1MB** — Separate from global 10MB limit
6. **Agent Token Constant-Time Compare** — Uses `subtle.ConstantTimeCompare`
7. **Graceful Shutdown with Drain** — API server waits for in-flight requests before shutdown
8. **Stale Deployment Reclamation** — Tier 100 fix reclaims deployments stuck in "deploying" state after crash

---

**Report Generated**: 2026-04-18
**Codebase Version**: master (f3cccb0)
**Purpose**: Architecture reconnaissance for vulnerability scanning
