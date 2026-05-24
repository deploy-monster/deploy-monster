# Verified Findings

## RESOLVED: JWT Client-Side Decoding
**File:** web/src/stores/auth.ts (previously line 27-38)
**Status:** FIXED

Removed `userFromTokenResponse()` which decoded JWT payload without signature verification. Login and register now use `/auth/me` endpoint for verified user info.

## RESOLVED: Admin Panel Missing RBAC
**File:** web/src/App.tsx (previously line 98)
**Status:** FIXED

Added an admin role helper and route/sidebar checks that require `role_super_admin`, matching backend `RequireSuperAdmin` enforcement for `/api/v1/admin/*`.

## RESOLVED: Docker SDK Vulnerabilities
**Package:** github.com/docker/docker@v28.5.2
**Status:** FIXED / NOT IMPORTED

The legacy Docker SDK module is no longer imported. Docker calls use split Moby client/API modules, and current `govulncheck ./...` reports 0 called vulnerabilities.

## RESOLVED: Predictable First-Run Super Admin Email
**File:** internal/auth/module.go
**Status:** FIXED

First-run setup now generates an unpredictable `admin-<random>@deploymonster.local` email when `MONSTER_ADMIN_EMAIL` is unset. A regression test verifies the legacy predictable email is not used.

## RESOLVED: Login 401 Masked TOTP Challenge
**Files:** web/src/api/client.ts, web/src/pages/Login.tsx
**Status:** FIXED

The API client can skip global refresh handling for `/auth/login`, preserving `TOTP code required` responses. The login page now renders an authentication-code input and resubmits with `totp_code`.

## RESOLVED: Frontend Password Policy Drift
**Files:** web/src/pages/Register.tsx, web/src/pages/Settings.tsx
**Status:** FIXED

Registration validation and account-settings copy now match the backend 12-character password policy with uppercase, lowercase, digit, and special-character requirements.

## RESOLVED: Direct DB Access in Migrations
**File:** internal/api/handlers/migrations.go
**Status:** FIXED

Migration status now goes through `core.Store.ListMigrations`, implemented by SQLite and Postgres stores. The handler no longer queries `_migrations` through `core.DB.SQL`.

## RESOLVED: TOTP Backup Codes Not Persisted
**Files:** internal/auth/totp_service.go, internal/db/users.go, internal/db/postgres.go
**Status:** FIXED

Backup-code hashes are now persisted by the database stores and consumed on successful validation, making each recovery code one-time.

## RESOLVED: Marketplace Weak Secret Defaults
**Files:** internal/marketplace/registry.go, internal/marketplace/templates_extra_test.go
**Status:** FIXED

Marketplace templates are sanitized during registry insertion. Weak sensitive fallbacks, hardcoded secret scalars, and weak passwords inside Compose connection strings are replaced with required environment-variable placeholders before templates are exposed.

## RESOLVED: Deploy Freeze Bypass
**Files:** internal/api/handlers/deploy_trigger.go, internal/api/handlers/compose.go, internal/api/handlers/marketplace_deploy.go, internal/api/handlers/topology.go, internal/api/router.go
**Status:** FIXED

Deploy handlers now check active tenant freeze windows through the configured Bolt store and reject deploy attempts with `423 Locked` before deployment state changes or resource creation.

## RESOLVED: Topology Encoded Path Traversal
**File:** internal/api/handlers/topology.go
**Status:** FIXED

Topology save, load, compile, and deploy now validate `projectId` and `environment` with a shared strict path-component validator before using them in Bolt keys or deployment working directories. Regression tests cover direct, URL-encoded, and double-encoded traversal attempts.

## RESOLVED: File Browser And Backup Key Traversal Hardening
**Files:** internal/api/handlers/filebrowser.go, internal/api/handlers/backups.go
**Status:** FIXED / HARDENED

