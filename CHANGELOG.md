# Changelog

All notable changes to DeployMonster will be documented in this file.

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
