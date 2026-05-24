# SC-Authz Results: Authorization Flaw Detection

## Scan Scope
- IDOR / broken access control on app-scoped endpoints
- Tenant isolation enforcement
- Admin route protection (RequireSuperAdmin middleware)
- Cross-tenant mutation tests
- Role-based access control on team invites

## Findings

No active authorization findings remain in the scanned API routing surface.

## Resolved Findings

### AUTHZ-001: No Fine-Grained RBAC on App Mutations Beyond Tenant Scope
- **Severity:** Medium
- **Confidence:** 80
- **Status:** Resolved
- **File:** `internal/api/router.go` (multiple protected handlers)
- **Vulnerability Type:** CWE-862 (Missing Authorization)
- **Description:** App-scoped mutation endpoints now use `protectedPerm(...)` with role permissions such as `app.create`, `app.delete`, `app.restart`, `app.deploy`, `secret.*`, `domain.manage`, and related resource-specific permissions. Remaining `protected` POST/PATCH routes are either self-account actions or validation/compare operations without persistent side effects.
- **Remediation:** Keep `TestMutatingRoutes_ViewerRoleRequiresPermission` updated when adding new side-effecting routes.
- **References:** https://cwe.mitre.org/data/definitions/862.html

### AUTHZ-002: Team Invite Creation Lacks Role Escalation Validation
- **Severity:** Medium
- **Confidence:** 75
- **Status:** Resolved
- **File:** `internal/api/handlers/invites.go:30-99`
- **Vulnerability Type:** CWE-269 (Improper Privilege Management)
- **Description:** Invite creation now verifies the inviter membership belongs to the authenticated tenant, the requested target role exists in the tenant or is builtin, and the invited role's permissions do not exceed the inviter role's permissions.
- **Remediation:** Regression tests cover tenant mismatch, unknown role, and permission escalation attempts.
- **References:** https://cwe.mitre.org/data/definitions/269.html

### AUTHZ-003: Notification Test Endpoint Required Only Authentication
- **Severity:** Low
- **Confidence:** 80
- **Status:** Resolved
- **File:** `internal/api/router.go`
- **Vulnerability Type:** CWE-862 (Missing Authorization)
- **Description:** `POST /api/v1/notifications/test` sends an external notification but was wrapped with `protected` only. Any authenticated tenant user could trigger configured notification providers.
- **Remediation:** The route now requires `webhook.manage` through `protectedPerm(auth.PermWebhookManage, ...)`, and the viewer-route permission regression test includes this endpoint.

## Checks Passed
- Mutating app/resource routes use `protectedPerm` or `adminOnly`; viewer-route regression tests assert 403 for side-effecting routes.
- `requireTenantApp` enforces tenant ownership on app-scoped handlers
- Cross-tenant mutation test (`router_cross_tenant_mutation_test.go`) confirms 404 for all tested foreign-tenant mutations
- Admin routes are protected by `adminOnly` wrapper stacking `RequireSuperAdmin` on top of `protected`
- `RequireSuperAdmin` correctly rejects non-super-admin roles (tested for developer, viewer, admin)
- `router_test.go` contains exhaustive admin route authorization regression tests
- API key auth looks up `RoleID` from user membership so RBAC works for API keys

## Summary
- **Total Active Findings:** 0
- **Resolved This Scan:** 3
- **Overall Status:** Authorization scan clean for reviewed routing and handler boundaries.
