# Dependency Audit

## Dependency Audit Summary
- Total dependencies: 489 (Go modules: 99, frontend packages: 390)
- Ecosystems scanned: Go, npm/pnpm
- Known vulnerabilities found: 0 open in local audit
- Typosquatting risks: 0
- Dependency confusion risks: 0
- License concerns: 0
- Outdated dependencies: 0 security-blocking

## Go Dependencies

### Resolved
| Package | Previous Version | Replacement | Issue |
|---------|------------------|-------------|-------|
| github.com/docker/docker | v28.5.2+incompatible | github.com/moby/moby/client v0.4.1 + github.com/moby/moby/api v1.54.2 | GHSA-x744-4wpc-v9h2 / CVE-2026-34040, GHSA-pxq6-2prw-chj9 / CVE-2026-33997 |

DeployMonster no longer imports the legacy `github.com/docker/docker`
module. Docker API calls use the split Moby client/API modules instead.

### SECURE
- golang.org/x/crypto v0.50.0
- golang.org/x/net v0.52.0
- github.com/golang-jwt/jwt/v5 v5.3.1
- github.com/gorilla/websocket v1.5.3

## Frontend Dependencies (web/package.json)
`pnpm audit --json` reports 0 vulnerabilities.

## Remediation
- Completed: replaced vulnerable Docker SDK dependency with Moby split SDK modules.
- Keep Docker daemon access restricted to trusted local/agent contexts; DeployMonster still depends on daemon-side controls for container lifecycle operations.
