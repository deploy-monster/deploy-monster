# Verified Security Findings — DeployMonster

## CRITICAL

### CRIT-1: API Key Authentication Completely Broken
**File**: `internal/api/middleware/middleware.go:207`

The middleware compares the plaintext API key from the `X-API-Key` header against `storedKey.KeyHash` — a SHA-256 hash stored in the DB. This comparison will **always fail** since the plaintext can never equal its own hash.

```go
// middleware.go:207
if subtle.ConstantTimeCompare([]byte(apiKey), []byte(storedKey.KeyHash)) != 1 {
```

- `GenerateAPIKey()` stores `Hash: HashAPIKey(key)` in DB (`apikey.go:27`)
- `KeyHash` is the SHA-256 of the original key
- Plaintext API key is never compared to a stored plaintext

**Impact**: All `X-API-Key` header authentication is non-functional. Only JWT Bearer token and `dm_access` cookie auth work.
**Fix**: Compare `HashAPIKey(apiKey)` against `storedKey.KeyHash`, or store the key differently.
**CVSS**: 9.1 — Complete auth bypass for API key clients.

---

### CRIT-2: MCP Endpoints — Zero Auth, Zero Rate-Limit, Zero CSRF
**File**: `internal/api/router.go:642-643`

```go
r.mux.HandleFunc("GET /mcp/v1/tools", mcpH.ListTools)
r.mux.HandleFunc("POST /mcp/v1/tools/{name}", mcpH.CallTool)
```

`/mcp/v1/*` is registered directly on the `*http.ServeMux` bypassing the full `middleware.Chain()`. No `RequireAuth`, no `GlobalRateLimiter` (which only covers `/api/` and `/hooks/`), no CSRF, no audit log.

**Impact**: Any unauthenticated network attacker can enumerate and call MCP tools — potentially exec, app deletion, secret extraction.
**Fix**: Wrap MCP routes with `protected` middleware and add a rate limiter.
**CVSS**: 9.1 — Network-exploitable, no auth, wide destructive surface.

---

### CRIT-3: Deploy Approval IDOR — Any User Can Approve/Reject Any Pending Deploy
**File**: `internal/api/handlers/deploy_approval.go:73`

```go
req, exists := h.pending[approvalID]  // NO TENANT CHECK
if exists && req.Status == "pending" {
    req.Status = "approved"
    req.ReviewedBy = claims.UserID
```

The `Approve`/`Reject` handlers look up from an in-memory map with no tenant or app ownership validation. A user from tenant A can approve/reject deployments for tenant B's apps.
**CVSS**: 8.1

---

## HIGH

### HIGH-1: CMDi via `sh -c` — Blocklist Bypassable
**Files**: `internal/api/handlers/exec.go:188`, `internal/api/ws/terminal.go:153`

```go
cmd = []string{"sh", "-c", req.Command}
output, err := h.runtime.Exec(r.Context(), containerID, cmd)
```

Only defense is a 10-pattern blocklist (`blockedPatterns`). Command chaining (`;`), subshell (`$(...)`), and pipes (`|`) are NOT blocked. Example bypass: `echo test; cat /etc/passwd`.

Note: `Terminal.SendCommand` has **no blocklist at all**.

**CVSS**: 8.0
**Fix**: Use `exec.CommandContext` with explicit args (no shell), or implement proper allowlist of permitted commands.

---

### HIGH-2: SSRF — No Internal IP Blocking in Git URL Validation
**File**: `internal/build/builder.go:200-205`

```go
switch parsed.Scheme {
case "https", "http", "ssh", "git", "file":
    // allowed
```

`http://` scheme is allowed with no check for private IP ranges. No `isPrivateIP`, `isLoopback`, or metadata endpoint blocking exists anywhere.

Attackable via `SourceURL` field — which is only length-checked (max 2048) at `apps.go:72` and `app_update.go:38`, not validated with `ValidateGitURL` until build time.

Possible targets:
- `http://169.254.169.254/latest/meta-data/` — cloud metadata
- `http://10.0.0.1/private-repo.git` — private subnets
- `http://127.0.0.1:8080/repo.git` — localhost port scan

**CVSS**: 8.1
**Fix**: Block `http://` scheme (use HTTPS-only), add private IP range checks, block cloud metadata endpoints.

---

