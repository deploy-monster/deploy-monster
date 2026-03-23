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
docker run -d -p 8443:8443 -v /var/run/docker.sock:/var/run/docker.sock deploymonster/deploymonster
```

Open `https://localhost:8443` — credentials are printed on first run.

## Features

- **Single Binary** — One 20MB binary. Not 5 Docker containers.
- **Zero Config** — Download, run, deploy. Auto-generates everything on first start.
- **Git-to-Deploy** — Push to GitHub/GitLab/Gitea, auto-build and deploy.
- **14 Language Support** — Auto-detects Node.js, Go, Python, Rust, PHP, Java, .NET, Ruby, and more.
- **Docker Compose** — Upload a compose file, get a running multi-service stack.
- **Built-in Reverse Proxy** — No Traefik/Nginx needed. Auto-SSL via Let's Encrypt.
- **Marketplace** — One-click deploy WordPress, Ghost, n8n, Ollama, and 12+ apps.
- **Managed Databases** — PostgreSQL, MySQL, Redis, MongoDB with one click.
- **Secret Vault** — AES-256-GCM encrypted secrets with `${SECRET:name}` resolution.
- **VPS Provisioning** — Create Hetzner/DigitalOcean/Vultr servers from the UI.
- **Multi-Server** — Docker Swarm support for scaling across nodes.
- **Team Management** — RBAC with 6 built-in roles, custom roles, invitations.
- **Billing** — Built-in plans, usage metering, Stripe integration.
- **MCP Server** — AI-native infrastructure management for LLMs.
- **White-Label** — Custom branding, reseller support, WHMCS integration.
- **Monitoring** — CPU/RAM/disk metrics, threshold alerts, Prometheus endpoint.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                  DeployMonster Binary (20MB)              │
├────────┬────────┬──────────┬────────┬──────────┬────────┤
│ Web UI │REST API│   SSE    │Webhooks│ Ingress  │  MCP   │
│(React) │/api/v1 │/events   │/hooks  │:80/:443  │Server  │
├────────┴────────┴──────────┴────────┴──────────┴────────┤
│                     20 Modules                           │
│ auth│deploy│build│ingress│discovery│dns│secrets│billing  │
│ db  │backup│vps  │swarm  │marketplace│notifications│mcp │
├─────────────────────────────────────────────────────────┤
│    SQLite + BBolt │ Docker SDK │ SSH Pool │ Event Bus   │
└─────────────────────────────────────────────────────────┘
```

## Configuration

Copy `monster.example.yaml` to `monster.yaml`, or use environment variables:

```bash
export MONSTER_PORT=8443
export MONSTER_DOMAIN=deploy.example.com
export MONSTER_ADMIN_EMAIL=admin@example.com
export MONSTER_ADMIN_PASSWORD=your-secure-password
```

## CLI

```bash
deploymonster                    # Start server (default)
deploymonster serve --agent      # Start as worker node
deploymonster version            # Show version
deploymonster config             # Validate configuration
```

## Development

```bash
# Backend
go mod tidy
make dev

# Frontend
cd web && npm install && npm run dev

# Tests
make test

# Full build (React + Go)
bash scripts/build.sh
```

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.26+ |
| Frontend | React 19, Vite 8, Tailwind CSS 4, Zustand 5 |
| Database | SQLite (default), PostgreSQL (enterprise) |
| Container | Docker SDK |
| Auth | JWT + bcrypt + TOTP 2FA + OAuth SSO |

## License

AGPL-3.0 (core) — Commercial license available for enterprise features.

---

**Built by [ECOSTACK TECHNOLOGY](https://ecostack.dev)**
