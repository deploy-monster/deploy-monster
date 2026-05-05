# DeployMonster

<div align="center">

**Self-hosted PaaS — single Go binary, embedded React UI**

Turn any VPS with Docker into a multi-tenant deployment platform.

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)
[![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/v0.1.8-release-brightgreen)](./)

[📚 Docs](docs/) · [🏗 ADRs](docs/adr/) · [🛣 Roadmap](docs/archive/ROADMAP.md)

</div>

> **Status: v0.1.8 (conditional-go).** Self-hosted single-tenant: ready.
> Multi-tenant SaaS: closing residual Sprint 1–3 items. See
> [`PRODUCTION-READY.md`](PRODUCTION-READY.md) for the current verdict.

---

## Quick start

```bash
# One-line install (systemd)
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.1.8/scripts/install.sh \
  | bash -s -- --version=v0.1.8

deploymonster setup             # interactive: domain, SSL, admin account
sudo systemctl restart deploymonster

# Or Docker (bind mounts the socket and a persistent volume)
docker run -d --name deploymonster \
  -p 8443:8443 -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dm-data:/var/lib/deploymonster \
  ghcr.io/deploy-monster/deploy-monster:v0.1.8
```

Open `http://<host>:8443`. First-run admin credentials are printed to
the console or injected by `deploymonster setup`.

---

## Two admin levels

| Role | Scope | Capabilities |
|------|-------|--------------|
| **System Admin** | Platform-wide | Create tenants, manage servers, configure providers, view all resources |
| **Client Admin** | Tenant-level | Manage own projects, apps, databases, domains, team members |

Tenant isolation is enforced by `requireTenantApp()` at every
resource-scoped handler and pinned by two regression tests:
`FuzzRouter_CrossTenant` (38 GETs × foreign-tenant IDs) and
`TestRouter_CrossTenantMutationMatrix` (38 mutations × foreign-tenant
IDs). See ADR 0009 for the Store-interface design that keeps
tenant checks at one layer.

---

## What ships today

### Deploy pipelines
- **Git-to-deploy** — GitHub, GitLab, Gitea, Gogs, Bitbucket webhooks
  with HMAC signature verification.
- **14 language detectors** — Node, Go, Python, Rust, PHP, Java,
  .NET, Ruby, Elixir, Deno, Bun, static, Docker, custom.
- **Docker Compose** multi-service stacks from YAML.
- **Marketplace** — 91 curated one-click templates across 16
  categories (WordPress, Ghost, n8n, Grafana, Ollama, …).

### Platform
- **234 REST API routes**, all tracked in `docs/openapi.yaml`; CI
  (`openapi-gen`) fails on code/spec drift.
- **Custom reverse proxy** — no Traefik/Nginx dependency, automatic
  Let's Encrypt via `autocert`. Five LB strategies (round-robin,
  least-conn, IP-hash, random, weighted + canary).
- **Secret vault** — AES-256-GCM with Argon2id KDF, scoped hierarchy
  (global → tenant → project → app), `${SECRET:name}` template syntax
  for env vars and compose files. Per-deployment salt stored in BBolt;
  legacy installs migrate on first boot. See ADR 0008.
- **Managed databases** — PostgreSQL, MySQL, MariaDB, Redis, MongoDB.
- **Backups** — local + S3/MinIO/R2, cron schedules, retention.
- **Prometheus metrics** at `/metrics`, health at `/health`.

### Infrastructure
- **VPS provisioning** — DigitalOcean, Hetzner, Vultr, Linode, or
  any SSH-reachable server (Custom-SSH). All four cloud providers
  wire `SSHKeyID` through to their respective create-instance API.
  AWS EC2 is intentionally deferred to post-1.0.
- **Master/agent** — the same binary in two modes (control plane /
  worker node) speaking a versioned WebSocket protocol. See ADR 0007.