File-browser paths are decoded before traversal checks, including double-encoded traversal attempts. Backup restore/download keys now use strict tenant-prefixed key validation and reject encoded rewrites, empty or dot segments, repeated slashes, trailing slashes, backslashes, and unsafe characters before storage access.

## RESOLVED: Custom Dockerfile Path Traversal
**File:** internal/build/builder.go
**Status:** FIXED

Custom Dockerfile paths now require relative paths and are resolved with a build-context containment check before `docker build -f` receives the path. Tests cover nested paths, absolute paths, parent traversal, dot paths, and null bytes.

## RESOLVED: Invitation Role Assignment Boundary
**File:** internal/api/handlers/invites.go
**Status:** FIXED

Team invitation creation now validates tenant-bound inviter membership, target role existence and tenant ownership, and ensures target-role permissions do not exceed the inviter role's permissions. Regression tests cover unknown roles, cross-tenant membership, and custom roles with extra permissions.

## RESOLVED: Import Manifest Source URL Policy Drift
**File:** internal/api/handlers/import_export.go
**Status:** FIXED

App manifest import validation now calls `build.ValidateGitURL`, matching the app create/update path for unsafe schemes, shell metacharacters, local file access, private/link-local IPs, and Docker image references.

## RESOLVED: Redirect Rule Validation Gaps
**File:** internal/api/handlers/redirects.go
**Status:** FIXED

Redirect rule creation now validates source/destination shape, rejects CRLF, disallows protocol-relative and non-http(s) destinations, validates rule type, and keeps rewrite destinations path-only. Regression tests cover unsafe rule shapes.

## REVALIDATED: Stale SSRF, File Upload, And XSS Candidates
**Files:** internal/build/builder.go, internal/api/handlers/event_webhooks.go, internal/notifications/providers.go, internal/api/handlers/certificates.go, web/src
**Status:** NOT CURRENT FINDINGS

Git clone SSRF controls, outbound webhook URL controls, certificate PEM parsing/domain matching, JSON-only app import, and absence of dangerous frontend HTML sinks were rechecked against the current working tree.

## REVALIDATED: Docker SDK Vulnerability Finding
**Files:** go.mod, internal/deploy/docker.go
**Status:** NOT CURRENT FINDING

The project no longer imports the legacy `github.com/docker/docker@v28.5.2+incompatible` module. Docker calls use split Moby modules, and current `govulncheck ./...` reports 0 called vulnerabilities.

## REVALIDATED: Mass Assignment Candidate
**Files:** internal/api/handlers/sessions.go, internal/api/handlers/app_update.go, internal/api/handlers/tenant_settings.go
**Status:** NOT CURRENT FINDING

Reviewed PATCH endpoints decode into explicit request DTOs and copy only allowlisted fields onto loaded store models. Unknown JSON fields are ignored by the decoder but are not persisted.

## OPEN: Deploy Approval Not Enforced By Deploy Trigger
**Files:** internal/api/handlers/deploy_trigger.go, internal/api/handlers/deploy_approval.go
**Status:** RESIDUAL PRODUCT GAP

Deploy approval endpoints maintain pending requests, but direct deploy triggers do not currently require or consume approval state before execution. Treat this as a workflow gap if approvals are intended to be blocking.

## RESOLVED: Deploy Approval Double-Processing Race
**File:** internal/api/handlers/deploy_approval.go
**Status:** FIXED

Approval approve/reject handlers now perform status checks and state transitions under one mutex lock, preventing concurrent approve/reject requests from both processing the same pending approval. Processed approvals return `409 Conflict` and remain immutable.

## OPEN: Composite App Operation Coordination
**Files:** internal/api/handlers, internal/db/bolt.go
**Status:** RESIDUAL ENGINEERING RISK

Deployment version allocation is atomic, and outbound event webhook plus redirect rule lists now use a transactional KV mutation helper. App-level workflows and other KV list-style `Get`/mutate/`Set` paths can still interleave under concurrent requests. Continue migrating list-like buckets to the transaction-scoped helper and add app-level optimistic locking for high-churn workflows.

