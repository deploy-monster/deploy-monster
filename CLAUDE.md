# DeployMonster - Development Guidelines

## Project Overview
DeployMonster is a self-hosted PaaS (Platform as a Service) — single binary, modular monolith, event-driven architecture with embedded React UI.

## Architecture
- **Backend**: Go 1.26+ modular monolith, 20 modules auto-registered via `init()`
- **Frontend**: React 19 + Vite 8 + Tailwind CSS 4 + shadcn/ui + Zustand 5 (embedded via `embed.FS`)
- **Database**: SQLite (`modernc.org/sqlite` pure Go) + BBolt KV (30+ buckets) — PostgreSQL-ready via `core.Store` interface
- **Module System**: Every feature implements `core.Module` interface (ID, Init, Start, Stop, Health, Routes, Events)
- **Event System**: In-process pub/sub with sync/async handlers, prefix matching, typed payloads
- **Master/Agent**: Same binary runs as master (full platform) or agent (worker node)
- **Deploy Pipeline**: Webhook → Build (14 detectors, 12 Dockerfiles) → Deploy (recreate/rolling)

## Key Interfaces (in `core/`)
- `Store` — DB-agnostic repository (10 sub-interfaces: Tenant, User, App, Deployment, Domain, Project, Role, Audit, Secret, Invite). SQLite default, PostgreSQL planned. Never use `*db.SQLiteDB` directly
- `ContainerRuntime` — Docker operations (Exec, Stats, Images, Networks, Volumes). Local or remote via agent
- `BoltStorer` — BBolt KV operations (Set, Get, Delete, List, Close). Used for config, state, metrics
- `VPSProvisioner` — Hetzner, DigitalOcean, Vultr, Linode, custom SSH
- `BackupStorage` — Local, S3/MinIO/R2
- `DNSProvider` — Cloudflare, extensible
- `GitProvider` — GitHub, GitLab, Gitea, Bitbucket
- `SecretResolver` — AES-256-GCM vault with `${SECRET:name}` syntax
- `NotificationSender` — Slack, Discord, Telegram, webhook
- `OutboundWebhookSender` — HMAC-signed HTTP deliveries

## Module Registration
Modules use `init()` + `core.RegisterModule()` pattern. Main.go imports them with `_`. Dependency order is resolved automatically via topological sort. API module depends on all other modules to ensure correct init order.

## Project Stats
- **86K+ LOC** (27K Go source + 47K Go tests + 12K React)
- **224 API endpoints** — 115 handlers, all wired to real services (0 placeholders)
- **92.8% avg test coverage** — 194 Go test files, 6 React test files, 7 fuzz tests
- **22MB binary** (16MB stripped) with embedded React UI
- **20 modules**, 25 marketplace templates, 30+ BBolt buckets

## Commands
- `make build` — Build Go binary with embedded UI
- `make dev` — Run in development mode
- `make test` — Run all tests (194 files, 20 suites)
- `make lint` — Run golangci-lint
- `make bench` — Run benchmarks (38 functions)
- `scripts/build.sh` — Full pipeline: React build → embed copy → Go build with ldflags

## CLI
```
deploymonster              Start server (default)
deploymonster serve        Start server explicitly
deploymonster serve --agent  Start as agent/worker node
deploymonster init         Generate monster.yaml config
deploymonster version      Show version info
deploymonster config       Validate and display config
```

## Code Conventions
- Use `log/slog` for structured logging
- Use `context.Context` everywhere
- Error wrapping: `fmt.Errorf("context: %w", err)`
- No global state — dependency injection via `core.Core`
- Interfaces defined where consumed
- Table-driven tests
- All data access via `core.Store` interface, never concrete DB types
- BBolt KV for per-app/per-user config and state persistence
- All handlers must use real services (no static/placeholder responses)
- React: shadcn/ui components, `cn()` utility, `@/` path aliases, `useApi` hook

## Project Documentation
- `·project/SPECIFICATION.md` — Full product specification
- `·project/IMPLEMENTATION.md` — Implementation patterns and code examples
- `·project/TASKS.md` — Ordered task checklist (251 tasks, 15 phases, 100% complete)
- `docs/openapi.yaml` — OpenAPI 3.0.3 specification
- `docs/examples/api-quickstart.md` — API usage examples with curl
