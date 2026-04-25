# SC-Privilege-Escalation Results: Privilege Escalation Vector Detection

## Scan Scope
- Role manipulation via request body
- JWT role claim tampering
- Admin endpoint protection
- Default/test admin accounts
- Path-based access control bypass

## Findings

### PRIVESC-001: Default Super Admin Created with Predictable Email
- **Severity:** High
- **Confidence:** 90
- **File:** `internal/auth/module.go:99-100`
- **Vulnerability Type:** CWE-798 (Hardcoded Credentials)
- **Description:** If `MONSTER_ADMIN_EMAIL` is not set, the default super admin email is `admin@deploy.monster`. An attacker who knows this default can target the account with credential stuffing or brute force (even though rate limiting exists, distributed attacks or password reuse could succeed). The password is either env-provided or auto-generated, but the email is predictable.
- **Impact:** Targeted attacks against a known super admin account.
- **Remediation:** Do not use a predictable default email. Instead, require the admin email to be set via environment variable before first run, or generate a random email prefix (e.g., `admin-<random>@deploy.monster`) and print it during setup.
- **References:** https://cwe.mitre.org/data/definitions/798.html

### PRIVESC-002: Role Assignment in Registration Hardcoded to `role_owner`
- **Severity:** Low
- **Confidence:** 80
- **File:** `internal/api/handlers/auth.go:263`
- **Vulnerability Type:** CWE-269 (Improper Privilege Management)
- **Description:** The `Register` handler hardcodes `role_owner` for all new users. While this prevents direct privilege escalation via request body manipulation (the request struct has no `role` field), it also means there is no self-service registration path for lower-privilege roles. This is a design note rather than an active vulnerability, but it means any registered user becomes an owner of their own tenant.
- **Impact:** Minimal direct security impact; noted for defense-in-depth.
- **Remediation:** Ensure that the registration endpoint never accepts a `role` field from the client (currently true). Document that `role_owner` is the intended default for self-registered users.
- **References:** https://cwe.mitre.org/data/definitions/269.html

### PRIVESC-003: Admin API Key Handler Lacks Additional Authorization Check Beyond Middleware
- **Severity:** Low
- **Confidence:** 65
- **File:** `internal/api/handlers/admin_apikeys.go:60-98`
- **Vulnerability Type:** CWE-862 (Missing Authorization)
- **Description:** The `Generate` handler relies solely on the router-level `adminOnly` middleware for authorization. While the middleware is correctly applied, the handler itself does not perform an additional authorization check. If the middleware were accidentally removed during a refactor, the endpoint would be exposed. Defense-in-depth suggests a redundant check inside the handler.
- **Impact:** Low probability (middleware is tested), but high impact if middleware is bypassed.
- **Remediation:** Add a redundant `claims.RoleID == RoleSuperAdmin` check inside the `Generate`, `Revoke`, and `List` handlers, returning 403 if the check fails.
- **References:** https://cwe.mitre.org/data/definitions/862.html

## Checks Passed
- `registerRequest` struct has no `role` field; role cannot be set by client
- `UpdateProfile` does not allow changing role
- Admin endpoints are protected by `RequireSuperAdmin` middleware
- `RequireSuperAdmin` checks `claims.RoleID` against an allowlist map
- JWT role claims are not blindly trusted for admin access; middleware enforces the check
- No default/test admin passwords in source code
- First-run super admin password is auto-generated (16 chars) if not provided via env
- Router tests verify 403 for developer/viewer roles on all admin routes
- `adminOnly` wrapper is explicitly documented and tested

## Summary
- **Total Findings:** 3 (1 High, 2 Low)
- **Overall Status:** Issues found; see above for details.
