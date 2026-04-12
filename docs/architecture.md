# DeployMonster Architecture

## Overview

DeployMonster is a **modular monolith** — a single 22MB binary containing everything needed to run a full PaaS platform. No microservices, no external dependencies, no Docker containers required to run the platform itself.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                        DeployMonster Binary (22MB)                            │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌──────────┐ ┌─────────┐ ┌──────────┐  │
│  │ Web UI  │ │REST API │ │   SSE   │ │ Webhooks │ │ Ingress │ │   MCP    │  │
│  │ React   │ │ 224 eps │ │ Stream  │ │  In+Out  │ │:80/:443 │ │ 9 Tools  │  │
│  └────┬────┘ └────┬────┘ └────┬────┘ └────┬─────┘ └────┬────┘ └────┬─────┘  │
│       │          │          │          │            │           │          │
│       └──────────┴──────────┴──────────┴────────────┴───────────┘          │
│                                    │                                        │
│  ┌─────────────────────────────────┴────────────────────────────────────┐  │
│  │                          Core Engine                                  │  │
│  │  ┌─────────────┐ ┌──────────────┐ ┌─────────────┐ ┌───────────────┐  │  │
│  │  │  Registry   │ │  EventBus    │ │   Store     │ │   Services    │  │  │
│  │  │ (modules)   │ │ (pub/sub)    │ │ (data)      │ │ (interfaces)  │  │  │
│  │  └─────────────┘ └──────────────┘ └─────────────┘ └───────────────┘  │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                                    │                                        │
│  ┌─────────────────────────────────┴────────────────────────────────────┐  │
│  │                      20 Auto-Registered Modules                       │  │
│  │                                                                        │  │
│  │  ┌──────┐ ┌──────┐ ┌───────┐ ┌────────┐ ┌─────────┐ ┌──────────────┐  │  │
│  │  │ auth │ │ build│ │deploy │ │ingress │ │ secrets │ │ notifications│  │  │
│  │  └──────┘ └──────┘ └───────┘ └────────┘ └─────────┘ └──────────────┘  │  │
│  │  ┌──────┐ ┌──────┐ ┌───────┐ ┌────────┐ ┌─────────┐ ┌──────────────┐  │  │
│  │  │  db  │ │ dns  │ │ backup│ │  vps   │ │ billing │ │  webhooks    │  │  │
│  │  └──────┘ └──────┘ └───────┘ └────────┘ └─────────┘ └──────────────┘  │  │
│  │  ┌──────┐ ┌──────┐ ┌───────┐ ┌────────┐ ┌─────────┐ ┌──────────────┐  │  │
│  │  │ api  │ │ swarm│ │compose│ │ market │ │   mcp   │ │  enterprise  │  │  │
│  │  └──────┘ └──────┘ └───────┘ └────────┘ └─────────┘ └──────────────┘  │  │
│  │  ┌──────┐ ┌──────┐                                                        │  │
│  │  │resource│ │discovery│ ...                                              │  │
│  │  └──────┘ └──────┘                                                        │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                                    │                                        │
│  ┌─────────────────────────────────┴────────────────────────────────────┐  │
│  │                          Infrastructure Layer                          │  │
│  │  ┌─────────────┐ ┌──────────────┐ ┌─────────────┐ ┌───────────────┐  │  │
│  │  │ SQLite+BBolt│ │  Docker SDK  │ │  SSH Pool   │ │ HTTP Clients  │  │  │
│  │  └─────────────┘ └──────────────┘ └─────────────┘ └───────────────┘  │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## 1. Module System

Every feature in DeployMonster is a **module** implementing the `core.Module` interface:

```go
type Module interface {
    // Identity
    ID() string              // Unique identifier: "auth", "deploy", "secrets"
    Name() string            // Display name: "Authentication", "Deploy Engine"
    Version() string         // Semantic version: "1.0.0"
    Dependencies() []string  // Module IDs this depends on: ["core.db"]

    // Lifecycle
    Init(ctx context.Context, core *Core) error   // Setup, receive dependencies
    Start(ctx context.Context) error               // Begin background work
    Stop(ctx context.Context) error                // Graceful shutdown

    // Observability
    Health() HealthStatus                          // ok / degraded / down

    // Integration
    Routes() []Route          // HTTP endpoints to register
    Events() []EventHandler   // Event subscriptions
}
```

