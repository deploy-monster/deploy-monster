# Contributing to DeployMonster

Thank you for your interest in contributing!

## Development Setup

### Prerequisites
- Go 1.26+
- Node.js 22+
- Docker

### Getting Started

```bash
# Clone the repository
git clone https://github.com/deploy-monster/DeployMonster_GO.git
cd DeployMonster_GO

# Install Go dependencies
go mod download

# Install frontend dependencies
cd web && npm install && cd ..

# Run in development mode
go run ./cmd/deploymonster

# Run tests
go test ./...

# Build
go build -o bin/deploymonster ./cmd/deploymonster
```

## Code Style

### Go
- Use `gofmt` for formatting
- Run `go vet` before committing
- Follow standard Go conventions
- Use table-driven tests

### React
- Use TypeScript
- Follow existing component patterns
- Use shadcn/ui components
- Run `npm run lint` before committing

## Commit Messages

Use conventional commits:
- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation
- `test:` tests
- `chore:` maintenance
- `refactor:` code refactoring

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## Code Review

All PRs require review before merging. Please:
- Respond to all comments
- Keep PRs focused and reasonably sized
- Update documentation as needed

## License

By contributing, you agree your contributions will be licensed under AGPL-3.0.

---

Built by [ECOSTACK TECHNOLOGY OÜ](https://ecostack.ee)
