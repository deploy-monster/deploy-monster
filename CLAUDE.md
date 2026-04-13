# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview
DeployMonster is a self-hosted PaaS (Platform as a Service) — single binary, modular monolith, event-driven architecture with embedded React UI.

## Build & Run Commands
```bash
make build              # Build Go binary with embedded UI
make dev                # Run in development mode (go run)
make test               # Run all tests with race detection + coverage
make test-short         # Run short tests only (skip integration)
make lint               # Run golangci-lint
make bench              # Run benchmarks
make fmt                # Format Go code
make vet                # Run go vet
make coverage           # Generate HTML coverage report
scripts/build.sh        # Full pipeline: React build → embed copy → Go build with ldflags
```

### Running a single test or package
```bash
go test -run TestFunctionName ./internal/deploy/...
go test -run TestFunctionName/subtestname ./internal/deploy/...
go test -v ./internal/build/...          # All tests in a package
go test -bench BenchmarkName ./internal/core/...
go test -fuzz FuzzName ./internal/auth/...
```

### Frontend (web/)
```bash
cd web && pnpm install && pnpm run build   # Build React UI
cd web && pnpm run dev                     # Dev server with HMR
cd web && pnpm run lint                    # ESLint
cd web && pnpm vitest run                  # Run tests
cd web && pnpm test:e2e                    # Playwright E2E (requires running server on :8443)
cd web && pnpm test:e2e --ui              # Playwright with interactive UI
```

## Architecture

### Backend: Go 1.26+ Modular Monolith
- **20 modules** auto-registered via `init()` + `core.RegisterModule()` in each module's `module.go`
- `cmd/deploymonster/main.go` imports all modules with blank `_` imports
- Dependency order resolved via topological sort on `Dependencies()` return values
- Graceful shutdown in reverse dependency order (30s timeout)
- Same binary runs as **master** (full platform) or **agent** (worker node via `--agent` flag)

### Module Lifecycle
Every module implements `core.Module`: ID, Name, Version, Dependencies, Init, Start, Stop, Health, Routes, Events.
```
Init(ctx, core) → receives *core.Core with Store, Config, EventBus, Logger, Services
Start(ctx)      → begin background workers, subscribe to events
Stop(ctx)       → graceful shutdown
Health()        → return HealthOK, HealthDegraded, or HealthDown
Routes()        → return []Route for API registration
Events()        → return []EventHandler for event subscriptions
```

### Key Interfaces (in `internal/core/`)
- `Store` — DB-agnostic repository composing 12 sub-interfaces (TenantStore, UserStore, AppStore, DeploymentStore, DomainStore, ProjectStore, RoleStore, AuditStore, SecretStore, InviteStore, UsageRecordStore, BackupStore). **Never use `*db.SQLiteDB` directly.**
- `ContainerRuntime` — Docker operations (CreateAndStart, Stop, Remove, Restart, Logs, Exec, Stats, ImagePull, etc.)
- `BoltStorer` — BBolt KV (Set, Get, Delete, List, Close). Used for config, state, metrics, API keys, webhook secrets
- `Services` — Factory registry for pluggable providers (DNS, Backup, VPS, Git)
- Other: `VPSProvisioner`, `BackupStorage`, `DNSProvider`, `GitProvider`, `SecretResolver`, `NotificationSender`, `OutboundWebhookSender`

### Event System (`internal/core/events.go`)
In-process pub/sub with the EventBus on `core.Core`:
- `Subscribe(eventType, handler)` — sync, `SubscribeAsync(eventType, handler)` — fire-and-forget
- `Publish(ctx, event)` — emit and wait for sync handlers
- `Emit(eventType, source, data)` / `EmitWithTenant(eventType, source, tenantID, userID, data)`
- **Matching**: exact (`"app.created"`), prefix (`"app.*"`), wildcard (`"*"`)
- **Naming convention**: `{domain}.{action}` — e.g. `app.deployed`, `build.started`, `domain.verified`, `container.died`, `system.started`

