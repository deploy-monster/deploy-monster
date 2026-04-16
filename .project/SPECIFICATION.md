# DeployMonster — SPECIFICATION.md

> **Tagline**: "Tame Your Deployments"
> **Version**: 1.0.0
> **Author**: Ersin / ECOSTACK TECHNOLOGY OÜ
> **License**: AGPL-3.0 (core) + Commercial (enterprise features)
> **Repository**: github.com/deploy-monster/deploy-monster
> **Domains**: deploy.monster · deploymonster.com

---

## 1. VISION & PHILOSOPHY

DeployMonster is a **self-hosted PaaS (Platform as a Service)** that transforms any VPS or bare-metal server into a full deployment platform. Single binary, zero configuration, production-ready in under 60 seconds.

### Core Principles

1. **Single Binary, Zero Config** — Download → Run → Deploy. `./deploymonster` and you're live.
2. **Minimal Dependencies** — Only Docker SDK and embedded database. No Redis, no external DB, no message queue.
3. **Developer-First UX** — If it takes more than 3 clicks, it's a bug.
4. **Modular Architecture** — Every subsystem is a pluggable module. Swap, extend, disable at will.
5. **Multi-Tenancy Native** — Admin + Customer panels from day one. SaaS-ready architecture.
6. **Edge-Native** — Works on a $5 VPS. Scales to Docker Swarm clusters.
7. **LLM-Native** — Built-in MCP server for AI-driven infrastructure management.

### What DeployMonster Replaces

DeployMonster isn't just another deploy tool — it's a complete infrastructure management platform that replaces an entire stack of tools:

| Tool | What's Wrong With It | What DeployMonster Does Better |
|------|---------------------|-------------------------------|
| Coolify | Clunky UI, limited marketplace, poor multi-server | Professional 3-panel UI, 56 built-in templates (community-contributed catalog growing), VPS provider API |
| Dokploy | Basic features, no billing, weak team mgmt | Full PaaS with billing, RBAC, team management |
| CapRover | Outdated UI, Captain-based, limited scaling | Modern React UI, Docker Swarm native, auto-scaling |
| Portainer | Container management only, no deploy pipeline | Full Git→Build→Deploy pipeline + container management |
| Dokku | CLI-only, single-server, no UI | Multi-server, professional UI, drag & drop topology |
| Vercel/Netlify | Vendor lock-in, expensive at scale, no self-host | Self-hosted, zero vendor lock-in, unlimited projects |
| Traefik + NPM | Separate tool, complex config | Built-in ingress with auto-SSL, label-based discovery |
| Watchtower | Only auto-updates, no rollback | Auto-update with instant rollback, blue-green, canary |
| Netdata/Grafana | Separate monitoring stack | Built-in metrics, alerts, real-time dashboards |
| Vault/Doppler | Separate secret management | Built-in secret vault with scoping & rotation |
| Stripe Billing | DIY integration | Built-in pay-per-usage billing for hosting providers |

**Key Differentiators vs Competitors:**
1. **Single binary** — Not 5 Docker containers like Coolify. One binary, one process.
2. **3-panel architecture** — Super Admin + Team Admin + Customer. Not just "admin and user".
3. **VPS provider integration** — Provision Hetzner/DO/Vultr servers from the UI. Coolify can't do this well.
4. **Visual topology** — Drag & drop your infrastructure. No competitor has this.
5. **Built-in billing** — Run a hosting business. Dokploy/Coolify have zero billing.
6. **Universal Git** — Any Git provider, not just GitHub/GitLab. Gitea, Gogs, Azure, custom.
7. **Docker Compose native** — Upload a compose file, get a running stack. First-class, not afterthought.
8. **Built-in marketplace** — 56 one-click templates at launch across 16 categories (databases, CMS, observability, dev tools, AI stacks); catalog grows via community contribution.
9. **Secret vault** — Scoped, versioned, rotatable secrets. No one else has this built-in.
10. **MCP server** — AI-native infrastructure management. Future-proof.

---

## 2. HIGH-LEVEL ARCHITECTURE

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
│          Embedded DB (SQLite/BBolt) + Event Bus                   │
└───────────────────────────────────────────────────────────────────┘
```

### Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go 1.23+ | Single binary, native concurrency, Docker SDK |
| Database | SQLite (CGo) + BBolt (pure Go) | SQLite for relational, BBolt for KV/sessions |
| Web UI | React 19 + Vite + Tailwind v4 + shadcn/ui | Embedded in binary via `embed.FS` |
| API | REST + WebSocket | REST for CRUD, WS for real-time logs/events |
| Auth | JWT + API Keys | Stateless, multi-tenant |
| Container Runtime | Docker SDK (moby/moby) | Direct Docker API, no shelling out |
| Orchestration | Docker Swarm Mode | Built-in, no K8s complexity |
| SSL | Let's Encrypt (ACME) via lego library | Auto-cert with HTTP-01 / DNS-01 challenge |
| Ingress | Custom reverse proxy (net/http/httputil) | Label-based routing, no Traefik dependency |
| DNS | Cloudflare API + Generic RFC2136 | Most common provider + standard fallback |

---

## 3. MODULE SYSTEM

DeployMonster uses a **module registry** pattern. Every feature is a module that registers itself.

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
    Routes() []Route       // HTTP routes this module exposes
    Events() []EventHandler // Events this module listens to
}
```

### Module Registry

| Module ID | Name | Category | Priority | Description |
|-----------|------|----------|----------|-------------|
| `core.db` | Database | Core | 0 | SQLite + BBolt embedded database |
| `core.auth` | Authentication | Core | 1 | JWT, API keys, RBAC |
| `core.events` | Event Bus | Core | 2 | In-process pub/sub event system |
| `core.scheduler` | Task Scheduler | Core | 3 | Cron jobs, delayed tasks |
| `core.ssh` | SSH Client | Core | 4 | SSH connection pool for remote servers |
| `ingress` | Ingress Gateway | Network | 10 | Reverse proxy, SSL termination |
| `ingress.acme` | ACME/SSL | Network | 11 | Let's Encrypt certificate management |
| `ingress.lb` | Load Balancer | Network | 12 | Multi-strategy load balancing |
| `discovery` | Service Discovery | Network | 13 | Label-based container discovery |
| `dns.sync` | DNS Synchronizer | Network | 14 | Cloudflare, Route53, Generic DNS |
| `deploy` | Deploy Engine | Deploy | 20 | Container lifecycle management |
| `deploy.git` | Git Deployer | Deploy | 21 | Universal Git → build → deploy pipeline |
| `deploy.image` | Image Deployer | Deploy | 22 | Direct Docker image → deploy |
| `deploy.compose` | Compose Deployer | Deploy | 23 | docker-compose.yml → multi-service deploy |
| `deploy.registry` | Registry Manager | Deploy | 24 | Docker registry push/pull |
| `deploy.rollback` | Rollback Engine | Deploy | 25 | Version history, instant rollback |
| `build` | Build Engine | Build | 30 | Dockerfile, Buildpacks, Nixpacks |
| `build.cache` | Build Cache | Build | 31 | Layer caching, artifact cache |
| `resource` | Resource Monitor | Ops | 40 | CPU, RAM, disk, network metrics |
| `resource.alerts` | Alert Engine | Ops | 41 | Threshold alerts, notifications |
| `backup` | Backup Engine | Ops | 50 | Volume snapshots, DB dumps |
| `backup.storage` | Backup Storage | Ops | 51 | Local, S3, SFTP backup targets |
| `database` | Database Manager | Services | 60 | PostgreSQL, MySQL, Redis provisioning |
| `storage` | Storage Manager | Services | 61 | Volume management, extra disks |
| `vps` | VPS Provider Manager | Infrastructure | 65 | Remote server provisioning via cloud APIs |
| `vps.hetzner` | Hetzner Provider | Infrastructure | 66 | Hetzner Cloud API integration |
| `vps.digitalocean` | DigitalOcean Provider | Infrastructure | 66 | DigitalOcean API integration |
| `vps.vultr` | Vultr Provider | Infrastructure | 66 | Vultr API integration |
| `vps.linode` | Linode Provider | Infrastructure | 66 | Akamai/Linode API integration |
| `vps.aws` | AWS Provider | Infrastructure | 66 | AWS EC2 integration |
| `vps.custom` | Custom SSH Server | Infrastructure | 66 | Any server via SSH + IP |
| `swarm` | Swarm Orchestrator | Cluster | 70 | Multi-node Docker Swarm |
| `swarm.agent` | Swarm Agent | Cluster | 71 | Worker node agent |
| `ui.admin` | Admin Panel | UI | 80 | Admin dashboard, system management |
| `ui.customer` | Customer Panel | UI | 81 | Customer dashboard, app management |
| `marketplace` | Marketplace Engine | Marketplace | 85 | Template registry, categories, search |
| `marketplace.registry` | Template Registry | Marketplace | 86 | Community + official template sync |
| `mcp` | MCP Server | Integration | 90 | AI/LLM infrastructure control |
| `webhooks` | Webhook Engine | Integration | 91 | Universal Git webhook receiver |
| `notifications` | Notifications | Integration | 92 | Email, Slack, Discord, Telegram |
| `gitsources` | Git Source Manager | Integration | 93 | OAuth + token management for all Git providers |
| `secrets` | Secret Vault | Security | 94 | Encrypted secret storage + scoping |
| `billing` | Billing Engine | Business | 95 | Plans, usage metering, invoicing |
| `billing.stripe` | Stripe Integration | Business | 96 | Stripe subscriptions + metered billing |
| `billing.metering` | Usage Metering | Business | 97 | Per-tenant resource usage tracking |
| `enterprise` | Enterprise Engine | Enterprise | 98 | White-label, reseller, HA, compliance |
| `enterprise.whmcs` | WHMCS Bridge | Enterprise | 99 | WHMCS provisioning/billing bridge |

---

## 4. DATA MODEL

### 4.1 Core Entities

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│    Tenant     │────<│    User       │     │   APIKey     │
│──────────────│     │──────────────│     │──────────────│
│ id           │     │ id           │     │ id           │
│ name         │     │ tenant_id    │     │ user_id      │
│ slug         │     │ email        │     │ key_hash     │
│ plan         │     │ password_hash│     │ scopes       │
│ limits_json  │     │ role         │     │ expires_at   │
│ status       │     │ status       │     │ last_used_at │
│ created_at   │     │ created_at   │     │ created_at   │
└──────────────┘     └──────────────┘     └──────────────┘
```

### 4.2 Project & Application

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Project    │────<│  Application  │────<│  Deployment  │
│──────────────│     │──────────────│     │──────────────│
│ id           │     │ id           │     │ id           │
│ tenant_id    │     │ project_id   │     │ app_id       │
│ name         │     │ name         │     │ version      │
│ description  │     │ type         │     │ image        │
│ environment  │     │ source_type  │     │ status       │
│ created_at   │     │ source_url   │     │ build_log    │
│              │     │ branch       │     │ started_at   │
│              │     │ dockerfile   │     │ finished_at  │
│              │     │ build_pack   │     │ commit_sha   │
│              │     │ env_vars_enc │     │ rollback_to  │
│              │     │ labels_json  │     │ created_at   │
│              │     │ status       │     │              │
│              │     │ created_at   │     │              │
└──────────────┘     └──────────────┘     └──────────────┘
```

### 4.3 Networking

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│    Domain    │     │  SSLCert     │     │  Route       │
│──────────────│     │──────────────│     │──────────────│
│ id           │     │ id           │     │ id           │
│ app_id       │     │ domain_id    │     │ domain_id    │
│ fqdn         │     │ cert_pem     │     │ app_id       │
│ type         │     │ key_pem_enc  │     │ path_prefix  │
│ dns_provider │     │ issuer       │     │ strip_prefix │
│ dns_synced   │     │ expires_at   │     │ middleware   │
│ verified     │     │ auto_renew   │     │ priority     │
│ created_at   │     │ created_at   │     │ lb_strategy  │
└──────────────┘     └──────────────┘     └──────────────┘
```

### 4.4 Infrastructure

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│    Server    │     │   Volume     │     │  Backup      │
│──────────────│     │──────────────│     │──────────────│
│ id           │     │ id           │     │ id           │
│ hostname     │     │ app_id       │     │ source_type  │
│ ip_address   │     │ name         │     │ source_id    │
│ role         │     │ mount_path   │     │ storage_id   │
│ docker_ver   │     │ size_mb      │     │ size_bytes   │
│ cpu_cores    │     │ driver       │     │ status       │
│ ram_mb       │     │ server_id    │     │ scheduled    │
│ disk_mb      │     │ created_at   │     │ retention    │
│ status       │     │              │     │ created_at   │
│ labels_json  │     │              │     │              │
│ joined_at    │     │              │     │              │
└──────────────┘     └──────────────┘     └──────────────┘

┌──────────────┐     ┌──────────────┐
│ ManagedDB    │     │ DNSRecord    │
│──────────────│     │──────────────│
│ id           │     │ id           │
│ tenant_id    │     │ domain_id    │
│ engine       │     │ type         │
│ version      │     │ name         │
│ port         │     │ value        │
│ credentials  │     │ ttl          │
│ container_id │     │ provider_id  │
│ volume_id    │     │ synced       │
│ backup_sched │     │ created_at   │
│ status       │     │              │
│ created_at   │     │              │
└──────────────┘     └──────────────┘
```

### 4.5 VPS Providers & Remote Servers

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ VPSProvider  │────<│ RemoteServer │     │ SSHKey       │
│──────────────│     │──────────────│     │──────────────│
│ id           │     │ id           │     │ id           │
│ tenant_id    │     │ provider_id  │     │ tenant_id    │
│ type         │     │ tenant_id    │     │ name         │
│ name         │     │ name         │     │ public_key   │
│ api_token_enc│     │ ip_address   │     │ private_enc  │
│ region       │     │ region       │     │ fingerprint  │
│ default_os   │     │ os           │     │ created_at   │
│ default_size │     │ size_slug    │     │              │
│ status       │     │ ssh_key_id   │     │              │
│ created_at   │     │ ssh_port     │     │              │
│              │     │ docker_ver   │     │              │
│              │     │ cpu_cores    │     │              │
│              │     │ ram_mb       │     │              │
│              │     │ disk_mb      │     │              │
│              │     │ monthly_cost │     │              │
│              │     │ provider_ref │     │ (provider ID)│
│              │     │ role         │     │ manager/worker│
│              │     │ swarm_joined │     │              │
│              │     │ agent_status │     │              │
│              │     │ status       │     │              │
│              │     │ created_at   │     │              │
└──────────────┘     └──────────────┘     └──────────────┘
```

### 4.6 Git Sources & Webhooks

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  GitSource   │     │  Webhook     │     │ WebhookLog   │
│──────────────│     │──────────────│     │──────────────│
│ id           │     │ id           │     │ id           │
│ tenant_id    │     │ app_id       │     │ webhook_id   │
│ type         │     │ git_source_id│     │ event_type   │
│ name         │     │ secret_hash  │     │ payload_hash │
│ base_url     │     │ events[]     │     │ commit_sha   │
│ api_url      │     │ branch_filter│     │ branch       │
│ auth_type    │     │ auto_deploy  │     │ status       │
│ token_enc    │     │ status       │     │ deploy_id    │
│ oauth_data   │     │ last_trigger │     │ received_at  │
│ ssh_key_id   │     │ created_at   │     │ processed_at │
│ verified     │     │              │     │              │
│ created_at   │     │              │     │              │
└──────────────┘     └──────────────┘     └──────────────┘

GitSource.type enum:
  github | gitlab | gitea | gogs | bitbucket |
  azure_devops | codecommit | custom_git

GitSource.auth_type enum:
  oauth2 | personal_token | deploy_key | ssh_key | http_basic

Webhook.events[] enum:
  push | tag | pull_request | release | manual
```

### 4.7 Compose & Stack Definitions

```
┌──────────────┐     ┌──────────────┐
│ ComposeStack │────<│ComposeService│
│──────────────│     │──────────────│
│ id           │     │ id           │
│ app_id       │     │ stack_id     │
│ raw_yaml     │     │ service_name │
│ parsed_json  │     │ image        │
│ version      │     │ container_id │
│ source_type  │     │ ports_json   │
│ source_url   │     │ env_json     │
│ status       │     │ volumes_json │
│ created_at   │     │ labels_json  │
│              │     │ depends_on   │
│              │     │ replicas     │
│              │     │ status       │
│              │     │ created_at   │
└──────────────┘     └──────────────┘

ComposeStack.source_type enum:
  upload | git | url | inline
```

### 4.8 Marketplace Templates

```
┌──────────────┐     ┌──────────────┐
│MarketplaceApp│────<│ AppInstall   │
│──────────────│     │──────────────│
│ id           │     │ id           │
│ slug         │     │ template_id  │
│ name         │     │ tenant_id    │
│ description  │     │ app_id       │
│ icon_url     │     │ config_json  │
│ category     │     │ version      │
│ subcategory  │     │ status       │
│ tags[]       │     │ installed_at │
│ author       │     │ updated_at   │
│ source       │     │              │
│ compose_yaml │     │              │
│ config_schema│     │ (JSON Schema │
│ min_resources│     │  for user    │
│ version      │     │  config)     │
│ featured     │     │              │
│ downloads    │     │              │
│ rating       │     │              │
│ verified     │     │              │
│ created_at   │     │              │
│ updated_at   │     │              │
└──────────────┘     └──────────────┘
```

### 4.9 DeployMonster Labels (Container Discovery)

DeployMonster uses Docker labels for service discovery and routing, similar to Traefik but with its own namespace:

```
# Enable DeployMonster management
monster.enable=true

# Routing
monster.http.routers.myapp.rule=Host(`app.example.com`)
monster.http.routers.myapp.entrypoints=websecure
monster.http.routers.myapp.tls=true
monster.http.routers.myapp.tls.certresolver=letsencrypt

# Service binding
monster.http.services.myapp.loadbalancer.server.port=3000
monster.http.services.myapp.loadbalancer.strategy=round-robin

# Health check
monster.http.services.myapp.healthcheck.path=/health
monster.http.services.myapp.healthcheck.interval=10s

# Middleware
monster.http.routers.myapp.middlewares=ratelimit,cors,compress

# Resource limits
monster.resources.cpu=0.5
monster.resources.memory=512m

# Backup
monster.backup.volumes=true
monster.backup.schedule=0 2 * * *

# Metadata
monster.project=my-project
monster.environment=production
monster.team=backend
```

---

## 5. INGRESS GATEWAY

The Ingress Gateway is DeployMonster's built-in reverse proxy that replaces Traefik/Nginx.

### 5.1 Architecture

```
Internet
    │
    ▼
┌──────────────────────────────┐
│       Ingress Gateway        │
│  ┌────────┐  ┌────────────┐  │
│  │ :80    │  │ :443       │  │
│  │ HTTP   │  │ HTTPS/TLS  │  │
│  └───┬────┘  └─────┬──────┘  │
│      │    redirect  │         │
│      └──────►───────┤         │
│                     ▼         │
│  ┌──────────────────────────┐ │
│  │    Router Table          │ │
│  │  Host + Path matching    │ │
│  │  Priority-based          │ │
│  └───────────┬──────────────┘ │
│              ▼                │
│  ┌──────────────────────────┐ │
│  │   Middleware Chain        │ │
│  │  Rate Limit → CORS →     │ │
│  │  Auth → Compress → Cache │ │
│  └───────────┬──────────────┘ │
│              ▼                │
│  ┌──────────────────────────┐ │
│  │   Load Balancer          │ │
│  │  Strategy: RR/LC/Hash    │ │
│  │  Health-aware routing    │ │
│  └───────────┬──────────────┘ │
│              ▼                │
│  ┌──────────────────────────┐ │
│  │   Backend Pool           │ │
│  │  container:port targets  │ │
│  └──────────────────────────┘ │
└──────────────────────────────┘
```

### 5.2 Entrypoints

| Entrypoint | Port | Protocol | Purpose |
|-----------|------|----------|---------|
| `web` | 80 | HTTP | Redirect to HTTPS (configurable) |
| `websecure` | 443 | HTTPS | Main TLS entrypoint |
| `api` | 8443 | HTTPS | DeployMonster API + UI |
| `metrics` | 9090 | HTTP | Internal metrics endpoint |

### 5.3 Router Matching Rules

```
Host(`example.com`)                         # Exact host
Host(`*.example.com`)                       # Wildcard subdomain
HostRegexp(`{subdomain:[a-z]+}.example.com`) # Regex host
Path(`/api`)                                # Exact path
PathPrefix(`/api/`)                         # Path prefix
Method(`GET`, `POST`)                       # HTTP method
Headers(`X-Custom`, `value`)                # Header match
Query(`debug`, `true`)                      # Query param
ClientIP(`192.168.1.0/24`)                  # IP range

# Combinations with && and ||
Host(`example.com`) && PathPrefix(`/api/`)
Host(`a.com`) || Host(`b.com`)
```

### 5.4 Built-in Middleware

| Middleware | Purpose | Key Config |
|-----------|---------|------------|
| `ratelimit` | Request rate limiting | requests/s, burst, by IP/header |
| `cors` | CORS headers | origins, methods, headers |
| `compress` | Gzip/Brotli compression | min size, content types |
| `auth-basic` | HTTP Basic auth | user:password pairs |
| `auth-jwt` | JWT validation | JWKS URL, claims |
| `auth-forward` | Forward auth to service | URL, headers to copy |
| `headers` | Custom headers | add/remove/override |
| `redirect` | URL redirect | permanent/temporary, regex |
| `stripprefix` | Strip URL prefix | prefixes to remove |
| `retry` | Retry failed requests | attempts, conditions |
| `circuitbreaker` | Circuit breaker | threshold, timeout |
| `ipwhitelist` | IP access control | allowed CIDRs |
| `buffering` | Request buffering | max body size |

### 5.5 SSL/TLS Management

