# DeployMonster

**Tame Your Deployments** — Self-hosted PaaS that transforms any VPS into a full deployment platform. Single binary, zero configuration, production-ready in under 60 seconds.

[![CI](https://github.com/deploy-monster/deploy-monster/actions/workflows/ci.yml/badge.svg)](https://github.com/deploy-monster/deploy-monster/actions/workflows/ci.yml)
[![Go Report](https://goreportcard.com/badge/github.com/deploy-monster/deploy-monster)](https://goreportcard.com/report/github.com/deploy-monster/deploy-monster)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

## Quick Start

```bash
# Download and run
curl -fsSL https://get.deploy.monster | bash
deploymonster

# Or with Docker
docker run -d -p 8443:8443 -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dm-data:/var/lib/deploymonster \
  deploymonster/deploymonster
```

Open `https://localhost:8443` — credentials are printed on first run.

## Why DeployMonster?

| Problem | DeployMonster Solution |
|---------|----------------------|
| Coolify needs 5 Docker containers | **Single 22MB binary** |
| Dokploy has no billing/teams | **Built-in billing, RBAC, teams** |
| CapRover has outdated UI | **Modern React 19 + Tailwind** |
| Vercel/Netlify lock you in | **Self-hosted, zero vendor lock-in** |
| Traefik needs separate config | **Built-in reverse proxy + auto-SSL** |
| No AI integration anywhere | **MCP server for LLM-driven infra** |

## Features

### Deploy Anything
- **Git-to-Deploy** — Push to GitHub/GitLab/Gitea/Bitbucket, auto-build and deploy
- **14 Language Support** — Auto-detects Node.js, Next.js, Go, Python, Rust, PHP, Java, .NET, Ruby, and more
- **Docker Image** — Deploy any image from any registry
- **Docker Compose** — Upload YAML, get a running multi-service stack
- **Marketplace** — One-click deploy 25+ apps (WordPress, Ghost, n8n, Ollama, Grafana, etc.)

### Platform
- **223 REST API Endpoints** — Complete platform API
- **Custom Reverse Proxy** — No Traefik/Nginx. Auto-SSL via Let's Encrypt
- **5 Load Balancer Strategies** — Round-robin, least-conn, IP-hash, random, weighted
- **Secret Vault** — AES-256-GCM encryption with `${SECRET:name}` resolution
- **Managed Databases** — PostgreSQL, MySQL, MariaDB, Redis, MongoDB
- **Backup Engine** — Local + S3 storage, cron scheduler, retention policies
- **Monitoring** — CPU/RAM/disk metrics, threshold alerts, Prometheus `/metrics`
- **DNS Sync** — Cloudflare + Route53 with auto-subdomain generation

### Infrastructure
- **VPS Provisioning** — Hetzner, DigitalOcean, Vultr, Linode, or any server via SSH
- **Multi-Server** — Docker Swarm support for scaling across nodes
- **SSH Key Management** — Generate Ed25519 keys, manage per-server access
- **Server Bootstrap** — Cloud-init scripts, Docker install, agent deployment

### Team & Business
- **Team Management** — RBAC with 6 built-in roles + custom roles
- **2FA & SSO** — TOTP, Google OAuth, GitHub OAuth
- **Billing** — Plans (Free/Pro/Business/Enterprise), Stripe integration, usage metering
- **White-Label** — Custom branding, reseller support, WHMCS integration
- **GDPR** — Data export and right to erasure compliance
- **Audit Log** — Every state-changing action recorded

### AI-Native
- **MCP Server** — 9 AI-callable tools for LLM-driven infrastructure management
- **HTTP Transport** — `GET /mcp/v1/tools` + `POST /mcp/v1/tools/{name}`

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                   DeployMonster Binary (22MB)                  │
├────────┬────────┬────────┬────────┬──────────┬──────────────┤
│ Web UI │  API   │  SSE   │Webhooks│ Ingress  │  MCP Server  │
│(React) │223 eps │Stream  │In+Out  │:80/:443  │  9 AI Tools  │
├────────┴────────┴────────┴────────┴──────────┴──────────────┤
│                    20 Auto-Registered Modules                 │
│ auth│deploy│build│ingress│discovery│dns│secrets│billing│mcp  │
│ db│backup│vps│swarm│marketplace│notifications│resource│...   │
├──────────────────────────────────────────────────────────────┤
│  SQLite + BBolt │ Docker SDK │ SSH Pool │ EventBus │ Store  │
└──────────────────────────────────────────────────────────────┘
```

## CLI

```bash
deploymonster                      # Start server (default)
deploymonster serve --agent        # Start as agent/worker node
deploymonster init                 # Generate monster.yaml config
deploymonster version              # Show version info
deploymonster config               # Validate configuration
```

## Configuration

```bash
deploymonster init  # Creates monster.yaml
```

Or use environment variables:

```bash
export MONSTER_PORT=8443
export MONSTER_DOMAIN=deploy.example.com
export MONSTER_ADMIN_EMAIL=admin@example.com
export MONSTER_ADMIN_PASSWORD=your-secure-password
export MONSTER_ACME_EMAIL=ssl@example.com
```

## Development

```bash
# Prerequisites: Go 1.26+, Node.js 22+, Docker

# Backend
make dev

# Frontend
cd web && npm install && npm run dev

# Tests (20 suites, 89 test files, 70%+ coverage on critical paths)
make test

# Full build (React UI → embed → Go binary)
bash scripts/build.sh
```

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.26+, 23K LOC, 221 files |
| Frontend | React 19, Vite 8, Tailwind CSS 4, Zustand 5 |
| Database | SQLite (default), PostgreSQL (enterprise) |
| Container | Docker SDK (moby/moby) |
| Auth | JWT + bcrypt + TOTP 2FA + OAuth SSO |
| Encryption | AES-256-GCM + Argon2id |
| Proxy | Custom net/http/httputil (no Traefik) |

## Documentation

- [Getting Started](docs/getting-started.md) — Install to first deploy in 5 minutes
- [Architecture](docs/architecture.md) — Module system, Store interface, EventBus
- [API Reference](docs/api-reference.md) — All 127 endpoints documented
- [Deployment Guide](docs/deployment-guide.md) — Production deployment, domains, backups

## License

AGPL-3.0 (core) — Commercial license available for enterprise features.

See [LICENSE](LICENSE) for details.

---

**Built by [ECOSTACK TECHNOLOGY OÜ](https://ecostack.dev)** | [deploy.monster](https://deploy.monster)