### API Layer (`internal/api/`)
- Go 1.22+ `http.ServeMux` with `METHOD /path` pattern syntax
- Middleware chain: RequestID → APIVersion → BodyLimit(10MB) → Timeout(30s) → Recovery → RequestLogger → CORS → AuditLog
- Auth: JWT Bearer token OR X-API-Key header. Auth levels: AuthNone, AuthAPIKey, AuthJWT, AuthAdmin, AuthSuperAdmin
- JWT: HS256, access=15min, refresh=7days. Claims: UserID, TenantID, RoleID, Email
- Webhooks: `POST /hooks/v1/{webhookID}` with HMAC signature verification

### Database
- **SQLite** (`modernc.org/sqlite` pure Go) — default, file-based
- **BBolt KV** — 30+ buckets for config, state, metrics, API keys
- **PostgreSQL** — planned via `core.Store` interface (enterprise)
- All data access through `core.Store` interface only

### Deploy Pipeline
Webhook received → Git clone → Build (14 language detectors, 12 Dockerfiles) → Deploy (recreate/rolling strategy)

### Frontend: React 19 + Vite 8 + TypeScript
- Embedded via `embed.FS` — built React copied to `internal/api/static/`
- **State**: Zustand 5 stores (`web/src/stores/`) — no TanStack Query; data-fetch state lives in the custom `useApi` hook family (see line below)
- **Routing**: React Router v7 with lazy-loaded pages
- **API client**: `web/src/api/client.ts` — base `/api/v1`, auto token refresh on 401
- **Hooks**: `useApi<T>(path)` for GET, `useMutation<TInput, TOutput>(method, path)` for mutations, `usePaginatedApi<T>(path, perPage)`
- **Components**: shadcn/ui patterns, `cn()` utility, `@/` path aliases (→ `./src/*`)

## Config
YAML file `monster.yaml` + environment variable overrides (`MONSTER_*` prefix). Key sections: Server, Database, Ingress, ACME, DNS, Docker, Backup, Notifications, Swarm, VPSProviders, GitSources, Marketplace, Registration, SSO, Secrets, Billing, Limits, Enterprise.

## Code Conventions
- `log/slog` for structured logging, always with `"module"` key
- `context.Context` as first parameter everywhere
- Error wrapping: `fmt.Errorf("context: %w", err)`
- No global state — dependency injection via `core.Core`
- Interfaces defined where consumed
- Table-driven tests with `t.Run` subtests
- Mocks: implement interface with optional function fields + call tracking booleans (see `internal/deploy/mock_test.go`)
- React: shadcn/ui components, `cn()` utility, `@/` path aliases, `useApi` hook

## Project Documentation
- `.project/SPECIFICATION.md` — Full product specification
- `.project/IMPLEMENTATION.md` — Implementation patterns and code examples
- `.project/TASKS.md` — Ordered task checklist (251 tasks, 15 phases, 100% complete)
- `docs/openapi.yaml` — OpenAPI 3.0.3 specification
- `docs/examples/api-quickstart.md` — API usage examples with curl

<!-- rtk-instructions v2 -->
# RTK (Rust Token Killer) - Token-Optimized Commands

## Golden Rule

**Always prefix commands with `rtk`**. If RTK has a dedicated filter, it uses it. If not, it passes through unchanged. This means RTK is always safe to use.

**Important**: Even in command chains with `&&`, use `rtk`:
```bash
# ❌ Wrong
git add . && git commit -m "msg" && git push

# ✅ Correct
rtk git add . && rtk git commit -m "msg" && rtk git push
```

## RTK Commands by Workflow

### Build & Compile (80-90% savings)
```bash
rtk cargo build         # Cargo build output
rtk cargo check         # Cargo check output
rtk cargo clippy        # Clippy warnings grouped by file (80%)
rtk tsc                 # TypeScript errors grouped by file/code (83%)
rtk lint                # ESLint/Biome violations grouped (84%)
rtk prettier --check    # Files needing format only (70%)
rtk next build          # Next.js build with route metrics (87%)
```