```
┌──────────────────────────────┐
│       ACME Manager           │
│  ┌────────────────────────┐  │
│  │  Certificate Store     │  │ ← SQLite (encrypted PEM)
│  └────────────────────────┘  │
│  ┌────────────────────────┐  │
│  │  HTTP-01 Challenge     │  │ ← Port 80 /.well-known/acme-challenge/
│  └────────────────────────┘  │
│  ┌────────────────────────┐  │
│  │  DNS-01 Challenge      │  │ ← Cloudflare/Route53 API
│  └────────────────────────┘  │
│  ┌────────────────────────┐  │
│  │  Auto-Renewal Cron     │  │ ← 30 days before expiry
│  └────────────────────────┘  │
│  ┌────────────────────────┐  │
│  │  Wildcard Support      │  │ ← *.example.com via DNS-01
│  └────────────────────────┘  │
└──────────────────────────────┘
```

**Certificate Resolvers:**
- `letsencrypt` — Production Let's Encrypt (rate limited)
- `letsencrypt-staging` — Staging for testing
- `custom` — Upload custom cert + key
- `selfsigned` — Auto-generated self-signed (dev only)

---

## 6. BUILD ENGINE

### 6.1 Build Pipeline

```
Source Input                    Build Stage                    Output
───────────                    ───────────                    ──────
                              ┌─────────────┐
GitHub URL     ──────────────►│             │
GitLab URL     ──────────────►│  Git Clone   │
Bitbucket URL  ──────────────►│  + Checkout  │
Gitea URL      ──────────────►│  branch/tag  │
Gogs URL       ──────────────►│             │
Azure DevOps   ──────────────►│  Any Git!    │
Any Git SSH    ──────────────►│             │
Any Git HTTPS  ──────────────►│             │
                              └──────┬──────┘
                                     ▼
                              ┌─────────────┐
                              │  Detect      │
                              │  Build Type  │
                              │  (auto)      │
                              └──────┬──────┘
                                     ▼
                    ┌────────────────┼────────────────┐
                    ▼                ▼                ▼
             ┌────────────┐  ┌────────────┐  ┌────────────┐
             │ Dockerfile │  │ Buildpack  │  │ Nixpacks   │
             │ Build      │  │ (Heroku)   │  │ (auto)     │
             └─────┬──────┘  └─────┬──────┘  └─────┬──────┘
                   │               │               │
                   └───────────────┼───────────────┘
                                   ▼
                            ┌─────────────┐
                            │ Docker Image │───► Local Registry
                            │ Tagged       │───► Remote Registry
                            └──────┬──────┘
                                   ▼
                            ┌─────────────┐
                            │  Deploy      │
                            │  Container   │
                            └─────────────┘

Direct Deploy (No Build):
─────────────────────────
Docker Image URL  ──────────► Pull Image ──────────► Deploy Container
  nginx:latest                 docker pull
  ghcr.io/user/app:v2
  registry.example.com/app

Docker Compose YAML ────────► Parse YAML ──────────► Deploy Stack
  upload / git / URL           Validate              Multi-container
                               Resolve deps          Network creation
                               Generate labels       Volume mounting
```

### 6.2 Deploy Source Types

| Source Type | Input | Build Required | Example |
|-------------|-------|---------------|---------|
| `git` | Any Git repository URL | Yes | `https://github.com/user/app.git` |
| `image` | Docker image reference | No | `nginx:latest`, `ghcr.io/user/app:v2` |
| `compose` | docker-compose.yml | Partial (if build: in YAML) | Upload, URL, or inline YAML |
| `dockerfile` | Raw Dockerfile + context | Yes | Upload Dockerfile + source |
| `marketplace` | Template slug | No | `wordpress`, `ghost`, `n8n` |

### 6.3 Universal Git Source Support

| Provider | Clone Method | Auth Methods | Webhook Support |
|----------|-------------|-------------|-----------------|
| GitHub | HTTPS / SSH | OAuth2, PAT, Deploy Key, GitHub App | ✅ Push, PR, Release, Tag |
| GitLab | HTTPS / SSH | OAuth2, PAT, Deploy Token, Deploy Key | ✅ Push, MR, Tag, Release |
| Bitbucket | HTTPS / SSH | OAuth2, App Password, Deploy Key | ✅ Push, PR, Tag |
| Gitea | HTTPS / SSH | PAT, Deploy Key | ✅ Push, PR, Tag, Release |
| Gogs | HTTPS / SSH | PAT, Deploy Key | ✅ Push |
| Azure DevOps | HTTPS / SSH | PAT, OAuth2 | ✅ Push, PR |
| AWS CodeCommit | HTTPS / SSH | IAM Credentials, SSH Key | ✅ Push |
| Generic Git | HTTPS / SSH | HTTP Basic, SSH Key | ✅ Generic POST webhook |
| Self-hosted | Any | SSH Key, Token, HTTP Basic | ✅ Configurable payload |

**Git Clone Flow:**
```
1. Resolve GitSource credentials (decrypt token/key)
2. Clone via HTTPS (token auth) or SSH (deploy key)
3. Checkout specified branch/tag/commit
4. Apply .monsterignore (like .dockerignore but for build)
5. Detect project type
6. Build & Deploy
```

### 6.4 Auto-Detection Matrix

| Indicator File | Detected As | Build Strategy |
|---------------|-------------|----------------|
| `Dockerfile` | Docker | Use as-is |
| `docker-compose.yml` | Docker Compose | Parse and deploy services |
| `package.json` + `next.config.*` | Next.js | Node → build → standalone |
| `package.json` + `vite.config.*` | Vite/React | Node → build → nginx serve |
| `package.json` + `nuxt.config.*` | Nuxt.js | Node → build → standalone |
| `package.json` (generic) | Node.js | Node → npm start |
| `go.mod` | Go | Multi-stage Go build |
| `Cargo.toml` | Rust | Multi-stage Rust build |
| `requirements.txt` / `pyproject.toml` | Python | Python → pip install → run |
| `Gemfile` | Ruby | Ruby → bundle → run |
| `composer.json` | PHP | PHP-FPM + nginx |
| `pom.xml` / `build.gradle` | Java | Maven/Gradle → JAR → JRE |
| `*.sln` / `*.csproj` | .NET | dotnet publish |
| `index.html` (static) | Static Site | nginx serve |

### 6.5 Build Environment Variables

```
MONSTER_BUILD=true
MONSTER_APP_NAME=my-app
MONSTER_APP_ID=app_xxxx
MONSTER_DEPLOYMENT_ID=dep_xxxx
MONSTER_GIT_COMMIT=abc123
MONSTER_GIT_BRANCH=main
MONSTER_BUILD_NUMBER=42
```

---

## 7. DEPLOY ENGINE

### 7.1 Application Types

| Type | Description | Example |
|------|-------------|---------|
| `service` | Long-running web service | Next.js app, API server |
| `worker` | Background worker process | Queue consumer, cron runner |
| `static` | Static file serving | React SPA, documentation site |
| `database` | Managed database | PostgreSQL, MySQL, Redis |
| `cron` | Scheduled task | Backup script, report generator |
| `compose-stack` | Multi-service docker-compose stack | WordPress + MySQL + Redis |

### 7.2 Deploy Modes

| Mode | Input | What Happens | Use Case |
|------|-------|-------------|----------|
| **Git Deploy** | Git repo URL + branch | Clone → detect → build → containerize → deploy | Custom apps, source code |
| **Image Deploy** | Docker image reference | Pull → deploy (no build) | Pre-built images, CI/CD outputs |
| **Compose Deploy** | docker-compose.yml | Parse → create network → deploy all services | Multi-container stacks |
| **Marketplace Deploy** | Template slug | Load template → configure → deploy stack | One-click apps |
| **Upload Deploy** | Zip/tar archive | Extract → detect → build → deploy | Direct file upload |

### 7.2.1 Docker Image Deploy

Deploy any Docker image directly without building:

```
User provides: nginx:latest | ghcr.io/user/app:v2 | registry.example.com/myapp:1.0
        │
        ▼
┌──────────────────┐
│  Validate Image  │ ← Check image exists (docker pull --dry-run)
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Pull Image      │ ← Handle registry auth if needed
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Configure       │ ← Ports, env vars, volumes, labels
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Deploy + Route  │ ← Start container, attach to ingress
└──────────────────┘
```

**Supported Registries:**
- Docker Hub (default)
- GitHub Container Registry (ghcr.io)
- GitLab Container Registry
- AWS ECR
- Google GCR / Artifact Registry
- Azure ACR
- Self-hosted registries (with auth)
- Any OCI-compliant registry

### 7.2.2 Docker Compose Deploy

Deploy multi-service stacks from docker-compose.yml:

```
Input: docker-compose.yml (upload / git / URL / paste)
        │
        ▼
┌──────────────────┐
│  Parse YAML      │ ← Support Compose v2/v3 spec
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Validate        │ ← Check images, ports, volumes
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Resolve Vars    │ ← .env file, ${VAR} interpolation
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Create Network  │ ← Isolated network per stack
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Deploy Services │ ← Respect depends_on order
│  (ordered)       │ ← Build if `build:` specified
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Inject Labels   │ ← Auto monster.* labels for discovery
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Health Check    │ ← Wait for all services healthy
└──────────────────┘
```

**Compose Features:**
- Full Compose v2/v3 spec support
- `build:` directive → triggers Build Engine for services with source
- `depends_on:` → ordered startup with health check waiting
- `volumes:` → auto-create named volumes
- `.env` file support → upload alongside compose file
- `profiles:` → selective service deployment
- Port mapping → auto-ingress route creation
- Variable interpolation: `${VAR:-default}` syntax
- Override files: `docker-compose.override.yml` support
- Live edit: modify compose → redeploy changed services only

### 7.3 Deployment Strategies

| Strategy | Description | Use Case |
|---------|-------------|----------|
| `recreate` | Stop old → Start new | Simple, default for single instance |
| `rolling` | Gradual replacement | Zero-downtime for multi-instance |
| `blue-green` | Parallel deploy, swap traffic | Critical production services |
| `canary` | Gradual traffic shift (10% → 50% → 100%) | Risk-sensitive deployments |

### 7.4 Deployment Lifecycle

```
PENDING → BUILDING → BUILT → DEPLOYING → RUNNING → STOPPING → STOPPED
                 ↓                            ↓
              FAILED                       DEGRADED
                                             ↓
                                          CRASHED → AUTO_RESTART
```

### 7.5 Environment Variables Management

- **Plain text** — Stored encrypted at rest in SQLite
- **Secret references** — `${SECRET:db_password}` resolved at deploy time
- **Interpolation** — `DATABASE_URL=postgres://user:${SECRET:db_pass}@db:5432/app`
- **Inheritance** — Project-level vars inherited by all apps, app-level overrides
- **Preview** — See resolved vars before deploy (secrets masked)

---

## 8. SERVICE DISCOVERY

### 8.1 Discovery Flow

```
Docker Event Stream
        │
        ▼
┌──────────────────┐
│  Event Listener  │ ← container.start, container.stop, container.die
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Label Parser    │ ← Read monster.* labels
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Service Registry│ ← In-memory + persisted
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Router Update   │ ← Hot-reload ingress routes
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Health Checker  │ ← Periodic health verification
└──────────────────┘
```

### 8.2 Health Check Types

| Type | Method | Default Interval |
|------|--------|-----------------|
| `http` | GET request, expect 2xx | 10s |
| `tcp` | TCP connection | 10s |
| `exec` | Run command in container | 30s |
| `grpc` | gRPC health check | 10s |

---

## 9. LOAD BALANCER

### 9.1 Strategies

| Strategy | Description | Best For |
|----------|-------------|----------|
| `round-robin` | Sequential distribution | General purpose (default) |
| `least-connections` | Route to least busy | Varying request duration |
| `ip-hash` | Consistent by client IP | Session affinity |
| `random` | Random selection | Simple distribution |
| `weighted` | Weight-based distribution | Canary, A/B testing |
| `header-hash` | Hash specific header | Custom routing logic |

### 9.2 Sticky Sessions

```yaml
sticky:
  cookie:
    name: MONSTER_AFFINITY
    secure: true
    httpOnly: true
    maxAge: 3600
    sameSite: lax
```

---

## 10. DNS SYNCHRONIZATION

### 10.1 Provider Support

| Provider | Method | Features |
|----------|--------|----------|
| Cloudflare | REST API v4 | A, AAAA, CNAME, TXT, Proxy toggle |
| AWS Route 53 | AWS SDK | A, AAAA, CNAME, Alias records |
| DigitalOcean DNS | REST API | A, AAAA, CNAME, TXT |
| Generic RFC2136 | DNS UPDATE protocol | Any compliant DNS server |
| Manual | No sync | User manages DNS externally |

### 10.2 Sync Flow

```
Domain Created/Changed in DeployMonster
        │
        ▼
┌──────────────────┐
│  DNS Sync Queue  │
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Provider API    │ ← Create/Update DNS records
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Verify Record   │ ← DNS lookup to confirm propagation
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Mark Synced     │ ← Update domain status in DB
└──────────────────┘
```

### 10.3 Auto-DNS Features

- **Wildcard setup** — `*.app.deploy.monster → server IP` for instant subdomains
- **Auto-subdomain** — Deploy app "my-app" → `my-app.deploy.monster` automatically
- **Custom domain** — Add `app.example.com`, get DNS instructions or auto-sync
- **Proxy toggle** — Enable/disable Cloudflare proxy per domain

---

## 11. RESOURCE MANAGEMENT

### 11.1 Metrics Collection

```go
type ServerMetrics struct {
    Timestamp   time.Time
    CPUPercent  float64    // 0-100 per core
    CPUCores    int
    RAMUsedMB   int64
    RAMTotalMB  int64
    DiskUsedMB  int64
    DiskTotalMB int64
    NetworkRxMB int64
    NetworkTxMB int64
    LoadAvg     [3]float64 // 1m, 5m, 15m
}

type ContainerMetrics struct {
    ContainerID string
    Timestamp   time.Time
    CPUPercent  float64
    RAMUsedMB   int64
    RAMLimitMB  int64
    NetworkRxMB int64
    NetworkTxMB int64
    DiskReadMB  int64
    DiskWriteMB int64
    PIDs        int
}
```

### 11.2 HTTP / Ingress Metrics

The Ingress Gateway collects per-request metrics for every routed application:

```go
type HTTPMetrics struct {
    AppID          string
    DomainFQDN     string
    Timestamp      time.Time
    RequestCount   int64
    ErrorCount     int64     // 4xx + 5xx responses
    BytesIn        int64
    BytesOut       int64
    LatencyP50ms   float64
    LatencyP95ms   float64
    LatencyP99ms   float64
    StatusCodes    map[int]int64 // 200: 1000, 404: 5, 500: 1
    TopPaths       map[string]int64
    TopIPs         map[string]int64
    TopUserAgents  map[string]int64
    TopCountries   map[string]int64 // GeoIP from Cloudflare or MaxMind
}
```

**Metrics Retention:**

| Resolution | Retention | Storage |
|-----------|-----------|---------|
| 1-second raw | 1 hour | In-memory ring buffer |
| 1-minute rollup | 24 hours | SQLite |
| 1-hour rollup | 30 days | SQLite |
| 1-day rollup | 1 year | SQLite |
| Monthly summary | Forever | SQLite |

### 11.3 Alert Rules

```yaml
alerts:
  - name: high_cpu
    condition: "cpu_percent > 90"
    duration: 5m
    severity: warning
    channels: [email, slack]

  - name: disk_full
    condition: "disk_used_percent > 95"
    duration: 1m
    severity: critical
    channels: [email, slack, telegram]

  - name: container_crash_loop
    condition: "restart_count > 3 in 5m"
    severity: critical
    channels: [email, slack]

  - name: ssl_expiry
    condition: "cert_expires_in < 7d"
    severity: warning
    channels: [email]

  - name: high_error_rate
    condition: "http_error_rate > 5%"
    duration: 5m
    severity: critical
    channels: [email, slack, telegram]

  - name: high_latency
    condition: "http_latency_p95 > 2000ms"
    duration: 10m
    severity: warning
    channels: [email, slack]

  - name: bandwidth_limit_approaching
    condition: "monthly_bandwidth > plan_limit * 0.8"
    severity: warning
    channels: [email]

  - name: resource_limit_exceeded
    condition: "any_resource > plan_limit"
    severity: critical
    channels: [email]
    action: warn_and_log   # warn_and_log | block_new | throttle
```

### 11.4 Resource Quotas (Per Tenant/App)

```go
// Enforced via Docker container resource constraints
type ResourceQuota struct {
    // Per-Container Limits (set on docker create)
    CPUShares     int64   `json:"cpu_shares"`      // Relative weight (default: 1024)
    CPUQuota      int64   `json:"cpu_quota"`        // CFS quota μs (100000 = 1 core)
    CPUPeriod     int64   `json:"cpu_period"`       // CFS period (default: 100000μs)
    MemoryLimitMB int64   `json:"memory_limit_mb"`  // Hard limit, OOM kill
    MemorySoftMB  int64   `json:"memory_soft_mb"`   // Soft limit, reclaim target
    PidsLimit     int64   `json:"pids_limit"`       // Max processes
    IOReadBps     int64   `json:"io_read_bps"`      // Disk read B/s (0=unlimited)
    IOWriteBps    int64   `json:"io_write_bps"`     // Disk write B/s
    
    // Per-Tenant Aggregate Limits (enforced by DeployMonster)
    MaxApps          int   `json:"max_apps"`
    MaxContainers    int   `json:"max_containers"`
    MaxCPUCores      float64 `json:"max_cpu_cores"`      // Total across all apps
    MaxRAMMB         int64   `json:"max_ram_mb"`          // Total across all apps
    MaxDiskGB        int64   `json:"max_disk_gb"`         // Total volume storage
    MaxBandwidthGB   int64   `json:"max_bandwidth_gb"`    // Monthly egress
    MaxBuildMinutes  int64   `json:"max_build_minutes"`   // Monthly build time
    MaxDomains       int     `json:"max_domains"`
    MaxDatabases     int     `json:"max_databases"`
    MaxServers       int     `json:"max_servers"`
    MaxTeamMembers   int     `json:"max_team_members"`
    MaxBackupGB      int64   `json:"max_backup_gb"`
}
```

**Enforcement Points:**

| Resource | Where Enforced | How |
|----------|---------------|-----|
| CPU | Docker `--cpus` / `--cpu-quota` | cgroups v2 CPU controller |
| Memory | Docker `--memory` / `--memory-reservation` | cgroups v2 memory controller, OOM kill |
| Disk I/O | Docker `--device-read-bps` | blkio cgroup controller |
| Network bandwidth | `tc` (traffic control) via agent | Per-container bandwidth shaping |
| App count | API layer | Reject `POST /apps` if at limit |
| Build minutes | Build engine | Kill build process at timeout |
| Bandwidth | Ingress proxy | Real-time byte counting per tenant |
| Storage | Volume manager | Reject volume expansion at limit |

---

## 12. BACKUP ENGINE

### 12.1 Backup Types

| Type | What | Method |
|------|------|--------|
| Volume Backup | Docker volumes | tar + gzip snapshot |
| Database Backup | Managed DBs | pg_dump / mysqldump / redis BGSAVE |
| Config Backup | DeployMonster config | SQLite backup API |
| Full Backup | Everything above | Combined archive |

### 12.2 Storage Targets

| Target | Protocol | Config |
|--------|----------|--------|
| Local | Filesystem | Path, retention |
| S3 | AWS S3 API | Bucket, region, credentials |
| S3-Compatible | S3 API | Endpoint, bucket, credentials (MinIO, Backblaze, Wasabi, etc.) |
| SFTP | SSH/SFTP | Host, user, key |
| Rclone | Multiple | Any Rclone-supported backend |
| Cloudflare R2 | S3 API | Account ID, access key, bucket |

**S3/S3-Compatible Storage Configuration:**

```yaml
# Example: AWS S3
backup_targets:
  aws-s3:
    type: s3
    endpoint: ""                        # Empty = AWS default
    bucket: deploymonster-backups
    region: eu-central-1
    access_key: "${SECRET:global/aws_access_key}"
    secret_key: "${SECRET:global/aws_secret_key}"
    path_prefix: "backups/"             # Optional prefix inside bucket
    storage_class: STANDARD_IA          # STANDARD | STANDARD_IA | GLACIER
    encryption: AES256                  # AES256 | aws:kms
    retention_days: 90
    
  # Example: MinIO (self-hosted)
  minio:
    type: s3
    endpoint: https://minio.example.com
    bucket: monster-backups
    region: us-east-1
    access_key: "${SECRET:global/minio_key}"
    secret_key: "${SECRET:global/minio_secret}"
    force_path_style: true              # Required for MinIO
    
  # Example: Cloudflare R2
  r2:
    type: s3
    endpoint: https://<account_id>.r2.cloudflarestorage.com
    bucket: monster-backups
    access_key: "${SECRET:global/r2_key}"
    secret_key: "${SECRET:global/r2_secret}"
    region: auto

  # Example: SFTP to remote server
  offsite:
    type: sftp
    host: backup-server.example.com
    port: 22
    user: backup
    ssh_key_id: key_xxxx               # Reference to SSH key in vault
    path: /backups/deploymonster
```

**Storage Target UI (Admin Panel):**
```
┌─────────────────────────────────────────────────────────────┐
│  Backup Storage Targets                    [+ Add Target]   │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ 🪣 AWS S3 (eu-central-1)          Status: 🟢 Connected │ │
│  │    Bucket: deploymonster-backups   Used: 42 GB          │ │
│  │    Last verified: 2 hours ago      [Test] [Edit] [Del]  │ │
│  └────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ 📦 MinIO (minio.example.com)      Status: 🟢 Connected │ │
│  │    Bucket: monster-backups         Used: 18 GB          │ │
│  │    Last verified: 1 hour ago       [Test] [Edit] [Del]  │ │
│  └────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ 💾 Local (/var/lib/dm/backups)     Status: 🟢 Active   │ │
│  │    Available: 82 GB                Used: 15 GB          │ │
│  │    Auto-cleanup: 30 days           [Edit]               │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### 12.3 Backup Schedule

```yaml
backup:
  schedule: "0 2 * * *"     # Daily at 2 AM
  retention:
    daily: 7                 # Keep 7 daily backups
    weekly: 4                # Keep 4 weekly backups
    monthly: 3               # Keep 3 monthly backups
  encryption: aes-256-gcm   # Encrypt backups at rest
  compression: zstd          # Fast compression
  notify_on_failure: true
