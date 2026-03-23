# DeployMonster - Development Guidelines

## Project Overview
DeployMonster is a self-hosted PaaS (Platform as a Service) written in Go 1.23+. Single binary, modular monolith, event-driven architecture with embedded React UI.

## Architecture
- **Backend**: Go 1.23+ modular monolith
- **Frontend**: React 19 + Vite + Tailwind v4 + shadcn/ui (embedded via `embed.FS`)
- **Database**: SQLite (`modernc.org/sqlite` pure Go) + BBolt (KV store)
- **Module System**: Every feature is a module implementing `core.Module` interface
- **Event System**: In-process pub/sub event bus
- **API**: REST + WebSocket, JWT + API Key auth

## Key Conventions
- Go module path: `github.com/deploy-monster/deploy-monster`
- All business logic lives in `internal/` packages
- Each module has its own package under `internal/`
- Module registration happens in `internal/core/app.go`
- Database migrations are embedded SQL files in `internal/db/migrations/`
- Config loaded from `monster.yaml` with env var overrides

## Commands
- `make build` — Build Go binary with embedded UI
- `make dev` — Run in development mode
- `make test` — Run all tests
- `make lint` — Run golangci-lint
- `make clean` — Clean build artifacts

## Code Style
- Follow standard Go conventions (gofmt, effective Go)
- Use `log/slog` for structured logging
- Use `context.Context` for cancellation/timeouts
- Error wrapping with `fmt.Errorf("context: %w", err)`
- No global state — pass dependencies via constructors
- Interfaces defined where consumed, not where implemented
- Table-driven tests preferred

## Module Pattern
Every module must:
1. Implement `core.Module` interface (ID, Name, Version, Dependencies, Init, Start, Stop, Health, Routes, Events)
2. Have a `New()` constructor function
3. Register itself in `internal/core/app.go`
4. Use dependency injection via `core.Core` reference

## Project Documentation
- `·project/SPECIFICATION.md` — Full product specification
- `·project/IMPLEMENTATION.md` — Implementation patterns and code examples
- `·project/TASKS.md` — Ordered task checklist (223 tasks, 15 phases)

## Current Phase
Phase 1 — Foundation (v0.1.0): Core engine, DB, auth, API, Docker integration, React UI shell
