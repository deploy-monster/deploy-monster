# Changelog

All notable changes to DeployMonster will be documented in this file.

## [1.6.0] - 2026-03-29

### Highlights
- **Visual Topology Editor** — Railway-style drag-and-drop infrastructure designer with React Flow
- **5 component types** — Apps, Databases, Domains, Volumes, Workers
- **Wire connections** — Connect components visually with dependency tracking
- **One-click deploy** — Deploy entire topology from canvas

### Added
- Topology Editor page with React Flow canvas (`/topology`)
- Custom node components: AppNode, DatabaseNode, DomainNode, VolumeNode, WorkerNode
- Component palette for drag-and-drop infrastructure design
- Configuration panel for selected node properties
- Topology deployment API (`POST /api/v1/topology/deploy`)
- Auto-layout feature using dagre algorithm
- Environment selector (production, staging, development)
- `@xyflow/react` and `dagre` npm dependencies

---

## [1.5.0] - 2026-03-29

### Highlights
- **116 marketplace templates** — 60+ new apps added (Grafana, Keycloak, Home Assistant, etc.)
- **Repository migrated** to `github.com/deploy-monster/deploy-monster`
- **Competitive positioning** — Full comparison table vs Coolify, Dokploy, CapRover, Railway
- **Admin roles documentation** — System Admin vs Client Admin clarified

### Added
- 60+ marketplace templates across 15 categories
- Competitive comparison table in README
- Multi-tenancy documentation with admin role examples
- System Admin vs Client Admin role distinction

### Changed
- Repository URL: `github.com/deploy-monster/deploy-monster`
- GoReleaser config updated for new org
- Docker image: `ghcr.io/deploy-monster/deploymonster`
- All documentation URLs updated

### Categories (New Templates)
- **CMS**: Drupal, Strapi, Payload CMS
- **E-commerce**: Medusa, PrestaShop, Sylius
- **Monitoring**: Grafana, Prometheus, Loki, Tempo, Jaeger, cAdvisor
- **Communication**: Matrix Synapse, Rocket.Chat, Mattermost, Zulip
- **Media**: Jellyfin, Immich, Navidrome, PhotoPrism, Audiobookshelf
- **Productivity**: Paperless-NGX, BookStack, Wiki.js, Outline, NocoDB, Baserow
- **Security**: Keycloak, Authentik, Authelia, Portainer
- **AI/ML**: Open WebUI, LocalAI, Stable Diffusion
- **Automation**: Node-RED, ActivePieces, Huginn, Trigger.dev
- **DevTools**: GitLab CE, Gogs, Drone CI, Woodpecker CI, IT Tools
- **Storage**: Seafile, File Browser, ProjectSend
- **Analytics**: Umami, Matomo
- **Finance**: Actual Budget, Ghostfolio
- **IoT**: Home Assistant
## [0.1.0] - 2026-03-24

### Highlights
- **97% avg test coverage** across 34 packages (up from 92.8%)
- **Comprehensive ARCHITECTURE.md** with ASCII diagrams
- **247 Go test files** (up from 194)

---

## [1.4.0] - 2026-03-27

### Highlights
- **97% avg test coverage** across 34 packages (up from 92.8%)
- **Comprehensive ARCHITECTURE.md** with ASCII diagrams
- **247 Go test files** (up from 194)

### Added
- ARCHITECTURE.md with system diagrams, module dependencies, event taxonomy
- Resource module `collectOnce()` method for testability
- Webhooks Trigger error path test coverage
- Ingress ACME manager checkRenewals/issueCertificate tests

### Changed
- Updated README with ECOSTACK TECHNOLOGY OÜ branding
- Added creator info (Ersin KOÇ) with TR/EE context
- Updated Docker image paths to deploy-monster org
- Consolidated .gitignore patterns

### Fixed
- Resource collectionLoop coverage via extracted method
- Gitignore now properly ignores *.test, *.tmp, *.log files

## [1.3.0] - 2026-03-25

### Highlights
- **92.8% avg test coverage** across 20 packages (3 at 100%)
- **194 Go test files** + 6 React test files (50 tests)
- **115/115 handlers** wired to real services (zero placeholders)
- **Enterprise-grade UI** with shadcn/ui, hover transitions, micro-interactions

### Added
- Container exec API (POST /apps/{id}/exec) with real Docker SDK
- Container stats API (real-time CPU/RAM/network/IO per container)
- Docker image management (pull, list, remove, cleanup dangling)
- Docker network and volume listing APIs
- Deploy pipeline: webhook → build → deploy orchestration
- BBolt KV persistence for 30+ config/state buckets
- React component tests: Button, Card, Badge, Input (50 tests total)
- 7 Go fuzz tests for security-critical packages
- 38 Go benchmark functions
- OpenAPI 3.0.3 specification (docs/openapi.yaml)