```

---

## 13. DATABASE MANAGER

### 13.1 Supported Engines

| Engine | Versions | Features |
|--------|----------|----------|
| PostgreSQL | 14, 15, 16, 17 | Auto-backup, connection pooling, extensions |
| MySQL | 8.0, 8.4 | Auto-backup, replication ready |
| MariaDB | 10.11, 11.x | Auto-backup, drop-in MySQL replacement |
| Redis | 7.x | Persistence options, cluster mode |
| MongoDB | 7.x | Replica set support |

### 13.2 Database Provisioning Flow

```
User: "Add PostgreSQL 16"
        │
        ▼
┌──────────────────┐
│ Generate password │ ← Cryptographically secure
└────────┬─────────┘
         ▼
┌──────────────────┐
│ Create Volume     │ ← Persistent storage
└────────┬─────────┘
         ▼
┌──────────────────┐
│ Deploy Container  │ ← Official image, custom config
└────────┬─────────┘
         ▼
┌──────────────────┐
│ Health Check      │ ← Wait for ready
└────────┬─────────┘
         ▼
┌──────────────────┐
│ Register Service  │ ← Internal DNS: postgres.monster.internal
└────────┬─────────┘
         ▼
┌──────────────────┐
│ Provide Creds     │ ← Show connection string to user
└──────────────────┘
```

---

## 14. SWARM ORCHESTRATOR

### 14.1 Multi-Node Architecture

```
┌─────────────────────────────────────┐
│          Manager Node               │
│  ┌─────────────────────────────┐    │
│  │     DeployMonster (Full)    │    │
│  │  + Ingress Gateway          │    │
│  │  + Swarm Manager            │    │
│  │  + UI + API                 │    │
│  └─────────────────────────────┘    │
└─────────────┬───────────────────────┘
              │ Docker Swarm Overlay Network
    ┌─────────┼─────────┐
    ▼         ▼         ▼
┌────────┐ ┌────────┐ ┌────────┐
│Worker 1│ │Worker 2│ │Worker 3│
│────────│ │────────│ │────────│
│Monster │ │Monster │ │Monster │
│Agent   │ │Agent   │ │Agent   │
│(lite)  │ │(lite)  │ │(lite)  │
└────────┘ └────────┘ └────────┘
```

### 14.2 Agent Mode

When DeployMonster runs with `--agent` flag:
- Joins existing Swarm cluster
- Reports metrics to manager
- Accepts container placement
- No UI, no API, minimal footprint
- Auto-discovered by manager

### 14.3 Node Labels & Placement

```yaml
# Node labels for placement constraints
monster.node.zone=eu-west
monster.node.tier=compute
monster.node.gpu=true
monster.node.disk=ssd

# Placement rules
placement:
  constraints:
    - monster.node.zone == eu-west
    - monster.node.disk == ssd
  preferences:
    - spread: monster.node.zone
```

---

## 15. VPS PROVIDER MANAGER

DeployMonster can provision and manage remote servers from cloud providers, or connect to any existing server via SSH.

### 15.1 Supported Providers

| Provider | API | Server Create | DNS | Block Storage | Snapshots |
|----------|-----|--------------|-----|--------------|-----------|
| Hetzner Cloud | REST v1 | ✅ | ✅ | ✅ | ✅ |
| DigitalOcean | REST v2 | ✅ | ✅ | ✅ | ✅ |
| Vultr | REST v2 | ✅ | ✅ | ✅ | ✅ |
| Linode (Akamai) | REST v4 | ✅ | ✅ | ✅ | ✅ |
| AWS EC2 | AWS SDK | ✅ | Route53 | EBS | AMI |
| Custom SSH | SSH | ❌ (existing) | ❌ | ❌ | ❌ |

### 15.2 Server Provisioning Flow

```
User: "Add Hetzner server, cx22, Nuremberg"
        │
        ▼
┌──────────────────┐
│  Validate API Key │ ← Test provider credentials
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Create Server    │ ← Provider API: create VPS
│  (cloud-init)     │ ← Auto-install Docker + DeployMonster agent
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Wait for Boot   │ ← Poll provider API for status
└────────┬─────────┘
         ▼
┌──────────────────┐
│  SSH Bootstrap    │ ← Connect, verify Docker, install agent
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Join Swarm      │ ← Auto-join as worker (or manager)
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Register in DB  │ ← Save server details, start monitoring
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Configure DNS   │ ← Optional: A record for server IP
└──────────────────┘
```

### 15.3 Cloud-Init Script (Auto Bootstrap)

```yaml
#cloud-config
package_update: true
packages:
  - curl
  - ca-certificates

runcmd:
  # Install Docker
  - curl -fsSL https://get.docker.com | sh
  # Install DeployMonster Agent
  - curl -fsSL https://deploy.monster/install.sh | bash -s -- --agent
  # Join Swarm cluster
  - deploymonster agent join --manager=${MANAGER_IP} --token=${JOIN_TOKEN}
  # Report ready
  - deploymonster agent ready --callback=${CALLBACK_URL}
```

### 15.4 Custom SSH Server Connection

For existing servers not provisioned by DeployMonster:

```
User: "Connect existing server 198.51.100.42"
        │
        ▼
┌──────────────────┐
│  SSH Key Upload   │ ← User provides key or DeployMonster generates one
└────────┬─────────┘
         ▼
┌──────────────────┐
│  SSH Connect Test │ ← Verify access, check OS
└────────┬─────────┘
         ▼
┌──────────────────┐
│  System Check     │ ← CPU, RAM, disk, Docker installed?
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Install Docker   │ ← If missing, auto-install
│  (if needed)      │
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Install Agent    │ ← Deploy DeployMonster agent binary
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Join Swarm      │ ← Join as worker node
└──────────────────┘
```

### 15.5 Remote Server Management

| Action | Local | Remote (SSH) | Remote (API) |
|--------|-------|-------------|-------------|
| Deploy container | Docker SDK | SSH tunnel → Docker SDK | SSH tunnel → Docker SDK |
| View logs | Docker SDK | WebSocket → Agent → Docker | WebSocket → Agent → Docker |
| Exec terminal | Docker SDK | WebSocket → Agent → Docker | WebSocket → Agent → Docker |
| Metrics | /proc, Docker stats | Agent reports via gRPC/WS | Agent reports via gRPC/WS |
| Volume backup | Local tar | Agent → tar → SFTP/S3 | Agent → tar → SFTP/S3 |
| Server delete | N/A | SSH disconnect | Provider API: destroy |
| Resize | N/A | N/A | Provider API: resize |
| Snapshot | N/A | N/A | Provider API: snapshot |

### 15.6 Server Roles

| Role | Description | Services Running |
|------|-------------|-----------------|
| `manager` | Swarm manager + full DeployMonster | Full binary, ingress, UI, API, DB |
| `manager-replica` | Backup manager (HA) | Full binary, standby ingress |
| `worker` | Container execution | Agent only, Docker |
| `worker-build` | Dedicated build server | Agent + build engine |
| `worker-db` | Dedicated database server | Agent + DB containers |
| `edge` | Edge location ingress | Agent + ingress proxy |

---

## 16. GIT SOURCES & WEBHOOKS

### 16.1 Git Source Management

DeployMonster provides a unified interface to connect ANY Git provider:

```
┌──────────────────────────────────────────────────┐
│              Git Source Manager                    │
│                                                    │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  │
│  │  GitHub     │  │  GitLab    │  │  Bitbucket  │  │
│  │  OAuth2     │  │  OAuth2    │  │  OAuth2     │  │
│  └────────────┘  └────────────┘  └────────────┘  │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  │
│  │  Gitea      │  │  Gogs      │  │  Azure      │  │
│  │  PAT        │  │  PAT       │  │  DevOps     │  │
│  └────────────┘  └────────────┘  └────────────┘  │
│  ┌────────────┐  ┌─────────────────────────────┐  │
│  │  CodeCommit │  │  Custom Git (any SSH/HTTPS)  │  │
│  │  IAM        │  │  Token / SSH Key / Basic     │  │
│  └────────────┘  └─────────────────────────────┘  │
└──────────────────────────────────────────────────┘
```

**OAuth2 Flow (GitHub/GitLab/Bitbucket):**
1. User clicks "Connect GitHub"
2. Redirect to provider OAuth consent screen
3. User authorizes DeployMonster app
4. Callback with auth code → exchange for access token
5. Store encrypted token
6. List user's repositories via API
7. User selects repo → create app

**Token/SSH Flow (Gitea/Gogs/Custom):**
1. User provides base URL (e.g., `https://git.example.com`)
2. User provides PAT or uploads SSH key
3. DeployMonster tests auth (list repos or clone test)
4. Store encrypted credentials
5. Ready to deploy

### 16.2 Universal Webhook System

DeployMonster exposes a single webhook endpoint that handles ALL Git providers:

```
POST /hooks/v1/{webhook_id}
POST /hooks/v1/{webhook_id}/{provider}    # Provider-specific parsing

Webhook Secret: HMAC-SHA256 validation per webhook
```

**Webhook Registration Flow:**
```
App Created with Git Source
        │
        ▼
┌──────────────────┐
│  Generate Secret  │ ← Crypto-random webhook secret
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Generate URL     │ ← https://panel.deploy.monster/hooks/v1/wh_xxxx
└────────┬─────────┘
         ▼
┌──────────────────┐     ┌────────────────┐
│  Auto-Register?   │─Yes─│ Provider API    │ ← Auto-create webhook via API
└────────┬─────────┘     │ (GitHub/GitLab) │
         │ No            └────────────────┘
         ▼
┌──────────────────┐
│  Show Manual      │ ← Display URL + secret for user to add manually
│  Instructions     │
└──────────────────┘
```

**Webhook Event Processing:**
```
Incoming POST /hooks/v1/wh_xxxx
        │
        ▼
┌──────────────────┐
│  Verify Signature │ ← HMAC-SHA256 (GitHub), X-Gitlab-Token, etc.
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Detect Provider  │ ← Parse headers (X-GitHub-Event, X-Gitlab-Event, etc.)
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Parse Payload    │ ← Extract: commit, branch, author, message
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Branch Filter    │ ← Does branch match app's configured branch?
└────────┬─────────┘
         ▼ Yes
┌──────────────────┐
│  Event Filter     │ ← push? tag? release? PR merge?
└────────┬─────────┘
         ▼ Match
┌──────────────────┐
│  Auto Deploy?     │─Yes─► Trigger Build & Deploy Pipeline
└────────┬─────────┘
         │ No
         ▼
┌──────────────────┐
│  Queue for Manual │ ← Show in UI: "New commit detected, deploy?"
│  Approval         │
└──────────────────┘
```

**Provider-Specific Webhook Headers:**

| Provider | Event Header | Signature Header | Signature Method |
|----------|-------------|-----------------|------------------|
| GitHub | `X-GitHub-Event` | `X-Hub-Signature-256` | HMAC-SHA256 |
| GitLab | `X-Gitlab-Event` | `X-Gitlab-Token` | Token match |
| Bitbucket | `X-Event-Key` | `X-Hub-Signature` | HMAC-SHA256 |
| Gitea | `X-Gitea-Event` | `X-Gitea-Signature` | HMAC-SHA256 |
| Gogs | `X-Gogs-Event` | `X-Gogs-Signature` | HMAC-SHA256 |
| Azure DevOps | `N/A (in body)` | Basic Auth | HTTP Basic |
| Custom | Configurable | Configurable | HMAC-SHA256 / Token |

---

## 17. WEB UI

DeployMonster's UI is a professional-grade infrastructure management dashboard — not a toy. It competes directly with Coolify, Dokploy, CapRover, Portainer, and Vercel's dashboard. Every pixel matters.

### 17.1 Technology Stack

| Tech | Version | Purpose |
|------|---------|---------|
| React | 19 | UI framework (Server Components ready) |
| TypeScript | 5.7+ | Strict type safety, no `any` |
| Vite | 6.x | Build tool, HMR, tree-shaking |
| Tailwind CSS | 4.1 | Utility-first CSS, CSS-first config |
| shadcn/ui | latest | Radix-based component library (copy-paste, fully owned) |
| Lucide React | latest | Icon library (1000+ icons, tree-shakable) |
| React Router | 7.x | File-based routing, nested layouts |
| TanStack Query | 5.x | Data fetching, cache, optimistic updates |
| TanStack Table | 8.x | Headless table for data grids |
| Zustand | 5.x | Lightweight state management |
| React Flow | 12.x | Drag & drop topology canvas |
| Recharts | 2.x | Charts, metrics dashboards |
| xterm.js | 5.x | Terminal emulator (logs, exec) |
| Monaco Editor | latest | Code editor (env vars, YAML, Dockerfile) |
| react-hot-toast | latest | Toast notifications |
| date-fns | 4.x | Date formatting |
| zod | 3.x | Runtime schema validation |
| cmdk | latest | Command palette (⌘K) |

### 17.2 Design System

**Theme Engine:**

DeployMonster ships with a polished dark/light theme system using CSS custom properties + Tailwind CSS 4.1's native theming.

