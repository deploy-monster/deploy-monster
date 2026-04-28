# AGENTS.md

Compact guide for AI agents working in DeployMonster. See `CLAUDE.md` for architecture details.

## What This Is

Go 1.26+ modular monolith PaaS (single binary, master or agent mode) with embedded React 19 frontend. 21 Go packages in `internal/`, each with a `module.go` that auto-registers via `init()` + `core.RegisterModule()`. The `webhooks` package is a library, not a module.

## Build Pipeline (Critical)

The React SPA is embedded into the Go binary. **Order matters:**

```bash
# 1. Build React
cd web && pnpm install --frozen-lockfile && pnpm run build && cd ..

# 2. Copy to embed dir
rm -rf internal/api/static/*
cp -r web/dist/* internal/api/static/

# 3. Build Go binary
go build -o bin/deploymonster ./cmd/deploymonster
```

Or use `scripts/build.sh` which does all three. `make build` only builds Go (assumes UI is already embedded).

## Commands

### Backend
```bash
make test               # All tests, race detector, coverage
make test-short         # Skip integration tests
make lint               # golangci-lint
make fmt                # gofmt -s -w
make vet                # go vet ./...
make bench              # Benchmarks
make db-gate            # Writers-under-load perf gate
make openapi-check      # Drift between router.go and docs/openapi.yaml
scripts/ci-local.sh     # Full local CI (11 checks)
scripts/ci-local.sh --quick  # Skip slow tests
```

### Single Test / Package
```bash
go test -run TestFunctionName ./internal/deploy/...
go test -run TestFunctionName/subtestname ./internal/deploy/...
go test -v ./internal/build/...
go test -fuzz FuzzName ./internal/auth/...
```

### Integration Tests (Build Tags)
```bash
# SQLite integration (requires Docker for some tests)
go test -tags integration -v ./...

# Postgres integration (requires running Postgres)
TEST_POSTGRES_DSN='postgres://deploymonster:deploymonster@localhost:5432/deploymonster_test?sslmode=disable' \
  go test -tags pgintegration -v ./internal/db/...
```

Integration files are gated behind `//go:build integration` or `//go:build pgintegration`. Default `go test ./...` never compiles them. Shared store contract logic lives in `internal/db/store_contract_test.go` (runs under either tag).

### Frontend
```bash
cd web
pnpm install --frozen-lockfile
pnpm run build          # tsc -b && vite build
pnpm run dev            # Dev server with HMR
pnpm run lint           # ESLint
pnpm test               # vitest run
pnpm test:e2e           # Playwright (needs server on :8443)
```

## CI Gates to Know

| Gate | Threshold | Env Vars |
|------|-----------|----------|
| Coverage | 85% minimum | — |
| Bundle size | 300 KB gzip | — |
| Writers-under-load | 10% p95 regression | `DM_DB_GATE=1` |
| OpenAPI drift | Fails on un-allowlisted drift | — |
| Secrets scan | gitleaks on PR diff or full history | — |

Playwright E2E is **continue-on-error** in CI (drifted from current UI).

## Architecture Gotchas

- **All data access** goes through `core.Store` interface. Never use `*db.SQLiteDB` directly.
- **Event naming**: `{domain}.{action}` — e.g. `app.deployed`, `container.died`. Exact, prefix (`app.*`), or wildcard (`*`) matching.
- **Module lifecycle**: `Init(ctx, core)` → `Start(ctx)` → `Stop(ctx)`. Dependencies resolved by topological sort, shutdown in reverse order.
- **Config**: `monster.yaml` file + `MONSTER_*` env var overrides. See `monster.example.yaml` for all keys.
- **JWT**: HS256, access=15min, refresh=7days. Auth via Bearer token or `X-API-Key` header.
- **Same binary**: master (default) or agent (`--agent` flag).

## Code Conventions

- `log/slog` with `"module"` key for structured logging
- `context.Context` as first parameter
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Table-driven tests with `t.Run` subtests
- Mocks: implement interface with optional function fields + call tracking booleans (see `internal/deploy/mock_test.go`)
- No global state — DI via `core.Core`
- Interfaces defined where consumed, not where implemented
- Commit messages: conventional commits format

## Editor / Formatting

- `.editorconfig`: Go uses tabs, TS/JS/JSON/CSS/YAML use 2-space indent
- `.gitattributes`: forces LF on shell, Docker, YAML, Go files
- Go: `gofmt -s` (tabs)
- Frontend: ESLint flat config + Prettier defaults

## Pre-commit Hooks

Install with `scripts/setup-git-hooks.sh`. Not a framework — custom shell scripts:
- **pre-commit**: gofmt check on staged files, `go vet ./...`, `go mod tidy` drift
- **pre-push**: `./scripts/ci-local.sh --quick`

## File Locations

| What | Where |
|------|-------|
| Go entrypoint | `cmd/deploymonster/main.go` |
| API routes | `internal/api/router.go` |
| Module interface | `internal/core/module.go` |
| Store interface | `internal/core/` (12 sub-interfaces) |
| Event bus | `internal/core/events.go` |
| Config struct | `internal/core/` or `internal/database/` |
| OpenAPI spec | `docs/openapi.yaml` |
| Drift allowlist | `docs/openapi-drift-allowlist.txt` |
| React app | `web/src/` |
| Zustand stores | `web/src/stores/` |
| API client | `web/src/api/client.ts` |
| Loadtest harness | `tests/loadtest/` |
| Soak test harness | `tests/soak/` |
| Security report | `security-report/` |
| ADRs | `docs/adr/` |