### Changed
- All 115 handlers now use real services (SQLite Store, BBolt KV, Docker SDK)
- React UI completely redesigned with shadcn/ui components
- Login: gradient branding, glass-effect features, password toggle
- Dashboard: greeting banner, stat cards with trends, quick actions
- Marketplace: category-colored icons, Featured badges, deploy dialog
- Sidebar: collapsible groups, glow logo, theme toggle, Cmd+B shortcut
- All 19 pages: hover transitions, skeleton loading, rich empty states

### Fixed
- Compose parser nil pointer dereference (found by fuzzing)
- Marketplace nil pointer (module init order dependency)
- useApi hook double response unwrapping
- audit_log table name mismatch (audit_logs → audit_log)

## [0.1.0] - 2026-03-24

### Added

#### Core Platform
- Module system with auto-registration via `init()` and topological dependency sort
- EventBus with sync/async handlers, prefix matching, typed payloads
- Store interface (DB-agnostic) — SQLite default, PostgreSQL ready
- Services registry for cross-module communication
- Agent protocol for master/worker architecture
- Core scheduler for recurring tasks
- Configuration validation on startup
- ASCII art startup banner with system info

#### API (223 endpoints)
- Authentication: JWT, refresh tokens, 2FA TOTP, SSO OAuth (Google, GitHub)
- Applications: full CRUD, deploy, scale, rollback, clone, suspend/resume, transfer
- Deployments: versioning, diff, preview, scheduling, approval workflow
- Docker Compose: YAML parser, interpolation, dependency-ordered stack deploy
- Domains: CRUD, DNS verification, SSL status check, wildcard certificates
- Databases: managed PostgreSQL, MySQL, MariaDB, Redis, MongoDB provisioning
- Backups: local + S3 storage, cron scheduler, retention policies
- Secrets: AES-256-GCM vault with Argon2id KDF, ${SECRET:name} resolver
- Servers: Hetzner, DigitalOcean, Vultr, Linode, Custom SSH providers
- Git sources: GitHub, GitLab, Gitea, Bitbucket API providers
- Team: RBAC (6 roles), invitations, audit log
- Marketplace: 25 one-click templates
- Billing: plans, usage metering, Stripe client, quota enforcement
- MCP: 9 AI-callable tools with HTTP transport
- Admin: system info, tenants, branding, license, updates, API keys
- Monitoring: metrics, alerts, Prometheus /metrics endpoint

#### Networking
- Custom reverse proxy (no Traefik/Nginx dependency)
- ACME certificate manager with auto-renewal
- 5 load balancer strategies (round-robin, least-conn, IP-hash, random, weighted)
- Service discovery via Docker label watcher
- Backend health checking (HTTP/TCP)
- Per-app middleware: rate limiting, CORS, compression, basic auth, headers

#### Build Engine
- 14 project type auto-detection
- 12 Dockerfile templates (Node.js, Next.js, Go, Python, Rust, PHP, Java, .NET, Ruby, Vite, Nuxt, static)
- Git clone pipeline with token injection
- Concurrent build worker pool

#### Security
- AES-256-GCM secret encryption with Argon2id key derivation
- Per-app IP whitelist/denylist
- Request ID tracing on every request
- API key authentication (X-API-Key header)
- Audit logging middleware
- Quota enforcement middleware
- Request body size limiting (10MB)
- Request timeout (30s)
- GDPR data export and right to erasure

#### React UI (19 pages)
- Login, Register, Onboarding (5-step wizard)
- Dashboard with real-time stats, activity feed, announcements
- Applications: list (auto-refresh), detail (6 tabs), deploy wizard
- Marketplace with deploy dialog and config vars
- Databases, Servers, Domains, Settings (functional)
- Team (members, roles, audit log), Billing (plan comparison)
- Git Sources, Backups, Secrets, Admin (3 tabs)
- 404 page, error boundary, toast notifications
- CMD+K global search, dark/light/system theme
- Pagination component, loading spinners

#### Operations
- Single binary (22MB) with embedded React UI
- CLI: serve, version, config, init, --agent
- GitHub Actions CI pipeline
- GoReleaser configuration
- curl | bash installer script
- Docker HEALTHCHECK
- monster.example.yaml template

### Performance
- RoundRobin LB: 3.6 ns/op (0 allocations)
- IPHash LB: 26 ns/op (0 allocations)
- LeastConn LB: 55 ns/op (0 allocations)
- JWT Generate: 4.1 μs/op
- JWT Validate: 4.2 μs/op
- AES-256 Encrypt: 633 ns/op
- AES-256 Decrypt: 489 ns/op
- Compose Parse: 17.6 μs/op
- SQLite GetApp: 41 μs/op