### Multi-tenancy & business
- RBAC with 6 built-in roles + custom role creation.
- 2FA (TOTP) + Google / GitHub OAuth SSO.
- Billing scaffolding (Stripe) — Free / Pro / Business / Enterprise.
- White-label branding + reseller support.
- GDPR data export + right-to-erasure endpoints.
- Audit log with IP / timestamp / actor on every mutation.

### AI-native
- **MCP server** — 9 AI-callable tools at `GET /mcp/v1/tools`
  and `POST /mcp/v1/tools/{name}`.

---

## Architecture at a glance

```
┌─────────────────────────────────────────────────────────────────┐
│                DeployMonster single binary (~24 MB)             │
├─────────┬─────────┬─────────┬──────────┬─────────┬──────────────┤
│ Web UI  │  REST   │  SSE    │ Webhooks │ Ingress │  MCP server  │
│ shadcn  │ 234 rt  │ Stream  │  In+Out  │ :80/443 │  9 AI tools  │
├─────────┴─────────┴─────────┴──────────┴─────────┴──────────────┤
│                20 auto-registered modules                       │
│  auth │ deploy │ build │ ingress │ dns │ secrets │ billing │   │
│  db   │ backup │ vps   │ swarm   │ marketplace │ notifications │
├─────────────────────────────────────────────────────────────────┤
│   SQLite + BBolt   │   Docker SDK   │   EventBus   │   Store   │
└─────────────────────────────────────────────────────────────────┘
```

See [`docs/adr/`](docs/adr/) for the 10 ADRs explaining why it looks
like this.

---

## CLI

```bash
deploymonster                  # start as master (default)
deploymonster serve --agent    # start as agent / worker node
deploymonster setup            # interactive setup (domain, SSL, admin)
deploymonster init             # generate monster.yaml
deploymonster version          # build info
deploymonster config           # validate configuration
deploymonster health           # health-check probe (works in distroless)
```

---

## Configuration

```bash
# Environment-variable overrides (all prefixed MONSTER_)
export MONSTER_PORT=8443
export MONSTER_DOMAIN=deploy.example.com
export MONSTER_ADMIN_EMAIL=admin@example.com
export MONSTER_ADMIN_PASSWORD=<initial-password>
export MONSTER_ACME_EMAIL=ssl@example.com

# Or a YAML file
deploymonster init             # writes monster.yaml
$EDITOR monster.yaml
```

Full reference: [`docs/configuration.md`](docs/configuration.md).

---

## Tech stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.26+ |
| Frontend | React 19 + Vite 8 + Tailwind 4 + shadcn/ui, Zustand for client state, custom `useApi` hook for data (ADR 0010) |
| Database | SQLite (default, pure-Go driver per ADR 0004) + BBolt KV, PostgreSQL optional (same Store interface, ADR 0009) |
| Container | Docker Engine SDK |
| Auth | JWT (HS256, 32-char min secret) + bcrypt cost 13 + TOTP 2FA + OAuth SSO |
| Encryption | AES-256-GCM + Argon2id (ADR 0008) |
| Proxy | Custom `net/http` reverse proxy with Let's Encrypt `autocert` |

---

## Development

