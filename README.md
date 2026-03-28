# DeployMonster

<div align="center">

**Tame Your Deployments**

Self-hosted PaaS that transforms any VPS into a full deployment platform.

Single binary · Zero dependencies · Production-ready in 60 seconds

[![GitHub Repo](https://img.shields.io/badge/GitHub-Deploy--Monster/DeployMonster_GO-181717?logo=github)](https://github.com/Deploy-Monster/DeployMonster_GO)
[![Go 1.26+](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![React 19](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Coverage](https://img.shields.io/badge/Coverage-97%25-brightgreen)](.)

[🌐 deploy.monster](https://deploy.monster) · [📚 Documentation](docs/) · [🎮 Demo](https://deploy.monster/demo) · [💬 Discord](https://discord.gg/deploymonster)

</div>

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

Open `http://localhost:8443` — admin credentials are printed on first run.

---

## Why DeployMonster?

| Problem | DeployMonster |
|---------|---------------|
| Coolify needs 5+ containers | **Single 22MB binary** |
| Dokploy has no billing/teams | **Built-in billing, RBAC, teams** |
| CapRover has outdated UI | **React 19 + Tailwind 4 + shadcn/ui** |
| Vercel/Netlify lock you in | **Self-hosted, zero vendor lock-in** |
| Traefik needs separate config | **Built-in reverse proxy + auto-SSL** |
| No AI integration anywhere | **MCP server for LLM-driven infra** |

---

## Features

### 🚀 Deploy Anything
- **Git-to-Deploy** — Push to GitHub/GitLab/Gitea/Bitbucket, auto-build and deploy
- **14 Languages** — Node.js, Next.js, Go, Python, Rust, PHP, Java, .NET, Ruby, and more
- **Docker Images** — Deploy from any registry (GHCR, Docker Hub, private)
- **Docker Compose** — Upload YAML, get running multi-service stack
- **Marketplace** — One-click deploy 25+ apps (WordPress, Ghost, n8n, Ollama, Grafana...)

### 🏗️ Platform
- **224 REST API Endpoints** — Complete platform API with OpenAPI 3.0 spec
- **Custom Reverse Proxy** — No Traefik/Nginx dependency. Auto-SSL via Let's Encrypt
- **5 Load Balancer Strategies** — Round-robin, least-conn, IP-hash, random, weighted
- **Secret Vault** — AES-256-GCM encryption with `${SECRET:name}` resolution
- **Managed Databases** — PostgreSQL, MySQL, MariaDB, Redis, MongoDB
- **Backup Engine** — Local + S3 storage, cron scheduler, retention policies
- **Monitoring** — CPU/RAM/disk metrics, threshold alerts, Prometheus `/metrics`

### 🌍 Infrastructure
- **VPS Provisioning** — Hetzner, DigitalOcean, Vultr, Linode, or any server via SSH
- **Master/Agent Architecture** — Same binary, two modes (control plane / worker node)
- **SSH Key Management** — Ed25519 keys, per-server access control
- **Server Bootstrap** — Cloud-init scripts, Docker install, agent deployment

### 👥 Team & Business
- **RBAC** — 6 built-in roles + custom roles
- **2FA & SSO** — TOTP, Google OAuth, GitHub OAuth
- **Billing** — Plans (Free/Pro/Business/Enterprise), Stripe integration
- **White-Label** — Custom branding, reseller support
- **GDPR Compliance** — Data export, right to erasure
- **Audit Log** — Every action recorded with IP tracking

### 🤖 AI-Native
- **MCP Server** — 9 AI-callable tools for LLM-driven infrastructure
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
deploymonster init  # Creates monster.yaml
```

Or use environment variables:

```bash
export MONSTER_PORT=8443
export MONSTER_DOMAIN=deploy.example.com
export MONSTER_ADMIN_EMAIL=admin@example.com
export MONSTER_ADMIN_PASSWORD=secure-password
export MONSTER_ACME_EMAIL=ssl@example.com
```

---

## Development

```bash
# Prerequisites: Go 1.26+, Node.js 22+, Docker

# Backend
make dev

# Frontend
cd web && npm install && npm run dev

# Tests
make test                  # Go tests (97% coverage)
cd web && npm test         # React tests

# Build (React UI → embed → Go binary)
make build
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
| Testing | 97% Go coverage, 7 fuzz tests, 38 benchmarks |

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

[GitHub](https://github.com/Deploy-Monster/DeployMonster_GO) · [Discord](https://discord.gg/deploymonster)

</div>
