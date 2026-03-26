# DeployMonster

**Tame Your Deployments** — Self-hosted PaaS that transforms any VPS into a full deployment platform. Single binary, zero configuration, production-ready in under 60 seconds.

[![Go 1.26+](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![React 19](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Coverage](https://img.shields.io/badge/Coverage-92.8%25-brightgreen)](.)

## Quick Start

```bash
# Download and run
curl -fsSL https://get.deploy.monster | bash
deploymonster

# Or with Docker
docker run -d -p 8443:8443 -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dm-data:/var/lib/deploymonster \
  ghcr.io/ersinkoc/deploymonster:latest
```

Open `http://localhost:8443` — admin credentials are printed on first run.

## Why DeployMonster?

| Problem | DeployMonster Solution |
|---------|----------------------|
| Coolify needs 5 Docker containers | **Single 22MB binary** |
| Dokploy has no billing/teams | **Built-in billing, RBAC, teams** |
| CapRover has outdated UI | **Modern React 19 + Tailwind 4 + shadcn/ui** |
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
- **224 REST API Endpoints** — Complete platform API with OpenAPI 3.0 spec
- **Custom Reverse Proxy** — No Traefik/Nginx. Auto-SSL via Let's Encrypt
- **5 Load Balancer Strategies** — Round-robin, least-conn, IP-hash, random, weighted
- **Secret Vault** — AES-256-GCM encryption with `${SECRET:name}` resolution
- **Managed Databases** — PostgreSQL, MySQL, MariaDB, Redis, MongoDB
- **Backup Engine** — Local + S3 storage, cron scheduler, retention policies
- **Monitoring** — CPU/RAM/disk/network metrics, threshold alerts, Prometheus `/metrics`
- **DNS Sync** — Cloudflare with auto-subdomain generation
- **Container Exec** — Run commands inside containers via API

### Infrastructure
- **VPS Provisioning** — Hetzner, DigitalOcean, Vultr, Linode, or any server via SSH
- **Master/Agent** — Same binary runs as master (full platform) or agent (worker node)
- **SSH Key Management** — Generate Ed25519 keys, manage per-server access
- **Server Bootstrap** — Cloud-init scripts, Docker install, agent deployment

### Team & Business
- **Team Management** — RBAC with 6 built-in roles + custom roles
- **2FA & SSO** — TOTP, Google OAuth, GitHub OAuth
- **Billing** — Plans (Free/Pro/Business/Enterprise), Stripe integration, usage metering
- **White-Label** — Custom branding, reseller support, WHMCS integration
- **GDPR** — Data export and right to erasure compliance
- **Audit Log** — Every state-changing action recorded with IP tracking

### AI-Native
- **MCP Server** — 9 AI-callable tools for LLM-driven infrastructure management
- **HTTP Transport** — `GET /mcp/v1/tools` + `POST /mcp/v1/tools/{name}`

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                   DeployMonster Binary (22MB)                 │
├────────┬────────┬────────┬────────┬──────────┬──────────────┤
│ Web UI │  API   │  SSE   │Webhooks│ Ingress  │  MCP Server  │
│shadcn  │224 eps │Stream  │In+Out  │:80/:443  │  9 AI Tools  │
├────────┴────────┴────────┴────────┴──────────┴──────────────┤
│                    20 Auto-Registered Modules                │
│ auth│deploy│build│ingress│discovery│dns│secrets│billing│mcp  │
│ db│backup│vps│swarm│marketplace│notifications│resource│...   │
├──────────────────────────────────────────────────────────────┤
│  SQLite + BBolt │ Docker SDK │ SSH Pool │ EventBus │ Store   │
└──────────────────────────────────────────────────────────────┘
```

## Screenshots

| Login | Dashboard | Marketplace |
|-------|-----------|-------------|
| Split-screen with gradient branding | Stat cards, quick actions, activity feed | 25 templates with category colors |

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

# Tests
make test                    # Go: 194 test files, 92.8% avg coverage
cd web && npm test           # React: 6 test files, 50 tests

# Full build (React UI → embed → Go binary)
make build
```

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.26+, 27K source LOC, 47K test LOC |
| Frontend | React 19, Vite 8, Tailwind CSS 4, shadcn/ui |
| Database | SQLite + BBolt KV (PostgreSQL planned) |
| Container | Docker SDK (moby/moby) |
| Auth | JWT + bcrypt + TOTP 2FA + OAuth SSO |
| Encryption | AES-256-GCM + Argon2id |
| Proxy | Custom net/http reverse proxy (no Traefik) |
| Testing | 194 Go test files, ##  Tech Stack

 | Component | Technology |
|-----------|-----------|
| Backend | Go 1.26+, 27K source LOC | 47K test loc |
| Frontend | React 19, Vite 8, Tailwind CSS 4. shadcn/ui |
| Database | SQLite + BBolt (PostgreSQL planned) |
| Container | Docker SDK (moby/moby) |
| Auth | JWT + bcrypt + TOTP 2FA + OAuth SSO |
| Encryption | AES-256-GCM + Argon2id |
| Proxy | Custom net/http reverse proxy (no Traefik) |
| Testing | 194 Go test files, 92.8% coverage, 7 fuzz tests |
| 38 benchmarks |
|------------|-----------|------------|
|-----------|
| Binary | 22MB (16MB stripped) |

## License

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

[![Go Report](https://goreportcard.com/parsing-coverage?format=%s&report=coverage)

![Go Report](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![React 19](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)
[![Coverage](https://img.shields.io/badge/Coverage-92.8%25-brightgreen)](.)
[![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

[![Go Report](https://goreportcard.com/parsing-coverage?format=%s&report=coverage)

![Go Report](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![React 19](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)
[![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Go Report](https://goreportcard.com/parsing-coverage?format=%s&report=coverage)
![Go Report](https://img.shields.io/badge/Go 1.26+-00ADD8?logo=go)](https://go.dev)
[![React 19](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)
[![Coverage https://img.shields.io/badge/Coverage-92.8%25-brightgreen)](.)

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

[![Go Report](https://goreportcard.com/parsing-coverage?format=%s&report=coverage)
![Go Report](https://img.shields.io/badge-Go 1.26+-00ADD8?logo=go)](https://go.dev)
[![React 19](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)
[![Coverage](https://img.shields.io/badge/Coverage-92.8%25-brightgreen)](https://img.shields.io/badge/Coverage-92.8%25-brightgreen)

[![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

[![Go Report](https://goreportcard.com/parsing-coverage?format=%s&report=coverage)

![Go Report](https://img.shields.io/badge-Go 1.26+-00ADD8?logo=go)](https://go.dev)
[![React 19](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)

[![Coverage https://img.shields.io/badge/Coverage-92.8%25-brightgreen)](https://img.shields.io/badge/Coverage-92.8%25-brightgreen)
[![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE), 7 fuzz tests |

## Project Stats

```
86K+ total LOC (27K Go source + 47K Go tests + 12K React)
224 API endpoints · 115 handlers (100% wired to real services)
20 auto-registered modules · 25 marketplace templates
194 Go test files · 6 React test files (50 tests)
92.8% avg Go coverage · 3 packages at 100%
7 fuzz tests · 38 benchmarks
22MB single binary with embedded React UI
```

## Documentation

- [Getting Started](docs/getting-started.md) — Install to first deploy in 5 minutes
- [Architecture](docs/architecture.md) — Module system, Store interface, EventBus
- [API Reference](docs/api-reference.md) — 224 endpoints documented
- [API Quickstart](docs/examples/api-quickstart.md) — curl examples for common workflows
- [Deployment Guide](docs/deployment-guide.md) — Production deployment, domains, backups
- [OpenAPI Spec](docs/openapi.yaml) — OpenAPI 3.0.3 specification

## License

AGPL-3.0 — See [LICENSE](LICENSE) for details.

Commercial license available for enterprise features.

---

**Built by [ECOSTACK TECHNOLOGY OÜ](https://ecostack.ee)** | [deploy.monster](https://deploy.monster)