## RESOLVED: Outbound Event Webhook Lost-Update Window
**Files:** internal/db/bolt.go, internal/api/handlers/event_webhooks.go
**Status:** FIXED

`BoltStore.Mutate` now supports transaction-scoped read-modify-write updates on the SQLite-backed KV store. Outbound webhook create/delete use it, so concurrent tenant webhook changes do not overwrite each other in production storage. Regression coverage verifies concurrent webhook creation preserves all entries.

## RESOLVED: Redirect Rule Lost-Update Window
**Files:** internal/api/handlers/redirects.go, internal/db/bolt.go
**Status:** FIXED

Redirect rule create/delete now use the transaction-scoped Bolt mutation helper. Regression coverage verifies concurrent redirect rule creation preserves all entries.

## RESOLVED: Frontend Dependency Hygiene Drift
**Files:** web/package.json, web/pnpm-lock.yaml
**Status:** FIXED

The invalid `lodash@4 -> ^4.18.0` pnpm override was removed. The old unmaintained `dagre` dependency is not present; the frontend uses `@dagrejs/dagre`.

## REVALIDATED: Clickjacking Protection
**Files:** internal/api/middleware/security_headers.go, internal/api/spa.go
**Status:** NOT CURRENT FINDING

API and SPA responses include frame protection through `X-Frame-Options: DENY` and CSP `frame-ancestors 'none'`.

## REVALIDATED: JWT Issuer And Audience Validation
**File:** internal/auth/jwt.go
**Status:** FIXED / NOT CURRENT FINDING

JWT access and refresh tokens include issuer and audience claims, and both validation paths enforce `jwt.WithIssuer` and `jwt.WithAudience`.

## REVALIDATED: crypto/rand Fallback Candidate
**File:** internal/core/id.go
**Status:** NOT CURRENT FINDING

`GenerateID` and `GenerateSecret` now fail closed by panicking if `crypto/rand` is unavailable. `GeneratePassword` uses `crypto/rand.Reader` through `rand.Int`, so it is not a weak PRNG fallback.

## REVALIDATED: API Route Guard Coverage
**Files:** internal/api/router.go, internal/api/router_test.go, internal/api/router_cross_tenant_mutation_test.go, internal/api/router_fuzz_test.go
**Status:** CONTROLLED / ONGOING REVIEW AREA

The API surface is large, but current tests cover admin route authorization, viewer permission checks for mutating routes, cross-tenant mutation attempts, and tenant-scoped route fuzzing.

## REVALIDATED: Refresh Token Absolute Session Timeout
**File:** internal/auth/jwt.go
**Status:** FIXED / NOT CURRENT FINDING

Refresh tokens carry a first-issued-at timestamp and validation rejects refresh-token chains older than `MaxAbsoluteSessionSeconds` (30 days).

## RESOLVED: Compose YAML Parser Size Hardening
**Files:** internal/compose/parser.go, internal/api/handlers/compose.go, internal/marketplace/validator.go
**Status:** FIXED

Compose YAML documents are capped at 1 MiB before YAML unmarshaling. Direct YAML deploy requests return 413 for oversized input and marketplace validation rejects oversized compose templates.

## RESOLVED: SMTP Header Injection Hardening
**File:** internal/notifications/smtp.go
**Status:** FIXED

SMTP notification subjects, recipient headers, and configured sender display names now reject CR, LF, and NUL bytes before email headers are assembled. The SMTP envelope recipient is also normalized to the parsed email address.

## RESOLVED: Telegram Notification HTML Formatting Injection
**File:** internal/notifications/providers.go
**Status:** FIXED

Telegram notifications use HTML parse mode, so subject/body content is now escaped before provider-owned bold formatting is applied. Regression coverage verifies user-controlled markup is emitted as text, not Telegram HTML.
