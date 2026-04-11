# Changelog

All notable changes to DeployMonster will be documented in this file.

The format is loosely based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Grouped into **Breaking**, **Security**, **Features**, **Fixes**, and **Performance**
at the request of the Phase 7 roadmap.

## [1.6.0-rc.1] — 2026-04-11 — Phase 1–7 stabilization release candidate

The first release candidate to ship after the Phase 1–6 audit and the 88-tier
hardening sweep. Every line below is measured against HEAD, not against the
pre-audit aspirational numbers from the v1.4.0 era.

### Breaking

_No breaking config or database changes._ Existing installs upgrade in place with
`0002_add_indexes` applied additively. Min-from is `v1.0.0` per
`docs/upgrade-guide.md`; installs older than `v0.5.0` must step through `v0.5.2`
first so the historical schema is flattened correctly.

### Security

- **17 Dependabot alerts closed** (20 → 3). Remaining 3 are all upstream-blocked
  (`fixed=null`): 2 against `github.com/docker/docker` (daemon-side CVEs,
  duplicates of R-001) and 1 against `go.etcd.io/bbolt`
  (GHSA-6jwv-w5xf-7j27, no upstream fix published yet).
- **`go.opentelemetry.io/otel*` bumped `1.42.0 → 1.43.0`** — CVE-2026-39882
  (OTLP HTTP exporter reads unbounded HTTP response body).
- **`vite` bumped `8.0.3 → 8.0.8`** (direct devDep) — GHSA-p9ff-h696-f583,
  GHSA-v2wj-q39q-566r, GHSA-4w7w-66w2-5vf9 (dev-server path traversal +
  middleware bypass). `pnpm.overrides` added to pin transitive `vite@7` to
  `^7.3.2` via `vitest@3.2.4` → `@vitest/mocker` → `vite@7` chain.
- **`lodash` pinned `^4.18.0`** via `pnpm.overrides` — GHSA-r5fr-rjxr-66jc
  (prototype pollution) and GHSA-f23m-r3pf-42rh (ReDoS). Reached via the
  abandoned `dagre@0.8.5` → `graphlib@2.1.8` chain used by the topology editor.
- **Go toolchain bumped `1.26.1 → 1.26.2`** for the `crypto/tls` and
  `crypto/x509` upstream fixes. `GOTOOLCHAIN=auto` downloads automatically;
  CI pins `1.26.2` explicitly.
- **88 hardening tiers landed** across lifecycle, context-cancellation, replay,
  and DoS vectors that static analyzers cannot catch. Representative fixes:
  ws `DeployHub` Shutdown + concurrent-write mutex + dead-client eviction
  (Tier 77); swarm `AgentServer` wg drain, recover, stopCtx, closed flag
  (Tier 76); resource monitor lifecycle + stopCtx plumbing (Tier 75); deploy
  manager lifecycle + auto-rollback drain (Tier 74); ingress gateway lifecycle
  + ACME ctx plumbing (Tier 73); body-limit bypass fix (Tier 72); auth
  JWT/TOTP/OAuth PKCE hardening (Tiers 78-82); request-scope leak (Tier 83);
  tenant queue fairness (Tier 84). Full list in `docs/security-audit.md`.
- **Argon2id + AES-256-GCM vault** with per-install random salt persisted in
  BBolt at `vault/salt`. Legacy-salt migration path via `migrateLegacyVault()`
  for pre-Phase-2 installs (idempotent). Documented in
  [ADR 0008](docs/adr/0008-encryption-key-strategy.md).
- **`Module.RotateEncryptionKey`** — single-step re-encryption of every secret
  under a new master key.

### Features

- **OpenAPI drift checker** (`cmd/openapi-gen`, `make openapi-check`). Parses
  `internal/api/router.go` via regex scan of `r.mux.Handle*` calls and
  `docs/openapi.yaml` via `gopkg.in/yaml.v3`, diffs the two sets, fails CI on
  drift. Ratcheting allowlist at `docs/openapi-drift-allowlist.txt` ensures
  the file cannot rot silently: stale entries (drift closed but line remains)
  also fail the check. Wired into the `lint` job in CI. Known gap on first
  run: 232 routes in code vs 88 in spec (144-route backlog, parked in the
  allowlist and expected to shrink over time as operators fill in missing
  spec entries).
- **24-hour soak-test harness** (`tests/soak`, `make soak-test`) + 5-minute
  CI smoke variant (`make soak-test-short`). Sample interval, duration,
  concurrency all configurable.
- **Loadtest regression gate** (`tests/loadtest`, `make loadtest-check`) —
  fails on ≥10% p95 latency regression against a committed baseline at
  `tests/loadtest/baselines/http.json`.
- **Prometheus runtime-metric block on `/metrics/api`** — Go runtime stats
  (goroutines, GC, heap) + per-handler request counters and latency histograms.
- **`internal/db/migrations/0002_add_indexes.sql`** — 30+ indexes on hot query
  paths (`apps`, `deployments`, `audit_log`, `secrets`, `usage_records`).
  Additive and reversible via the paired `.down.sql`.
- **PostgreSQL store contract parity** — full end-to-end CRUD test suite at
  `internal/db/store_contract_test.go` runs against both SQLite and Postgres
  backends on every push via `sqlite` and `pgintegration` build tags.
- **Compile-time `var _ core.Store = (*SQLiteDB)(nil)`** contract assertions
  in both backends catch interface drift at `go build` time. Documented in
  [ADR 0009](docs/adr/0009-store-interface-composition.md).

### Fixes

