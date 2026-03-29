# DeployMonster

<div align="center">

**Self-Hosted PaaS — Single Binary, Zero Dependencies**

Transform any VPS into a production-ready deployment platform in 60 seconds.

Single binary · Zero dependencies · 97% test coverage

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)
[![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Coverage](https://img.shields.io/badge/Coverage-97%25-brightgreen)](.)

[🌐 Website](https://deploy.monster) · [📚 Docs](docs/)

</div>

> **Note:** This project is currently in development and not yet ready for production use.

---

## Quick Start

```bash
# One-line install
curl -fsSL https://get.deploy.monster | bash
deploymonster

# Or with Docker
docker run -d -p 8443:8443 -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dm-data:/var/lib/deploymonster \
  ghcr.io/deploy-monster/deploymonster:latest
```

Open `http://localhost:8443` — **System Admin** credentials are printed on first run.

---

## Two Admin Levels

DeployMonster distinguishes between platform and tenant administration:

| Role | Scope | Capabilities |
|------|-------|--------------|
| **System Admin** | Platform-wide | Create tenants, manage servers, configure providers, view all resources |
| **Client Admin** | Tenant-level | Manage own projects, apps, databases, domains, team members |

This separation enables **true multi-tenancy** — each client gets isolated access to their resources while the system admin maintains platform control.

---

## Features

### 🚀 Deploy Anything
- **Git-to-Deploy** — GitHub, GitLab, Gitea, Bitbucket webhooks
- **14 Languages** — Auto-detected build packs (Node.js, Go, Python, Rust, PHP, Java, .NET, Ruby...)
- **Docker Images** — Deploy from GHCR, Docker Hub, or private registries
- **Docker Compose** — Multi-service stacks from YAML
- **Marketplace** — 25+ one-click apps (WordPress, Ghost, n8n, Grafana, Ollama...)

### 🏗️ Platform
- **224 REST API Endpoints** — OpenAPI 3.0 specification
- **Custom Reverse Proxy** — No Traefik/Nginx dependency, built-in Let's Encrypt
- **5 Load Balancer Strategies** — Round-robin, least-conn, IP-hash, random, weighted
- **Secret Vault** — AES-256-GCM encryption with `${SECRET:name}` syntax
- **Managed Databases** — PostgreSQL, MySQL, MariaDB, Redis, MongoDB
- **Backup Engine** — Local + S3/MinIO/R2, cron schedules, retention policies
- **Monitoring** — CPU/RAM/disk metrics, alerts, Prometheus `/metrics` endpoint

### 🌍 Infrastructure
- **VPS Provisioning** — Hetzner, DigitalOcean, Vultr, Linode, or any SSH server
- **Master/Agent Architecture** — Same binary, two modes (control plane / worker node)
- **SSH Key Management** — Ed25519 keys, per-server access control
- **Server Bootstrap** — Cloud-init, Docker install, agent deployment

### 👥 Multi-Tenancy & Business
- **RBAC** — 6 built-in roles + custom role creation
- **2FA & SSO** — TOTP, Google OAuth, GitHub OAuth
- **Billing** — Plans (Free/Pro/Business/Enterprise), Stripe integration
- **White-Label** — Custom branding, reseller support
- **GDPR Compliance** — Data export, right to erasure
- **Audit Log** — Every action logged with IP, timestamp, actor

### 🤖 AI-Native
- **MCP Server** — 9 AI-callable tools for LLM-driven infrastructure management
- **HTTP Transport** — `GET /mcp/v1/tools` + `POST /mcp/v1/tools/{name}`

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    DeployMonster Binary (22MB)                   │
├─────────┬─────────┬─────────┬──────────┬─────────┬──────────────┤
│ Web UI  │   API   │   SSE   │ Webhooks │ Ingress │  MCP Server  │
│ shadcn  │ 224 eps │ Stream  │  In+Out  │ :80/443 │  9 AI Tools  │
├─────────┴─────────┴─────────┴──────────┴─────────┴──────────────┤
│                   20 Auto-Registered Modules                     │
│  auth │ deploy │ build │ ingress │ dns │ secrets │ billing │    │
│  db │ backup │ vps │ swarm │ marketplace │ notifications │ ...   │
├─────────────────────────────────────────────────────────────────┤
│    SQLite + BBolt   │   Docker SDK   │   EventBus   │   Store   │
└─────────────────────────────────────────────────────────────────┘
```

---

## CLI

```bash
deploymonster                  # Start server (default)
deploymonster serve --agent    # Start as agent/worker node
deploymonster init             # Generate monster.yaml config
deploymonster version          # Show version info
deploymonster config           # Validate configuration
```

---

## Configuration

```bash
# Environment variables
export MONSTER_PORT=8443
export MONSTER_DOMAIN=deploy.example.com
export MONSTER_ADMIN_EMAIL=admin@example.com
export MONSTER_ADMIN_PASSWORD=secure-password
export MONSTER_ACME_EMAIL=ssl@example.com

# Or use config file
deploymonster init  # Creates monster.yaml
```

---

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go 1.26+ (27K LOC source, 47K LOC tests) |
| Frontend | React 19 + Vite 8 + Tailwind CSS 4 + shadcn/ui |
| Database | SQLite + BBolt KV (PostgreSQL ready) |
| Container | Docker SDK (moby/moby) |
| Auth | JWT + bcrypt + TOTP 2FA + OAuth SSO |
| Encryption | AES-256-GCM + Argon2id |
| Proxy | Custom net/http reverse proxy |
| Testing | 97% coverage, 7 fuzz tests, 38 benchmarks |

---

## Development

```bash
# Prerequisites: Go 1.26+, Node.js 22+, Docker

# Backend
go run ./cmd/deploymonster

# Frontend
cd web && npm install && npm run dev

# Tests
go test ./...              # Go tests (97% coverage)
cd web && npm test         # React tests

# Build
bash scripts/build.sh      # React → embed → Go binary
```

---

## Project Stats

```
86K+ total LOC
├── 27K Go source
├── 47K Go tests
└── 12K React

224 API endpoints · 115 handlers
20 modules · 25 marketplace templates
97% test coverage · 7 fuzz tests · 38 benchmarks
22MB single binary with embedded UI
```

---

## Documentation

- [Getting Started](docs/getting-started.md)
- [Architecture](ARCHITECTURE.md)
- [API Reference](docs/api-reference.md)
- [OpenAPI Spec](docs/openapi.yaml)

---

## License

**AGPL-3.0** — See [LICENSE](LICENSE) for details.

Commercial licensing available for enterprise use.

---

<div align="center">

## Built by

<table>
<tr>
<td align="center">

**ECOSTACK TECHNOLOGY OÜ**

🇪🇪 Tallinn, Estonia

[ecostack.ee](https://ecostack.ee)

</td>
<td align="center">

**Created by**

🇹🇷 🇪🇪 Ersin KOÇ

[𝕏 @ersinkoc](https://x.com/ersinkoc) · [GitHub](https://github.com/ersinkoc)

</td>
</tr>
</table>

---

**[deploy.monster](https://deploy.monster)** · **[deploymonster.com](https://deploymonster.com)**

[GitHub](https://github.com/deploy-monster/deploy-monster) · [Discord](https://discord.gg/deploymonster)

</div>
