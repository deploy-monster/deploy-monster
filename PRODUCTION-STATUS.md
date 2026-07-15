# DeployMonster — Production Status

**Updated:** 2026-07-15
**Version:** v0.2.0
**Branch:** `master` (ahead of `origin/master` by 1 commit, clean)
**Readiness verdict:** CONDITIONAL GO
**Owner:** Ersin KOÇ / ECOSTACK TECHNOLOGY OÜ

---

## Verdict

| Deployment model | Status |
|---|---|
| **Self-hosted single-tenant** | ✅ **GO** — ready for production use |
| **Hosted multi-tenant SaaS** | ⏳ **CONDITIONAL GO** — all code and CI gates pass; requires real-infrastructure staging validation before launch |

---

## 1. Release Status

### v0.2.0 — 2026-07-12 (current)

DeployMonster v0.2.0 is **released and published**:

| Asset | Location |
|---|---|
| **GitHub release** | [v0.2.0](https://github.com/deploy-monster/deploy-monster/releases/tag/v0.2.0) |
| **Linux amd64/arm64** | Binary + SBOM in release assets |
| **Darwin amd64/arm64** | Binary + SBOM in release assets |
| **Windows amd64/arm64** | Binary + SBOM in release assets |
| **Checksums** | `checksums.txt` in release assets |
| **Docker image** | `ghcr.io/deploy-monster/deploy-monster:0.2.0` (Trivy-scanned, no vulnerabilities) |

### Fixes applied in v0.2.0

1. **Coverage raised to 91.2%** — from 85.1%, 64 new test files, 16 packages at 100%
2. **Go 1.26.5 toolchain** — upgraded from 1.26.4, fixes GO-2026-5856 (TLS ECH privacy leak)
3. **context.Background() eliminated in 3 handler constructors** — `NewDeployTriggerHandler`, `NewComposeHandler`, `NewMarketplaceDeployHandler` now accept signal-aware context (CRIT-1 class fix)
4. **Dashboard greeting** — now shows the logged-in user's name instead of hardcoded "admin"
5. **App create timestamps** — 201 response now reflects auto-populated fields (created_at, updated_at) instead of Go zero values
6. **Stale artifacts cleaned** — ~156 MB of binaries, coverage_detail.html removed from git tracking, loadtest-results cleaned

---

## 2. CI/CD Pipeline Status

All CI gates are **green on master**:

| Gate | Tool/Command | Status | Detail |
|---|---|---|---|
| **Go build** | `go build ./...` | ✅ PASS | All 44 packages compile |
| **Go vet** | `go vet ./...` | ✅ PASS | No static analysis issues |
| **Go vet (integration)** | `go vet -tags integration ./...` | ✅ PASS | No issues with integration build tags |
| **Go vet (pg integration)** | `go vet -tags pgintegration ./...` | ✅ PASS | No issues with postgres build tags |
| **Full Go test suite** | `go test -count=1 ./... -timeout 240s` | ✅ PASS | **44 packages, 0 FAIL** (total runtime ~2m30s) |
| **Coverage gate** | `go test -coverprofile` → filter → threshold check | ✅ PASS | **91.2%** (filtered, excl. load/soak harnesses; CI gate raised to 85%) |
| **OpenAPI drift** | `go run ./cmd/openapi-gen` | ✅ PASS | **236/236 routes** match between code and spec (allowlist: 0) |
| **Writers-under-load gate** | `TestStore_ConcurrentWrites_BaselineGate` | ✅ PASS | 6,772 ops/s, p95=26.7ms vs baseline 187.9ms |
| **Web unit tests** | `cd web && pnpm test` (Vitest) | ✅ PASS | **44 files, 405 tests** (completed in 10.29s) |
| **Web build** | `cd web && pnpm build` (Vite) | ✅ PASS | Built in ~350ms, main chunk 19.72 KB gzip (budget: 300 KB) |
| **E2E Playwright** | `cd web && pnpm test:e2e` | ✅ PASS | 13 spec files, blocking in CI (no `continue-on-error`) |
| **Frontend lint** | `cd web && pnpm run lint` (ESLint) | ✅ PASS | Clean |
| **pnpm audit** | `pnpm audit --audit-level moderate` | ✅ PASS | No known vulnerabilities |
| **govulncheck** | `govulncheck ./...` | ✅ PASS | No called vulnerabilities; GO-2026-5856 resolved by Go 1.26.5 upgrade |
| **golangci-lint** | `golangci-lint run ./...` | ✅ PASS | Clean |
| **Race detector** | `go test -race ./...` | ✅ PASS | Run in CI + nightly |
| **Bundle-size budget** | `pnpm run check:bundle` | ✅ PASS | Main entry: ~19 KB gzip (budget: 300 KB) |

### CI workflows

| Workflow | Trigger | Purpose |
|---|---|---|
| `ci.yml` | Push to master/main, PR | Full build, test, coverage, lint, E2E |
| `release.yml` | Tag `v*.*.*`, manual | Build UI → GoReleaser cross-compile → GHCR push → release assets |
| `loadtest-nightly.yml` | Scheduled (nightly) | HTTP load regression against baseline |
| `race-nightly.yml` | Scheduled (nightly) | Extended race detector run |
| `staging-smoke.yml` | Manual | Staging environment smoke check |

---

## 3. Test Quality

### Go backend

| Metric | Count |
|---|---|
| **Test files** | 398 |
| **Test packages** | 42 (all passing), 2 package-level no-test stubs |
| **Statement coverage** | **91.2%** (filtered, excl. load/soak harnesses) — up from 85.1% in v0.1.9 |
| **Packages at 100%** | **16** — api/apierr, awsauth, compose, cron, database, database/engines, deploy/graceful, discovery, enterprise, enterprise/integrations, gitsources, gitsources/providers, mcp, vps, vps/providers, webhooks |
| **Fuzz targets** | **17** — distributed across auth (JWT, password), db (secrets resolver), router (cross-tenant), marketplace (validator), webhooks (receiver), compose (parser), ingress (router) |
| **Benchmarks** | **44** — load balancer strategies, JWT gen/validate, AES encrypt/decrypt, compose parsing, SQLite operations, notifications |

### Web frontend

| Metric | Count |
|---|---|
| **Unit test files** | 44 |
| **Unit tests** | 405 (all passing) |
| **E2E test files** | 13 (Playwright, blocking in CI) |
| **Source files** | 172 (TSX + TS) |

### Performance benchmarks (selected)

| Operation | Performance |
|---|---|
| RoundRobin LB | 3.6 ns/op, 0 allocations |
| IPHash LB | 26 ns/op, 0 allocations |
| LeastConn LB | 55 ns/op, 0 allocations |
| JWT Generate | 4.1 μs/op |
| JWT Validate | 4.2 μs/op |
| AES-256 Encrypt | 633 ns/op |
| AES-256 Decrypt | 489 ns/op |
| Compose Parse | 17.6 μs/op |
| SQLite GetApp | 41 μs/op |

---

## 4. Security Posture

### Current findings

| Severity | Count | Notes |
|---|---|---|
| **CRITICAL** | 0 | No verified critical findings |
| **HIGH** | 0 | No verified high findings |
| **MEDIUM** | 3 | Residual product/coordination risks (documented in security report) |
| **LOW** | 4 | Hardening/documentation items (documented in security report) |

### Vulnerability management

- **govulncheck** — clean; no called vulnerabilities in Go dependencies
- **pnpm audit** — clean; no known vulnerabilities in frontend dependencies
- **Go CVE remediation** — 9 HIGH-severity CVEs in `golang.org/x/crypto` fixed in v0.1.9; GO-2026-5856 (TLS ECH privacy leak) fixed in v0.2.0 via Go 1.26.5 upgrade
- **Trivy** — Docker image scanned clean on release
- **Security audit report** — comprehensive 40-file report in `security-report/` directory

### Key security features implemented

- **JWT** (HS256, 32-char min secret) + **bcrypt** (cost 13) + **TOTP 2FA** + **OAuth SSO**
- **Secret vault** — AES-256-GCM with Argon2id KDF, scoped hierarchy (global → tenant → project → app), `${SECRET:name}` template syntax
- **Docker socket hardening** — documented procedure + hardened docker-compose with Tecnativa proxy
- **Audit logging** — IP, timestamp, actor recorded on every mutation
- **Tenant isolation** — `requireTenantApp()` at every resource-scoped handler, validated by `FuzzRouter_CrossTenant` (38 GETs) and `TestRouter_CrossTenantMutationMatrix` (38 mutations)
- **Handler context hygiene** — all handler constructors accept signal-aware context (no `context.Background()` in production request paths)
- **Rate limiting** — per-tenant and global, configurable
- **Request timeout** — 30s default, configurable via middleware
- **Security headers** — middleware applying industry-standard headers
- **GDPR** — data export + right-to-erasure endpoints

---

## 5. Codebase Overview

### Scale

| Dimension | Value |
|---|---|
| **Total Go LOC** | ~180,600 |
| — Production Go | ~57,000 |
| — Test Go | ~123,500 |
| **Go files** | 689 (291 source + 398 test) |
| **TypeScript/TSX files** | 172 source + 13 E2E |
| **Frontend test files** | 44 (Vitest) + 13 (Playwright) |
| **Modules** | 22 auto-registered |
| **API routes** | 236 (documented + drift-enforced) |
| **Marketplace templates** | 91 (19 categories) |
| **Architecture Decision Records** | 11 |
| **Binary size** | ~24 MB (single static binary with embedded UI) |

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                DeployMonster single binary (~24 MB)             │
├─────────┬─────────┬─────────┬──────────┬─────────┬──────────────┤
│ Web UI  │  REST   │  SSE    │ Webhooks │ Ingress │  MCP server  │
│ shadcn  │ 236 rt  │ Stream  │  In+Out  │ :80/443 │  9 AI tools  │
├─────────┴─────────┴─────────┴──────────┴─────────┴──────────────┤
│                22 auto-registered modules                       │
│  auth │ deploy │ build │ ingress │ dns │ secrets │ billing │   │
│  db   │ backup │ vps   │ swarm   │ marketplace │ notifications │
├─────────────────────────────────────────────────────────────────┤
│   SQLite + KV      │   Docker SDK   │   EventBus   │   Store   │
└─────────────────────────────────────────────────────────────────┘
```

### 22 Modules

`auth`, `api`, `autoscale`, `backup`, `billing`, `build`, `cron`, `database`, `db`, `discovery`, `dns`, `enterprise`, `gitsources`, `ingress`, `marketplace`, `mcp`, `notifications`, `resource`, `secrets`, `swarm`, `vps`, `deploy`

### Stack

| Component | Technology |
|---|---|
| **Backend** | Go 1.26+ (toolchain 1.26.5 — GO-2026-5856 resolved) |
| **Frontend** | React 19 + Vite 8 + Tailwind 4 + shadcn/ui |
| **State management** | Zustand (client), custom `useApi` hook (server data) |
| **Database** | SQLite (pure-Go, modernc.org/sqlite) + SQLite-backed KV; PostgreSQL optional via Store interface |
| **Container runtime** | Docker Engine SDK (moby/moby) |
| **Auth** | JWT (HS256) + bcrypt (cost 13) + TOTP + OAuth SSO |
| **Encryption** | AES-256-GCM + Argon2id KDF |
| **Reverse proxy** | Custom `net/http` with Let's Encrypt `autocert`, 5 LB strategies |
| **Observability** | Prometheus metrics, structured logging, OpenTelemetry SDK (stubbed) |

---

## 6. What Is Ready

### Deploy pipelines
- **Git-to-deploy** — GitHub, GitLab, Gitea, Gogs, Bitbucket webhooks with HMAC signature verification
- **14 language detectors** — Node.js, Go, Python, Rust, PHP, Java, .NET, Ruby, Elixir, Deno, Bun, static, Docker, custom
- **Docker Compose** — multi-service stacks from YAML parsing
- **Marketplace** — 91 one-click templates across 19 categories (WordPress, Ghost, n8n, Grafana, Ollama, etc.)
- **Deploy preview** — ephemeral deployments before production promotion
- **Auto-rollback** — health-check-gated automatic rollback on deploy failure
- **Canary deployments** — percentage-based traffic splitting via weighted LB strategy

### Platform
- **Custom reverse proxy** — 5 LB strategies (round-robin, least-conn, IP-hash, random, weighted + canary)
- **Let's Encrypt** — automatic SSL via `autocert` with wildcard SSL support
- **Secret vault** — AES-256-GCM + Argon2id KDF, `${SECRET:name}` template syntax
- **Managed databases** — PostgreSQL, MySQL, MariaDB, Redis, MongoDB
- **Backups** — local + S3/MinIO/R2, cron schedules, configurable retention
- **Monitoring** — Prometheus metrics at `/metrics`, health endpoints, resource alerts
- **DNS** — Cloudflare integration for automatic DNS record management
- **Handler context hygiene** — all handler constructors accept signal-aware (cancelable) context; no `context.Background()` in production request paths

### Multi-tenancy & business
- **RBAC** — 6 built-in roles + custom role creation
- **2FA (TOTP)** + Google/GitHub OAuth SSO
- **Billing** — Stripe integration with Free / Pro / Business / Enterprise plans
- **White-label branding** + reseller support
- **GDPR** — data export + right-to-erasure
- **Audit log** — IP/timestamp/actor on every mutation
- **Invites** — team member invitation flow
- **Sessions** — session management with JWT rotation

### Infrastructure
- **VPS provisioning** — DigitalOcean, Hetzner, Vultr, Linode, Custom-SSH (SSH-key-aware)
- **Master/agent** — same binary in two modes, versioned WebSocket protocol, TLS mutual auth
- **Resource monitoring** — CPU, memory, disk, network per-host

### AI-native
- **MCP server** — 9 AI-callable tools at `GET /mcp/v1/tools`

---

## 7. Known Limitations

These are explicitly **not** presented as bugs — they are intentional scope boundaries documented for operators:

| Limitation | Status | Reasoning |
|---|---|---|
| **Multi-master HA** | ❌ Not supported | Single-process control plane with SQLite-default store; PG-backed HA is post-1.0 |
| **Kubernetes orchestration** | ❌ Out of scope | DeployMonster provisions Docker containers directly (ADR 0003) |
| **AWS EC2 provisioning** | ❌ Not implemented | Other 5 cloud providers cover ~95% of users; AWS deferred until paying customer requests it |
| **Route53 DNS** | ❌ Not implemented | Cloudflare is the only DNS provider that ships today |
| **Distributed tracing** | ⏳ Stubbed | OTel SDK pulled in transitively but no OTLP exporter wired |
| **Plugin system** | ❌ Does not exist | Every builder, DNS provider, VPS provider, and notifier is first-party code |
| **OpenTelemetry spans** | ⏳ Not emitted | Middleware stubs exist; no span emission from module lifecycle |
| **Load balancer TLS passthrough** | ❌ Not supported | LB operates at HTTP layer only |

---

## 8. Operational Documentation

| Document | Purpose |
|---|---|
| [`docs/getting-started.md`](docs/getting-started.md) | First-deploy walkthrough |
| [`docs/deployment-guide.md`](docs/deployment-guide.md) | Production install, domains, backups, notifications |
| [`docs/upgrade-guide.md`](docs/upgrade-guide.md) | Version-to-version upgrade with rollback procedure |
| [`docs/runbook.md`](docs/runbook.md) | Operator runbook: P0/P1 incident response index |
| [`docs/secret-rotation.md`](docs/secret-rotation.md) | JWT secret rotation (routine + emergency) |
| [`docs/docker-socket-hardening.md`](docs/docker-socket-hardening.md) | Tecnativa-proxy pattern for safe Docker socket exposure |
| [`docs/sla.md`](docs/sla.md) | Published performance + availability targets for 1.0 |
| [`docs/configuration.md`](docs/configuration.md) | Complete YAML + environment variable reference |
| [`docs/api-reference.md`](docs/api-reference.md) | API endpoint overview (full spec: `docs/openapi.yaml`) |
| [`docs/troubleshooting.md`](docs/troubleshooting.md) | Common issues and resolutions |
| [`docs/staging-validation.md`](docs/staging-validation.md) | Pre-release staging validation runbook |
| [`docs/security-audit.md`](docs/security-audit.md) | Security audit findings and resolutions |
| [`docs/adr/`](docs/adr/) | 11 Architecture Decision Records |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Developer setup, test/perf gates, code style |

---

## 9. Staging Validation (Pre-SaaS-Launch Checklist)

The following steps from [`docs/staging-validation.md`](docs/staging-validation.md) must be completed on real infrastructure before a hosted multi-tenant SaaS launch:

1. Install or upgrade the release candidate on a disposable staging host
2. Run authenticated and public smoke checks
3. Validate real DNS and Let's Encrypt TLS
4. Test webhook delivery and HMAC signature failure handling
5. Perform tenant-isolation spot checks against live data
6. Execute backup creation and restore drill
7. Execute rollback drill
8. Run load check and short (5m+) soak test
9. Verify release artifact and Docker image publication

### Local staging progress (v0.2.0)

A local Docker-based staging validation was completed on 2026-07-15, covering all steps possible without real cloud infrastructure:

| Step | Result |
|---|---|
| Build release-shaped binary | ✅ 28 MB binary at `bin/deploymonster` |
| Server start + fresh SQLite DB | ✅ Running on `127.0.0.1:8443` |
| Public health checks | ✅ `/health`, `/api/v1/health` → `{"status":"ok"}` |
| First-run admin creation | ✅ Random `admin-*@deploymonster.local` email |
| Authenticated login + `/auth/me` | ✅ JWT + user profile with role_id, tenant_id |
| Marketplace templates | ✅ 91 templates across 19 categories |
| End-to-end deploy pipeline | ✅ Created app → deployed `nginx:alpine` → container running → status "running" |
| API dashboard / servers | ✅ Stats returned, 1 local server detected |
| OpenAPI spec / MCP tools | ✅ `/api/v1/openapi.json`, 9 tools at `/mcp/v1/tools` |

**Cannot validate locally:** Real DNS + Let's Encrypt ACME, webhook delivery from external providers, VPS provisioning, multi-tenant isolation at scale, backup/restore from S3, load/soak under real traffic patterns. These require a disposable VPS with Docker, a real domain, and cloud API credentials.

**Blocking gap:** A Hetzner API token (`HCLOUD_TOKEN`) or equivalent cloud provider credential is needed to provision the staging VPS. Once provided, the 9-step checklist in `docs/staging-validation.md` can be executed end-to-end.

Until these steps are executed and the evidence is attached to the release issue, the hosted SaaS verdict remains CONDITIONAL.

---

## 10. Quick Start

```bash
# One-line install (systemd)
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.2.0/scripts/install.sh \
  | bash -s -- --version=v0.2.0

deploymonster setup             # interactive: domain, SSL, admin account
sudo systemctl restart deploymonster

# Or Docker (recommended for evaluation)
docker run -d --name deploymonster \
  -p 8443:8443 -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dm-data:/var/lib/deploymonster \
  ghcr.io/deploy-monster/deploy-monster:v0.2.0
```

Open `http://<host>:8443`. First-run admin credentials are printed to the console or injected by `deploymonster setup`.

---

## References

- **README:** [`README.md`](README.md)
- **Architecture:** [`docs/architecture.md`](docs/architecture.md)
- **Architecture decisions:** [`docs/adr/`](docs/adr/)
- **Changelog:** [`CHANGELOG.md`](CHANGELOG.md)
- **Verification report:** [`docs/verification-report-2026-07-06.md`](docs/verification-report-2026-07-06.md)
- **Production readiness:** [`PRODUCTION-READY.md`](PRODUCTION-READY.md)
- **Project status:** [`docs/PROJECT_STATUS.md`](docs/PROJECT_STATUS.md)
- **Security report:** [`security-report/SECURITY-REPORT.md`](security-report/SECURITY-REPORT.md)
- **Full project analysis:** [`deploy-monster-analysis.md`](deploy-monster-analysis.md)
- **License:** [`LICENSE`](LICENSE) (AGPL-3.0)

---

<div align="center">

**ECOSTACK TECHNOLOGY OÜ** — 🇪🇪 Tallinn, Estonia

**Created by** Ersin KOÇ — [𝕏 @ersinkoc](https://x.com/ersinkoc) · [GitHub](https://github.com/ersinkoc)

[GitHub](https://github.com/deploy-monster/deploy-monster)

</div>
