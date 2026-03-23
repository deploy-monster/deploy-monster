# Architecture

## Overview

DeployMonster is a **modular monolith** — a single binary containing 20 auto-registered modules that communicate via an in-process event bus and typed service interfaces.

```
┌──────────────────────────────────────────────────────────────────┐
│                      DeployMonster Binary (20MB)                  │
├────────┬────────┬────────┬────────┬──────────┬──────────────────┤
│ Web UI │REST API│  SSE   │Webhooks│ Ingress  │   MCP Server     │
│(React) │76 eps  │Stream  │Receiver│:80/:443  │  AI Tools        │
├────────┴────────┴────────┴────────┴──────────┴──────────────────┤
│                        Core Engine                               │
│  Module Registry │ EventBus │ Store Interface │ Service Registry │
├──────────────────────────────────────────────────────────────────┤
│  20 Modules (auto-registered via init())                         │
│  db│auth│api│deploy│build│ingress│discovery│dns│secrets│billing  │
│  backup│database│vps│swarm│marketplace│notifications│mcp│...     │
├──────────────────────────────────────────────────────────────────┤
│  SQLite + BBolt │ Docker SDK │ SSH Pool │ HTTP Clients           │
└──────────────────────────────────────────────────────────────────┘
```

## Module System

Every feature is a **module** implementing `core.Module`:

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

Modules register themselves via `init()`:

```go
func init() {
    core.RegisterModule(func() core.Module { return New() })
}
```

The core engine resolves dependencies via **topological sort** and manages the lifecycle:
1. Register all modules
2. Resolve dependency graph
3. Init in dependency order
4. Start in dependency order
5. Wait for shutdown signal
6. Stop in reverse order

## Data Access — Store Interface

All database access goes through `core.Store`:

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
    Close() error
    Ping(ctx context.Context) error
}
```

**Implementations:**
- `internal/db` — SQLite (default, embedded, zero-config)
- PostgreSQL — enterprise (same interface, swap the driver)

## Event System

Modules communicate via `core.EventBus`:

- **Synchronous** handlers for critical paths
- **Asynchronous** handlers for notifications, logging
- **Prefix matching**: `"app.*"` catches `app.created`, `app.deployed`, etc.
- **Typed payloads**: `AppEventData`, `DeployEventData`, etc.

## Service Registry

Cross-module dependencies use typed interfaces in `core.Services`:

| Interface | Purpose | Implementers |
|-----------|---------|-------------|
| `ContainerRuntime` | Docker operations | deploy module |
| `SecretResolver` | `${SECRET:name}` resolution | secrets module |
| `NotificationSender` | Multi-channel dispatch | notifications module |
| `DNSProvider` | DNS record management | dns/cloudflare |
| `BackupStorage` | Backup upload/download | backup/local, backup/s3 |
| `VPSProvisioner` | Server provisioning | vps/hetzner, vps/do, etc. |
| `GitProvider` | Git API access | git/github, git/gitlab, etc. |

## Ingress Gateway

Custom reverse proxy replacing Traefik/Nginx:

```
Internet → :80 (HTTP redirect) → :443 (HTTPS)
                                    ↓
                              Route Table (host + path matching)
                                    ↓
                              Middleware Chain (rate limit, CORS, compress)
                                    ↓
                              Load Balancer (round-robin, least-conn, IP-hash, weighted)
                                    ↓
                              Backend Pool (container:port)
```

## Master/Agent Architecture

Same binary, two modes:

- **Master** (`deploymonster serve`): Full platform
- **Agent** (`deploymonster serve --agent`): Worker node

Communication: Agent connects to master via WebSocket, reports metrics, executes commands.

## Build Pipeline

```
Git Push → Webhook → Clone → Detect Type → Generate Dockerfile → Docker Build → Deploy
```

Supports 14 project types with auto-detection.
