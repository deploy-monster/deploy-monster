# DeployMonster Security Report
**Updated:** 2026-05-24
**Scan Type:** Current working-tree validation

---

## Executive Summary

No current critical or high findings are verified in the working tree.

The previous frontend findings are resolved:
- JWT payloads are no longer decoded client-side for authentication state; `web/src/stores/auth.ts` uses `/auth/me`.
- The admin UI is gated to `role_super_admin`, matching backend `/api/v1/admin/*` authorization.
- Login now supports the backend TOTP challenge flow instead of masking login 401s as session expiry.
- Registration and settings copy now match the backend 12-character password policy.

Current validation:
- `go test ./...`: passing
- `go test -tags integration ./...`: passing
- `go test -tags pgintegration ./internal/db/...`: passing
- `govulncheck ./...`: no called vulnerabilities
- `pnpm audit --audit-level moderate`: no known vulnerabilities
- `pnpm run lint`: passing
- `pnpm test`: 43 files / 381 tests passing
- `pnpm run build`: passing
- `pnpm run check:bundle`: main chunk 19.84 KB gzip / 300 KB budget
- `scripts/build.sh`: passing, UI embedded and Go binary built
- `staticcheck ./...`: clean
- `golangci-lint run`: clean
- `git diff --check`: clean
- `./scripts/ci-local.sh --allow-dirty --quick`: 10 passed, 0 failed, 7 skipped
- Focused follow-up: `go test ./internal/api/handlers ./internal/build ./internal/notifications` passing

**Risk Level: LOW-MEDIUM**

| Severity | Count | Notes |
|----------|-------|-------|
| CRITICAL | 0 | No verified critical findings |
| HIGH | 0 | No verified high findings |
| MEDIUM | 3 | Residual product/coordination risks |
| LOW | 4 | Hardening/documentation items |

---

## Resolved Findings

### JWT Decoded Client-Side Without Signature Verification
**Previous file:** `web/src/stores/auth.ts`
**Previous severity:** CRITICAL
**Status:** RESOLVED

Login, register, and initialization now derive user identity and role from the authenticated `/auth/me` response instead of trusting unverified JWT payload data in the browser.

### Admin Panel Missing/Misaligned Role-Based Access Control
**Previous file:** `web/src/App.tsx`
**Previous severity:** HIGH
**Status:** RESOLVED

The admin route now requires `role_super_admin`, which matches backend `RequireSuperAdmin` enforcement for `/api/v1/admin/*`. The sidebar also hides the Admin navigation item from tenant-level admins.

### Dependency Vulnerabilities
**Previous package:** `github.com/docker/docker@v28.5.2+incompatible`
**Previous severity:** HIGH
**Status:** RESOLVED / NOT REACHABLE IN CURRENT SCAN

The legacy Docker SDK module is no longer imported. Docker API calls use split Moby client/API modules. Current `govulncheck` reports 0 called vulnerabilities.

Frontend transitive audit findings in `brace-expansion` and `ws` were remediated through pnpm overrides to patched versions.

### Predictable First-Run Super Admin Email
**Previous file:** `internal/auth/module.go`
**Previous severity:** HIGH
**Status:** RESOLVED

When `MONSTER_ADMIN_EMAIL` is not set, first-run setup now generates an unpredictable `admin-<random>@deploymonster.local` email and logs it once with the bootstrap password.

### TOTP Login UX Gap
**Previous files:** `web/src/api/client.ts`, `web/src/pages/Login.tsx`
**Previous severity:** MEDIUM
**Status:** RESOLVED

The API client can skip global refresh handling for `/auth/login` 401 responses, preserving backend domain errors such as `TOTP code required`. The login page now prompts for an authentication code and resubmits it as `totp_code`.

### Frontend Password Policy Drift
**Previous files:** `web/src/pages/Register.tsx`, `web/src/pages/Settings.tsx`
**Previous severity:** MEDIUM
**Status:** RESOLVED

Frontend registration validation and settings guidance now match the backend policy: minimum 12 characters with uppercase, lowercase, digit, and special character.

### TOTP Backup Codes Not Persisted
**Previous file:** `internal/auth/totp_service.go`
**Previous severity:** MEDIUM
**Status:** RESOLVED

Backup-code hashes are now persisted by SQLite/Postgres stores and consumed on successful use so each code is one-time.