### Module Registration

Modules register themselves via Go's `init()` function:

```go
// internal/auth/module.go
func init() {
    core.RegisterModule(func() core.Module { return New() })
}
```

### Lifecycle Management

The core engine manages module lifecycle in dependency order:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Startup Sequence                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. DISCOVER: Scan all init() functions, collect modules        │
│                    ↓                                            │
│  2. RESOLVE: Build dependency graph, topological sort           │
│                    ↓                                            │
│  3. INIT: Call Init() in dependency order                       │
│     db → auth → secrets → deploy → api → ...                    │
│                    ↓                                            │
│  4. START: Call Start() in dependency order                     │
│     Background goroutines begin (cron jobs, listeners, etc.)     │
│                    ↓                                            │
│  5. SERVE: HTTP server starts, accept connections                │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                       Shutdown Sequence                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. SIGNAL: SIGINT/SIGTERM received                             │
│                    ↓                                            │
│  2. STOP: Call Stop() in REVERSE dependency order               │
│     api → deploy → secrets → auth → db                          │
│                    ↓                                            │
│  3. CLOSE: Close database connections, cleanup                  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Module List (20 modules)

| Module | ID | Purpose | Dependencies |
|--------|-----|---------|--------------|
| **db** | `core.db` | SQLite + BBolt storage | none |
| **auth** | `auth` | JWT, TOTP, OAuth, sessions | `core.db` |
| **secrets** | `secrets` | AES-256-GCM vault | `core.db` |
| **deploy** | `deploy` | Docker orchestration | `core.db` |
| **build** | `build` | 14 language detectors | `core.db` |
| **ingress** | `ingress` | Reverse proxy + SSL | `core.db` |
| **dns** | `dns` | Cloudflare DNS sync | `core.db` |
| **backup** | `backup` | Local + S3 backups | `core.db` |
| **vps** | `vps` | Server provisioning | `core.db` |
| **webhooks** | `webhooks` | Git webhook receiver | `core.db` |
| **notifications** | `notifications` | Slack, Discord, Email | `core.db` |
| **billing** | `billing` | Stripe integration | `core.db` |
| **enterprise** | `enterprise` | WHMCS, SSO, audit | `core.db` |
| **swarm** | `swarm` | Multi-server cluster | `core.db` |
| **compose** | `compose` | Docker Compose parser | `core.db` |
| **marketplace** | `marketplace` | 25 app templates | `core.db` |
| **mcp** | `mcp` | AI tool server | `core.db` |
| **api** | `api` | 224 REST endpoints | ALL |
| **discovery** | `discovery` | Container discovery | `core.db` |
| **resource** | `resource` | Metrics + monitoring | `core.db` |

---

## 2. Data Access — Store Interface

**Rule: Modules NEVER access the database directly.**

All data access goes through `core.Store` interface:

```go
type Store interface {
    TenantStore     // Tenants (organizations/teams)
    UserStore       // Users, passwords, memberships
    AppStore        // Applications
    DeploymentStore // Deployment history
    DomainStore     // Custom domains
    ProjectStore    // Project groupings
    RoleStore       // RBAC roles
    AuditStore      // Audit logs
    SecretStore     // Encrypted secrets
    InviteStore     // Team invitations

    Close() error
    Ping(ctx context.Context) error
}
```

### Store Sub-Interfaces

```go
// Example: AppStore
type AppStore interface {
    CreateApp(ctx context.Context, app *Application) error
    GetApp(ctx context.Context, id string) (*Application, error)
    UpdateApp(ctx context.Context, app *Application) error
    ListAppsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]Application, int, error)
    ListAppsByProject(ctx context.Context, projectID string) ([]Application, error)
    UpdateAppStatus(ctx context.Context, id, status string) error
    DeleteApp(ctx context.Context, id string) error
}
```

### Implementations

| Implementation | Package | Use Case |
|----------------|---------|----------|
| **SQLite** | `internal/db` | Default, embedded, zero-config |
| **PostgreSQL** | Planned | Enterprise, horizontal scaling |

### Usage Pattern

