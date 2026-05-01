# DeployMonster Security Report
**Generated:** 2026-05-01
**Scan Type:** Full Security Audit

---

## Executive Summary

2 critical/high findings require immediate attention. The frontend has a significant authentication vulnerability. The Go backend demonstrates strong security fundamentals.

**Risk Level: MEDIUM-HIGH**

| Severity | Count |
|----------|-------|
| CRITICAL | 1 |
| HIGH | 2 |
| MEDIUM | 3 |
| LOW | 2 |

---

## Critical Findings

### 1. JWT Decoded Client-Side Without Signature Verification
**File:** `web/src/stores/auth.ts:27-38`
**Severity:** CRITICAL (CVSS 8.2)

```typescript
function userFromTokenResponse(pair: TokenPair): User | null {
  const payload = pair.access_token.split('.')[1];
  const decoded = JSON.parse(atob(payload));  // NO SIGNATURE VERIFICATION
}
```

**Impact:** An attacker can forge arbitrary user attributes (role, tenant_id).

**Remediation:** Remove local JWT decoding. Use `/auth/me` response exclusively.

---

## High Findings

### 2. Admin Panel Missing Role-Based Access Control
**File:** `web/src/App.tsx:98`, `web/src/pages/Admin.tsx`
**Severity:** HIGH (CVSS 7.5)

The `/admin` route lacks role authorization. Any authenticated user can access admin functionality.

**Remediation:** Add role verification for `role_admin`.

### 3. Docker SDK Vulnerabilities (GO-2026-4887, GO-2026-4883)
**Package:** `github.com/docker/docker@v28.5.2+incompatible`
**Severity:** HIGH

AuthZ Plugin Bypass and Off-by-one privilege validation.

**Remediation:** Restrict Docker API network access. No patch available.

---

## Medium Findings

### 4. Direct DB Access in Migration Handler
**File:** `internal/api/handlers/migrations.go:26-27`

Bypasses Store interface abstraction.

### 5. gorilla/websocket Version Warning
**File:** `internal/api/ws/deploy.go`

Consider upgrading to latest version.

### 6. Git Token Transmission
**File:** `web/src/pages/GitSources.tsx:123-127`

Tokens sent in request body (not HTTP-only cookies).

---

## Positive Findings

| Category | Status |
|----------|--------|
| SQL Injection | PROTECTED |
| Command Injection | PROTECTED |
| SSRF | PROTECTED |
| Path Traversal | PROTECTED |
| CSRF Protection | SECURE |

---

## Remediation Priority

1. **Immediate:** Remove client-side JWT decoding
2. **Immediate:** Add admin role authorization
3. **Soon:** Upgrade gorilla/websocket
4. **Monitor:** Docker SDK for patches