```bash
# Prerequisites: Go 1.26+, Node.js 22+, pnpm 10+, Docker

# Backend
go run ./cmd/deploymonster

# Frontend — uses pnpm, NOT npm
cd web && pnpm install && pnpm run dev

# Tests
make test              # race detector + full suite
make test-short        # skip integration-tagged tests
make lint              # golangci-lint
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full guide
(integration tests, Postgres setup, perf gates, fuzz targets,
bundle-size budget).

---

## Project stats

- **~188 K** total LOC (~50 K Go source, ~117 K Go tests, ~22 K React/TS/CSS)
- **234** registered HTTP routes
- **20** auto-registered modules
- **91** marketplace templates
- **17** fuzz targets (input parsing, webhook HMAC, JWT validate, secret resolver, cross-tenant router)
- **44** benchmarks
- **~24 MB** single binary with embedded UI
- **Coverage:** statement-weighted **88.0 %** after stripping the
  `tests/loadtest` and `tests/soak` harness packages from the
  profile (their `*_test.go` files are one-line smoke tests that
  exist only so the binaries compile under `go test`). Raw coverage
  including those harnesses is 86.3 %. Hot packages run well above
  target: `webhooks` 99.0 %, `mcp` 98.9 %, `api` 93.8 %,
  `notifications` 93.9 %, `marketplace` 95.3 %, `deploy` 91.5 %,
  `auth` 91.1 %, `db` 88.4 %, `compose` 100 %. The CI gate
  enforces the filtered-85 % threshold (see
  `.github/workflows/ci.yml`).

---

## Known limitations

Things we are explicitly *not* pretending to be ready:

- **Multi-master HA** is not supported. The master is a
  single-process control plane with a SQLite-default store; running
  two masters against the same DB will corrupt state. A Postgres-backed
  HA story is on the post-1.0 roadmap.
- **Kubernetes is not a deploy target.** DeployMonster provisions
  Docker containers directly (ADR 0003). Running it *alongside* k8s
  works; orchestrating k8s pods *from* DeployMonster does not.
- **AWS EC2 provisioning** is not implemented. The other five cloud
  providers (DO, Hetzner, Vultr, Linode, Custom-SSH) cover ~95 % of
  the typical user base. AWS adds 16–20 h of vendor-SDK maintenance
  cost; deferred until a paying customer asks for it.
- **Route53 DNS** is deferred for the same reason. Cloudflare is
  the only DNS provider that ships today.
- **OpenAPI coverage is enforced** — registered routes and
  `docs/openapi.yaml` must match, with an empty drift allowlist.
- **E2E Playwright suite** is blocking (no `continue-on-error`),
  green on master. Retries enabled for flakiness resilience.
- **Distributed tracing** is stubbed (OpenTelemetry SDK pulled in
  transitively) but not wired. Add OTLP exporter + span emission
  from middleware + module lifecycle when needed.
- **Plugin system** does not exist — every builder, DNS provider,
  VPS provider, and notifier is first-party code.

This list is updated every sprint. New limitations discovered in
production land here, not in the roadmap, so operators can see the
trade-offs before committing.

---

## Documentation

| Doc | Purpose |
|---|---|
| [`docs/getting-started.md`](docs/getting-started.md) | First-deploy walkthrough |
| [`docs/deployment-guide.md`](docs/deployment-guide.md) | Production install, domains, backups, notifications |
| [`docs/upgrade-guide.md`](docs/upgrade-guide.md) | Version-to-version upgrade procedure, rollback |
| [`docs/runbook.md`](docs/runbook.md) | Operator runbook: scenario index for P0/P1 events |
| [`docs/secret-rotation.md`](docs/secret-rotation.md) | JWT secret rotation (routine + emergency) |
| [`docs/docker-socket-hardening.md`](docs/docker-socket-hardening.md) | Tecnativa-proxy pattern |
| [`docs/sla.md`](docs/sla.md) | Published performance + availability targets for 1.0 |
| [`docs/configuration.md`](docs/configuration.md) | Full YAML + env-var reference |
| [`docs/api-reference.md`](docs/api-reference.md) | API overview; `docs/openapi.yaml` for the machine-readable spec |
| [`docs/adr/`](docs/adr/) | 10 architecture decision records |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Dev setup, test/perf gates, code style |

---

## License

**AGPL-3.0** — see [`LICENSE`](LICENSE). Commercial licensing
available for users who need to ship modifications privately;
contact the address in `SECURITY.md`.

---

<div align="center">

### Built by

**ECOSTACK TECHNOLOGY OÜ** — 🇪🇪 Tallinn, Estonia — [ecostack.ee](https://ecostack.ee)

**Created by** 🇹🇷 🇪🇪 Ersin KOÇ — [𝕏 @ersinkoc](https://x.com/ersinkoc) · [GitHub](https://github.com/ersinkoc)

[GitHub](https://github.com/deploy-monster/deploy-monster)

</div>
