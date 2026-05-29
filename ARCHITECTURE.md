# DeployMonster Architecture

> A Go 1.26+ modular monolith PaaS with embedded React 19 frontend. Single binary, master or agent mode.

---

## Table of Contents

1. [Overview](#1-overview)
2. [High-Level Architecture](#2-high-level-architecture)
3. [Module System](#3-module-system)
4. [Core Structure](#4-core-structure)
5. [Store & Data Access](#5-store--data-access)
6. [Event System](#6-event-system)
7. [API Layer](#7-api-layer)
8. [Database Architecture](#8-database-architecture)
9. [Configuration](#9-configuration)
10. [Frontend Architecture](#10-frontend-architecture)
11. [Build Pipeline](#11-build-pipeline)
12. [Master/Agent Mode](#12-masteragent-mode)
13. [Security](#13-security)
14. [Key Patterns](#14-key-patterns)

---

## 1. Overview

DeployMonster transforms any VPS with Docker into a multi-tenant deployment platform.

```
~24 MB single binary with embedded React UI
~188K LOC total (~50K Go source, ~117K Go tests, ~22K React/TS/CSS)
22 auto-registered modules
236 documented REST API routes
85%+ coverage gate
```

### Tech Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.26+ |
| Frontend | React 19, TypeScript, Vite, Tailwind v4, shadcn/ui, Zustand |
| Database | SQLite (default), PostgreSQL (enterprise), SQLite-backed KV |
| Container | Docker SDK |
| Reverse Proxy | Custom (net/http/httputil) | Label-based routing, built-in ACME |
| Authentication | JWT (HS256), API Keys |

---

## 2. High-Level Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                       DeployMonster Binary                        │
├─────────────┬─────────────┬─────────────┬───────────┬────────────┤
│  Web UI     │  REST API   │  WebSocket  │  MCP      │  Webhooks  │
│  (React SPA)│  /api/v1/*  │  /ws/*      │  /mcp/*   │  /hooks/*  │
├─────────────┴─────────────┴─────────────┴───────────┴────────────┤
│                         Core Engine                               │
├──────┬──────┬──────┬──────┬──────┬──────┬──────┬────────┬────────┤
│Ingress│Deploy│Build │Disco-│Load  │DNS   │Resource│Backup │Market-│
│Gate  │Engine│Engine│very  │Bal.  │Sync  │Monitor │Engine │place  │
├──────┴──────┴──────┴──────┴──────┴──────┴──────┴────────┴────────┤
│                        Module System                              │
├──────────────────────────────────┬────────────────────────────────┤
│  Docker SDK ←→ Docker Engine     │  VPS Providers (SSH + API)     │
│  Local / Swarm / Compose         │  Hetzner│DO│Vultr│Linode│AWS   │
├──────────────────────────────────┴────────────────────────────────┤
│          Embedded DB (SQLite + KV) + Event Bus                    │
└───────────────────────────────────────────────────────────────────┘
```

### Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go 1.26+ | Single binary, native concurrency, Docker SDK |
| Database | SQLite + SQLite-backed KV | SQLite for relational data and JSON KV state |
| Web UI | React 19 + Vite + Tailwind v4 + shadcn/ui | Embedded in binary via `embed.FS` |
| API | REST + WebSocket | REST for CRUD, WS for real-time logs/events |
| Auth | JWT + API Keys | Stateless, multi-tenant |
| Container Runtime | Docker SDK (moby/moby) | Direct Docker API, no shelling out |
| Orchestration | Docker Swarm Mode | Built-in, no K8s complexity |
| SSL | Let's Encrypt (ACME) via lego library | Auto-cert with HTTP-01 / DNS-01 challenge |
| Ingress | Custom reverse proxy (net/http/httputil) | Label-based routing, no Traefik dependency |
| DNS | Cloudflare API + Generic RFC2136 | Most common provider + standard fallback |

---

## 3. Module System

### 3.1 Module Interface

Every subsystem implements the `Module` interface in `internal/core/module.go`:

```go
type Module interface {
    ID() string              // Unique module ID (e.g., "core.db", "deploy")
    Name() string           // Human-readable name
    Version() string        // Semantic version
    Dependencies() []string // Module IDs this depends on

    Init(ctx context.Context, core *Core) error // Initialize
    Start(ctx context.Context) error             // Begin operations
    Stop(ctx context.Context) error              // Graceful shutdown

    Health() HealthStatus   // OK, Degraded, Down
    Routes() []Route        // HTTP endpoints
    Events() []EventHandler // Event subscriptions
}
```

### 3.2 Module Registration

Modules auto-register via `init()` using `core.RegisterModule()`:

```go
// In every module's module.go:
func init() {
    core.RegisterModule(func() core.Module { return New() })
}

// main.go imports module packages so their init() registration runs:
_ "github.com/deploymonster/deploy-monster/internal/api"
_ "github.com/deploymonster/deploy-monster/internal/auth"
_ "github.com/deploymonster/deploy-monster/internal/deploy"
// ... remaining loadable modules
```

`deploy` and `swarm` are also imported normally by `cmd/deploymonster`
because the agent CLI path uses their exported constructors. Their `init()`
registration still runs during package import.

### 3.3 Lifecycle

```mermaid
flowchart LR
    subgraph "Registration Phase"
        Init1["init() calls<br/>RegisterModule()"]
        Factory["Factory added to<br/>moduleFactories"]
    end
    
    subgraph "Startup Phase"
        Resolve["Topological Sort<br/>by Dependencies"]
        InitAll["InitAll()<br/>dependency order"]
        StartAll["StartAll()<br/>dependency order"]
    end
    
    subgraph "Shutdown Phase"
        StopAll["StopAll()<br/>REVERSE order"]
    end
    
    Init1 --> Factory
    Factory --> Resolve
    Resolve --> InitAll
    InitAll --> StartAll
    StartAll --> StopAll
```

1. **Register** - All `init()` functions populate `moduleFactories`
2. **Resolve** - Topological sort via `Registry.Resolve()` based on `Dependencies()`
3. **Init All** - `Registry.InitAll()` calls `Init(ctx, core)` in dependency order
4. **Start All** - `Registry.StartAll()` calls `Start(ctx)`
5. **Stop All** - `Registry.StopAll()` calls `Stop(ctx)` in **reverse** order

### 3.4 All Modules

| Module ID | Name | Dependencies | Purpose |
|-----------|------|-------------|---------|
| `core.db` | Database | — | SQLite/PostgreSQL + SQLite-backed KV store |
| `core.auth` | Authentication | `core.db` | JWT, password hashing, sessions |
| `api` | REST API | `core.db`, `core.auth`, `marketplace`, `billing` | HTTP server, routes, middleware |
| `deploy` | Deploy Engine | `core.db` | Docker container management |
| `build` | Build Engine | `core.db`, `deploy` | Worker pool, Dockerfile generation |
| `ingress` | Ingress Gateway | `core.db`, `deploy` | Reverse proxy on :80/:443, ACME |
| `discovery` | Service Discovery | `deploy`, `ingress` | Docker events, route registration |
| `secrets` | Secret Vault | `core.db` | AES-256-GCM encryption |
| `notifications` | Notifications | `core.db` | Slack, Discord, Telegram, SMTP |
| `backup` | Backup Engine | `core.db` | Local/S3 storage, snapshots |
| `billing` | Billing Engine | `core.db` | Stripe, usage metering, plans |
| `marketplace` | Marketplace | `core.db`, `deploy` | Template registry, 91 templates |
| `dns.sync` | DNS Synchronizer | `core.db` | Cloudflare DNS sync |
| `vps` | VPS Provider | `core.db` | Multi-provider VPS provisioning |
| `gitsources` | Git Sources | `core.db` | GitHub/GitLab integration |
| `swarm` | Swarm Orchestrator | `deploy` | Multi-node Docker Swarm |
| `resource` | Resource Monitor | `core.db`, `deploy` | Metrics, alerting |
| `enterprise` | Enterprise | `core.db`, `billing` | White-label, reseller, GDPR |
| `mcp` | MCP Server | `core.db`, `deploy` | Model Context Protocol |
| `database` | Database Manager | `core.db`, `deploy` | Managed DB containers |
| `cron` | Cron Scheduler | `core.db` | Job scheduling and execution |
| `autoscale` | Autoscaler | `core.db`, `deploy` | Dynamic container scaling |

> **Note:** The `core.events` (EventBus), `core.scheduler` (Task Scheduler), and `core.ssh` (SSH Client) are built into `internal/core/` and are not separate loadable modules. `internal/webhooks`, `internal/compose`, `internal/topology`, and `internal/awsauth` are library packages consumed by modules/API handlers, not auto-registered modules.

---

## 4. Core Structure

### 4.1 Core Struct

Located in `internal/core/app.go`, `Core` is the dependency injection container:

```go
type Core struct {
    Config     *Config    // Application config
    ConfigPath string     // Hot-reload path
    Build      BuildInfo  // Version/commit/date from ldflags
    Registry   *Registry  // Module registry
    Events     *EventBus  // Central event system
    Scheduler  *Scheduler // Cron-like scheduler
    DB         *Database // SQL + KV wrappers
    Store      Store     // Unified data access
    Services   *Services  // Service provider registry (Container, Secrets, Notifications, DNS, Backup, VPS, Git)
    Logger     *slog.Logger
    draining   atomic.Bool // Graceful shutdown
}
```

### 4.2 Services Registry

Modules communicate via service interfaces, not direct imports:

```go
type Services struct {
    Container     ContainerRuntime     // Docker operations
    SSH           SSHClient           // SSH execution
    Secrets       SecretResolver      // ${SECRET:name} resolution
    Notifications NotificationSender  // Multi-channel notifications

    dnsProviders     map[string]DNSProvider
    backupStorages   map[string]BackupStorage
    vpsProvisioners  map[string]VPSProvisioner
    gitProviders     map[string]GitProvider
}
```

Key service interfaces:

| Interface | Purpose | Key Methods |
|-----------|---------|-------------|
| `ContainerRuntime` | Docker operations | `CreateAndStart`, `Stop`, `Remove`, `Restart`, `Logs`, `Exec`, `Stats`, `ImagePull`, `ListNetworks` |
| `SSHClient` | SSH execution | `Connect`, `Execute`, `Upload`, `Download`, `Close` |
| `SecretResolver` | Secret lookup | `Resolve(scope, name)`, `ResolveAll(scope, template)` |
| `NotificationSender` | Notifications | `Send(ctx, Notification)` |
| `OutboundWebhookSender` | Outbound webhooks | `Send`, `SendAsync` |
| `DNSProvider` | DNS records | `Create`, `Update`, `Delete`, `Verify` |
| `BackupStorage` | Backup storage | `Upload`, `Download`, `Delete`, `List` |
| `VPSProvisioner` | VPS provisioning | `ListRegions`, `ListSizes`, `Create`, `Delete`, `Status`, `ListInstances` |
| `GitProvider` | Git platforms | `ListRepos`, `ListBranches`, `GetRepoInfo`, `CreateWebhook`, `DeleteWebhook` |

---

## 5. Store & Data Access

**All data access goes through `Store`** — never concrete DB types.

```go
type Store interface {
    TenantStore
    UserStore
    AppStore
    DeploymentStore
    DomainStore
    ProjectStore
    RoleStore
    AuditStore
    SecretStore
    InviteStore
    UsageRecordStore
    BackupStore
    Close() error
    Ping(ctx context.Context) error
}
```

### Store Sub-Interfaces

```mermaid
classDiagram
    class Store {
        <<interface>>
    }
    class TenantStore {
        <<interface>>
        CreateTenant()
        GetTenant()
        GetTenantBySlug()
        UpdateTenant()
        DeleteTenant()
    }
    class UserStore {
        <<interface>>
        CreateUser()
        GetUser()
        GetUserByEmail()
        UpdatePassword()
        CountUsers()
        CreateUserWithMembership()
    }
    class AppStore {
        <<interface>>
        CreateApp()
        GetApp()
        GetAppByName()
        UpdateApp()
        ListAppsByTenant()
        UpdateAppStatus()
        DeleteApp()
    }
    class DeploymentStore {
        <<interface>>
        CreateDeployment()
        UpdateDeployment()
        GetLatestDeployment()
        ListDeploymentsByApp()
        AtomicNextDeployVersion()
    }
    class SecretStore {
        <<interface>>
        CreateSecret()
        CreateSecretVersion()
        ListSecretsByTenant()
        GetLatestSecretVersion()
    }
    
    Store <|-- TenantStore
    Store <|-- UserStore
    Store <|-- AppStore
    Store <|-- DeploymentStore
    Store <|-- SecretStore
```

### Database Implementations

| Implementation | File | Purpose |
|----------------|------|---------|
| SQLite | `internal/db/sqlite.go` | Default relational store |
| PostgreSQL | `internal/db/postgres.go` | Enterprise relational |
| SQLite KV | `internal/db/bolt.go` | KV store (rate limits, API keys, metrics) |

---

## 6. Event System

Located in `internal/core/events.go`, the event system enables loose coupling between modules.

### 6.1 Event Structure

```go
type Event struct {
    ID            string    // Unique ID for tracing
    Type          string    // Dot-namespaced (e.g., "app.deployed")
    Source        string    // Module ID
    Timestamp     time.Time
    TenantID      string    // Multi-tenant context
    UserID        string    // Actor
    CorrelationID string    // Request tracing
    Data          any       // Typed payload
}

type EventHandler struct {
    EventType string
    Name      string
    Handler   func(ctx context.Context, event Event) error
}
```

### 6.2 Event Matching

Events support pattern matching:

```go
// Exact match
"app.deployed"

// Prefix match (all app events)
"app.*"

// Wildcard match (all events)
"*"
```

### 6.3 Event Flow

```mermaid
flowchart TD
    subgraph "Publishing"
        Publisher["Module calls<br/>Events.Publish()"]
        Event["Event struct<br/>with Type, Data, TenantID"]
    end
    
    subgraph "Routing"
        Router["EventBus routes<br/>to handlers"]
        SyncHandler["Sync Handler<br/>Immediate execution"]
        AsyncHandler["Async Handler<br/>Worker pool"]
    end
    
    subgraph "Processing"
        Worker1["Worker 1"]
        Worker2["Worker 2"]
        WorkerN["Worker N"]
    end
    
    Publisher --> Event
    Event --> Router
    Router --> SyncHandler
    Router --> AsyncHandler
    AsyncHandler --> Worker1
    AsyncHandler --> Worker2
    AsyncHandler --> WorkerN
```

### 6.4 Key Event Types

```go
// Application lifecycle
EventAppCreated, EventAppUpdated, EventAppDeployed, EventAppStopped
EventAppStarted, EventAppDeleted, EventAppCrashed, EventAppScaled

// Build pipeline
EventBuildStarted, EventBuildCompleted, EventBuildFailed

// Container
EventContainerStarted, EventContainerStopped, EventContainerDied

// Deployment
EventDeployFinished, EventDeployFailed, EventRollbackDone

// Domain
EventDomainAdded, EventDomainRemoved

// Server
EventServerAdded, EventServerRemoved

// Webhook (inbound)
EventWebhookReceived  // Payload: WebhookEventData {WebhookID, Provider, EventType, Branch, CommitSHA, RepoURL}

// Webhook (outbound)
EventOutboundSent, EventOutboundFailed

// Backup
EventBackupStarted, EventBackupCompleted, EventBackupFailed

// Alerting
EventAlertTriggered, EventAlertResolved

// User & Auth
EventUserInvited

// Secrets
EventSecretCreated

// Database
EventDatabaseCreated

// Notifications
EventNotificationSent, EventNotificationFailed

// Billing
EventInvoiceGenerated, EventPaymentReceived, EventPaymentFailed
EventBillingSubscriptionUpdated, EventBillingSubscriptionCanceled
EventBillingCheckoutCompleted, EventBillingUsageReported

// Project
EventProjectCreated, EventProjectDeleted

// Cron Jobs
EventCronJobCreated, EventCronJobDeleted

// DNS
EventDNSRecordDeleted

// Webhooks CRUD
EventEventWebhookDeleted

// Redirects
EventRedirectCreated, EventRedirectDeleted

// Autoscaling
EventAutoscaleUpdated

// Basic Auth
EventBasicAuthUpdated

// GPU
EventGPUConfigUpdated

// System
EventSystemStarted, EventSystemStopping, EventConfigReloaded
```

---

## 7. API Layer

### 7.1 Middleware Chain

The actual middleware chain (in `internal/api/router.go:Handler()`):

```mermaid
flowchart LR
    Request["Incoming Request"]
    --> MID1["Request ID<br/>X-Request-ID"]
    --> MID2["Tracing<br/>Request ID propagation"]
    --> MID3["IP Allowlist<br/>CIDR-based access control"]
    --> MID4["Graceful Shutdown<br/>Drain"]
    --> MID5["Global Rate Limit<br/>Per-IP (120/min)"]
    --> MID6["Security Headers<br/>HSTS, X-Frame"]
    --> MID7["API Metrics<br/>Request counter + latency"]
    --> MID8["API Version<br/>X-API-Version header"]
    --> MID9["Body Limit<br/>10MB"]
    --> MID10["Timeout<br/>30s"]
    --> MID11["Recovery<br/>Panic handler"]
    --> MID12["Request Logger<br/>Slow request >5s warn"]
    --> MID13["CORS<br/>Allowlist mode"]
    --> MID14["CSRF Protect<br/>Double-submit cookie"]
    --> MID15["Idempotency<br/>KV-backed deduplication"]
    --> MID16["Audit Log<br/>Store-backed request log"]
    --> Handler["API Handler"]
```

> **Actual order vs. documented:** The middleware chain applies in the order shown above. The `globalRL` applies per-IP rate limiting only to `/api/*` and `/hooks/*` prefixes (not to static assets) to prevent browser sessions from exhausting the limit.

### 7.2 Authentication Levels

```go
const (
    AuthNone       AuthLevel = iota  // Public endpoints
    AuthAPIKey                       // X-API-Key header
    AuthJWT                          // Bearer token or dm_access cookie
    AuthAdmin                        // Admin role required
    AuthSuperAdmin                   // Super admin role required
)
```

### 7.3 Key Route Groups

| Group | Example Endpoints |
|-------|-------------------|
| Auth | `/api/v1/auth/login`, `/api/v1/auth/register`, `/api/v1/auth/refresh`, `/api/v1/auth/me` |
| Apps | `/api/v1/apps`, `/api/v1/apps/{id}`, `/api/v1/apps/{id}/deploy`, `/api/v1/apps/{id}/scale` |
| Deployments | `/api/v1/apps/{id}/deployments`, `/api/v1/apps/{id}/rollback` |
| Projects | `/api/v1/projects`, `/api/v1/projects/{id}/environment` |
| Databases | `/api/v1/databases/engines`, `/api/v1/databases` |
| Domains | `/api/v1/domains`, `/api/v1/certificates`, `/api/v1/domains/{id}/verify` |
| Servers | `/api/v1/servers/provision`, `/api/v1/servers/test-ssh` |
| Secrets | `/api/v1/secrets` |
| Billing | `/api/v1/billing/plans`, `/api/v1/billing/usage` |
| Marketplace | `/api/v1/marketplace`, `/api/v1/marketplace/deploy` |
| Webhooks | `/hooks/v1/{webhookID}`, `/api/v1/apps/{id}/webhooks`, `/api/v1/apps/{id}/webhooks/{webhookId}`, `/api/v1/webhooks/outbound` |
| MCP | `/mcp/v1/tools`, `/mcp/v1/tools/{name}` |
| Streaming | `/api/v1/apps/{id}/logs/stream` (SSE), `/api/v1/events/stream` |
| Admin | `/api/v1/admin/*` (RequireSuperAdmin) |

### 7.4 SPA Fallback

All non-API routes serve the embedded React SPA:

```go
r.mux.Handle("/", newSPAHandler())  // Serves embedded React UI
```

---

## 8. Database Architecture

### 8.1 Dual-Store Design

```mermaid
graph TB
    subgraph "Store Interface"
        SQL["SQLite or PostgreSQL"]
    end
    
    subgraph "SQLite-backed KV Store"
        Idempotency["idempotency bucket"]
        RateLimit["ratelimit bucket"]
        Metrics["metrics_ring bucket"]
        APIKeys["api_keys bucket"]
        Vault["vault bucket"]
        WebhookSecrets["webhooks bucket"]
        BuildQueue["build_queue bucket"]
    end
    
    SQL --> Store["Store Interface"]
    Idempotency --> KV["SQLite KV"]
    RateLimit --> KV
    Metrics --> KV
    APIKeys --> KV
    Vault --> KV
    WebhookSecrets --> KV
    BuildQueue --> KV
```

### 8.2 KV Buckets

The SQLite-backed KV store uses buckets for various purposes:

| Bucket | Purpose |
|--------|---------|
| `sessions` | WebSocket session data |
| `ratelimit` | Per-IP/tenant rate limit counters |
| `buildcache` | Build cache entries |
| `metrics_ring` | Time-series container metrics (ring buffer, 288 points per app/server) |
| `cronjobs` | Cron job schedules and state |
| `app_pins` | Pinned app versions |
| `autoscale` | Autoscaling rules and state |
| `basic_auth` | App basic auth credentials |
| `api_keys` | API key hashes + metadata |
| `deploy_freeze` | Deployment freeze state |
| `deploy_notify` | Deployment notification settings |
| `deploy_approval` | Deployment approval workflows |
| `maintenance` | App maintenance mode settings |
| `app_middleware` | App middleware configuration |
| `container_metrics` | Container metrics snapshots |
| `announcements` | System announcements |
| `certificates` | SSL certificate data |
| `ssh_keys` | SSH key storage |
| `log_retention` | Log retention policies |
| `event_webhooks` | Event-based webhook configurations |
| `webhook_logs` | Webhook delivery logs |
| `webhooks` | Webhook records (AppID, AutoDeploy, BranchFilter, secret hash) |
| `revoked_tokens` | Revoked JWT tokens for logout |
| `vault` | Per-deployment Argon2id salt (encrypted secrets) |
| `git_provider_connections` | Git provider connection data |
| `build_queue` | Tenant build queue persistence |

Additional buckets are created lazily by feature handlers when needed
(`user_sessions`, `registries`, `tenant_ratelimit`, `redirects`,
`sticky_sessions`, `error_pages`, `response_headers`, and similar
feature-owned KV state).

### 8.3 Metrics Ring Buffer Policy

The `metrics_ring` bucket stores time-series metrics with automatic cleanup:

| Setting | Value |
|---------|-------|
| Collection interval | 30 seconds |
| Points retained per entity | 288 (maxRingPoints) |
| Resolution | 5-minute effective (30s × 288 = 24h window) |
| Cleanup | Automatic trimming on insert |

**How it works:**
1. `appendPoint(key, point)` reads existing ring from KV storage
2. Appends new data point
3. Trims to maxRingPoints (288) if exceeded — oldest points dropped
4. Writes back to KV storage atomically via `BatchSet()`

**Data retention:**
- Server metrics: key format `server:{serverID}:24h`
- Container metrics: key format `{appID}:24h`
- Each point stores: Timestamp, CPUPercent, MemoryMB, NetworkRx/TxMB

**TTL behavior:**
- No automatic TTL — ring buffer is self-limiting by point count
- Older data is dropped when ring is full
- Effective 24-hour window at 5-minute resolution

### 8.4 Database Module Lifecycle

```go
type Module struct {
    sqlite   *SQLiteDB    // Relational data
    postgres *PostgresDB  // Enterprise relational
    bolt     *BoltStore   // Legacy field name; SQLite-backed KV store
    driver   string       // "sqlite" or "postgres"
}
```

---

## 9. Configuration

### 9.1 Config Sources (Priority)

```
1. monster.yaml file
2. MONSTER_* environment variable overrides
```

### 9.2 Config Structure

```yaml
server:
  host: 0.0.0.0
  port: 8443
  domain: ""                         # Platform domain
  secret_key: ""                     # Auto-generated, JWT encryption (min 32 chars)
  previous_secret_keys: []           # Old keys for graceful JWT rotation
  cors_origins: ""                   # Derived from domain if empty
  enable_pprof: false                # Opt-in: expose /debug/pprof/*
  log_level: info                    # debug, info, warn, error
  log_format: text                   # text or json
  rate_limit_per_minute: 120         # Global per-IP rate limit; 0 = disabled
  allowed_cidrs: []                  # CIDR ranges allowed to access API; empty = allow all

database:
  driver: sqlite                    # or postgres
  path: deploymonster.db
  url: ""                            # PostgreSQL connection string
  query_timeout_sec: 0               # Per-query timeout in seconds; 0 = 5s default
  ssl_mode: require                 # postgres only: disable, allow, require, verify-ca, verify-full
  replication_mode: ""               # Off by default. Set to "streaming" for read replicas
  replica_url: ""                    # Read replica URL when replication is enabled

ingress:
  http_port: 80
  https_port: 443
  enable_https: true
  force_https: true                 # 301 redirect to HTTPS for non-ACME traffic

acme:
  email: ""
  staging: false
  provider: http-01                  # or dns-01
  cert_dir: ""                       # Custom certificate directory

dns:
  provider: cloudflare              # cloudflare, route53, manual
  cloudflare_token: ""               # Required when provider is cloudflare
  auto_subdomain: ""                  # e.g., deploy.monster

docker:
  host: unix:///var/run/docker.sock
  api_version: ""                   # Docker API version override
  tls_verify: false                 # Enable TLS verification
  default_cpu_quota: 100000         # Microseconds per 100ms period (100000 = 1 core)
  default_memory_mb: 512            # Default memory limit per container in MB

backup:
  schedule: "02:00"
  retention_days: 30
  storage_path: /var/lib/deploymonster/backups
  encryption: true
  encryption_key: ""                 # AES-256 key, 32 bytes base64-encoded
  s3:
    bucket: ""
    region: ""
    endpoint: ""                     # Custom endpoint for MinIO/R2
    access_key: ""
    secret_key: ""
    path_style: false                # Required for MinIO
  alertmanager_url: ""               # Prometheus Alertmanager webhook endpoint

notifications:
  smtp:
    host: ""
    port: 587
    username: ""
    password: ""
    from: ""
    from_name: ""
    use_tls: true                   # Implicit TLS (port 465); false = STARTTLS (port 587)
    insecure_skip_verify: false      # For dev/self-signed relays only
  slack_webhook: ""
  discord_webhook: ""
  telegram_token: ""
  telegram_chat_id: ""

swarm:
  enabled: false
  manager_ip: ""
  join_token: ""
  tls_cert_file: ""                  # Client certificate for mutual TLS
  tls_key_file: ""
  tls_ca_cert_file: ""               # CA cert to verify server certificate

vps_providers:
  enabled: true

git_sources:
  github_client_id: ""
  github_client_secret: ""
  gitlab_client_id: ""
  gitlab_client_secret: ""

marketplace:
  enabled: true
  templates_dir: marketplace/templates
  community_sync: false

registration:
  mode: open                         # open, invite_only, approval, disabled

secrets:
  encryption_key: ""                 # AES-256 key for secret vault

billing:
  enabled: false
  stripe_secret_key: ""
  stripe_webhook_key: ""

limits:
  max_apps_per_tenant: 100
  max_build_minutes: 30
  max_concurrent_builds: 5
  max_concurrent_builds_per_tenant: 2

enterprise:
  enabled: false
  license_key: ""

observability:
  loki_url: ""                       # Loki endpoint for structured logs
  loki_timeout: 5                     # seconds
  log_format: text                    # text, json, loki
  tracing_url: ""                     # OTLP/gRPC endpoint for OpenTelemetry traces
  service_name: deploymonster         # Service name for traces
```

### 9.3 Hot Reload

SIGHUP triggers `Core.ReloadConfig()` which updates:
- Log level and format (`server.log_level`, `server.log_format`)
- CORS origins (`server.cors_origins`)
- Registration mode (`registration.mode`)
- Backup schedule (`backup.schedule`)
- Resource limits (`limits.*`)
- Observability log format (`observability.log_format`)

### 9.4 Secret Audit

On startup, `Config.AuditSecrets()` warns about plaintext secrets in config that should use environment variables:
- `MONSTER_CLOUDFLARE_TOKEN`
- `MONSTER_GITHUB_CLIENT_SECRET`
- `MONSTER_GITLAB_CLIENT_SECRET`
- `MONSTER_ENCRYPTION_KEY`
- `MONSTER_STRIPE_SECRET_KEY`
- `MONSTER_STRIPE_WEBHOOK_KEY`
- `MONSTER_LICENSE_KEY`
- `MONSTER_SLACK_WEBHOOK`
- `MONSTER_DISCORD_WEBHOOK`
- `MONSTER_TELEGRAM_TOKEN`
- `MONSTER_S3_ACCESS_KEY`
- `MONSTER_S3_SECRET_KEY`
- `MONSTER_BACKUP_ENCRYPTION_KEY`
- `MONSTER_JOIN_TOKEN`

---

## 10. Frontend Architecture

### 10.1 Tech Stack

| Category | Technology |
|----------|------------|
| Framework | React 19 |
| Language | TypeScript |
| Build Tool | Vite |
| Styling | Tailwind CSS + shadcn/ui |
| State Management | Zustand |
| Routing | React Router v7 |
| Data Fetching | Custom API client |

### 10.2 Directory Structure

```
web/src/
├── App.tsx                        # Root component, routing, auth gate
├── main.tsx                       # Entry point
├── api/
│   ├── client.ts                  # Fetch wrapper with retry, refresh, CSRF
│   ├── auth.ts                    # Auth API endpoints
│   ├── apps.ts                    # Apps API endpoints
│   ├── deployments.ts            # Deployments API endpoints
│   ├── servers.ts                # Servers/VPS API endpoints
│   ├── databases.ts              # Databases API endpoints
│   ├── domains.ts                # Domains API endpoints
│   ├── secrets.ts                # Secrets API endpoints
│   ├── backups.ts                # Backups API endpoints
│   ├── billing.ts                # Billing API endpoints
│   ├── team.ts                   # Team API endpoints
│   ├── admin.ts                  # Admin API endpoints
│   ├── monitoring.ts             # Monitoring API endpoints
│   ├── marketplace.ts            # Marketplace API endpoints
│   └── git-sources.ts            # Git sources API endpoints
├── stores/
│   ├── auth.ts                   # Zustand auth state
│   ├── theme.ts                  # Theme preferences
│   ├── toastStore.ts             # Toast notifications
│   └── topologyStore.ts          # Topology editor state
├── hooks/
│   ├── useApi.ts                 # Generic GET API hook
│   ├── useMutation.ts            # Mutation hook (POST/PUT/DELETE)
│   ├── usePaginatedApi.ts        # Paginated API hook
│   ├── useDebouncedValue.ts      # Debounce utility hook
│   └── useDeployProgress.ts      # Deployment progress tracking
├── components/
│   ├── layout/
│   │   ├── AppLayout.tsx         # Main app layout with sidebar
│   │   └── Sidebar.tsx           # Navigation sidebar
│   ├── topology/
│   │   ├── TopologyCanvas.tsx   # Visual canvas for topology
│   │   ├── CompileModal.tsx     # Compile topology modal
│   │   ├── ComponentPalette.tsx # Component selection palette
│   │   ├── ConfigPanel.tsx      # Configuration panel
│   │   ├── DeployModal.tsx      # Deployment modal
│   │   └── CustomNodes/         # Custom node renderers
│   │       ├── AppNode.tsx      # App node renderer
│   │       ├── DatabaseNode.tsx # Database node renderer
│   │       ├── DomainNode.tsx   # Domain node renderer
│   │       ├── VolumeNode.tsx   # Volume node renderer
│   │       └── WorkerNode.tsx   # Worker node renderer
│   ├── ui/                       # shadcn/ui components
│   │   ├── button.tsx, input.tsx, label.tsx, card.tsx
│   │   ├── dialog.tsx, sheet.tsx, alert-dialog.tsx
│   │   ├── dropdown-menu.tsx, select.tsx
│   │   ├── tabs.tsx, switch.tsx, checkbox.tsx
│   │   ├── table.tsx, skeleton.tsx, progress.tsx
│   │   ├── badge.tsx, avatar.tsx, separator.tsx
│   │   └── tooltip.tsx, scroll-area.tsx
│   ├── ErrorBoundary.tsx         # React error boundary
│   ├── Spinner.tsx              # Loading spinner
│   ├── Toast.tsx                # Toast notification system
│   └── SearchDialog.tsx         # Global search dialog
├── pages/
│   ├── Login.tsx                # Login page
│   ├── Register.tsx             # Registration page
│   ├── Onboarding.tsx          # First-time setup wizard
│   ├── Dashboard.tsx            # Main dashboard
│   ├── Apps.tsx                # App list/management
│   ├── AppDetail.tsx           # Single app detail view
│   ├── DeployWizard.tsx        # New app deployment wizard
│   ├── Marketplace.tsx         # Template marketplace
│   ├── TemplateDetail.tsx      # Template detail view
│   ├── Domains.tsx             # Domain management
│   ├── Databases.tsx           # Database management
│   ├── Servers.tsx             # Server management
│   ├── GitSources.tsx          # Git source integration
│   ├── Backups.tsx             # Backup management
│   ├── Secrets.tsx             # Secrets management
│   ├── Team.tsx                # Team management
│   ├── Billing.tsx             # Billing/subscription
│   ├── Admin.tsx               # Admin panel
│   ├── Settings.tsx           # User settings
│   ├── Monitoring.tsx          # Metrics/monitoring
│   ├── Topology.tsx            # Visual topology editor
│   └── NotFound.tsx            # 404 page
└── lib/
    ├── utils.ts                # Utility functions (cn, formatting)
    └── generatedSecrets.ts     # Generated secret utilities
```

### 10.3 API Client Features

```mermaid
sequenceDiagram
    participant React as React App
    participant Client as API Client
    participant Server as Backend
    
    React->>Client: API Request
    Client->>Client: Add CSRF token
    Client->>Server: Fetch with retry
    alt 502/503/504
        Client->>Client: Exponential backoff
        Client->>Server: Retry
    end
    alt 401 Unauthorized
        Client->>Client: Refresh token
        Client->>Server: Retry original
    end
    Server-->>Client: Response
    Client-->>Client: Sanitize (XSS prevention)
    Client-->>React: Unwrapped data
```

Key features:
- 30s default timeout, 10s for refresh
- Exponential backoff with jitter
- CSRF token injection for mutations
- JWT refresh with coalescing (single in-flight refresh)
- Error sanitization

### 10.4 Pages

```
/login, /register, /onboarding       # Public
/dashboard                           # Overview
/apps, /apps/:id, /apps/:id/deploy   # App management
/domains, /certificates              # Domain management
/databases                           # Managed databases
/servers                             # VPS provisioning
/git-sources                         # Git integration
/marketplace, /marketplace/:id       # Template marketplace
/team, /billing, /backups, /secrets  # Tenant management
/monitoring, /topology               # Observability
/admin                               # Admin panel
/settings                            # User settings
```

---

## 11. Build Pipeline

### 11.1 Build Flow

```mermaid
flowchart LR
    subgraph "1. Frontend Build"
        Web["cd web && pnpm install && pnpm run build"]
    end
    
    subgraph "2. Copy to Embed"
        Copy["cp -r web/dist/* internal/api/static/"]
    end
    
    subgraph "3. Go Build"
        Go["CGO_ENABLED=0 go build<br/>-ldflags version/commit/date"]
    end
    
    Web --> Copy --> Go
```

### 11.2 Build Script (`scripts/build.sh`)

```bash
#!/bin/bash
set -e

# 1. Build React UI
cd web && pnpm install --frozen-lockfile && pnpm run build

# 2. Copy to embed directory
rm -rf internal/api/static/*
cp -r web/dist/* internal/api/static/

# 3. Build Go binary
CGO_ENABLED=0 go build \
    -ldflags "-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE" \
    -o bin/deploymonster
```

### 11.3 Embedded Static Files

The React UI is embedded via Go's `embed.FS`:

```go
//go:embed all:static
var staticFS embed.FS  // contains /index.html, /assets/, /chunks/, etc.

// In spa.go — newSPAHandler() serves from the embedded filesystem
sub, _ := fs.Sub(staticFS, "static")
fileServer := http.FileServer(http.FS(sub))
// Falls back to index.html for client-side routing (/apps, /admin, etc.)
```

### 11.4 Webhook Auto-Deploy Flow

Webhook-triggered automatic deployment bridges git events to the build pipeline:

```mermaid
flowchart LR
    GitPush["Git Push"] --> Webhook["POST /hooks/v1/{webhookID}"]
    Webhook --> Verify["Signature Verification"]
    Verify --> Parse["parsePayload()"]
    Parse --> Emit["emit EventWebhookReceived"]
    Emit --> Handler["DeployTriggerHandler.SubscribeWebhookDeploys"]
    Handler --> Lookup["GetApp(webhookID as appID)"]
    Lookup --> Check["git source && branch allowed && not frozen"]
    Check --> GitBuild["deployGitApp(triggeredBy=webhook)"]
    GitBuild --> Builder["build.Builder.Build()"]
    Builder --> Deploy["Create container, persist deployment"]
    Deploy --> Discovery["Publish app.deployed → discovery syncRoutes"]
    Discovery --> Route["Domain route live"]
```

**Components:**
- `internal/webhooks/receiver.go` — Inbound webhook endpoint, signature verification, payload parsing
- `internal/api/router.go` — Wires inbound webhook receiver, delivery tracker, and deploy trigger subscription
- `internal/api/handlers/deploy_trigger.go` — Shared manual/webhook git build+deploy path, deployment persistence, old-container cleanup
- `internal/api/handlers/event_webhooks.go` — Outbound event webhook CRUD API
- `internal/api/handlers/webhook_rotate.go`, `webhook_test_delivery.go`, `webhook_logs.go`, `webhook_replay.go` — webhook operations
- `internal/db/bolt.go` — legacy filename for SQLite-backed KV webhook secret storage

**Branch Filter Patterns:**
- Current app-level webhook deploy uses exact branch matching against the app's
  configured branch. Empty app branch or empty webhook branch does not block the
  deploy.

---

## 12. Master/Agent Mode

Same binary runs as master (control plane) or agent (worker node) via the `serve` command:

```bash
# Master (default)
deploymonster serve

# Agent (worker) — joins cluster via WebSocket
deploymonster serve --agent --master=host:8443 --token=<join-token>
```

Agent mode connects to the master's WebSocket endpoint and authenticates with a join token. No UI or API runs on agent nodes — only container operations dispatched from the master.

> **Note:** The agent WebSocket endpoint (`/api/v1/agent/ws`) is implemented in `internal/swarm/` module. The master listens on the agent WebSocket route for incoming agent connections and dispatches operations via the Swarm module's client/server communication.

### 12.1 Agent Protocol

```mermaid
sequenceDiagram
    participant Master
    participant Agent

    Master->>Agent: WebSocket connection (agent route)
    Agent-->>Master: Connect with join token

    loop Operations
        Master->>Agent: ContainerCreate/Stop/Remove/Restart
        Master->>Agent: ContainerLogs/Exec/ImagePull
        Master->>Agent: MetricsCollect/HealthCheck
        Agent-->>Master: Result/Error/MetricsReport
        Agent-->>Master: ContainerEvent/LogStream
    end
```

### 12.2 Message Types

**Master → Agent:**
```go
AgentMsgPing, AgentMsgContainerCreate, AgentMsgContainerStop,
AgentMsgContainerRemove, AgentMsgContainerRestart, AgentMsgContainerList,
AgentMsgContainerLogs, AgentMsgContainerExec, AgentMsgImagePull,
AgentMsgNetworkCreate, AgentMsgVolumeCreate, AgentMsgMetricsCollect,
AgentMsgHealthCheck, AgentMsgConfigUpdate
```

**Agent → Master:**
```go
AgentMsgPong, AgentMsgResult, AgentMsgError,
AgentMsgMetricsReport, AgentMsgContainerEvent,
AgentMsgServerStatus, AgentMsgLogStream
```

---

## 13. Security

### 13.1 Authentication & Authorization

| Mechanism | Details |
|-----------|---------|
| JWT | HS256, access=15min, refresh=7days |
| API Keys | bcrypt hash verification |
| Passwords | bcrypt cost 13 |
| Webhook Verification | HMAC signature |
| CSRF Protection | Double-submit cookie (`__Host-dm_csrf` + `X-CSRF-Token` header) |

### 13.2 CSRF Implementation

**Cookie-based double-submit pattern** — protects browser-authenticated requests:

| Component | Value |
|-----------|-------|
| Cookie name | `__Host-dm_csrf` (HttpOnly=false, Secure, SameSite=Lax) |
| Header name | `X-CSRF-Token` |
| Token length | 16 bytes (32 hex chars) |
| MaxAge | 24 hours |
| Token generation | `crypto/rand.Read` |

**Exemptions:**
- Safe methods (GET, HEAD, OPTIONS) — no CSRF check
- Requests with `Authorization` header (Bearer token) — programmatic clients
- Requests with `X-API-Key` header — API key auth

**Flow:**
1. User authenticates (login/register/refresh) → `SetCSRFCookie()` sets cookie
2. Frontend reads cookie via `document.cookie` and sends as `X-CSRF-Token` header
3. Mutating requests (POST/PUT/PATCH/DELETE) validate header matches cookie
4. Mismatch → 403 Forbidden response

**Security notes:**
- `__Host-` prefix enforced by browsers — prevents subdomain cookie injection
- `Secure` flag set only when TLS detected (works in dev without certs)
- Cookie is not HttpOnly so JavaScript can read it for header injection

### 13.3 Secrets Encryption

```mermaid
flowchart TD
    Secret["Plain Secret"] --> KDF["Argon2id KDF"]
    KDF --> Key["256-bit Key"]
    Secret --> AES["AES-256-GCM"]
    Key --> AES
    Nonce["Unique Nonce"] --> AES
    AES --> Encrypted["Encrypted Secret"]

    subgraph "Storage"
        Meta["Metadata (KV vault bucket)"]
        Data["Encrypted Data"]
    end

    Encrypted --> Meta
    Encrypted --> Data
```

### 13.4 Secret Rotation (JWT)

JWT signing keys support graceful rotation via `previous_secret_keys`:

| Setting | Value |
|---------|-------|
| Grace period | 20 minutes (RotationGracePeriod) |
| Min secret length | 32 characters (256 bits) |
| Previous keys | Unlimited |

**How it works:**
1. Current key signs new tokens
2. Previous keys accepted for validation within grace period
3. After grace period, previous keys are purged automatically

**Rotation procedure:**
```yaml
# 1. Add new secret_key
server:
  secret_key: "new-32-char-minimum-key-here"
  # 2. Move old key to previous_secret_keys
  previous_secret_keys:
    - "old-32-char-minimum-key-here"
```

**Runtime rotation (no restart):**
```go
jwtSvc.AddPreviousKey("old-key-here")  // Register old key as fallback
// After 20 minutes, old key is purged automatically
```

**Emergency revocation:**
```go
revoked := jwtSvc.RevokeAllPreviousKeys()  // Immediately invalidates all old keys
```

**Config environment variable:**
```bash
MONSTER_PREVIOUS_SECRET_KEYS=key1,key2,key3  # comma-separated list
```

### 13.4 Rate Limiting

- Per-IP global limit (default: 120/min)
- Per-tenant limits
- Per-endpoint limits
- KV-backed counters

### 13.5 Input Validation

- Request body size limit (10MB)
- Request timeout (30s)
- CSRF token validation
- XSS sanitization on error messages

---

## 14. Key Patterns

### 14.1 Dependency Injection

No global state — all dependencies injected via `Core`:

```go
func (m *Module) Init(ctx context.Context, c *core.Core) error {
    m.core = c
    m.store = c.Store
    m.events = c.Events
    return nil
}
```

### 14.2 Service Lookup

Interface-based lookup without imports:

```go
container := m.core.Services.Container
// Instead of direct import: deploymodule.NewDockerClient()
```

### 14.3 Module Communication

Modules communicate via:
1. **Direct method calls** — through service interfaces
2. **Events** — for loose coupling and async operations

### 14.4 Graceful Shutdown

```mermaid
sequenceDiagram
    participant OS as OS (SIGHUP/SIGTERM)
    participant Core
    participant Modules
    participant EventBus
    
    OS->>Core: Shutdown signal
    Core->>Modules: StopAll() in reverse dep order
    Modules->>EventBus: Drain async handlers
    Modules->>Modules: Cancel in-flight operations
    Core->>Modules: 30s deadline
    Modules-->>Core: Graceful stop
    Core-->>OS: Shutdown complete
```

### 14.5 Idempotency

Mutating requests include `Idempotency-Key` header:

```go
// Stored in KV idempotency bucket
// Prevents duplicate operations on retry
```

### 14.6 Test Patterns

| Pattern | Description |
|---------|-------------|
| Mock interfaces | Optional function fields + call tracking |
| Table-driven tests | `t.Run` subtests |
| Integration tests | Gated with `//go:build integration` tags |
| Contract tests | Shared logic in `store_contract_test.go` |
| Fuzzing | `go test -fuzz` for parsers |

### 14.7 Integration Test Infrastructure

**Running Tests:**

```bash
# All tests with race detection + coverage
make test

# Short tests only (skip integration)
make test-short

# Run integration tests only
go test -tags=integration ./...

# Run specific package tests
go test ./internal/api/...

# Run tests with verbose output
go test -v ./internal/deploy/...

# Run benchmarks
go test -bench=. ./internal/core/...

# Fuzzing tests
go test -fuzz=FuzzName ./internal/auth/...
```

**Test Categories:**

| Tag | Purpose | Location |
|-----|---------|----------|
| `integration` | Full stack tests requiring database/docker | `*_integration_test.go` |
| (default) | Unit tests, no external dependencies | `*_test.go` |

**Test Database Setup:**

Integration tests use an in-memory SQLite database by default. For PostgreSQL integration tests:

```bash
# Start PostgreSQL container
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=testpass postgres:16

# Run tests with PostgreSQL
MONSTER_DB_URL="postgres://postgres:testpass@localhost:5432/testdb?sslmode=disable" \
  go test -tags=integration ./internal/db/...
```

**Key Test Files:**

| File | Purpose |
|------|---------|
| `internal/api/integration_test.go` | API integration tests |
| `internal/api/router_cross_tenant_mutation_test.go` | Multi-tenant security tests |
| `internal/swarm/master_agent_integration_test.go` | Master/agent WebSocket protocol |
| `internal/deploy/container_test.go` | Container operations |
| `internal/auth/password_test.go` | Password hashing tests |
| `internal/notifications/notifications_100_test.go` | Notification delivery |

**Mock Patterns:**

```go
// Interface with optional function fields for testing
type ContainerManager struct {
    CreateAndStartFunc func(ctx context.Context, app *core.App) error
    StopFunc           func(ctx context.Context, appID string) error
}

func (m *ContainerManager) CreateAndStart(ctx context.Context, app *core.App) error {
    if m.CreateAndStartFunc != nil {
        return m.CreateAndStartFunc(ctx, app)
    }
    return errors.New("not implemented")
}
```

**Contract Tests:**

Store implementations (SQLite, PostgreSQL) share contract tests that verify:
- CRUD operations
- Pagination
- Error handling
- Concurrent access

Run contract tests:
```bash
go test -run Contract ./internal/db/...
```

---

## 15. Key Source Files

For accurate line counts, run:
```bash
find internal web/src -name "*.go" -o -name "*.ts" -o -name "*.tsx" | xargs wc -l
```

Key source files (see actual file for current line counts):

| Path | Purpose |
|------|---------|
| `cmd/deploymonster/main.go` | Entry point, CLI commands, serve + agent mode |
| `internal/core/module.go` | Module interface definition |
| `internal/core/app.go` | Core struct, Run lifecycle, RegisterModule |
| `internal/core/registry.go` | Module registration, topological sort |
| `internal/core/store.go` | Store interface + all data models |
| `internal/core/events.go` | EventBus with sync/async handlers, WebhookEventData |
| `internal/core/interfaces.go` | Service interfaces (ContainerRuntime, legacy-named BoltStorer KV interface) |
| `internal/core/config.go` | Config struct + MONSTER_* env overrides |
| `internal/core/scheduler.go` | Cron-like task scheduler |
| `internal/api/router.go` | HTTP route registration, middleware chain, webhook handlers |
| `internal/api/module.go` | API module lifecycle |
| `internal/api/middleware/middleware.go` | Auth, CORS, Recovery, RequestLogger |
| `internal/api/middleware/*.go` | Rate limiting, metrics, CSRF, idempotency, audit |
| `internal/api/handlers/event_webhooks.go` | Outbound event webhook CRUD API |
| `internal/webhooks/receiver.go` | Inbound webhook endpoint + signature verification |
| `internal/db/module.go` | SQLite/Postgres/KV initialization |
| `internal/db/bolt.go` | SQLite-backed KV store, GetWebhook, GetWebhookSecret |
| `internal/deploy/module.go` | Docker container management |
| `internal/secrets/module.go` | AES-256-GCM encrypted vault |
| `internal/ingress/module.go` | Reverse proxy + ACME certificate management |
| `internal/notifications/module.go` | Multi-channel notifications |
| `internal/swarm/module.go` | Swarm Orchestrator (master/agent WebSocket) |
| `web/src/App.tsx` | React routing + auth gate |
| `web/src/api/client.ts` | Fetch wrapper with JWT refresh, CSRF, retry |

---

## 16. Getting Started

### Development

```bash
# Clone and setup
git clone https://github.com/deploymonster/deploy-monster
cd deploy-monster

# Install git hooks
./scripts/setup-git-hooks.sh

# Build and run (full pipeline: React build → embed → Go build)
./scripts/build.sh && ./bin/deploymonster serve

# Or run in development mode (no embedded UI)
go run ./cmd/deploymonster serve

# Run tests
make test           # All tests with race detector + coverage
make test-short     # Skip integration tests
make lint           # golangci-lint
```

### Docker

```bash
# Build Docker image
docker build -t deploymonster .

# Run container
docker run -p 8443:8443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd)/data:/data \
  deploymonster serve
```
