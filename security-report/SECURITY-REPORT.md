# DeployMonster Security Audit Report

**Audit Date:** 2026-04-30
**Codebase:** deploy-monster (master branch, commit 8d55adc)
**Tech Stack:** Go 1.26 + React 19 + SQLite/PostgreSQL + Docker
**Coverage:** Full codebase — backend (Go), frontend (React/TypeScript), infrastructure (Docker, CI/CD)

---

## Executive Summary

| Severity | Count |
|----------|-------|
| Critical | 1 |
| High | 5 |
| Medium | 18 |
| Low | 14 |
| Info | 10+ |

**No critical vulnerabilities found in existing code.** The single critical finding (AUTHZ-009, MCP tools) was introduced in a recent commit and has no patch yet.

The most severe issues are:
1. **MCP tools lack admin-only protection** (CRITICAL — cross-tenant resource creation, commit c6105a2)
2. **Auto-generated admin credentials written to disk** (HIGH — credential exposure)
3. **Docker client v28.5.2 known vulnerabilities** (HIGH — upstream, no patch yet)

The codebase benefits from good security fundamentals: bcrypt password hashing, AES-256-GCM encryption, JWT with rotation, CSRF middleware, parameterized SQL, and consistent tenant isolation. The issues are primarily defense-in-depth gaps.

---

## Phase 1: Reconnaissance

### Tech Stack
- **Backend:** Go 1.26.1, 20-module architecture, event-driven
- **Frontend:** React 19.2.5, React Router 7, Zustand 5, Vite 8, Tailwind 4
- **Database:** SQLite (default), PostgreSQL via pgx v5 (enterprise)
- **KV Store:** BBolt (config, state, metrics, API keys, sessions)
- **Container:** Docker client v28.5.2 (known vulnerable — DEP-006/DEP-007)
- **Auth:** JWT HS256 (access 15min, refresh 7 days) + API keys
- **Secrets:** AES-256-GCM with Argon2id KDF, key rotation

### Architecture
```
cmd/deploymonster/main.go
  └── internal/{api,auth,secrets,db,deploy,build,compose,swarm,dns,webhooks,ingress,...}
web/src/   (embedded at build time → internal/api/static/)
```

### Entry Points
- **CLI:** `deploymonster serve|config|setup|rotate-keys|init`
- **HTTP:** `net/http.ServeMux` with 70+ routes under `/api/v1/`
- **WebSocket:** Terminal, logs, deploy progress, events
- **Agent:** `--agent --master=<url> --token=<token>` for worker nodes

### Security Boundaries
1. **Auth layer** — JWT + API key dual authentication
2. **Tenant isolation** — `requireTenantApp` on all app-scoped endpoints
3. **RBAC** — `protectedPerm()` + role hierarchy checks
4. **Secrets vault** — encryption at rest with master key
5. **Webhook signatures** — HMAC verification for GitHub/GitLab/Stripe

---

## Phase 2: Vulnerability Findings

### CRITICAL

#### AUTHZ-009: MCP Tools Lack Admin-Only Protection — Cross-Tenant Resource Creation
- **CWE:** CWE-284 (Improper Authorization)
- **File:** `internal/api/router.go:653`, `internal/api/handlers/handler.go:72-517`
- **CVSS:** AV:N/AC:L/PR:L/UI:N/S:C/C:H/I:H/A:H — 9.9 Critical
- **Confidence:** 95/100

MCP tool routes use only `protected` middleware (auth + tenant rate-limit) without `adminOnly` or `protectedPerm` checks. The handler implements cross-tenant operations:

- `listApps` with empty `tenant_id` returns ALL tenant apps (`handler.go:72-85`)
- `deployApp` accepts arbitrary `tenant_id`, defaults to first available (`handler.go:102-160`)
- `addDomain` associates domain with any `app_id` without tenant verification (`handler.go:324-383`)
- `marketplaceDeploy` deploys to any tenant (`handler.go:386-455`)
- `provisionServer` creates servers without tenant restriction (`handler.go:457-517`)

**Impact:** Any authenticated user can create resources in any tenant, transfer apps between tenants, or enumerate all apps across the platform.

**Remediation:** Add `adminOnly` middleware to all MCP routes, or implement RBAC with explicit tool-level permissions.

