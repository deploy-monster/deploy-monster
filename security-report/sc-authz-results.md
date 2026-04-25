# SC-Authz Results: Authorization Flaw Detection

## Scan Scope
- IDOR / broken access control on app-scoped endpoints
- Tenant isolation enforcement
- Admin route protection (RequireSuperAdmin middleware)
- Cross-tenant mutation tests
- Role-based access control on team invites

## Findings

### AUTHZ-001: No Fine-Grained RBAC on App Mutations Beyond Tenant Scope
- **Severity:** Medium
- **Confidence:** 80
- **File:** `internal/api/router.go` (multiple protected handlers)
- **Vulnerability Type:** CWE-862 (Missing Authorization)
- **Description:** Most app-scoped mutation endpoints (e.g., `DELETE /api/v1/apps/{id}`, `POST /api/v1/apps/{id}/restart`, `PUT /api/v1/apps/{id}/env`) are wrapped only with `protected` (auth + tenant rate limit), not with role-based permission checks. Within a tenant, any authenticated user (viewer, developer, admin) can perform destructive actions if they know the app ID, because `requireTenantApp` only checks `app.TenantID == claims.TenantID` and does not inspect the user's role or permissions.
- **Impact:** A viewer-role user in a tenant can mutate or delete applications, violating the principle of least privilege.
- **Remediation:** Add a `requirePermission(perm string)` middleware or helper that checks the current user's role permissions (from `store.GetRole`) before allowing mutations. Apply it to all destructive app endpoints.
- **References:** https://cwe.mitre.org/data/definitions/862.html

### AUTHZ-002: Team Invite Creation Lacks Role Escalation Validation
- **Severity:** Medium
- **Confidence:** 75
- **File:** `internal/api/handlers/invites.go:30-99`
- **Vulnerability Type:** CWE-269 (Improper Privilege Management)
- **Description:** The `Create` invite handler checks `role.HasPermission(auth.PermMemberInvite)` but does not validate that the inviting user's role is higher than or equal to the `role_id` being assigned in the invite. A tenant owner could theoretically invite someone as `role_super_admin` (if that role ID is in the tenant's role list), or a developer could invite someone as an admin.
- **Impact:** Privilege escalation within a tenant via invitation.
- **Remediation:** Enforce a role hierarchy check: the inviter's role must be strictly higher than (or equal to, depending on policy) the invited role. Reject invites that would grant equal or higher privileges.
- **References:** https://cwe.mitre.org/data/definitions/269.html

## Checks Passed
- `requireTenantApp` enforces tenant ownership on all app-scoped endpoints
- Cross-tenant mutation test (`router_cross_tenant_mutation_test.go`) confirms 404 for all tested foreign-tenant mutations
- Admin routes are protected by `adminOnly` wrapper stacking `RequireSuperAdmin` on top of `protected`
- `RequireSuperAdmin` correctly rejects non-super-admin roles (tested for developer, viewer, admin)
- `router_test.go` contains exhaustive admin route authorization regression tests
- API key auth looks up `RoleID` from user membership so RBAC works for API keys

## Summary
- **Total Findings:** 2 (2 Medium)
- **Overall Status:** Issues found; see above for details.
