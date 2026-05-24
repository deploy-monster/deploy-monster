# SC-Privilege-Escalation Results: Privilege Escalation Vector Detection

## Scan Scope
- Role manipulation via request body
- JWT role claim handling
- Admin endpoint protection
- First-run super admin bootstrap behavior
- Path-based access-control bypass

## Current Findings

No active privilege-escalation findings remain in the scanned paths.

## Resolved Findings

### PRIVESC-001: Predictable Bootstrap Super Admin Email
- **Status:** Resolved
- **Evidence:** When `MONSTER_ADMIN_EMAIL` is unset, first-run setup now generates an email in the form `admin-<random>@deploymonster.local` and logs it with the generated bootstrap password. Tests assert the legacy predictable email is not used.
- **Files:** `internal/auth/module.go:131-138`, `internal/auth/auth_coverage_test.go`

### PRIVESC-002: Role Assignment In Registration
- **Status:** Accepted Design
- **Evidence:** The registration request has no role field, so clients cannot self-assign elevated platform roles. Self-registration creates a tenant owner for the user's own tenant, not a platform super admin.
- **Files:** `internal/api/handlers/auth.go`, `internal/api/handlers/auth_handler_test.go`

### PRIVESC-003: Admin API Key Handler Relies On Router-Level Authorization
- **Status:** Resolved
- **Evidence:** Admin API key list/generate/revoke handlers now repeat the `role_super_admin` authorization check through a shared handler helper. Regression tests verify missing claims return 401 and tenant admins receive 403.
- **Files:** `internal/api/handlers/admin_apikeys.go`, `internal/api/handlers/admin_apikeys_handler_test.go`

### PRIVESC-004: Invitation Role Assignment Did Not Validate Target Role Boundary
- **Status:** Resolved
- **Evidence:** Team invitation creation now verifies the inviter membership belongs to the authenticated tenant, validates that the requested target role exists and belongs to the same tenant or is builtin, and rejects target roles whose permissions exceed the inviter role's permissions. Regression tests cover unknown roles, cross-tenant membership, and custom roles with extra permissions.
- **Files:** `internal/api/handlers/invites.go`, `internal/api/handlers/invites_handler_test.go`

## Checks Passed
- `registerRequest` has no client-controlled `role` field
- Profile updates do not allow role changes
- Admin routes are wrapped with `RequireSuperAdmin`
- `RequireSuperAdmin` permits only `role_super_admin`
- Admin API key handlers enforce `role_super_admin` even if invoked outside the router
- Team invitation target roles must exist in the authenticated tenant or be builtin
- Team invitation target-role permissions cannot exceed inviter-role permissions
- Router tests verify developer/viewer roles receive 403 on admin routes
- Frontend admin route and sidebar visibility are limited to `role_super_admin`
- First-run super admin password is random when not provided via environment
- First-run super admin email is random when not provided via environment

## Summary
- **Open Findings:** 0
- **Resolved Findings:** 3
- **Accepted Design Notes:** 1
- **Overall Status:** No active privilege-escalation findings remain in the scanned paths.
