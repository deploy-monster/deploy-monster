# Verified Findings

## RESOLVED: JWT Client-Side Decoding
**File:** web/src/stores/auth.ts (previously line 27-38)
**Status:** FIXED

Removed `userFromTokenResponse()` which decoded JWT payload without signature verification. Login and register now use `/auth/me` endpoint for verified user info.

## RESOLVED: Admin Panel Missing RBAC
**File:** web/src/App.tsx (previously line 98)
**Status:** FIXED

Added `AdminRoute` component that checks `user.role === 'role_admin'` before rendering Admin panel. Non-admin users are redirected to `/`.

## PENDING: Docker SDK Vulnerabilities
**Package:** github.com/docker/docker@v28.5.2
**Severity:** HIGH
**Status:** No patch available

AuthZ bypass (GO-2026-4887) and privilege validation error (GO-2026-4883). Mitigate via network-level Docker API access restrictions.

## PENDING: Direct DB Access in Migrations
**File:** internal/api/handlers/migrations.go:26-27
**Severity:** MEDIUM
**Status:** Architectural concern, refactor recommended

Bypasses Store interface abstraction. Current query is static (no injection risk).