```go
// ❌ WRONG: Direct database access
db.Exec("INSERT INTO apps...")

// ✅ CORRECT: Use Store interface
func (m *Module) DoSomething(ctx context.Context) error {
    app, err := m.store.GetApp(ctx, appID)
    if err != nil {
        return err
    }
    app.Status = "running"
    return m.store.UpdateApp(ctx, app)
}
```

---

## 3. Event System

Modules communicate via `core.EventBus` — an in-process pub/sub system.

### Event Flow

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   deploy     │────▶│   EventBus   │────▶│ notifications│
│  (publisher) │     │  (router)    │     │ (subscriber) │
└──────────────┘     └──────────────┘     └──────────────┘
                            │
                            ▼
                     ┌──────────────┐
                     │   billing    │
                     │ (subscriber) │
                     └──────────────┘
```

### Event Types

```go
// Defined in internal/core/events.go
const (
    EventAppCreated    = "app.created"
    EventAppDeployed   = "app.deployed"
    EventAppDeleted    = "app.deleted"
    EventDomainAdded   = "domain.added"
    EventBackupComplete = "backup.complete"
    // ...
)
```

### Publishing Events

```go
// Deploy module publishes
m.events.Publish(ctx, core.NewEvent(core.EventAppDeployed, "deploy",
    core.DeployEventData{
        AppID:       appID,
        Version:     5,
        Image:       "ghcr.io/user/app:v5",
        ContainerID: "abc123",
    },
))
```

### Subscribing to Events

```go
// Notifications module subscribes
func (m *Module) Events() []core.EventHandler {
    return []core.EventHandler{
        {
            Event:  "app.*",  // Prefix matching
            Sync:   false,    // Async handling
            Handle: m.handleAppEvent,
        },
    }
}

