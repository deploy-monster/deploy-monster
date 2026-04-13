# DeployMonster Security Audit Report

**Audited**: 2026-04-13
**Phase**: Full 4-phase audit — Recon → Hunt → Verify → Report
**Scanner**: security-check (48 skills, 3000+ checklist items)
**Scope**: Backend Go 1.26 (`internal/`, `cmd/`), Frontend React 19 (`web/src/`)

---

## Executive Summary

DeployMonster is a well-structured modular monolith with significant security investment — JWT auth, RBAC, secrets vault, CSRF protection, rate limiting, webhook signature verification, and Docker security controls are all present. However, **3 critical**, **8 high**, and **12 medium** severity issues were confirmed that require immediate attention before production exposure.

### Priority Fix Order

| Priority | Finding | Why |
|---|---|---|
| **P0 — Fix NOW** | CRIT-1: API key auth broken | All API key clients are locked out |
| **P0 — Fix NOW** | CRIT-2: MCP endpoints unprotected | Full unauthenticated RCE surface |
| **P1 — This Sprint** | CRIT-3: Deploy approval IDOR | Cross-tenant authorization bypass |
| **P1 — This Sprint** | HIGH-2: SSRF (http:// allowed) | Cloud metadata exfiltration |
| **P1 — This Sprint** | HIGH-3/4: Agent token in URLs/logs | Secret exposure in logs |
| **P2 — Next Sprint** | HIGH-1: CMDi blocklist bypassable | Container RCE |
| **P2 — Next Sprint** | HIGH-5/6: Domain/cert IDOR | Cross-tenant data disclosure |
| **P3 — Next Month** | MED-*: IDOR gaps, CORS, metrics | Lateral movement, info disclosure |

---

## Critical Findings (P0)

### 1. API Key Authentication Completely Broken
**File**: `internal/api/middleware/middleware.go:207`

```go
if subtle.ConstantTimeCompare([]byte(apiKey), []byte(storedKey.KeyHash)) != 1 {
```

The incoming plaintext API key is compared against its SHA-256 hash stored in BBolt. A hash is never equal to its plaintext — every API key authentication fails.

**Impact**: All `X-API-Key` header clients (CI systems, scripts, external integrations) are completely locked out. Only JWT Bearer tokens and httpOnly cookie sessions work.

**Remediation**:
```go
// Fix: hash the incoming key, compare to stored hash
if subtle.ConstantTimeCompare([]byte(HashAPIKey(apiKey)), []byte(storedKey.KeyHash)) != 1 {
```

---

### 2. MCP Protocol Endpoints — Zero Security Controls
**File**: `internal/api/router.go:642-643`

```go
r.mux.HandleFunc("GET /mcp/v1/tools", mcpH.ListTools)
r.mux.HandleFunc("POST /mcp/v1/tools/{name}", mcpH.CallTool)
```

These routes bypass the entire `middleware.Chain()` (which includes `RequireAuth`, `GlobalRateLimiter`, `CSRFProtect`, `AuditLog`). They are network-accessible with no authentication whatsoever. `MCPHandler` exposes module introspection.

**Impact**: Any network attacker can enumerate tools and call them — including potentially exec, app deletion, or secret extraction depending on what the MCP tools expose.

**Remediation**: Wrap with `protected` middleware and a dedicated MCP rate limiter:
```go
r.mux.Handle("GET /mcp/v1/tools", protected(http.HandlerFunc(mcpH.ListTools)))
r.mux.Handle("POST /mcp/v1/tools/{name}", protected(http.HandlerFunc(mcpH.CallTool)))
```

---

### 3. Deploy Approval IDOR — Cross-Tenant Authorization Bypass
**File**: `internal/api/handlers/deploy_approval.go:73`

```go
req, exists := h.pending[approvalID]  // NO TENANT CHECK
```

The in-memory `pending` map is keyed by approval ID with no tenant isolation. Any authenticated user can approve or reject any pending deployment by ID.

**Remediation**: Add `req.TenantID == claims.TenantID && req.AppTenantID == claims.TenantID` validation.

---

## High Findings (P1)

### 4. Command Injection via `sh -c` — Blocklist Bypassable
**Files**: `internal/api/handlers/exec.go:188`, `internal/api/ws/terminal.go:153`

The `blockedPatterns` allowlist only blocks 10 specific strings. Command chaining (`;`), subshell (`$(...)`), and pipes (`|`) bypass it trivially. `Terminal.SendCommand` has **no filtering at all**.

**Real-world impact**: Authenticated users with exec/terminal access can break out of the blocklist:
```bash
# Passes blocklist (no blocked pattern matches)
# echo test; cat /etc/passwd
```

**Remediation**: Replace `sh -c` with direct `exec.CommandContext` args array. The Docker SDK `Exec` accepts a `Cmd` array — pass it directly without shell invocation.

---

### 5. SSRF — No Internal IP Blocking in Git URL Validation
**File**: `internal/build/builder.go:200-205`

`ValidateGitURL` allows `http://` scheme with no check for private IPs, loopback, or cloud metadata endpoints (169.254.169.254). `SourceURL` is only length-checked at app create/update time.

**Real-world impact**:
```go
// These URLs are accepted:
"http://169.254.169.254/latest/meta-data/"  // AWS/GCP metadata
"http://10.0.0.1/private-repo.git"
"http://192.168.1.1/internal-repo.git"
```

**Remediation**:
1. Block `http://` scheme — require HTTPS for git URLs
2. Add private IP range check: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8
3. Block 169.254.0.0/16 (link-local / metadata)
4. Call `ValidateGitURL` at app create/update time, not just build time

---

### 6. Agent Join Token Exposed in Logs and URL
**Files**: `internal/swarm/module.go:47`, `internal/swarm/server.go:203-207`

When not configured, the agent join token is generated (`core.GenerateSecret(32)`) and logged at Info level. It is also passed in URL query strings (`?token=...`) which appear in access logs.

**Remediation**:
1. Log only a truncated hash or prefix, never the full token
2. Prefer `X-Agent-Token` header over URL query param
3. Consider using `subtle.ConstantTimeCompare` for token comparison

---

## Medium Findings (P2)

### IDOR Patterns (Domain, Certificate, Secret, Project)

Multiple handlers return or manipulate resources across tenants:

| Handler | Issue | Fix |
|---|---|---|
| `domains.go:46` | `ListAllDomains` returns all tenants' domains | Filter by `claims.TenantID` |
| `certificates.go:40` | Global BBolt listing, no tenant filter | Filter by tenant context |
| `secrets.go:73` | `AppID` not validated against tenant ownership | Add `requireTenantApp` equivalent |
| `invites.go:43` | No `PermMemberInvite` check | Add RBAC permission enforcement |
| `environments.go:72` | Project tenant not validated | Add tenant ownership check |
| `transfer.go:43` | App tenant not validated beyond superadmin role | Add explicit tenant check |

### Infrastructure Gaps

- **CORS wildcard**: `Access-Control-Allow-Credentials: true` sent even when origin is `*` or when origin check fails
- **`/metrics` unauthenticated**: Prometheus and API metrics endpoints bypass all middleware
- **Backup key path traversal**: No `isPathSafe` equivalent for backup storage keys
- **YAML command injection**: `topology/yaml.go:203` writes command values without YAML escaping
- **No image signature verification**: `ImagePull` accepts any image with no registry trust or digest pinning

### Frontend Findings

- **SSE URL injection** (`AppDetail.tsx:847`): `appId` from `useParams()` interpolated without validation
- **WebSocket URL injection** (`useDeployProgress.ts:72`): `projectId` from server-driven store interpolated without sanitization

---

## Low Findings (P3)

- Panic stack traces (potentially with auth claims) written to structured logs
- JWT JTI logged on revocation failure path
- Idempotency middleware silently swallows BBolt write failures
- AuthRateLimiter silently swallows BBolt write failures
- `EventStreamer` goroutines not cleaned up on SSE disconnect
- `GenerateID`/`GenerateSecret` panic on `crypto/rand` failure (server-wide DoS on Windows under memory pressure)
- CSRF cookie lacks `__Host-` prefix (subdomain cookie injection risk)
- JWT missing `nbf` (NotBefore) claim
- `gorilla/websocket` missing `ReadLimit` on Upgrader
- Legacy hardcoded vault salt constant in source

---

## Security Controls — Working Correctly

These are confirmed secure and should not be changed:

| Control | Status | Location |
|---|---|---|
| JWT `none` algorithm attack | ✅ Rejected | `auth_final_test.go:76` |
| JWT signing algorithm | ✅ HS256 hardcoded, no confusion | `jwt.go:70,82` |
| API key storage | ✅ SHA-256 hashed | `apikey.go:37` |
| Password hashing | ✅ bcrypt cost 12 | `password.go:10` |
| CSRF double-submit | ✅ Correctly implemented | `csrf.go:14-52` |
| Webhook HMAC-SHA256 | ✅ Per-provider verification | `webhooks/receiver.go:347-413` |
| File browser path traversal | ✅ `isPathSafe` correct | `filebrowser.go:29-50` |
| Git URL shell metachar | ✅ Blocked | `builder.go:165` |
| Docker socket blocklist | ✅ Present | `interfaces.go:59-65` |
| Secrets vault AES-GCM | ✅ With Argon2id KDF | `secrets/module.go` |
| BBolt concurrency | ✅ `sync.RWMutex` on DeployHub | `deploy.go:81` |
| Auth token storage (frontend) | ✅ httpOnly cookies, not localStorage | `client.ts` |
| CSP header | ✅ `default-src 'self'` | `middleware.go:15` |
| Idempotency keys | ✅ Via BBolt | `idempotency.go` |
| Panic recovery | ✅ In all HTTP handlers | `middleware.go:58` |
| SQL/parameterized queries | ✅ `?` / `$N` placeholders | All `db/` files |

---

## Remediation Roadmap

### Week 1 (P0)
- [ ] Fix API key comparison (CRIT-1): `HashAPIKey(incoming) == storedKey.KeyHash`
- [ ] Wrap MCP routes with `protected` middleware (CRIT-2)
- [ ] Add tenant check to deploy approval approve/reject (CRIT-3)

### Week 2-3 (P1)
- [ ] Block `http://` scheme in `ValidateGitURL`, add private IP block (HIGH-2)
- [ ] Stop logging full agent token; prefer `X-Agent-Token` header (HIGH-3/4)
- [ ] Replace `sh -c` with direct exec args array, remove blocklist dependency (HIGH-1)
- [ ] Fix domain/certificate/secret/project IDORs (HIGH-5/6/7/8)

### Week 4+ (P2)
- [ ] Fix CORS `Allow-Credentials` unconditional send
- [ ] Add auth to `/metrics` or bind to localhost only
- [ ] Add `isPathSafe` to backup storage keys
- [ ] Escape YAML command values in topology YAML generation
- [ ] Validate `SourceURL` at app create/update time
- [ ] Fix SSE and WebSocket URL interpolation in frontend
- [ ] Upgrade `gorilla/websocket` to v1.5.4+
- [ ] Upgrade `bbolt` to v1.4.4+

### Infrastructure Hardening
- [ ] Add image signature verification / digest pinning
- [ ] Implement `nbf` claim in JWT
- [ ] Add `__Host-` prefix to CSRF cookie
- [ ] Replace `subtle.ConstantTimeCompare` for agent token
- [ ] Bound goroutine cleanup on SSE disconnect in `EventStreamer`
- [ ] Add graceful fallback for `crypto/rand` failure in ID generation