- **`cmd/openapi-gen`** regex-filters router registrations against the
  `routeMethods` allowlist so string matches on unknown methods like
  `PROPFIND` don't pollute the drift set.
- **`web/src/hooks/useApi.ts`** — `useMutation` narrows by method so
  `api.delete` (which takes `(path, opts)` not `(path, body, opts)`) type-checks
  cleanly.
- **`web/src/stores/__tests__/topologyStore.test.ts`** — custom-type test used
  the literal `'connection'` which is not in the `TopologyEdgeType` union
  (`'default' | 'dependency' | 'mount' | 'dns'`). Switched to `'mount'`.
- **Pre-existing TypeScript drift** fixed as a side effect of the Tier 91
  `pnpm build` verification pass.

### Performance

- **Loadtest baseline** at `tests/loadtest/baselines/http.json` captures the
  committed p50/p95/p99/throughput against which regressions are measured.
- **30+ new indexes** from `0002_add_indexes` — expected impact on hot-path
  reads is quantified in the soak run output, not in this changelog.
- **85%+ CI-enforced test coverage gate** — coverage below 85% fails the
  `test` job in `.github/workflows/ci.yml`.

### Documentation

- **Two new ADRs**: [0008-encryption-key-strategy.md](docs/adr/0008-encryption-key-strategy.md)
  and [0009-store-interface-composition.md](docs/adr/0009-store-interface-composition.md).
  ADR 0010 (in-process event bus) was deliberately skipped because
  [ADR 0006](docs/adr/0006-event-bus-in-process.md) already covers the topic
  completely.
- **`docs/upgrade-guide.md`** — per-version compatibility matrix covering every
  shipped tag from `v0.1.0` through `v1.6.0-rc.1` (min-from, min Go, config
  break, DB migrations, agent protocol) + three-phase migration checklist
  (pre-flight, during-upgrade, post-upgrade).
- **`docs/security-audit.md`** — "Phase 1–5 hardening fixes (closed)" table
  with 18 rows (H-001 through H-018) + "Residual risk register" with 9 rows
  (R-001 through R-009) ranked by severity.
- **`README.md` + `CLAUDE.md` accuracy pass** — every metric measured against
  HEAD: 240 API endpoints (was 224), 222 handlers (was 115), ~50K Go source
  (was 27K), ~117K Go tests (was 47K), 85%+ CI-enforced coverage (was
  "97% avg"), 56 marketplace templates across 16 categories (was "116",
  which was a naive-grep double-count), ~24MB binary (was 22MB). CLAUDE.md
  state section fixed: the project uses the custom `useApi`/`useMutation`/
  `usePaginatedApi` hook family, **not** TanStack React Query
  (no `@tanstack/react-query` in `package.json`).

### Release engineering

- **`.goreleaser.yml` migrated to v2 schema** — `archives.format: tar.gz` →
  `formats: [tar.gz]` and `archives.format_overrides.format: zip` →
  `formats: [zip]` (both deprecated in goreleaser v2).
- **`goreleaser release --snapshot --clean` validated** — 6 platforms built
  in 46 seconds (`linux/darwin/windows × amd64/arm64`), archives contain
  binary + LICENSE + README + CHANGELOG + example config, checksums.txt
  covers every archive, binary metadata (`--version`) returns expected fields.
- **Known operator footgun**: tags `v1.0.0` through `v1.5.0` are topologically
  orphaned. They were cut on 2026-03-25 pointing to aspirational pre-audit
  commits, but `v0.5.2` was cut later (2026-03-26) from what became the real
  trunk. `git rev-list v0.5.2..v1.5.0` returns 0 commits; `git rev-list
  v1.5.0..HEAD` returns 186. When cutting `v1.6.0-rc.1`, the tag lands on
  the real ancestry line and `goreleaser` will correctly pick it up. The
  orphan `v1.x` tags should be **left in place** (deleting them is destructive
  to any user who installed from them).

---

## [1.5.0] - 2026-03-29

### Highlights
- **56 marketplace templates** across 16 categories (Grafana, Keycloak,
  Home Assistant, etc.). Earlier drafts of this entry claimed "116" — that
  figure was a naive grep artifact that double-counted `Slug:` references in
  `Related`/`Featured` lists. The authoritative count comes from
  `marketplace.LoadBuiltins()` → `marketplace.Count()` at runtime.
- **Repository migrated** to `github.com/deploy-monster/deploy-monster`
- **Competitive positioning** — Full comparison table vs Coolify, Dokploy, CapRover, Railway
- **Admin roles documentation** — System Admin vs Client Admin clarified

### Added
- Marketplace templates across 16 categories
- Competitive comparison table in README
- Multi-tenancy documentation with admin role examples
- System Admin vs Client Admin role distinction

### Changed
- Repository URL: `github.com/deploy-monster/deploy-monster`
- GoReleaser config updated for new org
- Docker image: `ghcr.io/deploy-monster/deploymonster`
- All documentation URLs updated

### Categories (representative templates)
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

### Visual topology editor (landed as part of the 1.5 line, previously
miscredited to an aspirational 1.6 entry)
- Topology Editor page with React Flow canvas (`/topology`)
- Custom node components: AppNode, DatabaseNode, DomainNode, VolumeNode, WorkerNode
- Component palette for drag-and-drop infrastructure design
- Configuration panel for selected node properties
- Topology deployment API (`POST /api/v1/topology/deploy`)
- Auto-layout feature using dagre algorithm
- Environment selector (production, staging, development)
- `@xyflow/react` and `dagre` npm dependencies

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