---

### HIGH

#### VULN-001: Auto-Generated Super Admin Credentials Written to Unencrypted File
- **CWE:** CWE-798 (Hardcoded Credentials)
- **File:** `internal/auth/module.go:126-135`
- **CVSS:** AV:L/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N — 7.8 High
- **Confidence:** 95/100

During first-run setup, if `MONSTER_ADMIN_PASSWORD` is not set, a 16-character password is auto-generated and written to a `.credentials` file with `0600` permissions. No automatic deletion after first login.

**Remediation:** Print credentials to stdout only. Require `MONSTER_ADMIN_PASSWORD` env var in production.

#### VULN-002: Docker Client v28.5.2 Known AuthZ and Plugin Privilege Issues
- **CWE:** CWE-863 / CWE-250
- **File:** `go.mod:9`
- **CVSS:** AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H — 8.8 High
- **Confidence:** 90/100

`github.com/docker/docker v28.5.2+incompatible` carries known AuthZ bypass and plugin privilege escalation vulnerabilities (DEP-006, DEP-007). Project maintainers acknowledged: *"DeployMonster does not use AuthZ plugins, so not affected in practice."* Upgrade to v29+ when available.

#### VULN-003: Predictable Default Super Admin Email
- **CWE:** CWE-521 (Weak Password Requirements)
- **File:** `internal/auth/module.go:99`
- **Confidence:** 85/100

Default super admin email is `admin@deploy.monster` when `MONSTER_ADMIN_EMAIL` is unset. This predictable account is a target for credential stuffing.

**Remediation:** Make `MONSTER_ADMIN_EMAIL` mandatory; refuse to start if unset.

#### VULN-004: No Multi-Factor Authentication (MFA/TOTP)
- **CWE:** CWE-308 (Use of Single-factor Authentication)
- **File:** `internal/api/handlers/auth.go`
- **Confidence:** 90/100

Login accepts only email and password. The `users` table has an unused `totp_enabled` column.

**Remediation:** Implement TOTP enrollment, verification, and backup codes.

#### CRYPTO-001: Hardcoded Legacy Vault Salt Seed
- **CWE:** CWE-321 (Use of Hard-coded Cryptographic Key)
- **File:** `internal/secrets/vault.go:25`
- **CVSS:** AV:L/AC:L/PR:H/UI:N/S:U/C:H/I:N/A:N — 6.8 Medium
- **Confidence:** 80/100

```go
const legacyVaultSaltSeed = "deploymonster-vault-salt-v1"
```

Pre-Phase-2 deployments share identical salt derivation for encrypted secrets. If master password is compromised, all pre-Phase-2 secrets across all deployments are recoverable.

**Remediation:** Document removal timeline. Ensure all installations have migrated to Phase-2 key derivation.

#### AUTHZ-010: MCP `list_apps` with Empty tenant_id Returns All Tenant Apps
- **CWE:** CWE-639 (Authorization Bypass Through User-Controlled Key)
- **File:** `internal/api/handlers/handler.go:72-85`
- **Confidence:** 90/100

`listApps` calls `store.ListAppsByTenant(ctx, "", 50, 0)` with empty tenant ID when no tenant is specified, returning apps from ALL tenants.

**Remediation:** Always require a valid tenant ID; reject empty string.

---

### MEDIUM

