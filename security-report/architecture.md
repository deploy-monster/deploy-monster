# Security Audit — Phase 1: Architecture Map

## Tech Stack

| Component | Technology | Version |
|---|---|---|
| Language | Go | 1.26.1 |
| Frontend | React 19 + Vite + TypeScript | In `/web` |
| Relational DB | SQLite (modernc.org/sqlite) | v1.48.0 |
| KV Store | BBolt | v1.4.3 |
| Auth | JWT HS256 + API Keys (SHA-256) | golang-jwt/jwt/v5 v5.3.1 |
| Container Runtime | Docker SDK | v28.5.2 |
| WebSocket | gorilla/websocket | v1.5.3 |

## Auth Mechanism

Three strategies tried in order:
1. **JWT Bearer** — `Authorization: Bearer <token>` (15min access / 7day refresh)
2. **JWT Cookie** — `dm_access` httpOnly cookie
3. **API Key** — `X-API-Key: dm_...` header with prefix-lookup + constant-time hash comparison

**CRITICAL**: API key comparison at `middleware.go:207` compares plaintext API key against SHA-256 hash — always fails.

**RBAC**: 30+ permissions in `internal/auth/rbac.go`. `RequireSuperAdmin` stacked on all `adminOnly` routes.

## Middleware Chain

```
RequestID → GracefulShutdown → GlobalRateLimiter → SecurityHeaders
→ APIMetrics → APIVersion → BodyLimit(10MB) → Timeout(30s) → Recovery
→ RequestLogger → CORS → CSRFProtect → IdempotencyMiddleware → AuditLog → Handler
```

- Global RL: 120 req/min per IP, prefixes `/api/` and `/hooks/` only
- CSRF: double-submit cookie, bypasses for Bearer/API-key auth
- CORS: `Access-Control-Allow-Credentials: true` unconditionally

## Key Modules

| Module | Responsibility |
|---|---|
| `internal/api` | REST router, 150+ handlers, WebSocket hub |
| `internal/auth` | JWT, API keys, RBAC |
| `internal/deploy` | Container lifecycle via Docker SDK |
| `internal/build` | Build pipeline: clone → detect → docker build |
| `internal/secrets` | AES-256-GCM vault, Argon2id KDF, key rotation |
| `internal/webhooks` | Inbound webhook receiver with HMAC verification |
| `internal/swarm` | Master/Agent via WebSocket, token join |

## External Integrations

Docker socket, Git providers (GitHub/GitLab/Gitea/Bitbucket webhooks), Let's Encrypt, S3/MinIO backup storage, Stripe billing, SMTP, Slack/Discord/Telegram notifications, agent nodes.

## Security Controls Present

- JWT with key rotation, refresh token revocation
- API keys: SHA-256 hashed, prefix-lookup, constant-time comparison, expiry
- bcrypt cost 12 for passwords
- RBAC with 30+ permissions, SuperAdmin guard
- Global + per-tenant + per-auth rate limiting
- CSRF double-submit cookie
- HMAC-SHA256 webhook signature verification
- AES-256-GCM secrets vault with per-deployment salt
- Security headers (CSP, HSTS, X-Frame-Options, etc.)
- Path traversal protection (`isPathSafe`) in file browser
- Command injection blocklist (10 patterns) in exec
- Git URL validation (blocks shell metachar, `file://` allowed)
- Panic recovery with structured logging
- Slow request logging (>5s threshold)
- Audit log middleware
- BBolt for idempotency, rate limit counters, JWT revocation