### HIGH-3: Agent Join Token Logged in Plaintext at Startup
**File**: `internal/swarm/module.go:47`

```go
m.logger.Info("generated agent join token...", "token", token)
```

When `swarm.join_token` is not configured, a fresh 32-byte secret is generated and logged at Info level. Any log reader (operators, log aggregators, cloud logging) gains full agent access.

**CVSS**: 7.7
**Fix**: Log only a hash or prefix of the token, not the full secret.

---

### HIGH-4: Agent Token in URL Query String — Direct String Comparison
**File**: `internal/swarm/server.go:203-207`

```go
token := r.URL.Query().Get("token")
if token != s.expectedToken {
    return
}
```

Token appears in access logs, browser history, and Referer headers. Direct string comparison (not `subtle.ConstantTimeCompare`). Also accepted via `X-Agent-Token` header — prefer the header.
**CVSS**: 7.1

---

### HIGH-5: Domain Handler IDOR — `ListAllDomains` Leaks Cross-Tenant Data
**File**: `internal/api/handlers/domains.go:46`

```go
domains, err = h.store.ListAllDomains(r.Context())  // NO TENANT FILTER
```

When `app_id` query param is absent, returns ALL domains across ALL tenants.
**CVSS**: 6.5

---

### HIGH-6: Certificate Handler IDOR — Global Certificate Listing
**File**: `internal/api/handlers/certificates.go:40`

```go
_ = h.bolt.Get("certificates", "all", &cs)  // NO TENANT FILTER
```

Reads all certificates from a global BBolt bucket. Includes `cert_pem` and `key_pem` data.
**CVSS**: 7.5

---

### HIGH-7: Secret Handler — App Scope Without Ownership Validation
**File**: `internal/api/handlers/secrets.go:73`

```go
AppID: req.AppID,  // NO VALIDATION that user owns this app
```

When creating a secret with `scope: "app"`, `req.AppID` is accepted without verifying app ownership or tenant membership.
**CVSS**: 6.5

---

### HIGH-8: Invite Handler — No Permission Check
**File**: `internal/api/handlers/invites.go:43`

```go
// No check for PermMemberInvite or PermMemberManage
if req.Email == "" || req.RoleID == "" {
```

Any authenticated user (including `role_viewer`) can invite new users at any role level.
**CVSS**: 6.5

---

## MEDIUM

### MED-1: Path Traversal in Backup Storage Key
**File**: `internal/backup/local.go:31,47,56`

```go
path := filepath.Join(l.basePath, key)
f, err := os.Create(path)
```

Backup `key` parameter is joined directly to `basePath` with no sanitization. `../../../etc/passwd` could escape the backup directory.

**CVSS**: 6.5
**Fix**: Add `isPathSafe` equivalent for backup keys.

---

### MED-2: CORS Wildcard + `Allow-Credentials: true`
**File**: `internal/api/middleware/middleware.go:116,132`

```go
if allowedOrigins == "*" {
    w.Header().Set("Access-Control-Allow-Origin", "*")
}
w.Header().Set("Access-Control-Allow-Credentials", "true")  // always sent
```

Browsers drop credentials when origin is `*`. Also, credentials header is sent unconditionally even when origin check fails.
**CVSS**: 6.5

---

### MED-3: `/metrics` Endpoints Unauthenticated
**File**: `internal/api/router.go:666-668`

Prometheus metrics and API metrics registered via `HandleFunc` directly, bypassing all middleware. Exposes internal operational data to network attackers.
**CVSS**: 5.3

---

### MED-4: SourceURL Not Validated at App Create/Update
**Files**: `internal/api/handlers/apps.go:72`, `internal/api/handlers/app_update.go:38`

Only length check (max 2048). `ValidateGitURL` is never called — URL is stored and only validated at build time.
**CVSS**: 6.5

---

### MED-5: Invite Token / API Key Returned in Response Body
**Files**: `internal/api/handlers/invites.go:80`, `internal/api/handlers/admin_apikeys.go:93`

Full token returned in response. While "displayed once" is correct UX, no TLS enforcement warning.
**CVSS**: 6.5 each

---

### MED-6: YAML Config Injection — Unescaped Command in Compose YAML
**File**: `internal/topology/yaml.go:203`

```go
sb.WriteString(fmt.Sprintf("%scommand: %s\n", pad, s.Command))
```