| ID | Finding | CWE | File | Confidence |
|----|---------|-----|------|------------|
| AUTH-005 | HS256 JWT — secret key compromise allows token forgery | CWE-347 | `auth/jwt.go:142` | 85/100 |
| AUTH-006 | SameSite=Lax (not Strict) on auth cookies | CWE-1275 | `auth.go:49-76` | 85/100 |
| AUTHZ-006 | No fine-grained RBAC on app mutations beyond tenant | CWE-862 | `router.go` | 80/100 |
| AUTHZ-007 | Invite creation lacks role hierarchy validation | CWE-269 | `invites.go:30-99` | 75/100 |
| CMD-001 | Command blocklist bypass potential in exec | CWE-78 | `exec.go:18-31` | 75/100 |
| RCE-001 | Build pipeline runs user-controlled git/docker | CWE-78 | `builder.go:59-156` | 80/100 |
| SSRF-001 | Git URL validation allows git:// and ssh:// | CWE-918 | `builder.go:206-258` | 70/100 |
| SSRF-002 | Outbound webhooks may reach internal networks | CWE-918 | `webhooks/` | 65/100 |
| FRAME-001 | No X-Frame-Options or CSP frame-ancestors | CWE-1021 | `router.go:76-94` | 75/100 |
| PATH-001 | File browser may allow path traversal | CWE-22 | `router.go:366` | 65/100 |
| MASS-001 | PATCH handlers may allow mass assignment | CWE-915 | `router.go:140` | 60/100 |
| UPLOAD-001 | Certificate upload without file type validation | CWE-434 | `router.go:431-432` | 65/100 |
| INJECT-001 | DB backup password in env variable | CWE-258 | `backup/engine.go:193-197` | 70/100 |
| ID-001 | JWT JTI fallback has reduced entropy | CWE-331 | `id.go:14-24` | 70/100 |
| CONFIG-001 | TLS minimum 1.2 (should be 1.3) | CWE-326 | `ingress/module.go:261` | 80/100 |
| CSP-001 | CSP allows unsafe-inline for scripts | CWE-1033 | `web/index.html:13` | 80/100 |
| RATE-001 | GlobalRateLimiter trusts XFF without validation | CWE-285 | `global_ratelimit.go:138` | 70/100 |
| SMTP-001 | SMTP InsecureSkipVerify option exists | CWE-295 | `smtp.go:39` | 60/100 |

---

### LOW

| ID | Finding | CWE | File |
|----|---------|-----|------|
| AUTH-001 | JWT secret min 16 in config, 32 in service (inconsistent) | CWE-256 | `config.go:261`, `jwt.go:58` |
| AUTH-002 | Invite token hashed with SHA256, not bcrypt | CWE-327 | `invites.go:127-130` |
| AUTH-003 | API key prefix lookup allows enumeration | CWE-287 | `bolt.go:216-243` |
| AUTH-004 | Master secret held in `string` type (cannot be zeroed) | CWE-316 | `module.go:39` |
| AUTHZ-001 | Deploy approval returns 200 OK on non-pending status | CWE-667 | `deploy_approval.go:80-108` |
| AUTHZ-002 | Topology load missing tenant isolation | CWE-639 | `topology.go:144-184` |
| AUTHZ-003 | Bulk ops silently omits cross-tenant IDs (info leak) | CWE-285 | `bulk.go:66-70` |
| AUTHZ-004 | Role hierarchy not enforced on all assignment paths | CWE-269 | `invites.go:64-68` |
| CRYPTO-001 | Argon2id uses only 1 iteration | CWE-327 | `vault.go:67` |
| CRYPTO-002 | Self-signed certs use P-256 (could be P-384) | CWE-327 | `tls.go:135-136` |
| CRYPTO-003 | GenerateID fallback uses sequential big.Int | CWE-338 | `id.go:19-23` |
| SESSION-001 | No automatic secret zeroing after use | CWE-316 | `secrets/module.go` |
| SESSION-002 | No core dump protection | CWE-535 | — |
| ERROR-001 | Panic recovery logs stack trace (could contain secrets) | CWE-209 | `middleware.go:57-73` |
| PPROF-001 | pprof endpoints lack CSRF protection | CWE-352 | `router.go:682-688` |

---

## Positive Security Patterns

