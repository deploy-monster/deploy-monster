# DeployMonster - Development Guidelines

## Project Overview
DeployMonster is a self-hosted PaaS (Platform as a Service) — single binary, modular monolith, event-driven architecture with embedded React UI. Replaces Coolify/Dokploy/CapRover with enterprise-grade features.

## Architecture
- **Backend**: Go 1.26+ modular monolith, 20 modules auto-registered via `init()`
- **Frontend**: React 19 + Vite 8 + Tailwind CSS 4 + Zustand 5 (embedded via `embed.FS`)
- **Database**: SQLite (`modernc.org/sqlite` pure Go) + BBolt KV — PostgreSQL-ready via `core.Store` interface
- **Module System**: Every feature implements `core.Module` interface (ID, Init, Start, Stop, Health, Routes, Events)
- **Event System**: In-process pub/sub with sync/async handlers, prefix matching, typed payloads
- **Master/Agent**: Same binary runs as master (full platform) or agent (worker node)

## Key Interfaces (in `core/`)
- `Store` — DB-agnostic repository. SQLite default, PostgreSQL planned. Never use `*db.SQLiteDB` directly
- `ContainerRuntime` — Docker operations. Local or remote via agent
- `NodeExecutor` — Master/agent transparent execution
- `VPSProvisioner` — Hetzner, DigitalOcean, Vultr, custom SSH
- `BackupStorage` — Local, S3/MinIO/R2
- `DNSProvider` — Cloudflare, extensible
- `GitProvider` — GitHub, GitLab, Gitea
- `SecretResolver` — AES-256-GCM vault with `${SECRET:name}` syntax
- `NotificationSender` — Slack, Discord, Telegram, webhook
- `OutboundWebhookSender` — HMAC-signed HTTP deliveries

## Module Registration
Modules use `init()` + `core.RegisterModule()` pattern. Main.go imports them with `_`. Dependency order is resolved automatically via topological sort.

## Commands
- `make build` — Build Go binary with embedded UI
- `make dev` — Run in development mode
- `make test` — Run all tests
- `make lint` — Run golangci-lint
- `scripts/build.sh` — Full pipeline: React build → embed copy → Go build with ldflags

## CLI
```
deploymonster              Start server (default)
deploymonster serve        Start server explicitly
deploymonster serve --agent  Start as agent/worker node
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

## Project Documentation
- `·project/SPECIFICATION.md` — Full product specification
- `·project/IMPLEMENTATION.md` — Implementation patterns and code examples
- `·project/TASKS.md` — Ordered task checklist (223 tasks, 15 phases)