Command value written directly to YAML without escaping. Newlines or YAML special chars could inject arbitrary YAML structure.
**CVSS**: 5.3

---

### MED-7: `${SECRET:...}` References Not Masked
**File**: `internal/api/handlers/envvars.go:103`

```go
if strings.HasPrefix(value, "${SECRET:") {
    return value  // exposed as-is
}
```

Secret names disclosed in GET env response — aids targeted vault attacks.
**CVSS**: 3.5

---

### MED-8: Env Var Export — Value Injection
**File**: `internal/api/handlers/env_import.go:116-117`

```go
w.Write([]byte(v.Key + "=" + v.Value + "\n"))
```

Newlines in `v.Value` could inject additional env vars into exported `.env`.
**CVSS**: 3.1

---

### MED-9: No Image Signature Verification
**File**: `internal/deploy/docker.go:86,331`

`ImagePull` accepts any image string. No registry auth, no Content Trust, no SHA256 digest pinning.
**CVSS**: 5.3

---

### MED-10: Workspace Probing via `curl | sh` Pattern (SSRF Adjacent)
**File**: `internal/build/builder.go:278-279`

Build args from EnvVars could be used to probe internal networks via curl in Dockerfile templates (not currently exploitable — EnvVars not passed to BuildOpts in production).

---

### MED-11: SSE URL from Unsanitized Route Parameter
**File**: `web/src/pages/AppDetail.tsx:847`

```typescript
new EventSource(`/api/v1/apps/${appId}/logs/stream`)
```

`appId` from `useParams()` interpolated directly into URL without validation.
**CVSS**: 6.5

---

### MED-12: WebSocket URL from Server-Returned projectId
**File**: `web/src/hooks/useDeployProgress.ts:72`