### Direct DB Access in Migration Status Handler
**Previous file:** `internal/api/handlers/migrations.go`
**Severity:** MEDIUM
**Status:** RESOLVED

The admin migration-status endpoint now reads applied migration metadata through `core.Store.ListMigrations`. SQLite and Postgres implement the Store-level method, and handler tests no longer depend on raw `*sql.DB` access.

### Marketplace Templates With Weak Secret Defaults
**Previous file:** `internal/marketplace/`
**Severity:** LOW
**Status:** RESOLVED

Marketplace template registration now sanitizes weak sensitive defaults across `${VAR:-weak}` fallbacks, hardcoded YAML secret scalars, URL userinfo passwords, and query-string password parameters. Built-in template tests assert sanitized templates contain no weak secret defaults.

### Deploy Surfaces Ignored Active Freeze Windows
**Previous file:** `internal/api/handlers/`
**Severity:** LOW
**Status:** RESOLVED

Manual app, compose stack, marketplace, and topology deploys now enforce active tenant deploy-freeze windows and return `423 Locked` before changing app status, creating app records, or creating deployment resources.

### Topology Path Components Accepted Encoded Traversal
**Previous file:** `internal/api/handlers/topology.go`
**Severity:** MEDIUM
**Status:** RESOLVED

Topology save, load, compile, and deploy now share strict `projectId`/`environment` validation before values are used in Bolt keys or deployment working directories. The validator rejects URL-encoded and double-encoded traversal, path separators, dots, whitespace, colons, and other non path-component characters.

### Path Traversal Boundary Hardening
**Files:** `internal/api/handlers/filebrowser.go`, `internal/api/handlers/backups.go`
**Severity:** LOW-MEDIUM
**Status:** RESOLVED / HARDENED

The file-browser path validator now rejects encoded and double-encoded traversal before normalization. Backup restore/download keys now require strict tenant-prefixed key syntax and reject encoded rewrites, empty/dot segments, repeated slashes, trailing slashes, backslashes, and non key-safe characters before reaching storage.

### Custom Dockerfile Path Could Escape Build Context
**Previous file:** `internal/build/builder.go`
**Severity:** MEDIUM
**Status:** RESOLVED

Custom build Dockerfile paths now resolve through a containment helper that rejects absolute paths, dot paths, null bytes, and parent traversal, then verifies the final path remains inside the generated build directory before invoking `docker build -f`.

### Invitation Role Assignment Boundary
**Previous file:** `internal/api/handlers/invites.go`
**Severity:** MEDIUM
**Status:** RESOLVED

Team invitations now validate that the inviter membership belongs to the authenticated tenant, the requested target role exists in that tenant or is builtin, and the target role's permissions do not exceed the inviter role's permissions. This prevents future custom-role privilege escalation through invite creation.

### TOTP Service Errors Reflected Internal Details
**Previous file:** `internal/api/handlers/sessions.go`
**Severity:** LOW
**Status:** RESOLVED

TOTP enrollment, confirmation, disable, and backup-code handlers now allowlist expected user-facing states and return generic server errors for internal store/vault failures. Regression tests verify wrapped backend errors are not reflected in API responses.

### Notification Test Endpoint Required Only Authentication
**Previous file:** `internal/api/router.go`
**Severity:** LOW
**Status:** RESOLVED

The external notification test endpoint now requires `webhook.manage` via `protectedPerm`, instead of allowing any authenticated tenant user to trigger configured notification providers. The viewer permission regression table includes this route.

### Token Cookie Could Bypass CSRF When CSRF Cookie Was Missing
**Previous file:** `internal/api/middleware/csrf.go`
**Severity:** LOW
**Status:** RESOLVED

Cookie-authenticated mutating requests now require a CSRF cookie/header pair whenever `dm_access` or `dm_refresh` is present. Bearer-token and API-key requests remain exempt because browsers do not auto-send those credentials.

### WebSocket Wildcard Origin Accepted Cross-Origin Upgrades
**Previous file:** `internal/api/ws/deploy.go`
**Severity:** LOW
**Status:** RESOLVED

Deploy progress WebSocket origin checks now reject wildcard `*` and empty Origin headers, and only allow exact configured origins during upgrade.

### Import Manifest Used Weaker Source URL Validation
**Previous file:** `internal/api/handlers/import_export.go`
**Severity:** LOW
**Status:** RESOLVED

App import manifests now reuse the same `build.ValidateGitURL` policy as normal app create/update. This closes drift where imports could accept schemes or source formats that the primary app handlers reject.