```
┌──────────────────────────────────────────────────────────┐
│  Theme System                                             │
│                                                           │
│  ┌──────────────────┐  ┌──────────────────────────────┐  │
│  │  Theme Provider   │  │  CSS Custom Properties       │  │
│  │  (React Context)  │  │  --background: hsl(...)      │  │
│  │                   │  │  --foreground: hsl(...)      │  │
│  │  • dark (default) │  │  --primary: hsl(...)         │  │
│  │  • light          │  │  --muted: hsl(...)           │  │
│  │  • system (auto)  │  │  --accent: hsl(...)          │  │
│  │                   │  │  --destructive: hsl(...)     │  │
│  │  Persisted in     │  │  --border: hsl(...)          │  │
│  │  localStorage     │  │  --ring: hsl(...)            │  │
│  └──────────────────┘  │  --radius: 0.5rem            │  │
│                         └──────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

**Brand Colors:**

| Token | Dark Mode | Light Mode | Usage |
|-------|-----------|------------|-------|
| `--background` | `hsl(224 71% 4%)` | `hsl(0 0% 100%)` | Page background |
| `--foreground` | `hsl(210 20% 98%)` | `hsl(224 71% 4%)` | Primary text |
| `--primary` | `hsl(142 76% 46%)` | `hsl(142 76% 36%)` | Monster Green™ — buttons, links, accents |
| `--primary-foreground` | `hsl(0 0% 100%)` | `hsl(0 0% 100%)` | Text on primary |
| `--secondary` | `hsl(215 25% 15%)` | `hsl(210 40% 96%)` | Secondary surfaces |
| `--muted` | `hsl(215 25% 20%)` | `hsl(210 40% 96%)` | Muted backgrounds |
| `--accent` | `hsl(262 83% 58%)` | `hsl(262 83% 58%)` | Monster Purple — highlights |
| `--destructive` | `hsl(0 84% 60%)` | `hsl(0 84% 60%)` | Errors, delete actions |
| `--warning` | `hsl(38 92% 50%)` | `hsl(38 92% 50%)` | Warnings, degraded |
| `--success` | `hsl(142 76% 46%)` | `hsl(142 76% 36%)` | Success, healthy |
| `--card` | `hsl(215 25% 8%)` | `hsl(0 0% 100%)` | Card surfaces |
| `--sidebar` | `hsl(215 30% 6%)` | `hsl(210 40% 98%)` | Sidebar background |
| `--border` | `hsl(215 25% 18%)` | `hsl(214 32% 91%)` | Borders |

**Design Principles:**
- **Dark-first** — Dark mode is the default. Infrastructure tools are used at 2 AM.
- **Information density** — Show data, not whitespace. Every pixel earns its place.
- **Keyboard-first** — ⌘K command palette, keyboard shortcuts for everything
- **Zero-scroll dashboards** — Critical info above the fold
- **Consistent patterns** — Every list, form, detail page follows the same layout
- **Real-time feedback** — WebSocket-driven live updates, no polling, no stale data
- **Mobile responsive** — Usable on tablet/phone for emergency ops (not primary target)

**Command Palette (⌘K):**
```
┌──────────────────────────────────────────────────┐
│  🔍 Type a command or search...                   │
│──────────────────────────────────────────────────│
│  Recently used                                    │
│  ├── 📦 my-nextjs-app                  → App     │
│  ├── 🐘 production-db                  → DB      │
│  └── 📡 api.example.com               → Domain   │
│                                                   │
│  Actions                                          │
│  ├── 🚀 Deploy new app              Ctrl+Shift+D │
│  ├── 📋 View logs                    Ctrl+L       │
│  ├── 💻 Open terminal                Ctrl+T       │
│  ├── 🔑 Manage secrets              Ctrl+Shift+S │
│  ├── 📊 System metrics              Ctrl+M       │
│  └── ⚙ Settings                     Ctrl+,       │
│                                                   │
│  Navigation                                       │
│  ├── Dashboard                       Ctrl+1       │
│  ├── Applications                    Ctrl+2       │
│  ├── Topology                        Ctrl+3       │
│  ├── Databases                       Ctrl+4       │
│  └── Marketplace                     Ctrl+5       │
└──────────────────────────────────────────────────┘
```

### 17.3 Multi-Panel Architecture

DeployMonster has **three distinct panels**, each with its own layout, navigation, and permission scope:

```
┌─────────────────────────────────────────────────────────────────┐
│                    DeployMonster Panels                          │
│                                                                  │
│  ┌──────────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │  SUPER ADMIN      │  │  TEAM ADMIN   │  │  CUSTOMER        │  │
│  │  /admin/*          │  │  /team/*      │  │  /app/*          │  │
│  │                    │  │              │  │                   │  │
│  │  Platform owner    │  │  Team lead   │  │  Individual       │  │
│  │  manages:          │  │  manages:    │  │  developer        │  │
│  │  • All tenants     │  │  • Own team  │  │  manages:         │  │
│  │  • All servers     │  │  • Members   │  │  • Own apps       │  │
│  │  • Billing/plans   │  │  • Projects  │  │  • Assigned       │  │
│  │  • VPS providers   │  │  • RBAC      │  │    projects       │  │
│  │  • Marketplace     │  │  • Secrets   │  │  • Deploy + logs  │  │
│  │  • System config   │  │  • Billing   │  │  • Domains        │  │
│  │  • Monitoring      │  │  • Limits    │  │  • Secrets        │  │
│  │  • Registration    │  │  • Webhooks  │  │  • Metrics        │  │
│  │  • Audit logs      │  │              │  │                   │  │
│  └──────────────────┘  └──────────────┘  └──────────────────┘  │
│                                                                  │
│  URL Routing:                                                    │
│  /admin/*  → Super Admin panel (platform-level)                  │
│  /team/*   → Team Admin panel (tenant-level management)          │
│  /app/*    → Customer panel (individual workspace)               │
│  /auth/*   → Auth pages (login, register, SSO, 2FA)             │
│  /public/* → Public pages (landing, pricing, docs)               │
└─────────────────────────────────────────────────────────────────┘
```

**Panel Switching:**
Users with multiple roles see a panel switcher in the top-left:
```
┌──────────────────────────────────────┐
│  🐲 DeployMonster  ▼                  │
│  ┌────────────────────────────────┐  │
│  │  👑 Super Admin    /admin      │  │
│  │  👥 Team: Acme Corp /team     │  │
│  │  💻 My Workspace   /app       │  │
│  └────────────────────────────────┘  │
└──────────────────────────────────────┘
```

### 17.4 Super Admin Panel (/admin/*)

The Super Admin panel is the **platform control center** — for the person who runs the DeployMonster instance.

```
Super Admin Panel (/admin/*)
─────────────────────────────
├── Dashboard
│   ├── Platform overview (total apps, users, servers, containers)
│   ├── Resource utilization heatmap (all servers)
│   ├── Revenue summary (MRR, active subscriptions, churn)
│   ├── Active alerts
│   └── Recent audit events
├── Tenants
│   ├── List / Search / Filter (plan, status, usage)
│   ├── Create tenant (with plan assignment)
│   ├── Tenant detail (usage, members, apps, invoices)
│   ├── Impersonate user (login as tenant member)
│   ├── Suspend / Activate tenant
│   └── Override resource quotas
├── Servers
│   ├── Cluster topology map (visual, all nodes)
│   ├── Server list (status, CPU, RAM, containers, role)
│   ├── Add server (from VPS provider or SSH)
│   ├── Server detail (metrics, containers, labels)
│   ├── Maintenance mode (drain → evacuate containers)
│   └── Remove / Destroy server
├── VPS Providers
│   ├── Connected providers (Hetzner, DO, Vultr, Linode, AWS)
│   ├── Add provider (API key + test connection)
│   ├── Provision new server (size/region picker)
│   ├── Connect existing server (SSH)
│   ├── Cost overview (monthly spend per provider)
│   └── Server inventory per provider
├── Billing & Plans
│   ├── Plan management (CRUD: Free, Starter, Pro, Enterprise)
│   ├── Overage pricing configuration
│   ├── Revenue dashboard (MRR chart, per-plan breakdown)
│   ├── Outstanding invoices
│   ├── Payment provider config (Stripe keys)
│   ├── Tax settings
│   └── Trial period settings
├── Marketplace Admin
│   ├── Template management (enable/disable/feature)
│   ├── Category management
│   ├── Community template review queue
│   ├── Template usage analytics
│   └── Force sync from registry
├── Registration & Auth
│   ├── Registration mode (open/invite/approval/disabled/SSO)
│   ├── SSO provider config (Google, GitHub, SAML)
│   ├── Invite management (create, revoke, track)
│   ├── Approval queue
│   ├── Allowed email domains
│   └── 2FA enforcement policy
├── Global Settings
│   ├── DNS providers (Cloudflare, Route53, etc.)
│   ├── Docker registries
│   ├── Backup storage targets (S3, SFTP, local)
│   ├── Email/SMTP configuration
│   ├── SSL defaults (ACME email, challenge type)
│   ├── Notification channels (Slack, Discord, Telegram)
│   └── System branding (logo, colors, custom domain)
├── Monitoring
│   ├── Server metrics (all nodes, stacked charts)
│   ├── Container metrics (top CPU/RAM consumers)
│   ├── Ingress analytics (requests/s, latency, errors)
│   ├── Build queue (running, queued, failed)
│   └── Audit log (every action, searchable, exportable)
└── System
    ├── DeployMonster version + update check
    ├── Database maintenance (vacuum, backup)
    ├── License management (if commercial features)
    └── Debug / Diagnostics
```

### 17.5 Team Admin Panel (/team/*)

For team leads and tenant admins who manage their organization's infrastructure.

```
Team Admin Panel (/team/*)
──────────────────────────
├── Dashboard
│   ├── Team overview (apps, members, resource usage vs plan)
│   ├── Recent deployments (who deployed what, when)
│   ├── Active alerts (for this team's resources)
│   └── Quick actions (deploy, add member, create project)
├── Team Members
│   ├── Member list (name, email, role, last active, 2FA status)
│   ├── Invite member (email + role assignment)
│   ├── Edit member role
│   ├── Remove member
│   ├── Pending invitations
│   └── Activity log per member
├── Roles & Permissions (RBAC)
│   ├── Built-in roles (Admin, Developer, Operator, Viewer)
│   ├── Custom roles (create with granular permissions)
│   ├── Role assignment matrix
│   └── Permission audit (who can do what)
├── Projects
│   ├── Project list (apps, environments, members)
│   ├── Create project
│   ├── Project settings (name, description, default env)
│   ├── Project members (subset of team, per-project access)
│   └── Project-level secrets
├── Applications (all team apps)
│   ├── Full app management (same as customer panel)
│   ├── Cross-project view
│   └── Bulk operations (restart all, update env across apps)
├── Secrets
│   ├── Team-level secrets (shared across projects)
│   ├── Project-level secrets
│   ├── Secret access audit (who viewed/changed what)
│   └── Rotation policies
├── Servers (if team has dedicated servers)
│   ├── Assigned servers
│   ├── Resource allocation per server
│   └── Server metrics
├── Billing
│   ├── Current plan + usage meters
│   ├── Team usage breakdown (per member, per project)
│   ├── Invoice history
│   ├── Payment methods (Stripe Customer Portal)
│   ├── Upgrade/downgrade plan
│   └── Cost forecast
├── Webhooks & Integrations
│   ├── Team-wide webhook management
│   ├── Notification preferences
│   └── API key management (team-scoped)
└── Settings
    ├── Team name, avatar, slug
    ├── Default deployment settings
    ├── Git source connections (team-wide)
    └── Danger zone (delete team)
```

### 17.6 Customer Panel (/app/*)

The individual developer workspace — focused, clean, fast.

```
Customer Panel (/app/*)
────────────────────────
├── Dashboard
│   ├── My apps overview (cards with status, domain, last deploy)
│   ├── Resource usage summary (visual bars vs limits)
│   ├── Recent deployments (timeline)
│   ├── Quick deploy button (→ Deploy Wizard)
│   └── Getting started guide (dismissable, for new users)
├── Projects
│   ├── Project list / Create
│   ├── Project detail (apps, environments, members)
│   └── Environment switching (staging / production)
├── Applications
│   ├── Deploy Wizard ★ (multi-source)
│   │   ├── Step 1: Source (Git / Docker Image / Compose / Marketplace / Upload)
│   │   ├── Step 2: Configure (env vars, port, resources)
│   │   ├── Step 3: Domain (auto-subdomain or custom)
│   │   ├── Step 4: Review & Deploy
│   │   └── Step 5: Live build log → success link
│   ├── App Detail Page
│   │   ├── Header: name, status, domain link, quick actions
│   │   ├── Tab: Overview (metrics mini-charts, recent deploys, resources)
│   │   ├── Tab: Deployments (history, build logs, rollback)
│   │   ├── Tab: Logs (real-time streaming, search, filter by level)
│   │   ├── Tab: Terminal (xterm.js exec into container)
│   │   ├── Tab: Environment (env vars + secrets editor, Monaco)
│   │   ├── Tab: Domains (custom domains, SSL status, routing)
│   │   ├── Tab: Metrics (CPU, RAM, requests, latency, errors)
│   │   ├── Tab: Volumes (mounted volumes, usage, backup)
│   │   ├── Tab: Webhooks (auto-deploy config, delivery logs)
│   │   └── Tab: Settings (rename, transfer project, danger zone)
│   └── App Actions: Restart | Stop | Start | Scale | Rollback | Delete
├── Compose Stacks
│   ├── Deploy compose stack (upload YAML / paste / from Git)
│   ├── Stack detail (service list, topology mini-view)
│   ├── Per-service: logs, exec, scale, restart
│   └── Edit compose → redeploy changed services
├── Databases
│   ├── Create managed DB (PG/MySQL/Redis/Mongo — one click)
│   ├── DB detail: connection string (click to copy), status, metrics
│   ├── Backups: list, create, restore
│   └── Logs
├── Domains
│   ├── Domain list with SSL status badge
│   ├── Add custom domain (with DNS verification instructions)
│   ├── Auto-subdomain management
│   └── DNS record viewer
├── Storage
│   ├── Volumes (list, size, mount info)
│   ├── Backup history per volume
│   └── Restore from backup
├── Secrets
│   ├── Secret vault (list with scope badges)
│   ├── Create / edit / delete secrets
│   ├── Bulk import (.env file)
│   ├── Version history per secret
│   ├── Diff view (staging vs production)
│   └── Copy secrets between environments
├── Marketplace ★
│   ├── Browse by category (grid with icons)
│   ├── Search & filter
│   ├── Template detail (screenshots, requirements, README)
│   ├── One-click deploy wizard (JSON Schema form)
│   ├── My installed apps (with update notifications)
│   └── Installed app settings
├── Git Sources
│   ├── Connected providers (GitHub 🟢, GitLab 🟢, etc.)
│   ├── Connect new provider (OAuth flow or token)
│   ├── Repository browser (search, select, deploy)
│   └── Deploy keys management
├── Servers (if allowed by plan/role)
│   ├── Server list + metrics
│   ├── Add server (VPS provider or SSH)
│   └── Server detail
├── Topology View ★
│   ├── Full drag & drop canvas (§17.8)
│   └── Everything is interconnected visually
├── Billing
│   ├── Current plan + usage meters (visual progress bars)
│   ├── Cost forecast (estimated next invoice)
│   ├── Invoice history (download PDF)
│   ├── Upgrade / Downgrade plan
│   └── Payment methods (→ Stripe Customer Portal)
└── Settings
    ├── Profile (name, email, avatar, password, 2FA)
    ├── API keys (create, revoke, scopes)
    ├── Notification preferences
    ├── SSH keys
    └── Sessions (active sessions, revoke)
```

### 17.7 RBAC & Team Management

DeployMonster implements a flexible RBAC system with built-in roles and custom role support.

**Role Hierarchy:**

```
┌─────────────────────────────────────────────────────────────┐
│  Platform Level (Super Admin only)                           │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  super_admin                                           │  │
│  │  └── Full platform access, all tenants, all servers    │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                              │
│  Tenant Level (per team/organization)                        │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  owner                                                 │  │
│  │  └── Full tenant access, billing, members, delete      │  │
│  │                                                         │  │
│  │  admin                                                 │  │
│  │  └── Manage members, projects, apps, secrets, servers  │  │
│  │                                                         │  │
│  │  developer                                             │  │
│  │  └── Deploy, manage apps, view logs, manage secrets    │  │
│  │                                                         │  │
│  │  operator                                              │  │
│  │  └── View apps, restart, view logs, view metrics       │  │
│  │      (no deploy, no config changes)                     │  │
│  │                                                         │  │
│  │  viewer                                                │  │
│  │  └── Read-only access to assigned projects              │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                              │
│  Project Level (optional fine-grained access)                │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  Users can have different roles per project:            │  │
│  │  • Admin on "Backend" project                          │  │
│  │  • Developer on "Frontend" project                     │  │
│  │  • Viewer on "Infrastructure" project                  │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**Permission Matrix:**

| Permission | super_admin | owner | admin | developer | operator | viewer |
|-----------|:-----------:|:-----:|:-----:|:---------:|:--------:|:------:|
| View apps & logs | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| View metrics | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Restart/stop app | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| Deploy app | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Create app | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Delete app | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| Manage env vars | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Manage secrets | ✅ | ✅ | ✅ | ✅* | ❌ | ❌ |
| Manage domains | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Manage databases | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Exec into container | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Create project | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| Manage team members | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| Manage roles | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| View billing | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| Manage billing | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Add/remove servers | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| Delete tenant | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Manage VPS providers | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Manage all tenants | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| System settings | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Impersonate user | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |

*developer can manage app-level secrets but not project/tenant-level secrets

**Custom Roles:**

Team admins can create custom roles with granular permissions:
```
┌─────────────────────────────────────────────────────────────┐
│  Create Custom Role                                          │
│                                                              │
│  Name: [QA Engineer                          ]              │
│  Description: [Can deploy to staging, view production    ]   │
│                                                              │
│  Permissions:                                                │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ Applications                                         │    │
│  │  ☑ View applications                                 │    │
│  │  ☑ View logs                                         │    │
│  │  ☑ Deploy to staging environment                     │    │
│  │  ☐ Deploy to production environment                  │    │
│  │  ☑ Restart applications                              │    │
│  │  ☐ Delete applications                               │    │
│  │  ☑ Exec into containers (staging only)               │    │
│  ├─────────────────────────────────────────────────────┤    │
│  │ Secrets                                               │    │
│  │  ☑ View secret names (values masked)                  │    │
│  │  ☑ Manage staging secrets                             │    │
│  │  ☐ Manage production secrets                          │    │
│  ├─────────────────────────────────────────────────────┤    │
│  │ Team                                                  │    │
│  │  ☐ Manage members                                     │    │
│  │  ☐ View billing                                       │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│  [Cancel]  [Create Role]                                     │
└─────────────────────────────────────────────────────────────┘
```

**Team Management Data Model:**

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│    Team      │────<│  TeamMember  │     │     Role     │
│ (= Tenant)   │     │──────────────│     │──────────────│
│──────────────│     │ id           │     │ id           │
│ id           │     │ team_id      │     │ team_id      │
│ name         │     │ user_id      │     │ name         │
│ slug         │     │ role_id      │     │ description  │
│ avatar_url   │     │ invited_by   │     │ permissions  │
│ plan_id      │     │ invited_at   │     │ is_builtin   │
│ owner_id     │     │ accepted_at  │     │ created_at   │
│ status       │     │ status       │     │              │
│ created_at   │     │ last_active  │     │              │
└──────────────┘     └──────────────┘     └──────────────┘

┌──────────────┐     ┌──────────────┐
│  Invitation  │     │ProjectMember │
│──────────────│     │──────────────│
│ id           │     │ id           │
│ team_id      │     │ project_id   │
│ email        │     │ user_id      │
│ role_id      │     │ role_id      │
│ invited_by   │     │ added_by     │
│ token_hash   │     │ created_at   │
│ expires_at   │     │              │
│ accepted_at  │     │ (overrides   │
│ status       │     │  team role   │
└──────────────┘     │  per project)│
                     └──────────────┘
```

**Team Management Features:**
- **Invite by email** — Send invite with role pre-assigned, 7-day expiry
- **Bulk invite** — Paste list of emails, all get same role
- **SSO auto-join** — Users from allowed domain auto-join team on first login
- **Activity feed** — Who deployed what, when, from which commit
- **Last active** — See when each member last logged in
- **2FA enforcement** — Require 2FA for all team members (admin toggle)
- **Session management** — See active sessions, force logout
- **Audit trail** — Per-member action history (deploy, config change, secret access)
- **Transfer ownership** — Owner can transfer team to another member
- **Leave team** — Members can leave, owner cannot (must transfer first)

### 17.8 Topology View (Drag & Drop)

The crown jewel of DeployMonster UI — a visual infrastructure canvas powered by React Flow.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ ┌──────────┐  Topology Canvas                              ┌─────────────┐ │
│ │ Palette  │                                                │  Properties │ │
│ │──────────│  ┌─────────┐    ┌──────────┐    ┌──────────┐  │─────────────│ │
│ │          │  │🌐 Globe │───►│📡 Domain │───►│⚛ Next.js│  │ Next.js App │ │
│ │ SOURCES  │  │ Internet│    │ api.com  │    │ :3000    │  │             │ │
│ │ ┌──────┐ │  └─────────┘    │ 🔒 SSL   │    │ 2 repl.  │  │ Image: ...  │ │
│ │ │🌐 Web│ │                 └──────────┘    └────┬─────┘  │ CPU: 0.5    │ │
│ │ └──────┘ │                                      │         │ RAM: 512MB  │ │
│ │ ┌──────┐ │                                      ▼         │ Env vars: 8 │ │
│ │ │📡 DNS│ │                               ┌──────────┐    │ Port: 3000  │ │
│ │ └──────┘ │  ┌─────────┐    ┌──────────┐  │🐘 Postgr│    │ Status: 🟢  │ │
│ │          │  │🌐 Globe │───►│📡 Domain │  │ :5432    │    │             │ │
│ │ SERVICES │  │ Internet│    │ app.com  │  │ 16GB     │    │ [Logs]      │ │
│ │ ┌──────┐ │  └─────────┘    └─────┬────┘  └────┬─────┘    │ [Terminal]  │ │
│ │ │⚛ App│ │                        │            │          │ [Metrics]   │ │
│ │ └──────┘ │                        ▼            ▼          │ [Restart]   │ │
│ │ ┌──────┐ │                 ┌──────────┐  ┌──────────┐    │ [Scale]     │ │
│ │ │🔧Work│ │                 │⚛ React  │  │🔴 Redis  │    │ [Rollback]  │ │
│ │ └──────┘ │                 │ SPA :80  │  │ :6379    │    │             │ │
│ │ ┌──────┐ │                 │ nginx    │  │ 256MB    │    │ [Edit Env]  │ │
│ │ │⏰Cron│ │                 └──────────┘  └──────────┘    │ [Domain]    │ │
│ │ └──────┘ │                                                │ [Secrets]   │ │
│ │          │                 ┌──────────┐                   │             │ │
│ │ DATA     │                 │📦 Volume │                   │             │ │
│ │ ┌──────┐ │                 │ uploads  │                   │             │ │
│ │ │🐘 PG │ │                 │ 10GB SSD │                   │             │ │
│ │ └──────┘ │                 └──────────┘                   │             │ │
│ │ ┌──────┐ │                                                │             │ │
│ │ │🔴Redis│ │  ┌──────────────────────────────────────────┐ │             │ │
│ │ └──────┘ │  │ 🗺 Minimap                                │ │             │ │
│ │ ┌──────┐ │  └──────────────────────────────────────────┘ │             │ │
│ │ │🍃Mongo│ │                                               │             │ │
│ │ └──────┘ │  [Zoom +/-] [Fit] [Export] [Import] [Snap]    │             │ │
│ │ ┌──────┐ │                                                │             │ │
│ │ │📦 Vol│ │                                                │             │ │
│ │ └──────┘ │                                                │             │ │
│ └──────────┘                                                └─────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Node Types (Palette):**

| Node | Icon | Category | Drag Action |
|------|------|----------|-------------|
| Internet Globe | 🌐 | Source | Creates external entry point |
| Domain | 📡 | Source | Prompts domain config (subdomain/custom) |
| Application | ⚛ | Service | Opens deploy wizard (Git/Image/Marketplace) |
| Worker | 🔧 | Service | Background process, no ingress |
| Cron Job | ⏰ | Service | Scheduled task with cron expression |
| PostgreSQL | 🐘 | Data | One-click PG provisioning |
| MySQL | 🐬 | Data | One-click MySQL provisioning |
| Redis | 🔴 | Data | One-click Redis provisioning |
| MongoDB | 🍃 | Data | One-click Mongo provisioning |
| Volume | 📦 | Storage | Persistent volume creation |
| S3 Bucket | 🪣 | Storage | External S3/MinIO connection |
| Load Balancer | ⚖️ | Network | LB config between replicas |

**Connection Types (Drag Line Between Nodes):**

| From → To | Connection Type | Auto-Generated |
|-----------|----------------|----------------|
| Globe → Domain | Ingress route | DNS record + SSL cert |
| Domain → App | HTTP routing | `monster.http.routers` labels |
| App → Database | DB connection | `DATABASE_URL` env var |
| App → Redis | Cache connection | `REDIS_URL` env var |
| App → App | Service mesh | Internal DNS + env var |
| App → Volume | Mount | Docker volume mount path |
| App → S3 | Storage | `S3_ENDPOINT`, `S3_BUCKET` env vars |
| LB → App (×N) | Load balanced | LB strategy config |
| Worker → Database | DB connection | `DATABASE_URL` env var |

**When you draw a line (App → PostgreSQL):**
```
1. Connection dialog opens:
   ┌────────────────────────────────────┐
   │  Connect: Next.js → PostgreSQL     │
   │                                    │
   │  Database: myapp_db                │
   │  Username: myapp_user              │
   │  Password: ••••••••  [Generate]    │
   │                                    │
   │  Inject as ENV:                    │
   │  ☑ DATABASE_URL                    │
   │  ☑ PGHOST                          │
   │  ☑ PGPORT                          │
   │  ☑ PGUSER                          │
   │  ☑ PGPASSWORD                      │
   │  ☑ PGDATABASE                      │
   │                                    │
   │  [Cancel]  [Connect & Deploy]      │
   └────────────────────────────────────┘

2. Secrets auto-created in vault
3. ENV vars injected into app container
4. App redeployed with new env vars
5. Connection line turns green (verified)
```

**When you drag Domain → App:**
```
1. Domain assignment dialog:
   ┌────────────────────────────────────┐
   │  Route: api.example.com → Next.js  │
   │                                    │
   │  ○ Auto-subdomain: app.deploy.monster │
   │  ● Custom domain: api.example.com  │
   │                                    │
   │  Port: [3000]                      │
   │  Path prefix: [/]                  │
   │  SSL: ☑ Auto (Let's Encrypt)       │
   │  Force HTTPS: ☑                    │
   │                                    │
   │  Middleware:                        │
   │  ☑ Rate limit (100 req/s)          │
   │  ☑ CORS (allow: *)                 │
   │  ☑ Gzip compression               │
   │  ☐ Basic auth                      │
   │  ☐ IP whitelist                    │
   │                                    │
   │  [Cancel]  [Apply Route]           │
   └────────────────────────────────────┘

2. Ingress route created
3. DNS record synced (if provider connected)
4. SSL certificate requested
5. Domain node turns green (live + SSL active)
```

**Node Context Menu (Right Click):**
```
┌──────────────────┐
│ 📋 View Logs     │
│ 💻 Open Terminal │
│ 📊 Metrics       │
│ ─────────────── │
│ ⚙ Settings      │
│ 🔑 Env Vars     │
│ 🔒 Secrets      │
│ 📡 Domains      │
│ ─────────────── │
│ ↕ Scale (1→N)   │
│ 🔄 Restart      │
│ ⏪ Rollback     │
│ 🗑 Delete       │
│ ─────────────── │
│ 📤 Export YAML   │
│ 🔗 Copy conn str │
└──────────────────┘
```

**Node Status Indicators:**
```
🟢 Green  = Running, healthy
🟡 Yellow = Starting, deploying, or degraded
🔴 Red    = Stopped, crashed, or unhealthy
🔵 Blue   = Building
⚪ Gray   = Not deployed (placeholder)
🟠 Orange = Scaling in progress
```

**Advanced Canvas Features:**
- **Multi-select** — Shift+click multiple nodes, bulk operations
- **Group** — Select nodes → right-click → "Group as Stack" → creates visual boundary
- **Snap to grid** — Optional grid alignment for clean layouts
- **Auto-layout** — Dagre/ELK algorithm for automatic positioning
- **Minimap** — Bottom-left overview for large topologies
- **Zoom levels** — Scroll to zoom, double-click to focus
- **Search** — Ctrl+F to find nodes by name
- **Undo/Redo** — Full action history (Ctrl+Z / Ctrl+Y)
- **Real-time sync** — See container status changes live (green→red on crash)
- **Connection health** — Lines pulse green (healthy) or turn red (broken)
- **Resource badges** — CPU/RAM mini-bars on each node
- **Traffic flow** — Animated dots flowing along connection lines (optional)
- **Import** — Paste docker-compose.yml → auto-generate topology
- **Export** — Export as docker-compose.yml, SVG image, or JSON
- **Templates** — Save topology as reusable template
- **Diff view** — Compare current vs previous topology state

### 17.9 Metrics & Monitoring UI

**Server Dashboard:**
```
┌─────────────────────────────────────────────────────────────┐
│  Server: prod-01 (Hetzner CX42)           Status: 🟢 Online │
│                                                              │
│  ┌──────────────────────┐  ┌──────────────────────┐        │
│  │ CPU Usage        82% │  │ RAM Usage        64% │        │
│  │ ████████████░░░░     │  │ ██████████░░░░░░     │        │
│  │ 4/4 cores            │  │ 10.2/16 GB           │        │
│  └──────────────────────┘  └──────────────────────┘        │
│                                                              │
│  ┌──────────────────────┐  ┌──────────────────────┐        │
│  │ Disk Usage       71% │  │ Network        ↑↓    │        │
│  │ █████████████░░░     │  │ In:  42 MB/s         │        │
│  │ 142/200 GB           │  │ Out: 128 MB/s        │        │
│  └──────────────────────┘  └──────────────────────┘        │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ CPU History (24h)                            Recharts │   │
│  │  100%│    ╱╲      ╱╲╱╲                               │   │
│  │   75%│   ╱  ╲    ╱    ╲    ╱╲                        │   │
│  │   50%│──╱────╲──╱──────╲──╱──╲──────────────         │   │
│  │   25%│ ╱      ╲╱        ╲╱    ╲╱                     │   │
│  │    0%│─────────────────────────────────────           │   │
│  │      00:00   06:00   12:00   18:00   24:00           │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  Container Breakdown:                                        │
│  ┌──────────┬──────┬───────┬──────────┬──────────┐         │
│  │ Name     │ CPU  │ RAM   │ Network  │ Status   │         │
│  ├──────────┼──────┼───────┼──────────┼──────────┤         │
│  │ nextjs   │ 32%  │ 512MB │ 45MB/s   │ 🟢 Run  │         │
│  │ postgres │ 18%  │ 2.1GB │ 12MB/s   │ 🟢 Run  │         │
│  │ redis    │ 2%   │ 64MB  │ 8MB/s    │ 🟢 Run  │         │
│  │ worker   │ 45%  │ 256MB │ 2MB/s    │ 🟡 High │         │
│  └──────────┴──────┴───────┴──────────┴──────────┘         │
└─────────────────────────────────────────────────────────────┘
```

**App-Level Metrics:**
```
┌─────────────────────────────────────────────────────────────┐
│  Application: my-nextjs-app                                  │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ Response Time (p50/p95/p99)              Recharts   │    │
│  │  200ms│         p99                                  │    │
│  │  150ms│    p95 ╱╲                                    │    │
│  │  100ms│───────╱──╲─────────────────────              │    │
│  │   50ms│─p50──╱────╲────────────────────              │    │
│  │    0ms│─────────────────────────────────             │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Requests/min │  │ Error Rate   │  │ Uptime       │      │
│  │    1,247     │  │   0.12%      │  │  99.97%      │      │
│  │   ↑ 12%      │  │   ↓ 0.03%   │  │  (30 days)   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ Resource Usage vs Limits                             │    │
│  │                                                       │    │
│  │ CPU:  ████████░░░░  0.8 / 1.0 cores     [Edit]      │    │
│  │ RAM:  ██████░░░░░░  384 / 512 MB        [Edit]      │    │
│  │ Disk: ████░░░░░░░░  2.1 / 5.0 GB        [Edit]      │    │
│  │ Net:  ██░░░░░░░░░░  12 / 100 GB/mo      [View]      │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│  Deployment History:                                         │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ v12 │ 2m ago  │ abc123 │ Fix auth bug    │ 🟢 Live  │   │
│  │ v11 │ 2h ago  │ def456 │ Add caching     │ ⬛ Prev  │   │
│  │ v10 │ 1d ago  │ ghi789 │ Update deps     │ ⬛ Prev  │   │
│  │     │         │        │                 │ [Rollback]│   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Tenant Usage Dashboard (Customer):**
```
┌─────────────────────────────────────────────────────────────┐
│  Usage Overview — March 2026              Plan: Pro ($29.99) │
│                                                              │
│  ┌──────────────────────────────────┐                       │
│  │ Estimated Invoice: $47.20        │  [Upgrade Plan]       │
│  │ (base $29.99 + overage $17.21)   │                       │
│  └──────────────────────────────────┘                       │
│                                                              │
│  Resource Usage vs Limits:                                   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Apps:       ████████████████░░░░  38 / 50            │   │
│  │ CPU:        ██████████████░░░░░░  5.6 / 8 cores      │   │
│  │ RAM:        ████████████░░░░░░░░  11.2 / 16 GB       │   │
│  │ Storage:    ████████░░░░░░░░░░░░  42 / 100 GB        │   │
│  │ Bandwidth:  ████████████████████  512 / 500 GB ⚠ Over│   │
│  │ Builds:     ██████████░░░░░░░░░░  1,050 / 2,000 min  │   │
│  │ Databases:  ████████░░░░░░░░░░░░  4 / 10             │   │
│  │ Domains:    ████░░░░░░░░░░░░░░░░  6 / 25             │   │
│  │ Servers:    ██░░░░░░░░░░░░░░░░░░  2 / 10             │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Cost Breakdown (pie chart)                 Recharts   │   │
│  │                                                       │   │
│  │   Base plan: $29.99 (63%)                             │   │
│  │   Extra bandwidth: $12.00 (25%)                       │   │
│  │   Extra DB (Redis): $5.00 (11%)                       │   │
│  │   Extra CPU: $0.21 (1%)                               │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## 18. REST API

### 18.1 API Design

- **Base**: `/api/v1`
- **Auth**: `Authorization: Bearer <jwt>` or `X-API-Key: <key>`
- **Format**: JSON request/response
- **Pagination**: `?page=1&per_page=20`
- **Filtering**: `?status=running&project_id=xxx`
- **Sorting**: `?sort=created_at&order=desc`
- **Rate Limit**: 100 req/min per API key

### 18.2 Endpoint Map

```
Authentication
  POST   /api/v1/auth/login              # Login, get JWT
  POST   /api/v1/auth/register           # Register (if enabled)
  POST   /api/v1/auth/refresh            # Refresh JWT
  DELETE /api/v1/auth/logout             # Invalidate token

Projects
  GET    /api/v1/projects                # List projects
  POST   /api/v1/projects                # Create project
  GET    /api/v1/projects/:id            # Get project
  PATCH  /api/v1/projects/:id            # Update project
  DELETE /api/v1/projects/:id            # Delete project

Applications
  GET    /api/v1/projects/:id/apps       # List apps in project
  POST   /api/v1/projects/:id/apps       # Create app
  GET    /api/v1/apps/:id                # Get app details
  PATCH  /api/v1/apps/:id                # Update app config
  DELETE /api/v1/apps/:id                # Delete app
  POST   /api/v1/apps/:id/deploy         # Trigger deployment
  POST   /api/v1/apps/:id/rollback       # Rollback to version
  POST   /api/v1/apps/:id/restart        # Restart app
  POST   /api/v1/apps/:id/stop           # Stop app
  POST   /api/v1/apps/:id/start          # Start app
  GET    /api/v1/apps/:id/logs           # Get logs (paginated)
  GET    /api/v1/apps/:id/metrics        # Get metrics
  GET    /api/v1/apps/:id/deployments    # Deployment history
  GET    /api/v1/apps/:id/env            # Get env vars
  PUT    /api/v1/apps/:id/env            # Set env vars

Domains
  GET    /api/v1/domains                 # List all domains
  POST   /api/v1/domains                 # Add domain
  GET    /api/v1/domains/:id             # Get domain
  DELETE /api/v1/domains/:id             # Remove domain
  POST   /api/v1/domains/:id/verify      # Verify DNS
  POST   /api/v1/domains/:id/ssl         # Request SSL cert

Databases
  GET    /api/v1/databases               # List managed DBs
  POST   /api/v1/databases               # Create managed DB
  GET    /api/v1/databases/:id           # Get DB details
  DELETE /api/v1/databases/:id           # Delete managed DB
  POST   /api/v1/databases/:id/backup    # Trigger backup
  GET    /api/v1/databases/:id/backups   # List backups
  POST   /api/v1/databases/:id/restore   # Restore from backup

Servers
  GET    /api/v1/servers                 # List servers
  POST   /api/v1/servers                 # Add server (agent token)
  GET    /api/v1/servers/:id             # Get server details
  DELETE /api/v1/servers/:id             # Remove server
  GET    /api/v1/servers/:id/metrics     # Server metrics

Backups
  GET    /api/v1/backups                 # List all backups
  POST   /api/v1/backups                 # Create backup
  GET    /api/v1/backups/:id             # Get backup details
  POST   /api/v1/backups/:id/restore     # Restore backup
  DELETE /api/v1/backups/:id             # Delete backup

Admin (admin role only)
  GET    /api/v1/admin/tenants           # List tenants
  POST   /api/v1/admin/tenants           # Create tenant
  PATCH  /api/v1/admin/tenants/:id       # Update tenant
  GET    /api/v1/admin/system            # System info
  POST   /api/v1/admin/settings          # Update settings
  GET    /api/v1/admin/audit-log         # Audit trail

VPS Providers
  GET    /api/v1/vps/providers           # List configured providers
  POST   /api/v1/vps/providers           # Add provider (API key)
  GET    /api/v1/vps/providers/:id       # Get provider details
  PATCH  /api/v1/vps/providers/:id       # Update provider
  DELETE /api/v1/vps/providers/:id       # Remove provider
  GET    /api/v1/vps/providers/:id/sizes # List available sizes
  GET    /api/v1/vps/providers/:id/regions # List regions
  GET    /api/v1/vps/providers/:id/images  # List OS images
  POST   /api/v1/vps/servers             # Provision new server
  GET    /api/v1/vps/servers             # List remote servers
  GET    /api/v1/vps/servers/:id         # Server details
  DELETE /api/v1/vps/servers/:id         # Destroy/disconnect server
  POST   /api/v1/vps/servers/:id/resize  # Resize server
  POST   /api/v1/vps/servers/:id/snapshot # Create snapshot
  POST   /api/v1/vps/servers/connect     # Connect existing via SSH

Git Sources
  GET    /api/v1/git/sources             # List connected providers
  POST   /api/v1/git/sources             # Add git source
  GET    /api/v1/git/sources/:id         # Get source details
  DELETE /api/v1/git/sources/:id         # Remove git source
  GET    /api/v1/git/sources/:id/repos   # List repositories
  GET    /api/v1/git/sources/:id/repos/:repo/branches  # List branches
  POST   /api/v1/git/oauth/callback/:provider  # OAuth callback

Webhooks
  GET    /api/v1/webhooks                # List webhooks
  POST   /api/v1/webhooks                # Create webhook
  GET    /api/v1/webhooks/:id            # Get webhook details
  DELETE /api/v1/webhooks/:id            # Remove webhook
  GET    /api/v1/webhooks/:id/logs       # Webhook delivery logs
  POST   /api/v1/webhooks/:id/redeliver/:log_id  # Redeliver

Compose Stacks
  GET    /api/v1/stacks                  # List compose stacks
  POST   /api/v1/stacks                  # Deploy compose stack
  GET    /api/v1/stacks/:id              # Stack details + services
  PATCH  /api/v1/stacks/:id              # Update compose YAML
  DELETE /api/v1/stacks/:id              # Destroy stack
  POST   /api/v1/stacks/:id/redeploy     # Redeploy stack
  GET    /api/v1/stacks/:id/services     # List stack services
  POST   /api/v1/stacks/:id/services/:svc/scale  # Scale service
  POST   /api/v1/stacks/validate         # Validate compose YAML

Marketplace
  GET    /api/v1/marketplace             # Browse templates
  GET    /api/v1/marketplace/categories  # List categories
  GET    /api/v1/marketplace/search      # Search templates
  GET    /api/v1/marketplace/:slug       # Template details + schema
  POST   /api/v1/marketplace/:slug/deploy # Deploy template
  GET    /api/v1/marketplace/installed    # My installed apps
  POST   /api/v1/marketplace/sync        # Force template sync

Secrets
  GET    /api/v1/secrets                  # List secrets (names only, values masked)
  POST   /api/v1/secrets                  # Create secret
  GET    /api/v1/secrets/:id              # Get secret metadata (value masked)
  PATCH  /api/v1/secrets/:id              # Update secret value (creates version)
  DELETE /api/v1/secrets/:id              # Delete secret
  GET    /api/v1/secrets/:id/versions     # List secret versions
  POST   /api/v1/secrets/:id/rollback     # Rollback to previous version
  POST   /api/v1/secrets/import           # Import from .env file
  GET    /api/v1/secrets/export           # Export as encrypted JSON
  GET    /api/v1/apps/:id/secrets         # List secrets referenced by app
  PUT    /api/v1/apps/:id/secrets         # Bulk set app secrets

Billing (Customer)
  GET    /api/v1/billing/plan             # Current plan + limits
  POST   /api/v1/billing/plan             # Change plan (upgrade/downgrade)
  GET    /api/v1/billing/usage            # Current period usage meters
  GET    /api/v1/billing/usage/history    # Historical usage data
  GET    /api/v1/billing/invoices         # Invoice list
  GET    /api/v1/billing/invoices/:id     # Invoice detail + PDF
  GET    /api/v1/billing/invoices/:id/pdf # Download invoice PDF
  GET    /api/v1/billing/forecast         # Estimated next invoice
  POST   /api/v1/billing/portal           # Get Stripe Customer Portal URL

Billing Admin
  GET    /api/v1/admin/billing/plans      # List all plans
  POST   /api/v1/admin/billing/plans      # Create plan
  PATCH  /api/v1/admin/billing/plans/:id  # Update plan
  GET    /api/v1/admin/billing/revenue    # Revenue dashboard data
  GET    /api/v1/admin/billing/mrr        # Monthly recurring revenue
  PATCH  /api/v1/admin/tenants/:id/plan   # Override tenant plan/limits
  PATCH  /api/v1/admin/tenants/:id/quotas # Override resource quotas

Team Management
  GET    /api/v1/team                    # Current team details
  PATCH  /api/v1/team                    # Update team settings
  GET    /api/v1/team/members            # List members
  POST   /api/v1/team/members/invite     # Send invite (email + role)
  POST   /api/v1/team/members/invite/bulk # Bulk invite (emails list)
  DELETE /api/v1/team/members/:id        # Remove member
  PATCH  /api/v1/team/members/:id/role   # Change member role
  GET    /api/v1/team/members/:id/activity # Member activity log
  GET    /api/v1/team/invitations        # List pending invitations
  DELETE /api/v1/team/invitations/:id    # Revoke invitation
  POST   /api/v1/team/invitations/:token/accept # Accept invite

Roles & Permissions
  GET    /api/v1/team/roles              # List roles (built-in + custom)
  POST   /api/v1/team/roles              # Create custom role
  PATCH  /api/v1/team/roles/:id          # Update custom role
  DELETE /api/v1/team/roles/:id          # Delete custom role (if not in use)
  GET    /api/v1/team/permissions        # List all available permissions

Project Members
  GET    /api/v1/projects/:id/members    # List project members
  POST   /api/v1/projects/:id/members    # Add member to project (with role)
  PATCH  /api/v1/projects/:id/members/:uid # Change project-level role
  DELETE /api/v1/projects/:id/members/:uid # Remove from project
```

### 18.3 WebSocket Endpoints

```
/ws/v1/apps/:id/logs          # Real-time log streaming
/ws/v1/apps/:id/exec          # Container exec terminal
/ws/v1/apps/:id/metrics       # Live metrics stream
/ws/v1/deployments/:id/build  # Build output streaming
/ws/v1/events                 # Global event stream
/ws/v1/servers/:id/metrics    # Remote server live metrics
```

### 18.4 Webhook Receiver Endpoints (Public)

```
# Universal webhook receiver (no auth required, signature verified)
POST /hooks/v1/{webhook_id}                # Auto-detect provider from headers
POST /hooks/v1/{webhook_id}/github         # GitHub-specific parser
POST /hooks/v1/{webhook_id}/gitlab         # GitLab-specific parser
POST /hooks/v1/{webhook_id}/bitbucket      # Bitbucket-specific parser
POST /hooks/v1/{webhook_id}/gitea          # Gitea-specific parser
POST /hooks/v1/{webhook_id}/gogs           # Gogs-specific parser
POST /hooks/v1/{webhook_id}/azure          # Azure DevOps-specific parser
POST /hooks/v1/{webhook_id}/generic        # Generic JSON payload

# Git OAuth callbacks
GET  /auth/callback/github                 # GitHub OAuth callback
GET  /auth/callback/gitlab                 # GitLab OAuth callback
GET  /auth/callback/bitbucket              # Bitbucket OAuth callback
GET  /auth/callback/azure                  # Azure DevOps OAuth callback
```

---

## 19. MCP SERVER

DeployMonster exposes an MCP (Model Context Protocol) server for AI-driven infrastructure management.

### 19.1 MCP Tools

| Tool | Description |
|------|-------------|
| `deploy_app` | Deploy an application from Git URL, image, or compose |
| `deploy_image` | Deploy a Docker image directly |
| `deploy_compose` | Deploy a docker-compose.yml stack |
| `deploy_marketplace` | Deploy a marketplace template |
| `list_apps` | List all running applications |
| `get_app_status` | Get detailed app status and metrics |
| `scale_app` | Scale app instances up/down |
| `view_logs` | View application logs |
| `create_database` | Provision a managed database |
| `add_domain` | Add custom domain to an app |
| `create_backup` | Trigger a backup |
| `get_system_metrics` | Get cluster resource usage |
| `rollback_app` | Rollback to previous version |
| `provision_server` | Create a new VPS via cloud provider |
| `list_servers` | List all connected servers |
| `connect_server` | Connect existing server via SSH |
| `list_git_sources` | List connected Git providers |
| `browse_repos` | Browse repositories from a Git source |
| `marketplace_search` | Search marketplace templates |
| `marketplace_deploy` | Deploy a marketplace app with config |

### 19.2 MCP Resources

| Resource | URI | Description |
|----------|-----|-------------|
| Apps | `monster://apps` | List of all applications |
| App Detail | `monster://apps/{id}` | Single app details |
| Servers | `monster://servers` | Cluster server list |
| Metrics | `monster://metrics/{server_id}` | Server metrics |
| Logs | `monster://logs/{app_id}` | Recent app logs |

---

## 20. SECURITY

### 20.1 Authentication Flow

```
                    ┌──────────┐
                    │  Login   │
                    └────┬─────┘
                         ▼
                  ┌──────────────┐
                  │ Verify Creds │
                  └──────┬───────┘
                         ▼
              ┌─────────────────────┐
              │ Generate JWT Pair   │
              │ Access (15m) +      │
              │ Refresh (7d)        │
              └──────────┬──────────┘
                         ▼
              ┌─────────────────────┐
              │ Set Refresh in      │
              │ HttpOnly Cookie     │
              └──────────┬──────────┘
                         ▼
              ┌─────────────────────┐
              │ Return Access Token │
              └─────────────────────┘
```

### 20.2 RBAC Roles

> Full RBAC details, permission matrix, and custom roles are defined in §17.7.

| Role | Scope | Level |
|------|-------|-------|
| `super_admin` | Platform | Full platform access — all tenants, servers, billing, settings |
| `owner` | Tenant | Full tenant control — billing, members, delete |
| `admin` | Tenant | Manage members, projects, apps, secrets, servers |
| `developer` | Tenant/Project | Deploy, manage apps, env vars, secrets (app-level), domains |
| `operator` | Tenant/Project | Restart, view logs/metrics — no deploy, no config changes |
| `viewer` | Tenant/Project | Read-only access to assigned projects |
| `custom` | Tenant/Project | Admin-defined granular permissions |

**Project-Level Override:** Users can have different roles per project (e.g., admin on "Backend", viewer on "Infrastructure").

### 20.3 Security Features

- **Encrypted secrets** — AES-256-GCM encryption at rest
- **Audit logging** — Every action logged with user, IP, timestamp
- **Rate limiting** — Per-IP and per-API-key
- **CORS** — Configurable per domain
- **CSP** — Content Security Policy headers
- **2FA** — TOTP-based two-factor authentication
- **IP Whitelist** — Restrict admin access by IP
- **Automatic lockout** — 5 failed logins → 15min lockout

### 20.4 Secret Management (Vault)

DeployMonster includes a built-in secret management system — no external Vault needed.

```
┌──────────────────────────────────────────────────────────┐
│                    Secret Vault                           │
│                                                           │
│  ┌────────────────────┐  ┌─────────────────────────────┐ │
│  │  Master Key         │  │  Secret Store (SQLite)      │ │
│  │  (derived from      │  │  ┌──────────────────────┐  │ │
│  │   MONSTER_SECRET    │  │  │ AES-256-GCM encrypted │  │ │
│  │   via Argon2id)     │  │  │ key-value pairs       │  │ │
│  └────────────────────┘  │  └──────────────────────┘  │ │
│                           │  ┌──────────────────────┐  │ │
│                           │  │ Version history       │  │ │
│                           │  │ (last 10 values)      │  │ │
│                           │  └──────────────────────┘  │ │
│                           └─────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

**Secret Types:**

| Type | Example | Storage |
|------|---------|---------|
| `env_var` | `DATABASE_URL=postgres://...` | Encrypted in SQLite |
| `file` | TLS cert, SSH key, .env file | Encrypted blob in SQLite |
| `docker_registry` | Registry auth credentials | Encrypted JSON |
| `dns_token` | Cloudflare API token | Encrypted string |
| `vps_token` | Hetzner/DO API key | Encrypted string |
| `git_token` | GitHub PAT, SSH deploy key | Encrypted string |
| `backup_creds` | S3 access key + secret | Encrypted JSON |
| `smtp_password` | Email SMTP password | Encrypted string |

**Secret Scoping:**

```
Global Secrets (admin only)
  └── Tenant Secrets
        └── Project Secrets
              └── Application Secrets
                    └── Environment Overrides (staging/production)
```

**Secret Resolution at Deploy Time:**
```
Container ENV: DATABASE_URL=${SECRET:project/db_url}
                    │
                    ▼
        ┌──────────────────┐
        │  Resolve Scope   │ ← App → Project → Tenant → Global
        └────────┬─────────┘
                 ▼
        ┌──────────────────┐
        │  Decrypt Value   │ ← AES-256-GCM with master key
        └────────┬─────────┘
                 ▼
        ┌──────────────────┐
        │  Inject into     │ ← Docker container env or mounted file
        │  Container       │
        └──────────────────┘
```

**Secret Features:**
- **Never in logs** — Secrets are masked in build logs, deploy logs, UI
- **Version history** — Last 10 values stored, rollback possible
- **Rotation** — Change secret → auto-redeploy affected containers
- **Shared secrets** — Share across apps within a project (e.g., DB password)
- **Import/Export** — `.env` file import, encrypted JSON export
- **Reference syntax** — `${SECRET:name}`, `${SECRET:project/name}`, `${SECRET:global/name}`
- **Bulk edit** — Edit all env vars in a textarea/code editor
- **Diff view** — Compare env vars between environments (staging vs production)
- **Copy between envs** — Copy all secrets from staging → production with one click

**Secret Data Model:**

```
┌──────────────┐     ┌──────────────┐
│    Secret    │────<│SecretVersion │
│──────────────│     │──────────────│
│ id           │     │ id           │
│ tenant_id    │     │ secret_id    │
│ project_id   │     │ value_enc    │
│ app_id       │     │ version      │
│ name         │     │ created_by   │
│ type         │     │ created_at   │
│ description  │     │              │
│ scope        │     │              │
│ current_ver  │     │              │
│ referenced_by│     │ (app IDs)    │
│ created_at   │     │              │
│ updated_at   │     │              │
└──────────────┘     └──────────────┘
```

### 20.5 Registration & Onboarding

Admin controls how customers access the platform:

**Registration Modes:**

| Mode | Description | Config |
|------|-------------|--------|
| `open` | Anyone can register | Public signup form |
| `invite_only` | Admin sends invite links | Email invite with expiring token |
| `approval` | User registers, admin approves | Pending queue in admin panel |
| `disabled` | No registration | Admin creates accounts manually |


**Registration Flow:**
```
┌─────────────────────────────────────────────────────┐
│  Registration Mode: invite_only                      │
│                                                       │
│  Admin Panel                    Customer              │
│  ───────────                    ────────              │
│  1. Admin creates invite        │                     │
│     → email + expiry + plan     │                     │
│  2. System sends invite email ──► 3. Click link       │
│                                  │  4. Set password    │
│                                  │  5. Choose plan     │
│                                  │  6. Dashboard       │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│  Registration Mode: approval                         │
│                                                       │
│  Customer                       Admin Panel           │
│  ────────                       ───────────           │
│  1. Fill registration form  ──► 2. Appears in queue   │
│  3. "Pending approval" page     4. Review + Approve   │
│  5. Activation email ◄──────── or Reject with reason  │
│  6. Login → Dashboard                                 │
└─────────────────────────────────────────────────────┘
```

**Onboarding Wizard (First Login):**
```
Step 1: Welcome
  → Choose: "Deploy from Git" | "Deploy Docker Image" | "Browse Marketplace"

Step 2: Connect Source (if Git chosen)
  → OAuth connect GitHub/GitLab or paste token

Step 3: Select Repository & Branch

Step 4: Configure Domain
  → Auto-subdomain (app-name.deploy.monster) or custom domain

Step 5: Deploy!
  → Real-time build log → success → link to live app
```

**SSO / OAuth Providers:**

| Provider | Protocol | Use Case |
|----------|----------|----------|
| Google | OAuth2/OIDC | Google Workspace orgs |
| GitHub | OAuth2 | Developer teams |
| GitLab | OAuth2/OIDC | Self-hosted GitLab orgs |
| SAML 2.0 | SAML | Enterprise SSO (Okta, Azure AD) |
| Custom OIDC | OIDC | Any OIDC-compliant provider |

---

## 20A. BILLING & PAY-PER-USAGE

DeployMonster includes a built-in billing system for SaaS hosting providers who want to charge their customers.

### 20A.1 Billing Architecture

```
┌──────────────────────────────────────────────────────────┐
│                   Billing Engine                          │
│                                                           │
│  ┌─────────────────┐  ┌──────────────────────────────┐  │
│  │  Plan Manager    │  │  Usage Meter                 │  │
│  │  (tiers/limits)  │  │  (CPU·hr, RAM·hr, GB, xfer) │  │
│  └────────┬────────┘  └──────────────┬───────────────┘  │
│           │                          │                    │
│           └──────────┬───────────────┘                    │
│                      ▼                                    │
│           ┌──────────────────┐                            │
│           │  Invoice Engine  │ ← Generate monthly invoice │
│           └────────┬─────────┘                            │
│                    ▼                                      │
│           ┌──────────────────┐                            │
│           │  Payment Gateway │ ← Stripe / Paddle / LemonSqueezy │
│           └────────┬─────────┘                            │
│                    ▼                                      │
│           ┌──────────────────┐                            │
│           │  Enforcement     │ ← Suspend / warn on overdue│
│           └──────────────────┘                            │
└──────────────────────────────────────────────────────────┘
```

### 20A.2 Plan System

```yaml
plans:
  free:
    name: "Free"
    price: 0
    limits:
      apps: 3
      domains: 1
      databases: 1
      servers: 1            # Local only
      build_minutes: 100    # Per month
      bandwidth_gb: 10
      storage_gb: 5
      team_members: 1
      cpu_cores: 1          # Shared
      ram_mb: 512
      backups: 3
      ssl: true
      custom_domain: false
      webhook_deploys: true
      marketplace: true
    features:
      - community_support

  starter:
    name: "Starter"
    price: 9.99             # USD/month
    limits:
      apps: 10
      domains: 5
      databases: 3
      servers: 2
      build_minutes: 500
      bandwidth_gb: 100
      storage_gb: 25
      team_members: 3
      cpu_cores: 2
      ram_mb: 2048
      backups: 10
      ssl: true
      custom_domain: true
      webhook_deploys: true
      marketplace: true
    features:
      - email_support
      - auto_backups
      - custom_domains

  pro:
    name: "Professional"
    price: 29.99
    limits:
      apps: 50
      domains: 25
      databases: 10
      servers: 10
      build_minutes: 2000
      bandwidth_gb: 500
      storage_gb: 100
      team_members: 10
      cpu_cores: 8
      ram_mb: 16384
      backups: 50
      ssl: true
      custom_domain: true
      webhook_deploys: true
      marketplace: true
    features:
      - priority_support
      - auto_backups
      - custom_domains
      - blue_green_deploy
      - canary_deploy
      - multi_server
      - resource_alerts

  enterprise:
    name: "Enterprise"
    price: 0                # Custom pricing
    limits: unlimited
    features:
      - dedicated_support
      - sla_guarantee
      - white_label
      - custom_integrations
      - on_premise
```

### 20A.3 Usage Metering

DeployMonster tracks granular resource usage per tenant:

| Metric | Unit | Measurement | Billing |
|--------|------|-------------|---------|
| CPU Usage | core·hours | Per container, per minute | Aggregated hourly |
| RAM Usage | MB·hours | Per container, per minute | Aggregated hourly |
| Disk Storage | GB·days | Volume size × days | Daily snapshot |
| Bandwidth (egress) | GB | Ingress proxy metering | Real-time counter |
| Build Minutes | minutes | Build start → finish | Per build |
| Backup Storage | GB | Backup file sizes | Daily snapshot |
| SSL Certificates | count | Active certs | Monthly count |
| Managed DBs | count × type | Active databases | Monthly count |

**Metering Flow:**
```
Every 60 seconds:
┌──────────────────┐
│  Collect Metrics  │ ← Docker stats API per container
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Map to Tenant   │ ← Container → App → Project → Tenant
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Aggregate       │ ← Sum CPU·sec, RAM·sec per tenant
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Store in DB     │ ← usage_records table (hourly rollup)
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Check Limits    │ ← Compare vs plan limits
└────────┬─────────┘
    ┌────┴────┐
    ▼         ▼
 Within    Exceeded
 limits    ┌──────────────┐
           │ Warn / Block │ ← Email + UI banner + optional hard limit
           └──────────────┘
```

### 20A.4 Resource Limits & Enforcement

| Limit Type | Soft Limit | Hard Limit |
|-----------|-----------|-----------|
| Apps count | Warning banner | Block new app creation |
| CPU cores | Throttle notification | Docker CPU quota enforcement |
| RAM | Warning at 80% | OOM kill + notification |
| Disk | Warning at 90% | Block new deploys |
| Bandwidth | Warning at 80% | Optional: throttle or charge overage |
| Build minutes | Warning at 80% | Block new builds until next cycle |
| Team members | N/A | Block invite |

**Per-Container Resource Limits:**
```go
type ResourceLimits struct {
    CPUShares    int64   // Docker CPU shares (relative weight)
    CPUQuota     int64   // CPU CFS quota (microseconds per period)
    CPUPeriod    int64   // CPU CFS period (default 100000μs)
    MemoryMB     int64   // Hard memory limit
    MemorySwapMB int64   // Memory + swap limit
    PidsLimit    int64   // Max number of processes
    DiskQuotaMB  int64   // Disk I/O limit (via cgroups)
    NetworkMbps  int     // Bandwidth limit (via tc)
}

// Default per plan
var PlanDefaults = map[string]ResourceLimits{
    "free":    {CPUShares: 256, MemoryMB: 256, PidsLimit: 100},
    "starter": {CPUShares: 512, MemoryMB: 512, PidsLimit: 256},
    "pro":     {CPUShares: 1024, MemoryMB: 2048, PidsLimit: 512},
}
```

**Admin Override:**
Admin can set custom limits per tenant, overriding plan defaults:
```
Admin Panel → Tenants → [tenant] → Resource Limits
  → CPU: 4 cores (override plan default)
  → RAM: 8 GB
  → Disk: 50 GB
  → Bandwidth: 500 GB/mo
```

### 20A.5 Invoice Generation

```
Monthly billing cycle (1st of month):
        │
        ▼
┌──────────────────┐
│  Aggregate Usage │ ← Sum all usage_records for billing period
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Apply Plan      │ ← Base price + overage charges
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Generate Invoice│ ← PDF with line items
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Send to Payment │ ← Stripe Invoice API or internal
└────────┬─────────┘
         ▼
┌──────────────────┐
│  Email Customer  │ ← Invoice PDF + payment link
└──────────────────┘
```

**Invoice Line Items:**
```
DeployMonster - Invoice #INV-2026-03-001
Billing Period: March 1-31, 2026
Tenant: Acme Corp

─────────────────────────────────────────────────
Description                    Qty      Amount
─────────────────────────────────────────────────
Pro Plan (base)                1 mo     $29.99
Extra CPU (over 8 cores)       2.3 core·mo  $11.50
Extra RAM (over 16 GB)         4 GB·mo  $8.00
Extra Bandwidth (over 500 GB)  120 GB   $6.00
Extra Backup Storage           15 GB    $1.50
Managed DB: PostgreSQL 16      1        $0.00 (included)
Managed DB: Redis 7            1        $5.00
─────────────────────────────────────────────────
Subtotal                                $62.00
Tax (20% VAT)                           $12.40
─────────────────────────────────────────────────
Total                                   $74.40
─────────────────────────────────────────────────
```

### 20A.6 Payment Integration

| Provider | Type | Features |
|----------|------|----------|
| Stripe | Primary | Subscriptions, invoices, usage metering, cards, SEPA |
| Paddle | Alternative | Tax handling, reseller model |
| LemonSqueezy | Alternative | Simple SaaS billing |
| Manual/Wire | Fallback | Enterprise custom billing |

**Stripe Integration:**
- **Stripe Subscriptions** for plan management
- **Stripe Metered Billing** for usage-based charges
- **Stripe Customer Portal** for self-service payment management
- **Stripe Webhooks** for payment event processing
- **Stripe Tax** for automatic tax calculation

### 20A.7 Billing Data Model

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│Subscription  │     │   Invoice    │     │  UsageRecord │
│──────────────│     │──────────────│     │──────────────│
│ id           │     │ id           │     │ id           │
│ tenant_id    │     │ tenant_id    │     │ tenant_id    │
│ plan_id      │     │ subscr_id    │     │ app_id       │
│ status       │     │ period_start │     │ metric_type  │
│ stripe_sub_id│     │ period_end   │     │ value        │
│ current_period│    │ subtotal     │     │ unit         │
│ trial_end    │     │ tax          │     │ timestamp    │
│ cancel_at    │     │ total        │     │ hour_bucket  │
│ created_at   │     │ status       │     │              │
│              │     │ paid_at      │     │              │
│              │     │ stripe_inv_id│     │              │
│              │     │ pdf_url      │     │              │
│              │     │ created_at   │     │              │
└──────────────┘     └──────────────┘     └──────────────┘

┌──────────────┐     ┌──────────────┐
│ InvoiceLine  │     │  Payment     │
│──────────────│     │──────────────│
│ id           │     │ id           │
│ invoice_id   │     │ invoice_id   │
│ description  │     │ amount       │
│ quantity     │     │ currency     │
│ unit_price   │     │ method       │
│ amount       │     │ stripe_pi_id │
│ type         │     │ status       │
│              │     │ paid_at      │
└──────────────┘     └──────────────┘
```

### 20A.8 Billing UI

**Admin Panel:**
```
├── Billing Settings
│   ├── Plans & Pricing (CRUD)
│   ├── Overage pricing per resource
│   ├── Payment provider config (Stripe keys)
│   ├── Tax settings
│   ├── Invoice template customization
│   └── Trial period settings
├── Revenue Dashboard
│   ├── MRR (Monthly Recurring Revenue)
│   ├── Churn rate
│   ├── Revenue per plan breakdown
│   ├── Outstanding invoices
│   └── Usage heatmap (which resources are most used)
```

**Customer Panel:**
```
├── Billing
│   ├── Current plan + usage meters (visual bars)
│   ├── Upgrade / Downgrade plan
│   ├── Invoice history (download PDF)
│   ├── Payment methods (Stripe Customer Portal)
│   ├── Usage breakdown (charts per resource)
│   └── Cost forecast (estimated next invoice)
```

---

## 21. MARKETPLACE — DOCKER PARADISE

DeployMonster Marketplace is a curated, searchable, one-click deployment catalog. Think "App Store for Docker" — from databases to full SaaS stacks.

### 21.1 Marketplace Architecture

```
┌─────────────────────────────────────────────────────────┐
│                 Marketplace Engine                        │
│                                                           │
│  ┌─────────────────┐  ┌──────────────────────────────┐  │
│  │ Built-in        │  │ Community Registry            │  │
│  │ Templates       │  │ (deploy.monster/marketplace)  │  │
│  │ (embedded YAML) │  │ Git-synced template repo      │  │
│  └────────┬────────┘  └──────────────┬───────────────┘  │
│           │                          │                    │
│           └──────────┬───────────────┘                    │
│                      ▼                                    │
│           ┌──────────────────┐                            │
│           │  Template Index  │ ← Search, filter, sort     │
│           └────────┬─────────┘                            │
│                    ▼                                      │
│           ┌──────────────────┐                            │
│           │  Config Wizard   │ ← JSON Schema form         │
│           └────────┬─────────┘                            │
│                    ▼                                      │
│           ┌──────────────────┐                            │
│           │  Deploy Pipeline │ ← Generate compose → deploy│
│           └──────────────────┘                            │
└─────────────────────────────────────────────────────────┘
```

### 21.2 Template Categories

| Category | Subcategory | Apps |
|----------|-------------|------|
| **CMS** | Blog | WordPress, Ghost, Hugo, Jekyll |
| | Headless CMS | Strapi, Directus, Payload, Sanity |
| | Wiki | BookStack, Wiki.js, Outline, Docusaurus |
| **E-Commerce** | Full Store | WooCommerce, Medusa, Saleor |
| | Payment | BTCPay Server |
| **Communication** | Chat | Rocket.Chat, Mattermost, Element/Matrix |
| | Email | Mailu, Docker Mailserver, Roundcube |
| | Forum | Discourse, Flarum, NodeBB |
| **Development** | Git | Gitea, Gogs, GitLab CE |
| | CI/CD | Drone, Woodpecker, Jenkins |
| | Code | Code-Server (VS Code), Jupyter, GitPod |
| | Registry | Docker Registry, Harbor |
| | API Tools | Hoppscotch, Swagger UI, Postman |
| **Databases** | Relational | PostgreSQL, MySQL, MariaDB, CockroachDB |
| | NoSQL | MongoDB, CouchDB, ArangoDB |
| | Cache | Redis, KeyDB, DragonflyDB, Memcached |
| | Search | Elasticsearch, Meilisearch, Typesense, OpenSearch |
| | Time Series | InfluxDB, TimescaleDB, QuestDB |
| | Graph | Neo4j, SurrealDB |
| | Vector | Qdrant, Weaviate, ChromaDB, Milvus |
| **Analytics** | Web Analytics | Plausible, Umami, Matomo, PostHog |
| | BI | Metabase, Apache Superset, Redash, Grafana |
| | Log Management | Graylog, Loki + Grafana |
| **Monitoring** | Uptime | Uptime Kuma, Gatus, Statping |
| | Infrastructure | Prometheus + Grafana, Netdata, Zabbix |
| **Storage** | Object Storage | MinIO, SeaweedFS, Garage |
| | File Sharing | Nextcloud, Seafile, Syncthing, FileBrowser |
| | Photo/Media | Immich, PhotoPrism, Jellyfin, Plex |
| **Automation** | Workflow | n8n, Huginn, Automatisch, Activepieces |
| | Scheduling | Cal.com, Calendso |
| **Security** | Passwords | Vaultwarden (Bitwarden), Passbolt |
| | Auth | Keycloak, Authentik, Authelia, Casdoor |
| | VPN | WireGuard, Headscale (Tailscale) |
| | Proxy | Squid, Shadowsocks |
| **AI / ML** | LLM | Ollama, LocalAI, text-generation-webui |
| | Tools | Langflow, Flowise, Open WebUI |
| | ML Ops | MLflow, Label Studio |
| **Networking** | DNS | Pi-hole, AdGuard Home, CoreDNS |
| | Reverse Proxy | Nginx, Caddy, Traefik (for nested use) |
| | Tunnel | Cloudflared, FRP, Rathole |
| **Business** | Project Mgmt | Plane, Focalboard, Taiga, Leantime |
| | CRM | Twenty, EspoCRM, SuiteCRM |
| | ERP | ERPNext, Odoo |
| | Invoicing | Invoice Ninja, Crater |
| | HR | OrangeHRM |
| **Stacks** | LAMP | Apache + MySQL + PHP |
| | LEMP | Nginx + MySQL + PHP-FPM |
| | MEAN | MongoDB + Express + Angular + Node |
| | MERN | MongoDB + Express + React + Node |
| | T3 Stack | Next.js + tRPC + Prisma + Tailwind |

**Total at v0.1.7: 56 built-in templates across 16 categories. Additional apps above (ERPNext, Odoo, Crater, OrangeHRM, LAMP/LEMP/MEAN/MERN/T3 stacks, and so on) are a forward-looking catalog plan — they are candidates for community contribution, not currently embedded. Growth roadmap is tracked in `.project/ROADMAP.md`.**

### 21.3 Template Manifest Format

```yaml
# marketplace/templates/plausible.yaml
apiVersion: monster/v1
kind: MarketplaceTemplate

metadata:
  slug: plausible
  name: Plausible Analytics
  description: >
    Lightweight, open-source, privacy-friendly 
    alternative to Google Analytics.
  version: "2.1"
  icon: plausible.svg
  screenshots:
    - dashboard.png
    - settings.png
  category: analytics
  subcategory: web-analytics
  tags: [analytics, privacy, gdpr, self-hosted]
  author: Plausible Analytics
  website: https://plausible.io
  source: https://github.com/plausible/analytics
  license: AGPL-3.0
  verified: true

requirements:
  min_cpu: 1
  min_ram_mb: 1024
  min_disk_mb: 5120
  docker_compose_version: "3"

config:
  # JSON Schema — generates UI form automatically
  type: object
  required: [domain, admin_email]
  properties:
    domain:
      type: string
      title: "Your Domain"
      description: "Domain where Plausible will be accessible"
      pattern: "^[a-zA-Z0-9.-]+$"
      placeholder: "analytics.example.com"
    admin_email:
      type: string
      format: email
      title: "Admin Email"
    admin_password:
      type: string
      format: password
      title: "Admin Password"
      minLength: 8
    disable_registration:
      type: boolean
      title: "Disable Public Registration"
      default: true
    maxmind_license:
      type: string
      title: "MaxMind License Key (optional, for GeoIP)"
      description: "Get free key at maxmind.com"

compose:
  services:
    plausible:
      image: ghcr.io/plausible/community-edition:v2.1
      ports: ["8000"]
      env:
        BASE_URL: "https://${config.domain}"
        SECRET_KEY_BASE: "${GENERATED_SECRET_64}"
        DATABASE_URL: "postgres://plausible:${GENERATED_PASSWORD}@db:5432/plausible"
        CLICKHOUSE_DATABASE_URL: "http://events-db:8123/plausible_events_db"
        DISABLE_REGISTRATION: "${config.disable_registration}"
        MAILER_EMAIL: "${config.admin_email}"
      depends_on:
        db:
          condition: service_healthy
        events-db:
          condition: service_healthy
      labels:
        monster.enable: "true"
        monster.http.routers.plausible.rule: "Host(`${config.domain}`)"
        monster.http.services.plausible.loadbalancer.server.port: "8000"
    db:
      image: postgres:16-alpine
      env:
        POSTGRES_USER: plausible
        POSTGRES_PASSWORD: "${GENERATED_PASSWORD}"
        POSTGRES_DB: plausible
      volumes:
        - db-data:/var/lib/postgresql/data
      healthcheck:
        test: ["CMD-SHELL", "pg_isready -U plausible"]
        interval: 5s
        timeout: 5s
        retries: 5
    events-db:
      image: clickhouse/clickhouse-server:24-alpine
      volumes:
        - events-data:/var/lib/clickhouse
      healthcheck:
        test: ["CMD", "wget", "--no-verbose", "--tries=1", "-O", "-", "http://localhost:8123/ping"]
        interval: 5s
        timeout: 5s
        retries: 5

  volumes:
    db-data:
    events-data:

hooks:
  post_deploy:
    - service: plausible
      command: >
        /app/bin/plausible eval 
        "Plausible.Auth.create_user(\"${config.admin_email}\", \"${config.admin_password}\")"
      wait_for_healthy: true

upgrade:
  strategy: rolling
  backup_before: true
  preserve_volumes: [db-data, events-data]
```

### 21.4 Template Variables

| Variable | Generated | Description |
|----------|-----------|-------------|
| `${config.xxx}` | No | User input from config schema |
| `${GENERATED_PASSWORD}` | Yes | 32-char crypto-random password |
| `${GENERATED_SECRET_64}` | Yes | 64-char base64 secret key |
| `${GENERATED_ROOT_PASSWORD}` | Yes | Separate root password |
| `${DOMAIN}` | Yes | Assigned domain (auto or custom) |
| `${SERVER_IP}` | Yes | Server public IP |
| `${TENANT_ID}` | Yes | Current tenant ID |
| `${APP_ID}` | Yes | Generated application ID |

### 21.5 Community Template Registry

```
github.com/deploy-monster/marketplace      ← Official templates
github.com/deploy-monster/community-apps   ← Community contributions

Sync Flow:
1. DeployMonster fetches template index (JSON) from registry
2. Downloads new/updated template YAMLs
3. Validates against schema
4. Makes available in UI marketplace
5. Periodic sync (configurable, default: 6 hours)
6. Offline mode: embedded templates always available
```

**Community Contribution:**
```bash
# Fork community-apps repo
# Add template YAML in correct category folder
# Submit PR
# Automated validation CI runs
# After merge → available to all DeployMonster instances
```

### 21.6 Marketplace UI Features

- **Browse by category** — Grid view with icons, descriptions
- **Search** — Full-text search across name, description, tags
- **Filter** — By category, resource requirements, license, verified
- **Sort** — Popular, newest, alphabetical, resource requirements
- **Preview** — Screenshots, README, resource requirements
- **One-click deploy** — Config wizard → domain selection → deploy
- **Version tracking** — Update notifications for installed apps
- **Ratings & reviews** — Community feedback (post v1.0)
- **My installed** — List of marketplace apps with update status

---

## 22. CLI INTERFACE

```bash
# Installation
curl -fsSL https://deploy.monster/install.sh | bash

# Server mode (full PaaS)
deploymonster serve                        # Start DeployMonster server
deploymonster serve --port 8443            # Custom port
deploymonster serve --agent                # Agent/worker mode

# App management
deploymonster deploy .                     # Deploy current directory
deploymonster deploy github.com/user/repo  # Deploy from any Git URL
deploymonster deploy --image nginx:latest  # Deploy Docker image directly
deploymonster deploy --compose docker-compose.yml  # Deploy compose stack
deploymonster apps list                    # List apps
deploymonster apps logs my-app             # View logs
deploymonster apps exec my-app -- bash     # Exec into container
deploymonster apps restart my-app          # Restart
deploymonster apps rollback my-app         # Rollback to previous

# Docker Compose stacks
deploymonster stack deploy -f docker-compose.yml   # Deploy compose
deploymonster stack deploy -f https://raw.../docker-compose.yml  # From URL
deploymonster stack list                   # List stacks
deploymonster stack services my-stack      # List stack services
deploymonster stack logs my-stack          # Combined stack logs
deploymonster stack down my-stack          # Stop & remove stack
deploymonster stack scale my-stack web=3   # Scale a service

# Docker Image deploy
deploymonster image deploy nginx:latest --name my-nginx --port 80
deploymonster image deploy ghcr.io/user/app:v2 --domain app.example.com

# Marketplace
deploymonster marketplace list             # Browse all templates
deploymonster marketplace search wordpress # Search templates
deploymonster marketplace info plausible   # Template details
deploymonster marketplace deploy wordpress # Interactive deploy wizard
deploymonster marketplace deploy ghost --domain blog.example.com

# Git Sources
deploymonster git add github --token ghp_xxxx     # Add GitHub PAT
deploymonster git add gitlab --url https://gitlab.example.com --token xxx
deploymonster git add --ssh-key ~/.ssh/id_ed25519 --url git@custom.com:user/repo.git
deploymonster git list                     # List connected sources
deploymonster git repos my-github          # List repos from source
deploymonster git deploy my-github user/repo --branch main

# VPS Providers
deploymonster vps provider add hetzner --token xxx    # Add cloud provider
deploymonster vps provider add digitalocean --token xxx
deploymonster vps provider list                        # List providers
deploymonster vps sizes hetzner                        # List sizes
deploymonster vps regions hetzner                      # List regions
deploymonster vps create hetzner --size cx22 --region nbg1 --name worker-1  # Provision
deploymonster vps connect 198.51.100.42 --ssh-key ~/.ssh/id_ed25519  # Existing server
deploymonster vps list                                 # List servers
deploymonster vps destroy worker-1                     # Destroy VPS

# Database
deploymonster db create postgres --name mydb
deploymonster db list
deploymonster db backup mydb
deploymonster db connect mydb              # Open psql/mysql client

# Domain
deploymonster domain add app.example.com --app my-app
deploymonster domain list
deploymonster domain ssl-status

# Backup
deploymonster backup create --all
deploymonster backup list
deploymonster backup restore bkp_xxxx

# Cluster
deploymonster cluster join <manager-ip> --token <token>
deploymonster cluster nodes
deploymonster cluster remove <node-id>

# Config
deploymonster config set smtp.host=smtp.gmail.com
deploymonster config get smtp.host
deploymonster config export > config.yaml
```

---

## 23. CONFIGURATION

### 23.1 Config File (monster.yaml)

```yaml
server:
  host: 0.0.0.0
  port: 8443
  domain: panel.deploy.monster     # Admin panel domain
  secret_key: "${MONSTER_SECRET}"  # Auto-generated on first run

database:
  driver: sqlite                   # sqlite | postgres (future)
  path: /var/lib/deploymonster/monster.db

ingress:
  http_port: 80
  https_port: 443
  dashboard_port: 8443

acme:
  email: admin@deploy.monster
  provider: letsencrypt            # letsencrypt | letsencrypt-staging
  dns_challenge: false             # true for wildcard certs
  storage: /var/lib/deploymonster/acme/

dns:
  provider: cloudflare             # cloudflare | route53 | manual
  cloudflare:
    api_token: "${CF_API_TOKEN}"
    zone_id: "${CF_ZONE_ID}"

docker:
  socket: /var/run/docker.sock
  network: monster-network
  registry:
    enabled: false
    url: registry.deploy.monster

backup:
  default_target: local
  targets:
    local:
      path: /var/lib/deploymonster/backups
    s3:
      bucket: deploymonster-backups
      region: eu-central-1
      access_key: "${AWS_ACCESS_KEY}"
      secret_key: "${AWS_SECRET_KEY}"

notifications:
  email:
    smtp_host: smtp.gmail.com
    smtp_port: 587
    from: noreply@deploy.monster
  slack:
    webhook_url: "${SLACK_WEBHOOK}"
  telegram:
    bot_token: "${TG_BOT_TOKEN}"
    chat_id: "${TG_CHAT_ID}"

swarm:
  enabled: false
  advertise_addr: ""               # Auto-detected
  join_token: ""                   # Auto-generated

vps_providers:                     # Pre-configured cloud providers
  hetzner:
    enabled: false
    api_token: "${HETZNER_API_TOKEN}"
    default_region: nbg1
    default_image: ubuntu-24.04
    default_type: cx22
    ssh_key_name: deploymonster
  digitalocean:
    enabled: false
    api_token: "${DO_API_TOKEN}"
    default_region: fra1
    default_size: s-1vcpu-2gb
  vultr:
    enabled: false
    api_key: "${VULTR_API_KEY}"
  linode:
    enabled: false
    api_token: "${LINODE_API_TOKEN}"
  aws:
    enabled: false
    access_key: "${AWS_ACCESS_KEY}"
    secret_key: "${AWS_SECRET_KEY}"
    region: eu-central-1

git_sources:                       # OAuth app credentials
  github:
    client_id: "${GITHUB_CLIENT_ID}"
    client_secret: "${GITHUB_CLIENT_SECRET}"
  gitlab:
    client_id: "${GITLAB_CLIENT_ID}"
    client_secret: "${GITLAB_CLIENT_SECRET}"
    base_url: "https://gitlab.com"   # Self-hosted override

marketplace:
  sync_enabled: true
  sync_interval: 6h
  registry_url: "https://raw.githubusercontent.com/deploy-monster/marketplace/main"
  community_url: "https://raw.githubusercontent.com/deploy-monster/community-apps/main"
  allow_custom_templates: true

registration:
  mode: invite_only              # open | invite_only | approval | disabled
  require_email_verification: true
  allowed_email_domains: []      # Empty = all domains, ["example.com"] = restrict
  default_plan: free
  default_role: developer
  trial_days: 14                 # 0 = no trial
  captcha_enabled: false         # hCaptcha for public registration

sso:
  google:
    enabled: false
    client_id: "${GOOGLE_CLIENT_ID}"
    client_secret: "${GOOGLE_CLIENT_SECRET}"
    allowed_domains: []          # Restrict to specific Google Workspace domains
  github:
    enabled: true                # Reuse git source OAuth app
    client_id: "${GITHUB_CLIENT_ID}"
    client_secret: "${GITHUB_CLIENT_SECRET}"
  saml:
    enabled: false
    idp_metadata_url: ""
    entity_id: "https://panel.deploy.monster"

secrets:
  master_key_derivation: argon2id  # Derive encryption key from MONSTER_SECRET
  auto_rotate_days: 0              # 0 = manual rotation only
  max_versions: 10                 # Keep last N versions per secret

billing:
  enabled: false                 # Set to true to enable billing
  provider: stripe               # stripe | paddle | lemonsqueezy | manual
  currency: USD
  tax_inclusive: false
  stripe:
    secret_key: "${STRIPE_SECRET_KEY}"
    publishable_key: "${STRIPE_PUBLISHABLE_KEY}"
    webhook_secret: "${STRIPE_WEBHOOK_SECRET}"
  overage_pricing:
    cpu_per_core_month: 5.00       # $/core/month over plan limit
    ram_per_gb_month: 2.00         # $/GB/month over plan limit
    bandwidth_per_gb: 0.05         # $/GB over plan limit
    storage_per_gb_month: 0.10     # $/GB/month over plan limit
    build_per_minute: 0.01         # $/minute over plan limit
    database_per_month: 5.00       # $/db/month over plan limit

limits:
  max_apps_per_tenant: 50
  max_domains_per_app: 10
  max_databases_per_tenant: 10
  max_servers_per_tenant: 20
  max_backup_size_gb: 50
  build_timeout_minutes: 30
  max_compose_services: 20
```

---

## 24. DIRECTORY STRUCTURE

```
deploy-monster/
├── cmd/
│   └── deploymonster/
│       └── main.go                    # Entry point
├── internal/
│   ├── core/                          # Core engine
│   │   ├── app.go                     # Application bootstrap
│   │   ├── config.go                  # Configuration loader
│   │   ├── module.go                  # Module interface & registry
│   │   └── events.go                  # Event bus
│   ├── auth/                          # Authentication module
│   │   ├── module.go
│   │   ├── jwt.go
│   │   ├── rbac.go
│   │   └── middleware.go
│   ├── api/                           # REST API
│   │   ├── module.go
│   │   ├── router.go
│   │   ├── handlers/
│   │   │   ├── apps.go
│   │   │   ├── projects.go
│   │   │   ├── domains.go
│   │   │   ├── databases.go
│   │   │   ├── servers.go
│   │   │   ├── backups.go
│   │   │   └── admin.go
│   │   ├── middleware/
│   │   │   ├── auth.go
│   │   │   ├── ratelimit.go
│   │   │   ├── cors.go
│   │   │   └── logging.go
│   │   └── ws/
│   │       ├── logs.go
│   │       ├── exec.go
│   │       └── events.go
│   ├── db/                            # Database layer
│   │   ├── module.go
│   │   ├── sqlite.go
│   │   ├── bolt.go
│   │   ├── migrations/
│   │   │   └── *.sql
│   │   └── models/
│   │       ├── tenant.go
│   │       ├── user.go
│   │       ├── project.go
│   │       ├── app.go
│   │       ├── deployment.go
│   │       ├── domain.go
│   │       ├── certificate.go
│   │       ├── server.go
│   │       ├── volume.go
│   │       ├── backup.go
│   │       └── database.go
│   ├── ingress/                       # Ingress Gateway
│   │   ├── module.go
│   │   ├── proxy.go                   # Reverse proxy core
│   │   ├── router.go                  # Route matching engine
│   │   ├── tls.go                     # TLS config & SNI
│   │   ├── acme.go                    # ACME/Let's Encrypt
│   │   ├── middleware/
│   │   │   ├── ratelimit.go
│   │   │   ├── cors.go
│   │   │   ├── compress.go
│   │   │   ├── headers.go
│   │   │   ├── auth.go
│   │   │   └── retry.go
│   │   └── lb/
│   │       ├── balancer.go            # Load balancer interface
│   │       ├── roundrobin.go
│   │       ├── leastconn.go
│   │       ├── iphash.go
│   │       └── weighted.go
│   ├── deploy/                        # Deploy Engine
│   │   ├── module.go
│   │   ├── deployer.go
│   │   ├── strategies/
│   │   │   ├── recreate.go
│   │   │   ├── rolling.go
│   │   │   ├── bluegreen.go
│   │   │   └── canary.go
│   │   └── rollback.go
│   ├── build/                         # Build Engine
│   │   ├── module.go
│   │   ├── builder.go
│   │   ├── detector.go                # Auto-detect project type
│   │   ├── git.go                     # Git clone & checkout
│   │   ├── dockerfile.go
│   │   ├── buildpack.go
│   │   └── templates/                 # Dockerfile templates
│   │       ├── nodejs.Dockerfile
│   │       ├── nextjs.Dockerfile
│   │       ├── go.Dockerfile
│   │       ├── python.Dockerfile
│   │       ├── rust.Dockerfile
│   │       ├── php.Dockerfile
│   │       └── static.Dockerfile
│   ├── discovery/                     # Service Discovery
│   │   ├── module.go
│   │   ├── watcher.go                 # Docker event watcher
│   │   ├── labels.go                  # Label parser
│   │   ├── registry.go               # Service registry
│   │   └── health.go                  # Health checker
│   ├── dns/                           # DNS Synchronization
│   │   ├── module.go
│   │   ├── sync.go
│   │   ├── providers/
│   │   │   ├── cloudflare.go
│   │   │   ├── route53.go
│   │   │   ├── digitalocean.go
│   │   │   └── rfc2136.go
│   │   └── verify.go
│   ├── resource/                      # Resource Monitor
│   │   ├── module.go
│   │   ├── collector.go
│   │   ├── alerts.go
│   │   └── metrics.go
│   ├── backup/                        # Backup Engine
│   │   ├── module.go
│   │   ├── scheduler.go
│   │   ├── volume.go
│   │   ├── database.go
│   │   ├── encryption.go
│   │   └── storage/
│   │       ├── local.go
│   │       ├── s3.go
│   │       └── sftp.go
│   ├── database/                      # Database Manager
│   │   ├── module.go
│   │   ├── provisioner.go
│   │   ├── engines/
│   │   │   ├── postgres.go
│   │   │   ├── mysql.go
│   │   │   ├── redis.go
│   │   │   └── mongodb.go
│   │   └── pooler.go
│   ├── swarm/                         # Swarm Orchestrator
│   │   ├── module.go
│   │   ├── manager.go
│   │   ├── agent.go
│   │   ├── placement.go
│   │   └── network.go
│   ├── vps/                           # VPS Provider Manager
│   │   ├── module.go
│   │   ├── manager.go                 # Provider lifecycle
│   │   ├── provisioner.go             # Server creation flow
│   │   ├── bootstrap.go               # SSH bootstrap + Docker install
│   │   ├── ssh.go                     # SSH connection pool
│   │   ├── cloudinit.go               # Cloud-init template
│   │   ├── providers/
│   │   │   ├── provider.go            # Provider interface
│   │   │   ├── hetzner.go
│   │   │   ├── digitalocean.go
│   │   │   ├── vultr.go
│   │   │   ├── linode.go
│   │   │   ├── aws.go
│   │   │   └── custom.go             # Custom SSH server
│   │   └── inventory.go               # Server inventory
│   ├── gitsources/                    # Universal Git Source Manager
│   │   ├── module.go
│   │   ├── manager.go
│   │   ├── providers/
│   │   │   ├── provider.go            # Git provider interface
│   │   │   ├── github.go
│   │   │   ├── gitlab.go
│   │   │   ├── bitbucket.go
│   │   │   ├── gitea.go
│   │   │   ├── gogs.go
│   │   │   ├── azure.go
│   │   │   ├── codecommit.go
│   │   │   └── generic.go             # Any Git over SSH/HTTPS
│   │   └── repos.go                   # Repository listing/browsing
│   ├── compose/                       # Docker Compose Deployer
│   │   ├── module.go
│   │   ├── parser.go                  # Compose YAML parser
│   │   ├── validator.go               # Schema validation
│   │   ├── deployer.go                # Multi-service deploy orchestrator
│   │   ├── interpolate.go             # Variable interpolation
│   │   └── converter.go               # Compose → DeployMonster models
│   ├── marketplace/                   # Marketplace Engine
│   │   ├── module.go
│   │   ├── registry.go                # Template registry + sync
│   │   ├── loader.go                  # Template YAML loader
│   │   ├── wizard.go                  # Config wizard (JSON Schema)
│   │   ├── deployer.go                # Template → Compose → Deploy
│   │   ├── search.go                  # Full-text search index
│   │   └── templates/                 # Built-in templates (embedded)
│   │       ├── cms/
│   │       │   ├── wordpress.yaml
│   │       │   ├── ghost.yaml
│   │       │   ├── strapi.yaml
│   │       │   └── ...
│   │       ├── databases/
│   │       │   ├── postgres.yaml
│   │       │   ├── mysql.yaml
│   │       │   ├── redis.yaml
│   │       │   └── ...
│   │       ├── monitoring/
│   │       │   ├── uptime-kuma.yaml
│   │       │   ├── plausible.yaml
│   │       │   └── ...
│   │       ├── dev-tools/
│   │       │   ├── gitea.yaml
│   │       │   ├── code-server.yaml
│   │       │   └── ...
│   │       ├── ai/
│   │       │   ├── ollama.yaml
│   │       │   ├── open-webui.yaml
│   │       │   └── ...
│   │       └── index.yaml             # Template index manifest
│   ├── mcp/                           # MCP Server
│   │   ├── module.go
│   │   ├── server.go
│   │   ├── tools.go
│   │   └── resources.go
│   ├── notifications/                 # Notification Engine
│   │   ├── module.go
│   │   ├── email.go
│   │   ├── slack.go
│   │   ├── discord.go
│   │   └── telegram.go
│   ├── secrets/                       # Secret Vault
│   │   ├── module.go
│   │   ├── vault.go                   # Encrypt/decrypt engine
│   │   ├── store.go                   # Secret CRUD + versioning
│   │   ├── resolver.go               # ${SECRET:x} resolution at deploy
│   │   ├── scoping.go                # Global → Tenant → Project → App
│   │   └── import_export.go          # .env import, encrypted JSON export
│   ├── billing/                       # Billing Engine
│   │   ├── module.go
│   │   ├── plans.go                   # Plan definitions + limits
│   │   ├── metering.go               # Usage collection (CPU·hr, RAM·hr, GB)
│   │   ├── quotas.go                  # Limit enforcement
│   │   ├── invoicing.go              # Invoice generation
│   │   ├── stripe.go                  # Stripe subscriptions + metered billing
│   │   └── webhook.go                # Stripe webhook handler
│   ├── enterprise/                    # Enterprise Features
│   │   ├── module.go
│   │   ├── whitelabel.go             # Branding engine (logo, colors, domain)
│   │   ├── reseller.go               # Reseller management
│   │   ├── provisioning.go           # Enterprise tenant provisioning API
│   │   ├── compliance.go             # GDPR tools (export, erasure, residency)
│   │   ├── sla.go                    # SLA tracking & reporting
│   │   ├── license.go                # License key validation
│   │   ├── ha.go                     # HA coordination (Litestream, failover)
│   │   └── integrations/
│   │       ├── whmcs.go              # WHMCS provisioning bridge
│   │       ├── prometheus.go          # /metrics OpenMetrics endpoint
│   │       └── ldap.go               # LDAP/AD directory sync
│   └── webhooks/                      # Universal Webhook Handler
│       ├── module.go
│       ├── receiver.go                # HTTP handler, signature verify
│       ├── dispatcher.go              # Route webhook → deploy pipeline
│       ├── parsers/
│       │   ├── parser.go              # Parser interface
│       │   ├── github.go
│       │   ├── gitlab.go
│       │   ├── bitbucket.go
│       │   ├── gitea.go
│       │   ├── gogs.go
│       │   ├── azure.go
│       │   └── generic.go             # Generic JSON payload parser
│       └── autoregister.go            # Auto-register webhook via provider API
├── web/                               # React UI (separate build)
│   ├── package.json
│   ├── vite.config.ts
│   ├── tailwind.config.ts
│   ├── src/
│   │   ├── main.tsx
│   │   ├── App.tsx
│   │   ├── api/                       # API client
│   │   ├── hooks/                     # Custom hooks
│   │   ├── stores/                    # Zustand stores
│   │   ├── components/
│   │   │   ├── ui/                    # shadcn/ui components
│   │   │   ├── layout/               # Shell, sidebar, navbar
│   │   │   ├── topology/             # React Flow canvas
│   │   │   ├── terminal/             # xterm.js terminal
│   │   │   ├── metrics/              # Recharts dashboards
│   │   │   └── shared/               # Common components
│   │   ├── pages/
│   │   │   ├── admin/                # Admin panel pages
│   │   │   │   ├── Dashboard.tsx
│   │   │   │   ├── Tenants.tsx
│   │   │   │   ├── Servers.tsx
│   │   │   │   ├── VPSProviders.tsx
│   │   │   │   ├── MarketplaceAdmin.tsx
│   │   │   │   ├── Settings.tsx
│   │   │   │   └── Monitoring.tsx
│   │   │   ├── app/                   # Customer panel pages
│   │   │   │   ├── Dashboard.tsx
│   │   │   │   ├── Projects.tsx
│   │   │   │   ├── Applications.tsx
│   │   │   │   ├── AppDetail.tsx
│   │   │   │   ├── DeployWizard.tsx   # Multi-source deploy wizard
│   │   │   │   ├── ComposeStacks.tsx
│   │   │   │   ├── StackDetail.tsx
│   │   │   │   ├── Databases.tsx
│   │   │   │   ├── Domains.tsx
│   │   │   │   ├── Storage.tsx
│   │   │   │   ├── Secrets.tsx        # Secret vault management
│   │   │   │   ├── SecretEditor.tsx   # Bulk env var / secret editor
│   │   │   │   ├── Marketplace.tsx
│   │   │   │   ├── MarketplaceDetail.tsx
│   │   │   │   ├── GitSources.tsx
│   │   │   │   ├── GitRepoBrowser.tsx
│   │   │   │   ├── Servers.tsx
│   │   │   │   ├── ServerProvision.tsx
│   │   │   │   ├── Webhooks.tsx
│   │   │   │   ├── Topology.tsx       # React Flow drag & drop canvas
│   │   │   │   ├── Metrics.tsx        # App-level metrics dashboard
│   │   │   │   ├── Billing.tsx        # Plan, usage meters, invoices
│   │   │   │   ├── BillingUsage.tsx   # Detailed usage breakdown
│   │   │   │   └── Settings.tsx
│   │   │   ├── auth/
│   │   │   │   ├── Login.tsx
│   │   │   │   ├── Register.tsx
│   │   │   │   ├── ForgotPassword.tsx
│   │   │   │   ├── ResetPassword.tsx
│   │   │   │   ├── AcceptInvite.tsx   # Invite-only registration
│   │   │   │   ├── PendingApproval.tsx # Approval mode waiting
│   │   │   │   ├── OAuthCallback.tsx  # SSO OAuth callback
│   │   │   │   ├── TwoFactor.tsx      # 2FA verification
│   │   │   │   └── Onboarding.tsx     # First-login wizard
│   │   │   ├── admin/                # Admin panel pages
│   │   │   │   ├── ...
│   │   │   │   ├── BillingSettings.tsx  # Plans, pricing, Stripe config
│   │   │   │   ├── Revenue.tsx          # MRR, churn, revenue dashboard
│   │   │   │   ├── TenantQuotas.tsx     # Per-tenant limit overrides
│   │   │   │   └── RegistrationSettings.tsx  # Open/invite/approval mode
│   │   │   └── public/
│   │   │       ├── Landing.tsx
│   │   │       └── Pricing.tsx        # Public pricing page
│   │   └── lib/
│   │       ├── utils.ts
│   │       ├── constants.ts
│   │       └── types.ts
│   └── dist/                          # Built output → embedded in Go
├── marketplace/                       # Template manifests
│   ├── wordpress.yaml
│   ├── ghost.yaml
│   └── ...
├── scripts/
│   ├── build.sh                       # Build Go + React
│   ├── install.sh                     # Install script (curl | bash)
│   └── release.sh                     # Cross-compile releases
├── deployments/
│   ├── Dockerfile                     # DeployMonster container image
│   └── docker-compose.dev.yaml        # Dev environment
├── docs/
│   ├── getting-started.md
│   ├── architecture.md
│   ├── api-reference.md
│   └── deployment-guide.md
├── go.mod
├── go.sum
├── Makefile
├── SPECIFICATION.md
├── IMPLEMENTATION.md
├── TASKS.md
├── BRANDING.md
├── README.md
├── LICENSE
└── .goreleaser.yaml
```

---

## 25. DEPENDENCY LIST

### Go Dependencies (Minimal)

| Package | Purpose | Why Needed |
|---------|---------|-----------|
| `github.com/docker/docker` | Docker SDK | Container management (unavoidable) |
| `github.com/docker/go-connections` | Docker helper | Part of Docker SDK |
| `modernc.org/sqlite` | SQLite (pure Go) | Embedded database, no CGo needed |
| `go.etcd.io/bbolt` | BoltDB | KV store for sessions, cache |
| `github.com/go-acme/lego/v4` | ACME client | Let's Encrypt certificates |
| `github.com/golang-jwt/jwt/v5` | JWT | Token generation/validation |
| `golang.org/x/crypto` | Crypto + SSH | bcrypt, encryption, SSH client |
| `golang.org/x/net` | Networking | WebSocket, HTTP/2 |
| `github.com/gorilla/websocket` | WebSocket | Real-time log/terminal streaming |
| `gopkg.in/yaml.v3` | YAML parser | docker-compose.yml & marketplace templates |

**Total: ~10 direct dependencies** (vs typical Go web app with 30-50+)

> **Note:** `golang.org/x/crypto` includes SSH client (`ssh` package), eliminating the need for a separate SSH library. VPS provider API calls use `net/http` from stdlib — no cloud SDKs needed.

### React Dependencies

Standard React stack — managed separately via npm, compiled into static files, embedded in Go binary via `embed.FS`.

---

## 26. BUILD & DISTRIBUTION

### 26.1 Build Command

```makefile
# Build everything
make build

# Steps:
# 1. Build React UI → web/dist/
# 2. Embed web/dist/ into Go binary via embed.FS
# 3. Cross-compile Go binary for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64

build:
	cd web && npm ci && npm run build
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" \
		-o bin/deploymonster ./cmd/deploymonster
```

### 26.2 Distribution

| Method | Command |
|--------|---------|
| Direct download | `curl -fsSL https://deploy.monster/install.sh \| bash` |
| Docker | `docker run -d -v /var/run/docker.sock:/var/run/docker.sock deploymonster/deploymonster` |
| GitHub Releases | Pre-built binaries for all platforms |
| Homebrew | `brew install deploy-monster/tap/deploymonster` |

### 26.3 System Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| RAM | 512 MB | 2 GB |
| Disk | 10 GB | 50 GB |
| OS | Linux (amd64/arm64) | Ubuntu 22.04+ |
| Docker | 24.0+ | Latest stable |

---

## 27. DEVELOPMENT PHASES

### Phase 1 — Foundation (v0.1.0)
- [ ] Core engine, module system, event bus
- [ ] SQLite + BBolt database layer
- [ ] Authentication (JWT + RBAC)
- [ ] REST API skeleton
- [ ] Docker container lifecycle (create, start, stop, delete, logs)
- [ ] Basic React UI shell (login, dashboard, app list)
- [ ] Docker image direct deploy (pull + run)

### Phase 2 — Ingress & SSL (v0.2.0)
- [ ] Reverse proxy with route matching
- [ ] Let's Encrypt ACME integration
- [ ] Service discovery via Docker labels
- [ ] Domain management UI
- [ ] SSL certificate management

### Phase 3 — Build & Deploy (v0.3.0)
- [ ] Universal Git source manager (GitHub, GitLab, Bitbucket, Gitea, Gogs, Azure, custom)
- [ ] OAuth2 flow for GitHub/GitLab/Bitbucket
- [ ] Token/SSH auth for Gitea/Gogs/custom
- [ ] Repository browser UI
- [ ] Project type auto-detection
- [ ] Dockerfile template generation
- [ ] Build pipeline with real-time logs
- [ ] Deploy strategies (recreate, rolling)
- [ ] Environment variable management
- [ ] Rollback engine

### Phase 4 — Webhooks & Auto-Deploy (v0.4.0)
- [ ] Universal webhook receiver endpoint
- [ ] Provider-specific payload parsers (GitHub, GitLab, Bitbucket, Gitea, etc.)
- [ ] Webhook signature verification (HMAC-SHA256, token)
- [ ] Auto-register webhooks via provider API
- [ ] Branch filtering, event filtering
- [ ] Auto-deploy on push (configurable)
- [ ] Manual approval queue
- [ ] Webhook delivery logs + redeliver

### Phase 5 — Docker Compose & Image Deploy (v0.5.0)
- [ ] docker-compose.yml parser (v2/v3 spec)
- [ ] Compose validation + variable interpolation
- [ ] Multi-service stack deployment with ordered startup
- [ ] Stack topology view in UI
- [ ] Per-service logs, exec, scale
- [ ] Docker image direct deploy UI (any registry)
- [ ] Registry authentication management
- [ ] compose override files support

### Phase 6 — Resource & Monitoring (v0.6.0)
- [ ] Server metrics collection
- [ ] Container metrics
- [ ] Alert engine
- [ ] Resource dashboard with charts
- [ ] Notification channels (email, Slack, Telegram, Discord)

### Phase 7 — Database & Backup (v0.7.0)
- [ ] Managed database provisioning
- [ ] Volume backup engine
- [ ] Database backup (pg_dump, mysqldump)
- [ ] Backup scheduling
- [ ] S3/SFTP storage targets
- [ ] Restore workflow

### Phase 8 — VPS Providers & Remote Servers (v0.8.0)
- [ ] VPS provider interface + Hetzner implementation
- [ ] DigitalOcean, Vultr, Linode, AWS EC2 providers
- [ ] Cloud-init auto-bootstrap (Docker + agent install)
- [ ] Custom SSH server connection
- [ ] SSH connection pool + tunneling
- [ ] Remote server metrics via agent
- [ ] Server provisioning UI (size/region picker)
- [ ] Auto-join Swarm cluster
- [ ] Server lifecycle (resize, snapshot, destroy)
- [ ] Cost overview dashboard

### Phase 9 — DNS & Topology (v0.9.0)
- [ ] Cloudflare DNS sync
- [ ] Auto-subdomain generation
- [ ] Topology canvas (React Flow drag & drop)
- [ ] Visual service connections
- [ ] docker-compose import to topology
- [ ] Load balancer strategies

### Phase 10 — Secret Vault & Registration (v0.10.0)
- [ ] Secret vault with AES-256-GCM encryption
- [ ] Secret scoping (Global → Tenant → Project → App)
- [ ] Secret versioning + rollback
- [ ] `${SECRET:name}` resolution at deploy time
- [ ] Secret masking in logs and UI
- [ ] `.env` file import/export
- [ ] Secret rotation → auto-redeploy
- [ ] Registration modes (open, invite-only, approval, disabled, SSO)
- [ ] Invite system (email with expiring tokens)
- [ ] Approval queue in admin panel
- [ ] SSO / OAuth login (Google, GitHub, GitLab, SAML)
- [ ] Onboarding wizard (first login flow)
- [ ] Password reset flow
- [ ] 2FA (TOTP)

### Phase 11 — Marketplace (v0.11.0)
- [ ] Marketplace engine + template YAML spec
- [x] 56 built-in templates embedded (v0.1.7); continued catalog growth tracked as a post-1.0 contribution goal
- [ ] Category browser, search, filter
- [ ] Config wizard (JSON Schema → form)
- [ ] One-click deploy pipeline
- [ ] Community template registry sync
- [ ] Template version tracking + update notifications
- [ ] "My installed" app management
- [ ] Marketplace admin panel (featured, categories)

### Phase 12 — Multi-Node & Swarm (v0.12.0)
- [ ] Docker Swarm integration
- [ ] Agent mode for worker nodes
- [ ] Node placement constraints
- [ ] Multi-node deploy distribution
- [ ] Overlay network management

### Phase 13 — Billing & Pay-Per-Usage (v0.13.0)
- [ ] Plan system (Free, Starter, Pro, Enterprise)
- [ ] Resource quotas per tenant (CPU, RAM, disk, bandwidth, apps, DBs)
- [ ] Usage metering (CPU·hr, RAM·hr, GB, bandwidth per minute)
- [ ] Docker cgroups enforcement (CPU quota, memory limit, pids limit)
- [ ] Soft limits (warnings) + hard limits (block at threshold)
- [ ] Invoice generation (monthly cycle, PDF with line items)
- [ ] Stripe integration (subscriptions, metered billing, customer portal)
- [ ] Overage pricing configuration
- [ ] Admin revenue dashboard (MRR, churn, per-plan breakdown)
- [ ] Customer billing UI (plan, usage bars, invoices, payment methods)
- [ ] Cost forecast (estimated next invoice)
- [ ] Public pricing page
- [ ] Trial period support
- [ ] Plan upgrade/downgrade with proration

### Phase 14 — Enterprise & White-Label (v0.14.0)
- [ ] White-label engine (logo, colors, domain, emails, copyright)
- [ ] Branding admin UI
- [ ] Custom email templates
- [ ] Custom subdomain suffix (*.cloud.acme.com)
- [ ] Reseller system (create reseller, wholesale pricing, isolation)
- [ ] Reseller dashboard
- [ ] Enterprise provisioning API (for WHMCS/automation)
- [ ] WHMCS provisioning module bridge
- [ ] HA support (Litestream SQLite replication)
- [ ] PostgreSQL backend option (for 1000+ tenants)
- [ ] GDPR compliance tools (data export, erasure, residency)
- [ ] Prometheus /metrics endpoint
- [ ] SLA management & reporting
- [ ] Enterprise license key system

### Phase 15 — Polish & Launch (v1.0.0)
- [ ] MCP server
- [ ] CLI tool
- [ ] Audit logging
- [ ] Documentation site
- [ ] Performance optimization + load testing
- [ ] Security audit
- [ ] Public beta launch

---

## 28. NON-FUNCTIONAL REQUIREMENTS

| Requirement | Target |
|------------|--------|
| Binary size | < 50 MB (Go + embedded React) |
| Memory usage (idle) | < 100 MB |
| Startup time | < 3 seconds |
| API response (p95) | < 100ms |
| Proxy latency added | < 5ms |
| SSL cert issuance | < 60 seconds |
| Build pipeline start | < 10 seconds |
| Max concurrent builds | 5 (configurable) |
| Max containers managed | 1000+ per node |
| WebSocket connections | 500+ concurrent |
| Database migrations | Automatic on startup |
| Zero-downtime upgrades | Yes, via rolling restart |

---

## 28A. ENTERPRISE & WHITE-LABEL (Hosting Provider Edition)

DeployMonster has two deployment modes:

| Mode | Target | License | Features |
|------|--------|---------|----------|
| **Community** | Individual developers, small teams | AGPL-3.0 | Full PaaS, single binary, all core features |
| **Enterprise** | Hosting providers, agencies, enterprises | Commercial | White-label, reseller, HA, priority support |

A hosting provider (e.g., Hetzner reseller, regional cloud, agency) should be able to take DeployMonster Enterprise and sell it as **their own branded product** — "AcmeCloud", "SuperHost Platform", "YourBrand Deploy" — with zero trace of DeployMonster visible to end customers.

### 28A.1 White-Label System

**Everything is rebrandable:**

```
┌──────────────────────────────────────────────────────────────┐
│  White-Label Configuration (Admin → Branding)                 │
│                                                               │
│  Brand Identity                                               │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  Product Name:  [AcmeCloud                         ] │    │
│  │  Logo (dark):   [Upload SVG/PNG] ← sidebar + login  │    │
│  │  Logo (light):  [Upload SVG/PNG] ← light theme      │    │
│  │  Favicon:       [Upload ICO/PNG]                     │    │
│  │  App Icon:      [Upload PNG 512x512] ← PWA icon     │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                               │
│  Colors                                                       │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  Primary:       [#2563EB] ← buttons, links, accents │    │
│  │  Accent:        [#7C3AED] ← highlights              │    │
│  │  Success:       [#16A34A] ← status indicators       │    │
│  │  Sidebar BG:    [#0F172A] ← dark sidebar            │    │
│  │  ☑ Auto-generate full palette from primary color     │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                               │
│  Domain & URLs                                                │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  Panel domain:  [cloud.acme.com                    ] │    │
│  │  API domain:    [api.cloud.acme.com                ] │    │
│  │  Docs URL:      [https://docs.acme.com/cloud       ] │    │
│  │  Support URL:   [https://support.acme.com           ] │    │
│  │  Status page:   [https://status.acme.com            ] │    │
│  │  Terms URL:     [https://acme.com/terms             ] │    │
│  │  Privacy URL:   [https://acme.com/privacy           ] │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                               │
│  Email & Communication                                        │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  From name:     [AcmeCloud                         ] │    │
│  │  From email:    [noreply@cloud.acme.com            ] │    │
│  │  Support email: [support@acme.com                  ] │    │
│  │  Email templates: [Upload custom HTML templates]     │    │
│  │  ☑ Remove all DeployMonster references from emails   │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                               │
│  Footer & Legal                                               │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  Copyright:     [© 2026 Acme Inc. All rights reserved]│    │
│  │  Footer links:  [Custom links configuration         ] │    │
│  │  ☑ Hide "Powered by DeployMonster"                    │    │
│  └──────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
```

**What gets white-labeled:**
- Login/register pages — custom logo, colors, product name
- Dashboard sidebar — custom logo, brand colors
- All email templates — invites, alerts, invoices, password reset
- Invoice PDFs — custom header, logo, company info, tax ID
- CLI tool — configurable binary name (e.g., `acmecloud` instead of `deploymonster`)
- Error pages — custom 404, 500, maintenance pages
- Documentation links — point to your own docs
- PWA manifest — custom app name, icons for mobile
- Browser tab title — "AcmeCloud Dashboard" not "DeployMonster"
- Auto-subdomain suffix — `*.cloud.acme.com` instead of `*.deploy.monster`

### 28A.2 Reseller / Multi-Tier Architecture

For hosting providers who want to sell to other resellers or agencies:

```
┌──────────────────────────────────────────────────────────────┐
│                    Platform Owner (You)                        │
│                    (Super Admin)                               │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  • Manages infrastructure (servers, VPS providers)    │    │
│  │  • Sets global plans & pricing                        │    │
│  │  • White-label configuration                          │    │
│  │  • Manages resellers                                  │    │
│  │  • Revenue overview                                   │    │
│  └──────────────────────────────────────────────────────┘    │
│                          │                                    │
│              ┌───────────┼───────────┐                        │
│              ▼           ▼           ▼                        │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐         │
│  │  Reseller A   │ │  Reseller B  │ │  Direct       │        │
│  │  (Agency)     │ │  (MSP)       │ │  Customers    │        │
│  │──────────────│ │──────────────│ │──────────────│         │
│  │ Own branding  │ │ Own branding │ │ Platform      │        │
│  │ Own pricing   │ │ Own pricing  │ │ branding      │        │
│  │ Own customers │ │ Own customers│ │               │        │
│  │ Own domain    │ │ Own domain   │ │               │        │
│  │ Commission %  │ │ Commission % │ │               │        │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘        │
│         │                │                │                   │
│    ┌────┴────┐     ┌────┴────┐     ┌────┴────┐              │
│    ▼         ▼     ▼         ▼     ▼         ▼              │
│  End      End    End       End   End       End              │
│  Users    Users  Users     Users Users     Users            │
│  (tenant) (tenant)(tenant) (tenant)(tenant)(tenant)         │
└──────────────────────────────────────────────────────────────┘
```

**Reseller Features:**
- **Own branding** — Each reseller gets their own white-label config
- **Own domain** — `cloud.reseller-a.com`, custom SSL
- **Own pricing** — Reseller sets their own plans/prices (must be ≥ wholesale)
- **Wholesale pricing** — Platform owner sets cost price, reseller sets retail
- **Commission tracking** — Automatic commission calculation
- **Resource allocation** — Platform owner allocates server pool to reseller
- **Customer isolation** — Reseller's customers can't see other resellers' customers
- **Reseller dashboard** — Revenue, customer count, resource usage

### 28A.3 Enterprise High Availability

```
┌─────────────────────────────────────────────────────────────────┐
│  Manager #1 (Primary) ←→ Manager #2 (Standby) ←→ Manager #3   │
│  SQLite (Litestream replication to S3)                          │
│  External LB (Cloudflare / HAProxy) routes to healthy manager   │
│  Workers: N compute nodes in Swarm cluster                      │
└─────────────────────────────────────────────────────────────────┘
```

**Enterprise PostgreSQL Option:** For 1000+ tenants, swap embedded SQLite for external PostgreSQL with connection pooling, streaming replication, and PITR.

### 28A.4 SLA & Compliance

- **SLA tiers** — 99.5% (basic), 99.9% (pro), 99.95% (enterprise) with auto credit calculation
- **GDPR** — Data residency constraints, right to erasure, data export, DPO config, consent management
- **SOC 2 ready** — Audit logging, encryption at rest/transit, access control, session management
- **Vulnerability scanning** — Trivy integration for container image scanning

### 28A.5 Enterprise Integrations

| Integration | Purpose |
|-------------|---------|
| WHMCS / Blesta | Hosting billing panel bridge (provision/suspend/terminate) |
| Freshdesk / Zendesk | Support ticket integration |
| PagerDuty / OpsGenie | Incident escalation |
| Okta / Azure AD / LDAP | Enterprise SSO federation |
| Prometheus + Grafana | External monitoring (OpenMetrics `/metrics` endpoint) |
| Terraform Provider | `terraform-provider-deploymonster` for IaC |

### 28A.6 Licensing

| Tier | Price | Target | Key Features |
|------|-------|--------|-------------|
| Community | Free (AGPL-3.0) | Individual devs | All core features, must open-source mods |
| Pro | $49/mo per server | Small providers | White-label, custom email, priority support |
| Enterprise | $299/mo | Hosting providers | Reseller, HA, PostgreSQL, WHMCS, SAML, GDPR |
| Enterprise+ | Custom | Large providers | Source access, custom dev, dedicated engineer |

### 28A.7 Hosting Provider Scenario

**"AcmeCloud" — Regional hosting provider uses DeployMonster Enterprise:**

```
Setup: 5 Hetzner servers + DeployMonster Enterprise + Stripe EUR
Brand: cloud.acme.com (zero DeployMonster mention)
Plans: Starter €4.99 | Business €19.99 | Enterprise €99.99

Result: 200 customers × €19.99 avg = €3,998/mo revenue
Cost:   5 servers (€200) + license (€299) + Stripe (€120) = €619/mo
Profit: ~€3,379/mo from 5 servers
```

### 28A.8 Enterprise Config (monster.yaml)

```yaml
enterprise:
  enabled: true
  license_key: "${MONSTER_LICENSE_KEY}"
  
  white_label:
    product_name: "AcmeCloud"
    logo_dark: "/var/lib/deploymonster/branding/logo-dark.svg"
    logo_light: "/var/lib/deploymonster/branding/logo-light.svg"
    favicon: "/var/lib/deploymonster/branding/favicon.ico"
    primary_color: "#2563EB"
    accent_color: "#7C3AED"
    panel_domain: "cloud.acme.com"
    auto_subdomain_suffix: "cloud.acme.com"
    support_email: "support@acme.com"
    terms_url: "https://acme.com/terms"
    copyright: "© 2026 Acme Inc."
    hide_powered_by: true
    
  reseller:
    enabled: false
    wholesale_discount_pct: 30
    
  ha:
    enabled: true
    replication: litestream
    litestream_s3_url: "s3://dm-replication/db"
    
  compliance:
    gdpr_enabled: true
    data_retention_days: 365
    dpo_email: "dpo@acme.com"
    
  integrations:
    whmcs:
      enabled: false
      api_url: "https://billing.acme.com/includes/api.php"
      api_identifier: "${WHMCS_API_ID}"
      api_secret: "${WHMCS_API_SECRET}"
    prometheus:
      enabled: true
      metrics_path: "/metrics"
```

---

## 29. FUTURE ROADMAP (Post v1.0)

- **Kubernetes support** — Optional K8s backend alongside Swarm
- **Edge functions** — Cloudflare Workers-like edge compute
- **CI/CD pipelines** — Built-in test → build → deploy pipeline with stages
- **Team collaboration** — Real-time multi-user topology editing (CRDT)
- **Plugin system** — Third-party module marketplace (custom modules in Go/WASM)
- **Mobile app** — iOS/Android management app (React Native)
- **Geographic routing** — Route by client location (geo-aware LB)
- **Auto-scaling** — Scale based on metrics/load (CPU/RAM triggers)
- **Log aggregation** — Built-in ELK-like log search across all containers
- **Multi-cloud networking** — Overlay networks across providers (WireGuard mesh)
- **Terraform provider** — `terraform-provider-deploymonster` for IaC
- **GitOps mode** — Declare infrastructure in Git, auto-sync
- **Active-active HA** — Multi-region active-active with conflict resolution
- **Marketplace revenue sharing** — Template authors earn from installs
- **AI assistant** — Chat with your infrastructure ("deploy my app to prod", "why is latency high")

---

*This specification is the single source of truth for DeployMonster development. All implementation decisions must align with this document. When in doubt, refer back to the core principles in Section 1.*