### Test (90-99% savings)
```bash
rtk cargo test          # Cargo test failures only (90%)
rtk vitest run          # Vitest failures only (99.5%)
rtk playwright test     # Playwright failures only (94%)
rtk test <cmd>          # Generic test wrapper - failures only
```

### Git (59-80% savings)
```bash
rtk git status          # Compact status
rtk git log             # Compact log (works with all git flags)
rtk git diff            # Compact diff (80%)
rtk git show            # Compact show (80%)
rtk git add             # Ultra-compact confirmations (59%)
rtk git commit          # Ultra-compact confirmations (59%)
rtk git push            # Ultra-compact confirmations
rtk git pull            # Ultra-compact confirmations
rtk git branch          # Compact branch list
rtk git fetch           # Compact fetch
rtk git stash           # Compact stash
rtk git worktree        # Compact worktree
```

Note: Git passthrough works for ALL subcommands, even those not explicitly listed.

### GitHub (26-87% savings)
```bash
rtk gh pr view <num>    # Compact PR view (87%)
rtk gh pr checks        # Compact PR checks (79%)
rtk gh run list         # Compact workflow runs (82%)
rtk gh issue list       # Compact issue list (80%)
rtk gh api              # Compact API responses (26%)
```

### JavaScript/TypeScript Tooling (70-90% savings)
```bash
rtk pnpm list           # Compact dependency tree (70%)
rtk pnpm outdated       # Compact outdated packages (80%)
rtk pnpm install        # Compact install output (90%)
rtk npm run <script>    # Compact npm script output
rtk npx <cmd>           # Compact npx command output
rtk prisma              # Prisma without ASCII art (88%)
```

### Files & Search (60-75% savings)
```bash
rtk ls <path>           # Tree format, compact (65%)
rtk read <file>         # Code reading with filtering (60%)
rtk grep <pattern>      # Search grouped by file (75%)
rtk find <pattern>      # Find grouped by directory (70%)
```

### Analysis & Debug (70-90% savings)
```bash
rtk err <cmd>           # Filter errors only from any command
rtk log <file>          # Deduplicated logs with counts
rtk json <file>         # JSON structure without values
rtk deps                # Dependency overview
rtk env                 # Environment variables compact
rtk summary <cmd>       # Smart summary of command output
rtk diff                # Ultra-compact diffs
```

### Infrastructure (85% savings)
```bash
rtk docker ps           # Compact container list
rtk docker images       # Compact image list
rtk docker logs <c>     # Deduplicated logs
rtk kubectl get         # Compact resource list
rtk kubectl logs        # Deduplicated pod logs
```

### Network (65-70% savings)
```bash
rtk curl <url>          # Compact HTTP responses (70%)
rtk wget <url>          # Compact download output (65%)
```

### Meta Commands
```bash
rtk gain                # View token savings statistics
rtk gain --history      # View command history with savings
rtk discover            # Analyze Claude Code sessions for missed RTK usage
rtk proxy <cmd>         # Run command without filtering (for debugging)
rtk init                # Add RTK instructions to CLAUDE.md
rtk init --global       # Add RTK to ~/.claude/CLAUDE.md
```

## Token Savings Overview

| Category | Commands | Typical Savings |
|----------|----------|-----------------|
| Tests | vitest, playwright, cargo test | 90-99% |
| Build | next, tsc, lint, prettier | 70-87% |
| Git | status, log, diff, add, commit | 59-80% |
| GitHub | gh pr, gh run, gh issue | 26-87% |
| Package Managers | pnpm, npm, npx | 70-90% |
| Files | ls, read, grep, find | 60-75% |
| Infrastructure | docker, kubectl | 85% |
| Network | curl, wget | 65-70% |

Overall average: **60-90% token reduction** on common development operations.
<!-- /rtk-instructions -->