### Redirect Rule Shape Was Under-Constrained
**Previous file:** `internal/api/handlers/redirects.go`
**Severity:** LOW
**Status:** RESOLVED

Per-app redirect rule creation now rejects non-path sources, protocol-relative destinations, non-http(s) absolute destinations, CRLF injection, unknown rule types, and external destinations for internal rewrite rules. Tenant-configured external `http(s)` redirects remain an intentional app feature.

### Stale SSRF, File Upload, And XSS Findings Revalidated
**Files:** `internal/build/builder.go`, `internal/api/handlers/event_webhooks.go`, `internal/notifications/providers.go`, `internal/api/handlers/certificates.go`, `web/src/`
**Severity:** INFO
**Status:** REVALIDATED

The SSRF report now reflects that `git://` is rejected, local git paths are opt-in only, outbound webhooks validate internal destinations, certificate upload is JSON PEM parsing rather than arbitrary file upload, app import does not extract archives, and reviewed frontend/API paths contain no dangerous HTML DOM sinks.

### Docker SDK And Mass Assignment Findings Revalidated
**Files:** `go.mod`, `internal/api/handlers/sessions.go`, `internal/api/handlers/app_update.go`, `internal/api/handlers/tenant_settings.go`
**Severity:** INFO
**Status:** REVALIDATED

The legacy Docker SDK vulnerability finding is no longer current because the project uses split Moby modules and `govulncheck` reports 0 called vulnerabilities. Reviewed PATCH handlers use explicit DTOs and field copies rather than binding request bodies directly to persisted models.

### Frontend Dependency Hygiene Findings Resolved
**Files:** `web/package.json`, `web/pnpm-lock.yaml`
**Severity:** LOW
**Status:** RESOLVED

The stale `lodash@4 -> ^4.18.0` pnpm override was removed, and the frontend already uses the maintained `@dagrejs/dagre` package instead of legacy `dagre`.

### Clickjacking And JWT Claim Findings Revalidated
**Files:** `internal/api/middleware/security_headers.go`, `internal/api/spa.go`, `internal/auth/jwt.go`
**Severity:** INFO
**Status:** REVALIDATED

Frame protection is present through `X-Frame-Options: DENY` and CSP `frame-ancestors 'none'`. JWT access and refresh tokens include issuer/audience claims and validation checks both values.

### Crypto And API Surface Reports Revalidated
**Files:** `internal/core/id.go`, `internal/notifications/smtp.go`, `internal/api/router.go`
**Severity:** INFO
**Status:** REVALIDATED

The previous `crypto/rand` fallback note is no longer current: ID/secret generation fails closed on entropy failure. SMTP `InsecureSkipVerify` is constrained to localhost/loopback/`.local` relay scenarios. API risk is now framed as ongoing review burden, with router authorization, permission, cross-tenant, and fuzz tests already present.

### Session Absolute Timeout Revalidated
**File:** `internal/auth/jwt.go`
**Severity:** INFO
**Status:** REVALIDATED

Refresh tokens now include a first-issued-at timestamp and validation rejects refresh-token chains older than 30 days, closing the previous indefinite sliding-session concern.

### Compose YAML Deserialization DoS Hardened
**Files:** `internal/compose/parser.go`, `internal/api/handlers/compose.go`, `internal/marketplace/validator.go`
**Severity:** LOW
**Status:** RESOLVED

Compose YAML parsing now has a 1 MiB parser-level limit. Direct YAML stack deploys return 413 for oversize input instead of silently truncating, and marketplace template validation rejects oversized compose YAML before unmarshaling.

### SMTP Email Header Injection Hardened
**File:** `internal/notifications/smtp.go`
**Severity:** LOW
**Status:** RESOLVED

SMTP notification subject, recipient header, and configured sender display name now reject CR, LF, and NUL bytes before RFC 5322 headers are assembled. The SMTP envelope recipient is normalized to the parsed email address.

### Telegram Notification HTML Formatting Injection Hardened
**File:** `internal/notifications/providers.go`
**Severity:** LOW
**Status:** RESOLVED

Telegram notification content is now HTML-escaped before being sent with `parse_mode=HTML`, preserving the provider's intended bold subject formatting while preventing subject/body text from injecting Telegram markup or links.

### Deploy Approval Double-Processing Race Hardened
**File:** `internal/api/handlers/deploy_approval.go`
**Severity:** LOW
**Status:** RESOLVED

