# Contributing to DeployMonster

Thank you for your interest in contributing to DeployMonster!

## Development Setup

### Prerequisites
- Go 1.26+
- Node.js 22+
- Docker (for testing)
- Make

### Getting Started

```bash
git clone https://github.com/deploy-monster/deploy-monster.git
cd deploy-monster

# Backend
go mod tidy
make dev

# Frontend (separate terminal)
cd web
npm install
npm run dev
```

### Running Tests

```bash
# All tests
make test

# Specific package
go test ./internal/core/ -v

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Benchmarks
go test -bench=. ./internal/ingress/lb/ -benchmem
```

## Project Structure

```
cmd/deploymonster/     Entry point (main.go)
internal/
  core/                Module system, EventBus, Store, Config
  api/                 REST API router and middleware
  api/handlers/        HTTP handlers (one file per resource)
  api/middleware/       Request middleware chain
  api/ws/              SSE streaming handlers
  auth/                JWT, RBAC, TOTP, OAuth
  db/                  SQLite implementation of Store
  deploy/              Docker SDK, strategies, rollback
  build/               Project detector, Dockerfile templates
  ingress/             Reverse proxy, TLS, load balancer
  discovery/           Docker label watcher, health checker
  compose/             Docker Compose parser and deployer
  webhooks/            Inbound/outbound webhook handling
  notifications/       Slack, Discord, Telegram, Webhook
  dns/                 DNS sync (Cloudflare, Route53)
  secrets/             AES-256-GCM vault
  backup/              Backup engine, S3 storage
  billing/             Plans, metering, Stripe
  database/            Managed DB provisioning
  vps/                 VPS providers (Hetzner, DO, Vultr, Linode)
  gitsources/          Git providers (GitHub, GitLab, Gitea, Bitbucket)
  marketplace/         Template registry and deployer
  resource/            Metrics collector, alerts
  swarm/               Docker Swarm manager
  enterprise/          White-label, WHMCS, GDPR
  mcp/                 MCP server for AI tools
web/                   React frontend (Vite + Tailwind)
docs/                  Documentation
```

## Code Style

- **Go**: Follow `gofmt` and `go vet`. Use `golangci-lint` for additional checks.
- **React**: TypeScript strict mode, functional components, Tailwind CSS.
- **Commits**: Conventional commits (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`).

## Module Pattern

Every new feature should be a module implementing `core.Module`:

```go
func init() {
    core.RegisterModule(func() core.Module { return New() })
}

type Module struct { ... }

func (m *Module) ID() string { return "my.module" }
func (m *Module) Init(ctx context.Context, core *core.Core) error { ... }
func (m *Module) Start(ctx context.Context) error { ... }
func (m *Module) Stop(ctx context.Context) error { ... }
```

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Write tests for new functionality
4. Ensure all tests pass (`make test`)
5. Ensure no lint errors (`make lint`)
6. Commit with conventional commit messages
7. Open a pull request against `main`

## License

By contributing, you agree that your contributions will be licensed under AGPL-3.0.
