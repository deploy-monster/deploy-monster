# Contributing to DeployMonster

This doc is aimed at someone with a fresh clone and a working Go +
Node toolchain. If a step below doesn't work, treat that as a bug in
this file — please open an issue or a PR fixing it rather than working
around it silently.

## Prerequisites

- **Go 1.26+** (`go version`)
- **Node.js 22+** and **pnpm 10+** (`node -v`, `pnpm -v`)
- **Docker** (for the container-runtime-backed paths and the optional
  Postgres integration tests)

The web build uses pnpm, not npm. `npm install` will create a broken
`package-lock.json` — always `pnpm install`.

## First run

```bash
# Go dependencies
go mod download

# Frontend — uses pnpm, caches into web/node_modules
cd web && pnpm install && cd ..

# Run in dev mode (backend + frontend served together on :8443)
make dev
```

`make dev` runs the Go binary with an in-process React dev proxy. For
HMR on the React side, run the Vite dev server separately:

```bash
# Terminal 1: backend
make dev

# Terminal 2: Vite dev server with proxy back to :8443
cd web && pnpm run dev
```

Default admin credentials are printed to the console on first boot;
the DB lives in `./deploymonster.db` and is recreated fresh each time.

## Tests

The Makefile is the authoritative reference for test commands:

```bash
make test              # go test -race -coverprofile ./... (all packages)
make test-short        # skip integration-tagged tests
make test-integration  # go test -tags integration -v ./...
make lint              # golangci-lint
make vet               # go vet
make fmt               # gofmt -w
make bench             # go test -bench=. -benchmem
make coverage          # HTML coverage report → coverage.html
```

### Single test or package

```bash
go test -run TestFunctionName ./internal/deploy/...
go test -run TestFunctionName/subtestname ./internal/deploy/...
go test -v ./internal/build/...
go test -bench BenchmarkName ./internal/core/...
go test -fuzz FuzzName -fuzztime 30s ./internal/secrets/...
```

### Integration tests

The `integration` build tag enables tests that need a real filesystem,
a Docker daemon, or sockets. `make test-integration` runs them against
SQLite with a temp directory.

Postgres-backed tests use a separate `pgintegration` tag and require a
running Postgres reachable via `TEST_POSTGRES_DSN`:

```bash
# Start a disposable Postgres
docker run --rm -d --name dm-pg -p 5432:5432 \
  -e POSTGRES_PASSWORD=pass -e POSTGRES_DB=deploymonster postgres:17

export TEST_POSTGRES_DSN='postgres://postgres:pass@localhost:5432/deploymonster?sslmode=disable'
make test-integration-postgres
```

### Frontend tests

```bash
cd web
pnpm test              # Vitest (unit)
pnpm run lint          # ESLint
pnpm run build         # production build
pnpm run check:bundle  # main-chunk size budget (300 KB gzip)
pnpm test:e2e          # Playwright — requires `make dev` on :8443
pnpm test:e2e --ui     # Playwright with interactive UI
```

### Performance gates

Two load-test tooling targets run against a server that must already be
up on `:8443`:

```bash
make db-gate           # writers-under-load gate vs committed baseline
make loadtest-check    # HTTP loadtest vs committed baseline (10% threshold)
make soak-test         # 24-hour soak run — artifacts to soak-results.json
```

Baselines live in `internal/db/testdata/` and `tests/loadtest/baselines/`.
If you legitimately need to move one, use the `-baseline` variant of the
make target and commit the result with a message explaining *why* the
baseline moved — not just "baseline update".

## Code style

### Go

- `log/slog` for structured logging, always with a `"module"` key.
- `context.Context` as the first parameter.
- Error wrapping: `fmt.Errorf("context: %w", err)`.
- No global state — dependency injection via `core.Core`.
- Interfaces defined where consumed (Go idiom).
- Table-driven tests with `t.Run` subtests.
- Mocks: implement the interface with optional function fields +
  call-tracking booleans — see `internal/deploy/mock_test.go` for the
  canonical pattern.
- **Never import `internal/db` outside `internal/db/` itself.** Every
  call site goes through `core.Store` (see ADR 0009).

### React / TypeScript

- shadcn/ui components with the `cn()` utility helper.
- `@/` path alias → `web/src/*`.
- Data fetching through `useApi` / `useMutation` / `usePaginatedApi`
  hooks — no TanStack Query, no SWR (see ADR 0010).
- Client-side state in Zustand stores under `web/src/stores/`.

### Commit messages

Conventional-commits style, with sprint scope for work tracked on the
roadmap:

```
feat(auth): add refresh-token rotation grace period
fix(e2e): stabilize deploy wizard selectors
sprint(3): lift internal/auth coverage 78.7% → 93.1%
docs(adr): add 0010 custom useApi over TanStack Query
```

Every commit authored by Claude appends
`Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`
for traceability.

## Pull requests

1. Fork + feature branch (`git checkout -b feature/my-feature`).
2. Make changes with tests.
3. `make check` (vet + test + build) — must pass.
4. `make lint` — must pass or the lint-report diff must be explained
   in the PR body.
5. PR body: what changed, why, and what you tested manually.
6. For security-adjacent changes, cite the finding being fixed or
   open a new one in `security-report/`.

## Architecture pointers

- **`.project/`** — the source of truth for product direction:
  - `ANALYSIS.md` — honest feature/security/quality snapshot
  - `ROADMAP.md` — sprint plan, closed/open items
  - `SPECIFICATION.md` — product spec
  - `IMPLEMENTATION.md` — patterns and code examples
  - `PRODUCTIONREADY.md` — go/no-go verdict
- **`docs/adr/`** — 10 Nygard-template ADRs covering the load-bearing
  decisions (start here if you're asking "why is it like this?").
- **`docs/openapi.yaml`** — API contract; regenerate with
  `make openapi-check` and update if you add a route.
- **`CLAUDE.md`** — agent-facing conventions; humans should read it
  too since it summarises the non-obvious project rules.

## Reporting bugs / proposing features

- **Bugs:** open an issue with a minimal reproduction and the commit
  hash. Logs with the `"module"` slog key make triage far faster.
- **Security:** do not open a public issue. Email `security@ecostack.ee`
  (or the address in `SECURITY.md` if that's present and fresher than
  this doc).
- **Features:** open an issue describing the user need before coding.
  Aim for "what problem are we solving for whom" rather than "here's
  my preferred implementation" — implementations evolve faster in
  review when the goal is shared.

## License

By contributing, you agree your contributions will be licensed under
AGPL-3.0 (see `LICENSE`).

---

Built by [ECOSTACK TECHNOLOGY OÜ](https://ecostack.ee)