```typescript
const wsUrl = `${protocol}//${window.location.host}/api/v1/topology/deploy/${projectId}/progress`
```

`projectId` from server-driven topology store interpolated without sanitization.
**CVSS**: 5.3

---

## LOW

### LOW-1: Agent Token Logged During Git Clone
**File**: `internal/build/builder.go:241-243`

Git token in HTTPS URL may appear in git's output to logWriter.
**CVSS**: 3.1

---

### LOW-2: Panic Stack Traces Written to Structured Logs
**Files**: `internal/api/middleware/middleware.go:65`, `internal/core/events.go:149`, `internal/core/safego.go:21`

Goroutine stack traces (potentially containing auth claims, request context) logged on panic.
**CVSS**: 3.5

---

### LOW-3: JWT JTI Logged on Revocation Failure
**File**: `internal/api/handlers/auth.go:312,363`

JTI of refresh token written to error logs on BBolt write failure.
**CVSS**: 3.5

---

### LOW-4: Idempotency Middleware — Swallowed BBolt Write Errors
**File**: `internal/api/middleware/idempotency.go:75`

```go
_ = bolt.Set(idempotencyBucket, scopedKey, entry, idempotencyTTLSecs)
```

Failed cache writes silently ignored — subsequent retries with same key re-execute the operation.
**CVSS**: 2.1

---

### LOW-5: AuthRateLimiter — Swallowed BBolt Write Errors
**File**: `internal/api/middleware/ratelimit.go:56,71`

Failed rate limit counter updates silently ignored — bypass possible if DB write fails.
**CVSS**: 2.1

---

### LOW-6: EventStreamer — Unbounded Goroutine Subscription Without Cleanup
**File**: `internal/api/ws/logs.go:117-124`

SubscribeAsync creates a goroutine per SSE connection that is never unregistered on disconnect.
**CVSS**: 2.5

---

### LOW-7: `GenerateID` Panic on Crypto Failure
**File**: `internal/core/id.go:14`

```go
if _, err := rand.Read(b); err != nil {
    panic("crypto/rand failed: " + err.Error())
}
```

Panics entire server if `crypto/rand` fails (possible on Windows under memory pressure).
**CVSS**: 4.3

---

### LOW-8: Legacy Hardcoded Vault Salt
**File**: `internal/secrets/vault.go:25`

```go
const legacyVaultSaltSeed = "deploymonster-vault-salt-v1"
```

Hardcoded salt for backwards-compat migration. Acceptable if master secret is strong.
**CVSS**: 5.3

---

### LOW-9: CSRF Cookie Missing `__Host-` Prefix
**File**: `internal/api/middleware/csrf.go:59`

Cookie named `dm_csrf` lacks `__Host-` prefix — vulnerable to subdomain cookie injection.
**CVSS**: 4.0

---

### LOW-10: JWT Missing `nbf` (Not Before) Claim
**File**: `internal/auth/jwt.go:58-69`

Access tokens have `ExpiresAt` and `IssuedAt` but no `NotBefore`. Minor time-drift replay window.
**CVSS**: 2.5

---

### LOW-11: Billing Permissions Not Enforced
**File**: `internal/api/handlers/billing.go`

RBAC constants `role_billing`, `PermBillingView`, `PermBillingManage` exist but handler uses only `protected` (no billing-specific permission check).
**CVSS**: 4.3

---

### LOW-12: Project Ownership Not Validated in ApplyPreset
**File**: `internal/api/handlers/environments.go:72`

No check that `project.TenantID == claims.TenantID`.
**CVSS**: 5.3

---

### LOW-13: gorilla/websocket — Missing ReadLimit on Upgrader
**File**: `internal/api/ws/deploy.go:105-124`

No `ReadLimit` set on `websocket.Upgrader`. Large frames could cause memory pressure.
**CVSS**: 4.3

---

### LOW-14: Transfer Handler — No App Tenant Check Beyond SuperAdmin
**File**: `internal/api/handlers/transfer.go:43`

App tenant not validated — only `adminOnly` middleware (role check). Superadmin from tenant A can transfer tenant B's app.
**CVSS**: 5.5

---

## SUMMARY BY CVSS

| ID | Severity | Description | File |
|----|----------|-------------|------|
| CRIT-1 | **CRITICAL** | API key auth broken (plaintext vs hash compare) | `middleware.go:207` |
| CRIT-2 | **CRITICAL** | MCP endpoints completely unprotected | `router.go:642-643` |
| CRIT-3 | **CRITICAL** | Deploy approval IDOR (cross-tenant approve/reject) | `deploy_approval.go:73` |
| HIGH-1 | **HIGH** | CMDi blocklist bypassable (sh -c) | `exec.go:188`, `terminal.go:153` |
| HIGH-2 | **HIGH** | SSRF — no internal IP blocking | `builder.go:200-205` |
| HIGH-3 | **HIGH** | Agent join token logged at startup | `swarm/module.go:47` |
| HIGH-4 | **HIGH** | Agent token in URL query, simple string compare | `swarm/server.go:203-207` |
| HIGH-5 | **HIGH** | Domain handler IDOR — ListAllDomains | `domains.go:46` |
| HIGH-6 | **HIGH** | Certificate handler IDOR — global listing | `certificates.go:40` |
| HIGH-7 | **HIGH** | Secret app scope without ownership check | `secrets.go:73` |
| HIGH-8 | **HIGH** | Invite handler — no permission check | `invites.go:43` |
| MED-1 | **MEDIUM** | Path traversal in backup key | `backup/local.go:31,47,56` |
| MED-2 | **MEDIUM** | CORS wildcard + credentials | `middleware.go:116,132` |
| MED-3 | **MEDIUM** | /metrics unauthenticated | `router.go:666-668` |
| MED-4 | **MEDIUM** | SourceURL not validated at create/update | `apps.go:72`, `app_update.go:38` |
| MED-5 | **MEDIUM** | Invite/API key token in response | `invites.go:80`, `admin_apikeys.go:93` |
| MED-6 | **MEDIUM** | YAML injection — unescaped command | `topology/yaml.go:203` |
| MED-7 | **MEDIUM** | ${SECRET:...} refs not masked | `envvars.go:103` |
| MED-8 | **MEDIUM** | Env export value injection | `env_import.go:116-117` |
| MED-9 | **MEDIUM** | No image signature verification | `deploy/docker.go:86,331` |
| MED-10 | **MEDIUM** | Workspace probing via SSRF | `builder.go:278-279` |
| MED-11 | **MEDIUM** | SSE URL injection (React) | `AppDetail.tsx:847` |
| MED-12 | **MEDIUM** | WebSocket URL injection (React) | `useDeployProgress.ts:72` |

**Total: 3 Critical, 8 High, 12 Medium, 14 Low = 37 confirmed findings**
