# Dependency Audit

## Go Dependencies

### HIGH RISK
| Package | Version | Issue |
|---------|---------|-------|
| github.com/docker/docker | v28.5.2 | GO-2026-4887 (AuthZ bypass), GO-2026-4883 (privilege validation) |

### MEDIUM RISK
| Package | Version | Issue |
|---------|---------|-------|
| github.com/gorilla/websocket | v1.5.3 | Version warning, consider upgrade |

### SECURE
- golang.org/x/crypto v0.50.0
- golang.org/x/net v0.52.0
- github.com/golang-jwt/jwt/v5 v5.3.1

## Frontend Dependencies (web/package.json)
All dependencies current. No known vulnerabilities.

## Remediation
- Docker SDK: No patch available. Restrict Docker API network access.
- gorilla/websocket: `go get github.com/gorilla/websocket@latest`