func (m *Module) handleAppEvent(ctx context.Context, evt core.Event) error {
    switch evt.Type {
    case core.EventAppDeployed:
        data := evt.Data.(core.DeployEventData)
        return m.sendNotification(ctx, data)
    }
    return nil
}
```

### Handler Types

| Type | Blocking | Use Case |
|------|----------|----------|
| **Sync** | Yes | Critical paths, must complete before proceeding |
| **Async** | No | Notifications, logging, non-critical work |

---

## 4. Service Registry

Modules expose functionality to each other via typed interfaces in `core.Services`:

```go
type Services struct {
    Container     ContainerRuntime     // Docker operations
    SSH           SSHClient            // Remote command execution
    Secrets       SecretResolver       // ${SECRET:name} resolution
    Notifications NotificationSender   // Multi-channel dispatch
    Webhooks      OutboundWebhookSender // External webhook delivery

    // Provider registries (multiple implementations)
    dnsProviders    map[string]DNSProvider     // "cloudflare", "route53"
    backupStorages  map[string]BackupStorage   // "local", "s3"
    vpsProvisioners map[string]VPSProvisioner  // "hetzner", "digitalocean"
    gitProviders    map[string]GitProvider     // "github", "gitlab"
}
```

### Registration Pattern

```go
// DNS module registers Cloudflare provider
func (m *Module) Init(ctx context.Context, c *core.Core) error {
    cf := cloudflare.New(c.Config.DNS.Cloudflare)
    c.Services.RegisterDNSProvider("cloudflare", cf)
    return nil
}
```

### Usage Pattern

```go
// Ingress module uses DNS provider
func (m *Module) issueCertificate(ctx context.Context, domain string) error {
    provider := m.core.Services.DNSProvider("cloudflare")
    if provider == nil {
        return errors.New("cloudflare not configured")
    }
    return provider.CreateRecord(ctx, core.DNSRecord{
        Type:  "TXT",
        Name:  "_acme-challenge." + domain,
        Value: challengeToken,
    })
}
```

---

## 5. Ingress Gateway

Custom reverse proxy — **no Traefik/Nginx required**.

```
┌───────────────────────────────────────────────────────────────────────────┐
│                           Ingress Gateway                                  │
├───────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│   Internet                                                                │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │                    :80 (HTTP)                                     │   │
│   │                    ↓                                              │   │
│   │              Redirect to HTTPS                                    │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │                    :443 (HTTPS)                                   │   │
│   │                    ↓                                              │   │
│   │              TLS Termination (Let's Encrypt / Self-signed)        │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │                    Route Table                                    │   │
│   │                                                                    │   │
│   │   app.example.com        → container:3000                         │   │
│   │   api.example.com        → api-handler:8443                      │   │
│   │   *.example.com          → wildcard-handler                       │   │
│   │   example.com/path/*     → path-based-routing                     │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │                    Middleware Chain                               │   │
│   │                                                                    │   │
│   │   Rate Limit → CORS → Compression → Auth → Logging               │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │                    Load Balancer                                  │   │
│   │                                                                    │   │
│   │   Strategies: round-robin | least-conn | ip-hash | random        │   │
│   │               weighted-round-robin                                │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │                    Backend Pool                                   │   │
│   │                                                                    │   │
│   │   container-1:3000  container-2:3000  container-3:3000           │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

### Certificate Management

```
┌─────────────────────────────────────────────────────────────────┐
│                    SSL Certificate Flow                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. Domain added to app                                         │
│         ↓                                                       │
│  2. Ingress detects no certificate                              │
│         ↓                                                       │
│  3. Request certificate from Let's Encrypt                      │
│         ↓                                                       │
│  4. DNS-01 challenge (via Cloudflare API)                       │
│         ↓                                                       │
│  5. Certificate issued, stored in BBolt                         │
│         ↓                                                       │
│  6. Auto-renewal 30 days before expiry                          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 6. Build Pipeline

```
┌───────────────────────────────────────────────────────────────────────────┐
│                          Build Pipeline                                    │
├───────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│   Git Push                                                                │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │   Webhook Receiver                                                │   │
│   │   GitHub / GitLab / Gitea / Bitbucket                            │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │   Git Clone                                                       │   │
│   │   git clone --depth 1 --branch main git@github.com:user/repo.git  │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │   Project Type Detection                                          │   │
│   │                                                                    │   │
│   │   ✓ package.json      → Node.js                                   │   │
│   │   ✓ next.config.js    → Next.js                                   │   │
│   │   ✓ go.mod            → Go                                        │   │
│   │   ✓ requirements.txt  → Python                                    │   │
│   │   ✓ Cargo.toml        → Rust                                      │   │
│   │   ✓ composer.json     → PHP                                       │   │
│   │   ✓ pom.xml           → Java                                      │   │
│   │   ✓ *.csproj          → .NET                                      │   │
│   │   ✓ Gemfile           → Ruby                                      │   │
│   │   ✓ Dockerfile        → Docker (use as-is)                        │   │
│   │   ✓ docker-compose.yml→ Docker Compose                            │   │
│   │   ✓ static files      → Static site                               │   │
│   │   ... 14 types total                                             │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │   Dockerfile Generation (if not provided)                         │     │
│   │                                                                    │   │
│   │   Templates in internal/build/dockerfiles/                        │   │
│   │   - node.Dockerfile                                               │   │
│   │   - nextjs.Dockerfile                                             │   │
│   │   - go.Dockerfile                                                 │   │
│   │   - python.Dockerfile                                             │   │
│   │   - ... 12 templates                                              │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │   Docker Build                                                    │   │
│   │   docker build -t ghcr.io/user/app:v1 .                          │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │   Push to Registry                                                │   │
│   │   docker push ghcr.io/user/app:v1                                │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│      │                                                                    │
│      ▼                                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │   Deploy                                                          │   │
│   │   Create new container, stop old, update routing                  │   │
│   └──────────────────────────────────────────────────────────────────┘   │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

---

## 7. Master/Agent Architecture

Same binary, two modes:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Master/Agent Mode                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   ┌─────────────────────────────┐     ┌─────────────────────────────┐  │
│   │        MASTER NODE          │     │        AGENT NODES          │  │
│   │   (deploymonster serve)     │     │ (deploymonster serve --agent)│  │
│   │                             │     │                             │  │
│   │   ┌───────────────────┐     │     │   ┌───────────────────┐     │  │
│   │   │ Full Platform     │     │     │   │ Worker Only       │     │  │
│   │   │ - Web UI          │     │     │   │ - Docker runtime  │     │  │
│   │   │ - API Server      │     │     │   │ - Execute builds  │     │  │
│   │   │ - Database        │     │     │   │ - Run containers  │     │  │
│   │   │ - Ingress         │     │     │   │ - Report metrics  │     │  │
│   │   │ - Billing         │     │     │   │                   │     │  │
│   │   └───────────────────┘     │     │   └───────────────────┘     │  │
│   │             │               │     │             │               │  │
│   │             │  WebSocket    │     │             │               │  │
│   │             │  Protocol     │     │             │               │  │
│   └─────────────┼───────────────┘     └─────────────┼───────────────┘  │
│                 │                                   │                   │
│                 └───────────────────────────────────┘                   │
│                              │                                           │
│                              ▼                                           │
│   ┌─────────────────────────────────────────────────────────────────┐  │
│   │                    WebSocket Protocol                            │  │
│   │                                                                  │  │
│   │   Agent connects: ws://master:8443/api/v1/agent/connect         │  │
│   │                                                                  │  │
│   │   Messages (JSON):                                               │  │
│   │   { "type": "ping", "ts": 1234567890 }                          │  │
│   │   { "type": "metrics", "cpu": 45.2, "mem": 1024, ... }          │  │
│   │   { "type": "build", "app_id": "abc", "image": "..." }          │  │
│   │   { "type": "deploy", "app_id": "abc", "container": "..." }     │  │
│   └─────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Agent Bootstrap

```
┌─────────────────────────────────────────────────────────────────┐
│                    Agent Bootstrap Flow                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. Master generates bootstrap command                          │
│     deploymonster serve --agent \                               │
│       --master https://master.example.com \                     │
│       --token join-abc123                                       │
│         ↓                                                       │
│  2. SSH into target server                                      │
│         ↓                                                       │
│  3. Install Docker (if needed)                                  │
│         ↓                                                       │
│  4. Download DeployMonster binary                               │
│     curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --version=v0.0.1  │
│         ↓                                                       │
│  5. Create systemd service                                      │
│     /etc/systemd/system/deploymonster-agent.service            │
│         ↓                                                       │
│  6. Start agent, connect to master                              │
│         ↓                                                       │
│  7. Agent appears in master dashboard                           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 8. Secret Vault

AES-256-GCM encrypted secrets with scope-based resolution.

```
┌─────────────────────────────────────────────────────────────────┐
│                    Secret Resolution                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   Scope Hierarchy (most specific to least):                     │
│                                                                 │
│   app/app123       → App-level secrets                          │
│         ↓                                                       │
│   project/proj456  → Project-level secrets                      │
│         ↓                                                       │
│   tenant/tenant789 → Tenant-level secrets                       │
│         ↓                                                       │
│   global           → Global secrets (fallback)                  │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   Usage in environment variables:                               │
│                                                                 │
│   DATABASE_URL=postgres://user:${SECRET:db-password}@host      │
│   API_KEY=${SECRET:api-key}                                     │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   Resolution Flow:                                              │
│                                                                 │
│   1. Parse template, find ${SECRET:name} patterns               │
│   2. Build scope hierarchy: [app/123, tenant/456, global]       │
│   3. For each pattern:                                          │
│      a. Try scope[0] → GetSecretByScopeAndName                  │
│      b. If not found, try scope[1]                              │
│      c. Continue until found or error                           │
│   4. Decrypt value using AES-256-GCM                            │
│   5. Replace placeholder with decrypted value                   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 9. API Structure

224 REST endpoints organized by feature:

```
/api/v1/
├── auth/
│   ├── POST   /login              # JWT token
│   ├── POST   /logout             # Invalidate session
│   ├── POST   /refresh            # Refresh token
│   ├── POST   /2fa/enable         # Enable TOTP
│   └── POST   /2fa/verify         # Verify TOTP code
│
├── apps/
│   ├── GET    /                   # List apps
│   ├── POST   /                   # Create app
│   ├── GET    /:id                # Get app details
│   ├── PUT    /:id                # Update app
│   ├── DELETE /:id                # Delete app
│   ├── POST   /:id/deploy         # Trigger deploy
│   ├── GET    /:id/logs           # Container logs
│   ├── POST   /:id/restart        # Restart container
│   └── GET    /:id/stats          # Resource metrics
│
├── domains/
│   ├── GET    /                   # List domains
│   ├── POST   /                   # Add domain
│   └── DELETE /:id                # Remove domain
│
├── secrets/
│   ├── GET    /                   # List secrets (metadata only)
│   ├── POST   /                   # Create secret
│   ├── PUT    /:id                # Update secret value
│   └── DELETE /:id                # Delete secret
│
├── deployments/
│   ├── GET    /                   # List deployments
│   └── GET    /:id/logs           # Build logs
│
├── servers/
│   ├── GET    /                   # List servers
│   ├── POST   /                   # Provision server
│   └── DELETE /:id                # Delete server
│
├── billing/
│   ├── GET    /plans              # List plans
│   ├── POST   /subscribe          # Subscribe to plan
│   └── GET    /usage              # Usage metrics
│
└── admin/
    ├── GET    /tenants            # List all tenants
    ├── GET    /users              # List all users
    └── GET    /audit              # Audit logs
```

---

## 10. Database Schema

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        SQLite Schema                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐              │
│   │   tenants   │     │    users    │     │team_members │              │
│   ├─────────────┤     ├─────────────┤     ├─────────────┤              │
│   │ id          │◀────│ tenant_id   │     │ id          │              │
│   │ name        │     │ id          │◀────│ user_id     │              │
│   │ slug        │     │ email       │     │ tenant_id   │────▶┐        │
│   │ plan_id     │     │ password    │     │ role_id     │     │        │
│   │ owner_id    │────▶│ name        │     │ status      │     │        │
│   │ status      │     │ totp_enabled│     └─────────────┘     │        │
│   └─────────────┘     │ last_login  │                         │        │
│         │             └─────────────┘                         │        │
│         │                   │                                 │        │
│         ▼                   │                                 ▼        │
│   ┌─────────────┐           │                           ┌─────────────┐ │
│   │  projects   │           │                           │    roles    │ │
│   ├─────────────┤           │                           ├─────────────┤ │
│   │ id          │           │                           │ id          │◀┘
│   │ tenant_id   │───────────┼───────────────────────────│ tenant_id   │
│   │ name        │           │                           │ name        │
│   │ environment │           │                           │ permissions │
│   └─────────────┘           │                           └─────────────┘
│         │                   │
│         ▼                   │
│   ┌─────────────┐           │
│   │    apps     │           │
│   ├─────────────┤           │
│   │ id          │           │
│   │ project_id  │───────────┘
│   │ tenant_id   │
│   │ name        │
│   │ source_url  │
│   │ status      │
│   └─────────────┘
│         │
│         ▼
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   │ deployments │     │   domains   │     │   secrets   │
│   ├─────────────┤     ├─────────────┤     ├─────────────┤
│   │ id          │     │ id          │     │ id          │
│   │ app_id      │────▶│ app_id      │────▶│ scope       │
│   │ version     │     │ fqdn        │     │ name        │
│   │ image       │     │ type        │     │ type        │
│   │ status      │     │ verified    │     │ current_ver │
│   │ build_log   │     └─────────────┘     └─────────────┘
│   └─────────────┘                               │
│                                                 ▼
│                                         ┌─────────────┐
│                                         │secret_vers  │
│                                         ├─────────────┤
│                                         │ id          │
│                                         │ secret_id   │
│                                         │ version     │
│                                         │ value_enc   │
│                                         └─────────────┘
│
├─────────────────────────────────────────────────────────────────────────┤
│                        BBolt Buckets                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   config.{key}         → Platform configuration                         │
│   cache.{key}          → TTL-based cache entries                        │
│   metrics.{host}       → Per-host metrics (5min retention)              │
│   sessions.{id}        → User sessions                                  │
│   certs.{domain}       → SSL certificates                               │
│   apikeys.{id}         → API key metadata                               │
│   webhooks.{id}        → Webhook configurations                         │
│   audit.{id}           → Audit log buffer                               │
│   ... 30+ buckets                                                      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 11. Security Model

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Security Layers                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   Layer 1: Network                                                      │
│   ┌─────────────────────────────────────────────────────────────────┐  │
│   │ TLS 1.3 + Let's Encrypt certificates                             │  │
│   │ HTTP → HTTPS redirect                                            │  │
│   │ HSTS headers                                                     │  │
│   └─────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│   Layer 2: Authentication                                               │
│   ┌─────────────────────────────────────────────────────────────────┐  │
│   │ JWT tokens (HS256, key rotation) with 15min expiry               │  │
│   │ bcrypt password hashing (cost 12)                                │  │
│   │ TOTP 2FA (RFC 6238)                                              │  │
│   │ OAuth 2.0 (Google, GitHub)                                       │  │
│   └─────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│   Layer 3: Authorization                                                │
│   ┌─────────────────────────────────────────────────────────────────┐  │
│   │ Role-Based Access Control (RBAC)                                 │  │
│   │ 6 built-in roles: super_admin, admin, developer, viewer, etc.   │  │
│   │ Tenant isolation (row-level security)                            │  │
│   │ API key scopes                                                   │  │
│   └─────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│   Layer 4: Data Protection                                              │
│   ┌─────────────────────────────────────────────────────────────────┐  │
│   │ AES-256-GCM encryption for secrets                               │  │
│   │ Argon2id key derivation                                          │  │
│   │ Environment variables encrypted at rest                          │  │
│   └─────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│   Layer 5: Audit                                                        │
│   ┌─────────────────────────────────────────────────────────────────┐  │
│   │ All state-changing operations logged                             │  │
│   │ IP address tracking                                              │  │
│   │ User agent logging                                               │  │
│   │ Retention policies                                               │  │
│   └─────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 12. File Structure

```
DeployMonster/
├── cmd/
│   └── deploymonster/          # Entry point
│       └── main.go
│
├── internal/
│   ├── core/                   # Core engine
│   │   ├── module.go           # Module interface
│   │   ├── interfaces.go       # Service interfaces
│   │   ├── store.go            # Store interface + models
│   │   ├── events.go           # EventBus
│   │   ├── registry.go         # Module registry
│   │   ├── config.go           # Configuration
│   │   └── ...
│   │
│   ├── db/                     # SQLite + BBolt storage
│   │   ├── sqlite.go
│   │   ├── bolt.go
│   │   └── models/
│   │
│   ├── api/                    # REST API
│   │   ├── server.go
│   │   ├── router.go
│   │   ├── handlers/           # 115 handlers
│   │   └── middleware/
│   │
│   ├── auth/                   # Authentication
│   ├── deploy/                 # Docker orchestration
│   ├── build/                  # Build pipeline
│   ├── ingress/                # Reverse proxy
│   ├── dns/                    # DNS providers
│   ├── secrets/                # Secret vault
│   ├── backup/                 # Backup storage
│   ├── vps/                    # Server provisioning
│   ├── webhooks/               # Git webhook receiver
│   ├── notifications/          # Alert channels
│   ├── billing/                # Stripe integration
│   ├── enterprise/             # Enterprise features
│   ├── swarm/                  # Multi-server clustering
│   ├── compose/                # Docker Compose parser
│   ├── marketplace/            # App templates
│   ├── mcp/                    # AI tool server
│   ├── discovery/              # Container discovery
│   └── resource/               # Metrics + monitoring
│
├── web/                        # React frontend
│   ├── src/
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── pages/
│   │   └── lib/
│   ├── package.json
│   └── vite.config.ts
│
├── docs/
│   ├── architecture.md         # This file
│   ├── api-reference.md
│   ├── openapi.yaml
│   └── ...
│
├── scripts/
│   └── build.sh                # Build script
│
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Summary

DeployMonster is a **modular monolith** that packs enterprise-grade PaaS capabilities into a single 22MB binary:

| Feature | Implementation |
|---------|---------------|
| **Architecture** | Modular monolith, 20 auto-registered modules |
| **Data** | SQLite + BBolt (PostgreSQL planned) |
| **API** | 224 REST endpoints |
| **Auth** | JWT + bcrypt + TOTP + OAuth |
| **Ingress** | Custom reverse proxy (no Traefik/Nginx) |
| **SSL** | Let's Encrypt auto-certificates |
| **Build** | 14 language detectors, 12 Dockerfile templates |
| **Secrets** | AES-256-GCM encrypted vault |
| **Scaling** | Master/Agent clustering |
| **AI** | MCP server with 9 tools |
| **Tests** | 245 Go test files, 65 React tests, 92.8% coverage |

The key design principle: **everything is a module**. Each feature implements the same `core.Module` interface, registers itself, and communicates via typed interfaces and the event bus. No direct database access, no cross-package imports, true modularity.
