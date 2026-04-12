# DeployMonster Architecture

This document provides a comprehensive overview of DeployMonster's architecture, design patterns, and component interactions.

## Table of Contents

1. [High-Level Overview](#high-level-overview)
2. [Core Architecture](#core-architecture)
3. [Module System](#module-system)
4. [Event System](#event-system)
5. [Data Layer](#data-layer)
6. [Master/Agent Architecture](#masteragent-architecture)
7. [Deployment Pipeline](#deployment-pipeline)
8. [Ingress & Load Balancing](#ingress--load-balancing)
9. [Service Interfaces](#service-interfaces)
10. [Frontend Architecture](#frontend-architecture)

---

## High-Level Overview

DeployMonster is a **self-hosted Platform as a Service (PaaS)** built as a **modular monolith** in Go. It replaces solutions like Coolify, Dokploy, and CapRover with enterprise-grade features while maintaining deployment simplicity.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              DeployMonster                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐                   │
│   │   React UI  │    │  REST API   │    │   WebSocket │                   │
│   │  (embedded) │◄──►│  (gin-like) │◄──►│   (agents)  │                   │
│   └─────────────┘    └──────┬──────┘    └──────┬──────┘                   │
│                             │                    │                          │
│                      ┌──────▼────────────────────▼──────┐                  │
│                      │           Core Engine            │                  │
│                      │  ┌─────────┐  ┌──────────────┐  │                  │
│                      │  │ Module  │  │  Event Bus   │  │                  │
│                      │  │Registry │  │  (pub/sub)   │  │                  │
│                      │  └─────────┘  └──────────────┘  │                  │
│                      │  ┌─────────┐  ┌──────────────┐  │                  │
│                      │  │Services │  │  Scheduler   │  │                  │
│                      │  │Registry │  │  (cron jobs) │  │                  │
│                      │  └─────────┘  └──────────────┘  │                  │
│                      └─────────────────────────────────┘                  │
│                                    │                                        │
│                      ┌─────────────▼─────────────┐                         │
│                      │      Store Interface      │                         │
│                      │   (SQLite / PostgreSQL)   │                         │
│                      └───────────────────────────┘                         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Statistics

| Metric | Value |
|--------|-------|
| Language | Go 1.26+ |
| Frontend | React 19 + Vite 8 + Tailwind CSS 4 |
| Source Lines of Code | 27,000+ |
| Test Lines of Code | 47,000+ |
| Test Coverage | 97%+ |
| Modules | 20 |
| API Endpoints | 224 |
| Binary Size | 22MB (16MB stripped) |

---

## Core Architecture

The `core` package is the heart of DeployMonster. It provides:

### Core Struct

```go
type Core struct {
    Config    *Config          // Application configuration
    Build     BuildInfo        // Version, commit, build date
    Registry  *Registry        // Module registry
    Events    *EventBus        // Inter-module communication
    Scheduler *Scheduler       // Cron job scheduler
    DB        *Database        // SQL + BBolt
    Store     Store            // DB-agnostic repository
    Services  *Services        // Service implementations
    Logger    *slog.Logger     // Structured logging
    Router    *http.ServeMux   // HTTP router
}
```

### Application Lifecycle

```
┌──────────────────────────────────────────────────────────────────────────┐
│                           Application Startup                            │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. LoadConfig()         Load configuration from monster.yaml + env     │
│          │                                                              │
│          ▼                                                              │
│  2. NewApp()             Create Core instance with empty registry       │
│          │                                                              │
│          ▼                                                              │
│  3. registerAllModules() Call all module factories (registered via      │
│          │               init() functions using core.RegisterModule)    │
│          ▼                                                              │
│  4. Registry.Resolve()   Topological sort based on Dependencies()       │
│          │                                                              │
│          ▼                                                              │
│  5. Registry.InitAll()   Initialize modules in dependency order         │
│          │               Each module receives *Core for dependency      │
│          │               injection and service registration              │
│          ▼                                                              │
│  6. Registry.StartAll()  Start modules (begin background workers,       │
│          │               HTTP listeners, etc.)                          │
│          ▼                                                              │
│  7. Scheduler.Start()    Start cron job processor                       │
│          │                                                              │
│          ▼                                                              │
│  8. Wait for signal     Block until SIGINT/SIGTERM                      │
│          │                                                              │
│          ▼                                                              │
│  9. Registry.StopAll()   Graceful shutdown (reverse dependency order)   │
│                                                                         │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Module System

DeployMonster is built around a **modular monolith** architecture. Each feature is a self-contained module that implements the `Module` interface.

### Module Interface

```go
type Module interface {
    // Identity
    ID() string              // Unique identifier (e.g., "deploy", "billing")
    Name() string            // Human-readable name
    Version() string         // Semantic version
    Dependencies() []string  // Module IDs this depends on

    // Lifecycle
    Init(ctx context.Context, core *Core) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    // Observability
    Health() HealthStatus

    // Integration
    Routes() []Route         // HTTP routes to register
    Events() []EventHandler  // Event subscriptions
}
```

### Module Registration Pattern

Modules self-register via `init()` functions:

```go
// internal/deploy/module.go
package deploy

func init() {
    core.RegisterModule(func() core.Module { return New() })
}
```

Main.go imports modules with blank imports:

```go
// cmd/deploymonster/main.go
import (
    _ "github.com/deploy-monster/deploy-monster/internal/api"
    _ "github.com/deploy-monster/deploy-monster/internal/auth"
    _ "github.com/deploy-monster/deploy-monster/internal/billing"
    _ "github.com/deploy-monster/deploy-monster/internal/deploy"
    // ... 20 modules total
)
```

### Module Dependency Graph

```
                           ┌─────────┐
                           │  core   │
                           │ (base)  │
                           └────┬────┘
                                │
           ┌────────────────────┼────────────────────┐
           │                    │                    │
           ▼                    ▼                    ▼
     ┌───────────┐        ┌───────────┐        ┌───────────┐
     │  core.db  │        │ core.log  │        │core.events│
     │ (database)│        │ (logging) │        │ (events)  │
     └─────┬─────┘        └───────────┘        └───────────┘
           │
     ┌─────┴─────┬─────────────┬─────────────┬───────────────┐
     │           │             │             │               │
     ▼           ▼             ▼             ▼               ▼
┌────────┐  ┌────────┐   ┌────────┐   ┌──────────┐   ┌──────────┐
│ auth   │  │ deploy │   │ secrets│   │ ingress  │   │ billing  │
│        │  │        │   │        │   │          │   │          │
└────────┘  └────┬───┘   └────────┘   └────┬─────┘   └──────────┘
                 │                         │
                 ▼                         ▼
           ┌───────────┐            ┌───────────┐
           │   build   │            │    dns    │
           │           │            │           │
           └───────────┘            └───────────┘
```

### All Modules

| Module | ID | Description | Dependencies |
|--------|-----|-------------|--------------|
| **API** | `api` | REST API handlers | All modules |
| **Auth** | `auth` | JWT, API keys, sessions | `core.db` |
| **Backup** | `backup` | Automated backups | `core.db` |
| **Billing** | `billing` | Usage metering, quotas | `core.db` |
| **Build** | `build` | CI/CD pipeline | - |
| **Database** | `database` | Managed databases | `core.db` |
| **DB** | `db` | SQLite/PostgreSQL store | `core.db` |
| **Deploy** | `deploy` | Container orchestration | `core.db` |
| **Discovery** | `discovery` | Service discovery | - |
| **DNS** | `dns` | DNS management | - |
| **Enterprise** | `enterprise` | SSO, audit logs | `core.db` |
| **Git Sources** | `gitsources` | GitHub, GitLab, etc. | - |
| **Ingress** | `ingress` | Reverse proxy, SSL | - |
| **Marketplace** | `marketplace` | App templates | - |
| **MCP** | `mcp` | AI agent integration | - |
| **Notifications** | `notifications` | Slack, email, etc. | - |
| **Resource** | `resource` | Metrics, alerts | `core.db`, `deploy` |
| **Secrets** | `secrets` | Encrypted vault | `core.db` |
| **Swarm** | `swarm` | Multi-node clustering | - |
| **VPS** | `vps` | Server provisioning | - |
| **Webhooks** | `webhooks` | Git webhooks | - |

---

## Event System

The `EventBus` provides **loose coupling** between modules through a publish/subscribe pattern.

### Event Structure

```go
type Event struct {
    ID        string    // Unique event ID
    Type      string    // Dot-namespaced type (e.g., "app.deployed")
    Source    string    // Module that emitted the event
    Timestamp time.Time // When the event occurred
    TenantID  string    // Tenant context
    UserID    string    // User who triggered the action
    Data      any       // Event-specific payload
}
```

### Subscription Patterns

```go
// Subscribe to specific event type
events.Subscribe("app.deployed", handler)

// Subscribe to all events
events.Subscribe("*", handler)

// Subscribe to event prefix (all "app.*" events)
events.Subscribe("app.*", handler)

// Async handler (runs in goroutine)
events.SubscribeAsync("build.completed", handler)
```

### Standard Event Types

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Event Taxonomy                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Application Lifecycle          Build Pipeline                              │
│  ────────────────────          ─────────────────                           │
│  • app.created                  • build.queued                              │
│  • app.updated                  • build.started                             │
│  • app.deployed                 • build.completed                           │
│  • app.started                  • build.failed                              │
│  • app.stopped                                                              │
│  • app.deleted                 Container Events                             │
│  • app.crashed                 ─────────────────                           │
│  • app.scaled                   • container.started                         │
│                                 • container.stopped                         │
│  Domain & SSL                   • container.died                            │
│  ───────────────                • container.healthy                         │
│  • domain.added                                                             │
│  • domain.removed              Server Events                                │
│  • domain.verified             ──────────────                               │
│  • ssl.issued                  • server.added                               │
│  • ssl.expiring                • server.removed                             │
│  • ssl.renewed                 • server.down                                │
│  • ssl.failed                  • server.recovered                           │
│                                                                             │
│  Webhook Events                 Billing Events                              │
│  ─────────────────             ───────────────                              │
│  • webhook.received            • quota.exceeded                             │
│  • webhook.processed           • quota.warning                              │
│  • webhook.failed              • invoice.generated                          │
│  • outbound.sent               • payment.received                           │
│  • outbound.failed             • payment.failed                             │
│                                                                             │
│  System Events                                                              │
│  ──────────────                                                             │
│  • system.started                                                          │
│  • system.stopping                                                          │
│  • module.health_changed                                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Event Flow Example

```
┌─────────┐     ┌─────────┐     ┌──────────┐     ┌───────────────┐
│ Webhook │────▶│ Pipeline│────▶│  Build   │────▶│    Deploy     │
│ Receiver│     │         │     │          │     │               │
└─────────┘     └────┬────┘     └────┬─────┘     └───────┬───────┘
                     │               │                    │
                     │               │                    │
                     ▼               ▼                    ▼
               ┌─────────────────────────────────────────────────┐
               │                   EventBus                      │
               │                                                 │
               │  Events: webhook.received → build.started →    │
               │           build.completed → deploy.started →   │
               │           deploy.finished                       │
               └─────────────────────┬───────────────────────────┘
                                     │
         ┌───────────────────────────┼───────────────────────────┐
         │                           │                           │
         ▼                           ▼                           ▼
   ┌───────────┐              ┌───────────┐              ┌───────────┐
   │Notification│              │  Audit    │              │  Billing  │
   │  Module   │              │  Module   │              │  Module   │
   └───────────┘              └───────────┘              └───────────┘
```

---

## Data Layer

### Store Interface Pattern

All data access goes through the `Store` interface, never through concrete database types:

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
    Close() error
    Ping(ctx context.Context) error
}
```

### Sub-Interfaces

```go
// Each domain has its own sub-interface
type AppStore interface {
    CreateApp(ctx context.Context, app *Application) error
    GetApp(ctx context.Context, id string) (*Application, error)
    UpdateApp(ctx context.Context, app *Application) error
    ListAppsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]Application, int, error)
    UpdateAppStatus(ctx context.Context, id, status string) error
    DeleteApp(ctx context.Context, id string) error
}
```

### Database Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Data Layer                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌──────────────────────────────────────────────────────────────────────┐  │
│   │                        core.Store Interface                          │  │
│   │  (10 sub-interfaces: Tenant, User, App, Deployment, Domain, etc.)   │  │
│   └────────────────────────────────┬─────────────────────────────────────┘  │
│                                    │                                        │
│              ┌─────────────────────┼─────────────────────┐                  │
│              │                     │                     │                  │
│              ▼                     ▼                     ▼                  │
│   ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐         │
│   │    SQLite        │  │    PostgreSQL    │  │    BBolt KV      │         │
│   │   (default)      │  │   (enterprise)   │  │   (30+ buckets)  │         │
│   │                  │  │                  │  │                  │         │
│   │ • Modernc pure   │  │ • High scale     │  │ • API keys       │         │
│   │   Go driver      │  │ • Multi-node     │  │ • Webhooks       │         │
│   │ • Zero CGO       │  │ • Replication    │  │ • Sessions       │         │
│   │ • Single file    │  │ • Full-text      │  │ • Rate limits    │         │
│   └──────────────────┘  └──────────────────┘  │ • Config cache   │         │
│                                                └──────────────────┘         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### BBolt Buckets

| Bucket | Purpose |
|--------|---------|
| `api_keys` | API key storage and lookup |
| `webhooks` | Outbound webhook secrets |
| `sessions` | User session tokens |
| `rate_limits` | API rate limit counters |
| `deploy_state` | Deployment state cache |
| `container_cache` | Container info cache |
| `metrics_rollup` | Aggregated metrics |
| `dns_records` | DNS record cache |
| `ssl_certs` | Certificate cache |
| `build_cache` | Build artifact cache |
| ... | 20+ more buckets |

---

## Master/Agent Architecture

DeployMonster can run in two modes using the **same binary**:

- **Master**: Full platform with API, database, ingress
- **Agent**: Lightweight worker node that connects to master

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Master/Agent Architecture                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                          Master Node                                 │   │
│   │                                                                      │   │
│   │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  │   │
│   │  │   API   │  │   DB    │  │ Ingress │  │  Build  │  │ Scheduler│ │   │
│   │  │ :8443   │  │ SQLite  │  │ :80,443 │  │ Engine  │  │ (cron)  │  │   │
│   │  └─────────┘  └─────────┘  └─────────┘  └─────────┘  └─────────┘  │   │
│   │                                                                      │   │
│   │  ┌─────────────────────────────────────────────────────────────┐    │   │
│   │  │                    WebSocket Hub                             │    │   │
│   │  │   • Agent connections                                        │    │   │
│   │  │   • Command routing                                          │    │   │
│   │  │   • Metrics aggregation                                      │    │   │
│   │  └─────────────────────────────────────────────────────────────┘    │   │
│   └────────────────────────────────┬────────────────────────────────────┘   │
│                                    │                                        │
│                         WebSocket  │ (wss://master:8443/agent)             │
│                                    │                                        │
│          ┌─────────────────────────┼─────────────────────────┐              │
│          │                         │                         │              │
│          ▼                         ▼                         ▼              │
│   ┌─────────────┐           ┌─────────────┐           ┌─────────────┐      │
│   │   Agent 1   │           │   Agent 2   │           │   Agent N   │      │
│   │             │           │             │           │             │      │
│   │ ┌─────────┐ │           │ ┌─────────┐ │           │ ┌─────────┐ │      │
│   │ │ Docker  │ │           │ │ Docker  │ │           │ │ Docker  │ │      │
│   │ │ Runtime │ │           │ │ Runtime │ │           │ │ Runtime │ │      │
│   │ └─────────┘ │           │ └─────────┘ │           │ └─────────┘ │      │
│   │             │           │             │           │             │      │
│   │ • Exec cmds │           │ • Exec cmds │           │ • Exec cmds │      │
│   │ • Metrics   │           │ • Metrics   │           │ • Metrics   │      │
│   │ • Logs      │           │ • Logs      │           │ • Logs      │      │
│   └─────────────┘           └─────────────┘           └─────────────┘      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Agent Protocol

Communication uses JSON messages over WebSocket:

```go
type AgentMessage struct {
    ID        string    `json:"id"`
    Type      string    `json:"type"`
    ServerID  string    `json:"server_id"`
    Timestamp time.Time `json:"timestamp"`
    Payload   any       `json:"payload"`
}
```

**Master → Agent Commands:**
- `ping` - Health check
- `container.create` - Create and start container
- `container.stop` - Stop container
- `container.remove` - Remove container
- `container.logs` - Stream logs
- `container.exec` - Execute command
- `image.pull` - Pull Docker image
- `metrics.collect` - Collect server metrics
- `health.check` - Health status

**Agent → Master Responses:**
- `pong` - Health check response
- `result` - Command success
- `error` - Command failure
- `metrics.report` - Periodic metrics
- `container.event` - Container lifecycle events
- `server.status` - Server status changes

### Starting an Agent

```bash
# On worker node
deploymonster serve --agent \
    --master=https://master.example.com:8443 \
    --token=join-token-from-master
```

---

## Deployment Pipeline

### Build → Deploy Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Deployment Pipeline                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐             │
│  │  Git    │────▶│  Build  │────▶│  Image  │────▶│  Deploy │             │
│  │ Webhook │     │ Pipeline│     │  Push   │     │ Strategy│             │
│  └─────────┘     └─────────┘     └─────────┘     └─────────┘             │
│                       │                                   │                 │
│                       ▼                                   ▼                 │
│               ┌───────────────┐              ┌───────────────────┐        │
│               │ 14 Detectors  │              │ Deploy Strategies │        │
│               │               │              │                   │        │
│               │ • Node.js     │              │ • recreate        │        │
│               │ • Python      │              │ • rolling         │        │
│               │ • Go          │              │ • blue-green      │        │
│               │ • Rust        │              │ • canary          │        │
│               │ • Ruby        │              └───────────────────┘        │
│               │ • PHP         │                                           │
│               │ • Java        │              ┌───────────────────┐        │
│               │ • .NET        │              │  Health Checks    │        │
│               │ • Dockerfile  │              │                   │        │
│               │ • Static      │              │ • HTTP probe      │        │
│               │ • Nixpacks    │              │ • TCP probe       │        │
│               │ • compose     │              │ • Command probe   │        │
│               │ • ...         │              └───────────────────┘        │
│               └───────────────┘                                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Project Type Detection

The build system automatically detects project types:

| Type | Indicators | Generated Dockerfile |
|------|------------|---------------------|
| Node.js | `package.json` | Multi-stage Node build |
| Python | `requirements.txt`, `pyproject.toml` | Python with pip/poetry |
| Go | `go.mod` | Multi-stage Go build |
| Rust | `Cargo.toml` | Cargo build |
| Ruby | `Gemfile` | Bundler + Ruby |
| PHP | `composer.json` | PHP-FPM + Nginx |
| Java | `pom.xml`, `build.gradle` | Maven/Gradle + JRE |
| .NET | `*.csproj`, `*.fsproj` | .NET SDK + Runtime |
| Static | `index.html` | Nginx static server |
| Dockerfile | `Dockerfile` | Uses existing |
| Compose | `docker-compose.yml` | Docker Compose up |

### Deployment Strategies

1. **Recreate**: Stop old container, start new (downtime)
2. **Rolling**: Gradual replacement with health checks
3. **Blue-Green**: Parallel deployment with instant switchover
4. **Canary**: Percentage-based traffic routing

---

## Ingress & Load Balancing

DeployMonster includes a built-in reverse proxy with automatic SSL.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Ingress Architecture                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│                          ┌──────────────────┐                               │
│                          │   Load Balancer   │                               │
│                          │   (Layer 4/7)    │                               │
│                          └────────┬─────────┘                               │
│                                   │                                         │
│              ┌────────────────────┼────────────────────┐                    │
│              │                    │                    │                    │
│              ▼                    ▼                    ▼                    │
│      ┌──────────────┐    ┌──────────────┐    ┌──────────────┐             │
│      │  HTTP :80    │    │  HTTPS :443  │    │  API :8443   │             │
│      │              │    │              │    │              │             │
│      │ • ACME chall │    │ • TLS term   │    │ • REST API   │             │
│      │ • HTTPS redir│    │ • SNI routing│    │ • WebSocket  │             │
│      └──────────────┘    └──────────────┘    └──────────────┘             │
│                                   │                                         │
│                                   ▼                                         │
│                         ┌──────────────────┐                               │
│                         │  Reverse Proxy   │                               │
│                         │                  │                               │
│                         │ • Route table    │                               │
│                         │ • Path stripping │                               │
│                         │ • Rate limiting  │                               │
│                         │ • Access logs    │                               │
│                         └────────┬─────────┘                               │
│                                  │                                          │
│         ┌────────────────────────┼────────────────────────┐                 │
│         │                        │                        │                 │
│         ▼                        ▼                        ▼                 │
│  ┌─────────────┐          ┌─────────────┐          ┌─────────────┐         │
│  │   App 1     │          │   App 2     │          │   App N     │         │
│  │ (container) │          │ (container) │          │ (container) │         │
│  │ :3000       │          │ :3001       │          │ :300N       │         │
│  └─────────────┘          └─────────────┘          └─────────────┘         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Route Table

```go
type RouteEntry struct {
    Host        string            // app.example.com
    PathPrefix  string            // /api
    Backends    []string          // ["10.0.0.1:3000", "10.0.0.2:3000"]
    StripPrefix bool              // Remove path prefix before forwarding
    Labels      map[string]string // monster.app.id, monster.tenant
}
```

### SSL/TLS

- **Automatic ACME**: Let's Encrypt integration
- **HTTP-01 Challenge**: Automatic validation
- **Auto-renewal**: 30-day renewal window
- **Self-signed fallback**: Development mode
- **Wildcard support**: DNS-based validation

---

## Service Interfaces

Modules communicate through interfaces defined in `core/interfaces.go`:

```go
type Services struct {
    // Singleton services
    Container     ContainerRuntime     // Docker operations
    SSH           SSHClient            // Remote execution
    Secrets       SecretResolver       // ${SECRET:name} resolution
    Notifications NotificationSender   // Multi-channel notifications
    Webhooks      OutboundWebhookSender // HMAC-signed webhooks

    // Provider registries (multiple implementations)
    dnsProviders    map[string]DNSProvider     // cloudflare, route53, etc.
    backupStorages  map[string]BackupStorage   // s3, local, sftp
    vpsProvisioners map[string]VPSProvisioner  // hetzner, digitalocean
    gitProviders    map[string]GitProvider     // github, gitlab, gitea
}
```

### Key Interfaces

| Interface | Purpose | Implementations |
|-----------|---------|-----------------|
| `ContainerRuntime` | Docker operations | Local Docker, Remote Agent |
| `DNSProvider` | DNS record management | Cloudflare, Route53, DigitalOcean |
| `BackupStorage` | Backup destination | Local filesystem, S3, SFTP |
| `VPSProvisioner` | Server provisioning | Hetzner, DigitalOcean, Vultr |
| `GitProvider` | Repository access | GitHub, GitLab, Gitea, Bitbucket |
| `SecretResolver` | Secret injection | AES-256-GCM vault |
| `NotificationSender` | Alerts & notifications | Slack, Discord, Email, Telegram |

---

## Frontend Architecture

The React frontend is embedded into the Go binary using `embed.FS`.

### Technology Stack

| Technology | Version | Purpose |
|------------|---------|---------|
| React | 19 | UI framework |
| Vite | 8 | Build tool |
| Tailwind CSS | 4 | Styling |
| shadcn/ui | Latest | Component library |
| Zustand | 5 | State management |
| React Router | 7 | Client-side routing |

### Build Process

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Frontend Build Process                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   web/                          scripts/build.sh                            │
│   ├── src/                      ─────────────────                          │
│   │   ├── components/                                                      │
│   │   ├── pages/                 1. npm install                            │
│   │   ├── hooks/                 2. npm run build                          │
│   │   └── store/                    → vite build                           │
│   │                                 → dist/                                │
│   ├── package.json                3. cp -r dist/ ./internal/api/embed/     │
│   └── vite.config.ts              4. go build                              │
│                                       → embed.FS embeds dist/              │
│                                                                             │
│   Binary Size Impact:                                                       │
│   ───────────────────                                                       │
│   • Go binary only:     ~12MB                                               │
│   • With embedded UI:   ~22MB                                               │
│   • Gzip in production: ~8MB (Nginx/brotli)                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### API Communication

```typescript
// Custom hook for API calls with auth
export function useApi<T>(endpoint: string) {
  const { token } = useAuthStore();

  return useQuery({
    queryKey: [endpoint],
    queryFn: () => fetch(`/api/v1${endpoint}`, {
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
    }).then(r => r.json()),
  });
}
```

---

## Configuration

DeployMonster uses YAML configuration with environment variable overrides:

```yaml
# monster.yaml
server:
  host: 0.0.0.0
  port: 8443
  domain: deploy.example.com
  secret_key: ${MONSTER_SECRET_KEY}

database:
  driver: sqlite        # sqlite | postgres
  path: deploymonster.db
  # postgres_url: postgres://user:pass@host:5432/db

ingress:
  http_port: 80
  https_port: 443
  enable_https: true

acme:
  email: admin@example.com
  staging: false

registration:
  mode: open           # open | invite | closed

limits:
  max_apps_per_tenant: 100
  max_concurrent_builds: 5
```

### Environment Variables

All config values can be overridden with `MONSTER_` prefix:

```bash
MONSTER_SERVER_PORT=9443
MONSTER_DATABASE_DRIVER=postgres
MONSTER_DATABASE_POSTGRES_URL=postgres://...
MONSTER_ACME_EMAIL=admin@company.com
```

---

## Observability

### Logging

Structured logging with `log/slog`:

```go
logger.Info("deployment started",
    "app", app.Name,
    "version", deploy.Version,
    "strategy", deploy.Strategy,
)
```

### Health Checks

```go
type HealthStatus int

const (
    HealthOK       HealthStatus = iota  // Fully operational
    HealthDegraded                       // Operational but impaired
    HealthDown                           // Not operational
)
```

### Metrics

- **Server metrics**: CPU, RAM, disk, network
- **Container metrics**: Per-container resource usage
- **Business metrics**: Deployments, builds, errors
- **Event metrics**: Published events, handler errors

---

## Security

### Authentication

- **JWT tokens**: Access (15m) + Refresh (7d)
- **API keys**: Prefix-based lookup (`dm_xxx_...`)
- **Session tokens**: Stored in BBolt with TTL

### Authorization

```go
type AuthLevel int

const (
    AuthNone       AuthLevel = iota  // Public endpoints
    AuthAPIKey                       // Valid API key
    AuthJWT                          // Valid JWT
    AuthAdmin                        // Admin role
    AuthSuperAdmin                   // Super admin role
)
```

### Encryption

- **Secrets**: AES-256-GCM with Argon2id key derivation
- **Passwords**: bcrypt with cost 12
- **Webhooks**: HMAC-SHA256 signatures

---

## Development

### Running Locally

```bash
# Development mode (with hot reload)
make dev

# Run tests
make test

# Run with coverage
go test ./internal/... -cover

# Build binary
make build
```

### Project Structure

```
deploy-monster/
├── cmd/deploymonster/     # Main application entrypoint
├── internal/
│   ├── api/                # REST API handlers
│   ├── auth/               # Authentication & authorization
│   ├── backup/             # Backup system
│   ├── billing/            # Usage metering & quotas
│   ├── build/              # Build pipeline
│   ├── compose/            # Docker Compose support
│   ├── core/               # Core engine & interfaces
│   ├── database/           # Managed databases
│   ├── db/                 # SQLite/PostgreSQL store
│   ├── deploy/             # Container orchestration
│   ├── discovery/          # Service discovery
│   ├── dns/                # DNS management
│   ├── enterprise/         # Enterprise features
│   ├── gitsources/         # Git provider integrations
│   ├── ingress/            # Reverse proxy & SSL
│   ├── marketplace/        # App templates
│   ├── mcp/                # AI agent integration
│   ├── notifications/      # Notification channels
│   ├── resource/           # Resource monitoring
│   ├── secrets/            # Secret vault
│   ├── swarm/              # Multi-node clustering
│   ├── vps/                # Server provisioning
│   └── webhooks/           # Git webhooks
├── web/                    # React frontend
├── docs/                   # OpenAPI spec & docs
└── Makefile                # Build commands
```

---

## Conclusion

DeployMonster's architecture prioritizes:

1. **Modularity**: Each feature is a self-contained module
2. **Testability**: 97%+ coverage through interface-based design
3. **Extensibility**: Provider interfaces allow easy additions
4. **Simplicity**: Single binary, embedded UI, zero external dependencies
5. **Scalability**: Master/agent architecture for horizontal scaling

For implementation details, see `.project/IMPLEMENTATION.md`.
For the full feature specification, see `.project/SPECIFICATION.md`.