Approval approve/reject status checks and state transitions now run under one mutex lock, preventing concurrent requests from both processing the same pending approval. Already processed approvals return `409 Conflict` and keep their existing state.

### Outbound Event Webhook Lost-Update Window Hardened
**Files:** `internal/db/bolt.go`, `internal/api/handlers/event_webhooks.go`
**Severity:** LOW
**Status:** RESOLVED

SQLite-backed KV storage now has `BoltStore.Mutate` for transaction-scoped read-modify-write updates. Outbound event webhook create/delete use this path, so concurrent tenant webhook changes are applied inside one write transaction instead of separate `Get`/`Set` calls.

### Redirect Rule Lost-Update Window Hardened
**Files:** `internal/api/handlers/redirects.go`, `internal/db/bolt.go`
**Severity:** LOW
**Status:** RESOLVED

Per-app redirect rule create/delete also use `BoltStore.Mutate`, so concurrent redirect rule changes are applied inside one write transaction instead of separate list `Get`/`Set` calls.

---

## Residual Risks

### Build/Exec Surface
**Files:** `internal/build/`, `internal/api/handlers/exec.go`, `internal/api/ws/terminal.go`, `internal/cron/module.go`, `internal/swarm/`
**Severity:** MEDIUM

Build execution and in-container exec are intentional PaaS features. Their safety depends on authentication, tenant isolation, Docker daemon hardening, and audit logging. Continue treating this as a high-sensitivity surface during future changes.

Current hardening blocks shell/eval flag escapes (`bash -c`, `sh -lc`, `python -c`, `node --eval`) in exec, terminal, one-off app command, cron, and node-executor paths, including explicit `args`.

### Deploy Approval Enforcement
**Files:** `internal/api/handlers/deploy_trigger.go`, `internal/api/handlers/deploy_approval.go`
**Severity:** LOW-MEDIUM

Deploy approval endpoints exist, but the direct deploy trigger does not currently require or consume approval state before starting a deploy/build. If approvals are meant to be a blocking workflow, the deploy trigger should create a pending approval or require an approved request tied to the exact app/version/image being deployed.

The approval state machine itself is hardened against double-processing; the remaining question is whether approval should be product policy for deploy execution.

### App Operation Concurrency
**Files:** `internal/api/handlers/`, `internal/db/bolt.go`
**Severity:** MEDIUM

Deployment versions use atomic allocation, outbound event webhook and redirect rule lists use transaction-scoped KV mutation, and database stores provide transaction-level safety. The remaining risk is composite app operations and other KV list-style read-modify-write handlers where concurrent requests can interleave at the workflow level. Add app-level optimistic locking and migrate remaining list-like buckets to `BoltStore.Mutate` for high-churn app configuration paths.

### JWT Emergency Key Revocation
**File:** `internal/auth/jwt.go`
**Severity:** LOW

Previous JWT signing keys are accepted for a 20-minute grace period and are purged automatically. `RevokeAllPreviousKeys` exists in code, but there is no reviewed super-admin endpoint or documented runtime operation for emergency active-key-only mode after key compromise.

---

## Positive Findings

| Category | Status |
|----------|--------|
| SQL injection | Parameterized/static queries in reviewed paths |
| Admin authorization | Backend and frontend aligned on `role_super_admin` |
| CSRF protection | Cookie auth paired with CSRF middleware |
| Security headers | CSP and frame protection present |
| Dependency audit | Go and frontend local audits clean |
| Mass assignment | Reviewed PATCH handlers use DTOs and explicit field copies |
| API route coverage | Admin, mutating permission, cross-tenant, and route-fuzz tests present |
| YAML parsing | Compose documents limited to 1 MiB before unmarshal |
| Email headers | SMTP notification headers reject CR/LF/NUL injection characters |
| External notification formatting | Telegram content is HTML-escaped before HTML parse mode |
| KV list mutation | Outbound event webhook and redirect rule lists mutate inside one SQLite transaction |

---

## Remediation Priority

1. Keep Docker daemon access restricted to trusted local/agent contexts.
2. Decide whether deploy approvals are blocking policy; if yes, wire approval state into deploy trigger execution.
3. Add app-level operation coordination for deploy/scale/restart/config mutation workflows.
4. Re-run `govulncheck`, `pnpm audit`, `staticcheck`, and full local CI before release.