| Pattern | Location | Assessment |
|---------|----------|------------|
| Passwords hashed with bcrypt cost 13 | `auth/password.go:10` | Strong |
| AES-256-GCM with random nonce per encryption | `secrets/vault.go:71-90` | Strong |
| Argon2id KDF (64MB, 4 threads) | `secrets/vault.go:67` | Strong |
| Key rotation re-encrypts all data (not just new) | `secrets/module.go:305-349` | Strong |
| JWT secret minimum 32 chars enforced | `auth/jwt.go:56-61` | Strong |
| JWT refresh token rotation with revocation | `auth.go:325-330` | Strong |
| Access token revocation on logout | `auth/jwt.go:199-223` | Strong |
| API keys bcrypt hashed | `auth/apikey.go:52-58` | Strong |
| crypto/rand for all sensitive randomness | Multiple files | Strong |
| Parameterized SQL queries | All DB handlers | Strong |
| Consistent tenant isolation via `requireTenantApp` | `helpers.go` | Strong |
| Session fixation prevention | `auth.go:126-129` | Strong |
| CSRF double-submit cookie with `__Host-` prefix | `middleware/csrf.go` | Strong |
| Stripe webhook HMAC-SHA256 with timestamp validation | `stripe.go:132-164` | Strong |
| Git provider webhook signature verification | `receiver.go:354-419` | Strong |
| Idempotency middleware with mutex locking | `middleware/idempotency.go` | Strong |
| Global rate limit 120 req/min/IP | `router.go:58` | Strong |
| Security headers (X-Content-Type-Options, CSP, HSTS) | `security_headers.go` | Strong |
| Docker capabilities dropped (non-privileged) | `deploy/docker.go:118-123` | Strong |
| Systemd hardening (NoNewPrivileges, ProtectSystem, etc.) | `deployments/deploymonster.service:45-54` | Strong |
| docker-socket-proxy for least-privilege Docker access | `deployments/docker-compose.hardened.yaml` | Best Practice |
| Trivy image scanning on release | `release.yml:67-75` | Strong |
| SBOM generation via GoReleaser | `.goreleaser.yml:43-46` | Strong |
| Gitleaks SHA256-pinned binary | `ci.yml:205-215` | Strong |
| --frozen-lockfile for pnpm | CI pipeline | Strong |
| AWS S3 uses SigV4 | `backup/s3.go:188-194` | Strong |

---

## Dependency Audit

### High Risk
| Dependency | Version | Issue | Status |
|------------|---------|-------|--------|
| `github.com/docker/docker` | v28.5.2 | AuthZ bypass + plugin privilege escalation (DEP-006/007) | Monitor for v29 |

### Medium Risk
| Dependency | Version | Issue | Status |
|------------|---------|-------|--------|
| `dagre` | v0.8.5 | Unmaintained since 2017 | Migrate to `@dagrejs/dagre` |
| `stale pnpm overrides` | — | Dead code (vite@7, lodash@4) | Remove |
| `modernc.org/sqlite` | stack | BSD-3 attribution required | Document in NOTICE |

### Current (No Issues)
- React 19.2.5, React Router 7.14.1, Zustand 5.0.12, Vite 8.0.10, Tailwind 4.2.2
- Go JWT v5.3.1, pgx v5.9.2, bbolt v1.4.3, gorilla/websocket v1.5.3
- golang.org/x/crypto v0.50.0

---

## Remediation Roadmap

### Immediate (P0)
1. Add `adminOnly` to MCP routes (`router.go:653`) — blocks cross-tenant resource creation
2. Make `MONSTER_ADMIN_EMAIL` and `MONSTER_ADMIN_PASSWORD` mandatory — refuses to start if unset
3. Print auto-generated credentials to stdout, do not write to disk

### High Priority (P1)
4. Upgrade Docker client to v29+ when available (monitor upstream)
5. Implement TOTP MFA for login
6. Enforce role hierarchy on all role assignment paths
7. Replace blocklist command filtering with allowlist or restricted shell
8. Disable `git://` scheme in `ValidateGitURL`

### Medium Priority (P2)
9. Add `protectedPerm` checks to all destructive app endpoints
10. Add `X-Frame-Options: DENY` and `CSP: frame-ancestors 'none'`
11. Upgrade TLS minimum to 1.3
12. Add private IP blocking to outbound webhook delivery
13. Validate file browser paths against container root
14. Implement field whitelisting for PATCH/PUT handlers
15. Add `go mod verify` to CI pipeline
16. Pin all GitHub Actions to SHA commits
17. Migrate `dagre` → `@dagrejs/dagre`

### Low Priority / Design (P3)
18. Consider RS256 for asymmetric JWT (cost: key management complexity)
19. Remove hardcoded `legacyVaultSaltSeed` after full migration
20. Use `pgpassfile` instead of `PGPASSWORD` env var
21. Fail closed on entropy exhaustion in `GenerateID`
22. Add before/after values to audit log entries
23. Use `SameSite=Strict` on auth cookies (break cross-origin API usage)

---

*Generated by security-check skill — Phase 4 Report — 2026-04-30*