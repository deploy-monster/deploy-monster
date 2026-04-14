# CORS Security Scan Results

**Scanner:** sc-cors  
**Date:** 2026-04-14  
**Target:** DeployMonster Codebase (D:\CODEBOX\PROJECTS\DeployMonster_GO)

---

## Executive Summary

This report details the findings of a comprehensive CORS (Cross-Origin Resource Sharing) security scan on the DeployMonster codebase. The scan focused on CORS middleware configuration, origin validation, credential handling, preflight requests, and WebSocket CORS policies.

### Overall Assessment: MOSTLY SECURE with Minor Issues

| Severity | Count | Status |
|----------|-------|--------|
| Critical | 0 | Resolved |
| High | 1 | Active |
| Medium | 1 | Active |
| Low | 2 | Active |

---

## Detailed Findings

### CORS-001: Wildcard Origin with Credentials - RESOLVED (Security Fix Applied)

- **Severity:** None (Previously Critical)  
- **Status:** FIXED  
- **CWE:** CWE-942 (Permissive Cross-domain Policy with Untrusted Domains)  
- **File:** `internal/api/middleware/middleware.go`  
- **Lines:** 109-161

**Description:**
The CORS middleware previously allowed wildcard origin "*" to be configured with credentials, which is a CORS specification violation and security risk. The code has been updated with security fixes:

1. A warning is logged when wildcard origin is used (line 123)
2. Credentials are only set when `originMatched` is true (lines 147-151)
3. The middleware correctly prevents `Access-Control-Allow-Credentials: true` when using wildcard origins

**Current Secure Implementation:**
```go
// SECURITY: Never allow wildcard + credentials (CORS spec forbids it).
// Wildcard is discouraged - log warning for production use
slog.Warn("CORS wildcard origin (*) configured - this is insecure for production use",
    "path", r.URL.Path,
    "origin", origin,
)
w.Header().Set("Access-Control-Allow-Origin", "*")
w.Header().Set("Vary", "Origin")

// Only set Allow-Credentials when origin explicitly matched
if originMatched {
    w.Header().Set("Access-Control-Allow-Credentials", "true")
}
```

**Remediation:** COMPLETE - Security fix properly implemented.

---

### CORS-002: WebSocket Origin Validation Hardened - PARTIALLY RESOLVED

- **Severity:** High  
- **Confidence:** High  
- **CWE:** CWE-942 (Permissive Cross-domain Policy with Untrusted Domains)  
- **File:** `internal/api/ws/deploy.go`  
- **Lines:** 105-129

**Description:**
The WebSocket upgrader's `CheckOrigin` callback previously allowed empty origin headers, creating a security vulnerability. The code has been updated with a SECURITY FIX:

1. Empty origin headers are now REJECTED (lines 112-116)
2. A warning is logged for rejected connections
3. Wildcard "*" origin is still allowed but should be restricted in production

**Current Implementation:**
```go
func (h *DeployHub) upgrader() websocket.Upgrader {
    return websocket.Upgrader{
        CheckOrigin: func(r *http.Request) bool {
            if h.allowedOrigins == "*" {
                return true
            }
            origin := r.Header.Get("Origin")
            // SECURITY FIX: Empty origin header no longer allowed
            // This prevents cross-origin WebSocket connections from tools that don't send Origin headers
            if origin == "" {
                h.logger.Warn("WebSocket connection rejected: empty origin header")
                return false
            }
            for _, allowed := range strings.Split(h.allowedOrigins, ",") {
                if strings.TrimSpace(allowed) == origin {
                    return true
                }
            }
            h.logger.Warn("WebSocket origin rejected", "origin", origin)
            return false
        },
        // ...
    }
}
```

**Remaining Risk:**
The WebSocket handler still allows wildcard "*" origins which could be exploited if an admin misconfigures `MONSTER_CORS_ORIGINS=*`.

**Remediation:**
1. Add validation at startup to reject wildcard CORS origins in production mode
2. Consider requiring explicit origin list for WebSocket connections

---

### CORS-003: CORS Headers Configuration Review

- **Severity:** Medium  
- **Confidence:** High  
- **CWE:** CWE-942 (Permissive Cross-domain Policy with Untrusted Domains)  
- **File:** `internal/api/middleware/middleware.go`  
- **Lines:** 141-144

**Description:**
The CORS middleware currently sets the following headers:

```go
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-DeployMonster-Version, X-API-Version")
w.Header().Set("Access-Control-Max-Age", "86400")
```

**Security Observations:**

1. **Positive:** `X-CSRF-Token` has been REMOVED from allowed headers (security fix applied)
2. All HTTP methods are allowed including destructive ones (DELETE, PATCH, PUT)
3. Sensitive headers like `Authorization` and `X-API-Key` are exposed to cross-origin requests
4. Max age of 86400 seconds (24 hours) caches preflight responses for a full day

**Recommendations:**
1. Consider making allowed methods configurable via `monster.yaml`
2. Review if all methods need to be exposed cross-origin
3. Consider reducing `Access-Control-Max-Age` for faster policy updates

---

### CORS-004: Origin Validation Implementation

- **Severity:** Low  
- **Confidence:** High  
- **File:** `internal/api/middleware/middleware.go`  
- **Lines:** 129-139

**Description:**
The origin validation implementation performs exact string matching:

```go
for _, allowed := range strings.Split(allowedOrigins, ",") {
    if strings.TrimSpace(allowed) == origin {
        originMatched = true
        w.Header().Set("Access-Control-Allow-Origin", origin)
        w.Header().Set("Vary", "Origin")
        break
    }
}
```

**Security Observations:**

1. **Positive:** Uses exact string matching (not substring/prefix matching)
2. **Positive:** Trims whitespace from allowed origins
3. **Concern:** No validation that origins use HTTPS scheme
4. **Concern:** No protection against subdomain wildcard bypasses (e.g., `evil.example.com` vs `example.com`)

**Recommendations:**
1. Add HTTPS enforcement for production deployments
2. Consider validating origin format (scheme://host:port)
3. Consider implementing stricter subdomain validation

---

### CORS-005: Preflight Request Handling

- **Severity:** Low  
- **Confidence:** High  
- **File:** `internal/api/middleware/middleware.go`  
- **Lines:** 153-156

**Description:**
Preflight (OPTIONS) requests are handled correctly:

```go
if r.Method == http.MethodOptions {
    w.WriteHeader(http.StatusNoContent)
    return
}
```

**Security Observations:**

1. **Positive:** Returns 204 No Content for OPTIONS requests
2. **Positive:** Handler chain is terminated early for preflight requests
3. **Positive:** All CORS headers are set before the check
4. **Positive:** Test coverage exists in `cors_test.go`

---

### CORS-006: Vary Header Implementation

- **Severity:** Informational  
- **Confidence:** High  
- **File:** `internal/api/middleware/middleware.go`  
- **Lines:** 128, 135

**Description:**
The middleware correctly sets `Vary: Origin` header for both wildcard and specific origins:

```go
w.Header().Set("Vary", "Origin")
```

**Security Assessment:** CORRECT IMPLEMENTATION

The `Vary: Origin` header is essential for:
- Preventing caching of CORS responses with different origin headers
- Ensuring browsers receive correct CORS headers for each origin
- Preventing cache poisoning attacks

---

## Configuration Analysis

### CORS Origins Derivation

**File:** `internal/core/config.go` (lines 234-241)

```go
// Derive CORS origins from server domain if not explicitly set
if cfg.Server.CORSOrigins == "" && cfg.Server.Domain != "" {
    origin := "https://" + cfg.Server.Domain
    if cfg.Server.Port != 443 && cfg.Server.Port != 80 {
        origin = fmt.Sprintf("https://%s:%d", cfg.Server.Domain, cfg.Server.Port)
    }
    cfg.Server.CORSOrigins = origin
}
```

**Security Observations:**

1. **Concern:** Always assumes HTTPS scheme (may fail if `EnableHTTPS=false`)
2. **Concern:** No validation that the derived origin is well-formed
3. **Positive:** Respects non-standard ports

**Recommendation:**
Consider checking `cfg.Ingress.EnableHTTPS` when deriving origins.

---

## Test Coverage Analysis

### CORS Test Suite

**File:** `internal/api/middleware/cors_test.go`

The test suite covers:
- Wildcard origin handling
- Specific allowed origins
- Disallowed origin rejection
- No origin header scenarios
- Preflight OPTIONS requests
- Non-preflight request passthrough

**Test Coverage Assessment:** COMPREHENSIVE

All major CORS scenarios are tested including positive and negative cases.

---

## Risk Matrix

| Risk | Likelihood | Impact | Risk Level |
|------|------------|--------|------------|
| Wildcard CORS in production | Low | High | Medium |
| WebSocket origin bypass | Low | High | Medium |
| HTTP origin in production | Low | Medium | Low |
| Overly permissive methods | Low | Low | Low |
| Cache poisoning | Very Low | Medium | Very Low |

---

## Remediation Summary

### Immediate Actions (High Priority)

1. **Add startup validation** to reject wildcard CORS origins in production mode
2. **Document** the requirement for explicit origin lists in production

### Short-term Actions (Medium Priority)

1. **Add HTTPS enforcement** for CORS origins when `EnableHTTPS=true`
2. **Consider** making allowed methods configurable via `monster.yaml`

### Long-term Actions (Low Priority)

1. **Add** CORS configuration monitoring/alerting
2. **Consider** origin validation using URL parsing for stricter checks
3. **Review** if `Access-Control-Max-Age` of 86400 is appropriate

---

## Positive Security Controls

The following security controls are properly implemented:

1. **Credential Protection:** Credentials are never sent with wildcard origins (CORS spec compliance)
2. **Vary Header:** `Vary: Origin` is properly set for caching safety
3. **Origin Validation:** Specific origins are validated against an allowlist
4. **X-CSRF-Token Removed:** Security fix applied to prevent CSRF token exfiltration
5. **WebSocket Hardening:** Empty origin headers are now rejected
6. **Security Headers:** `X-Frame-Options: DENY`, CSP, and other headers are applied
7. **CSRF Protection:** CSRF middleware is applied globally

---

## Configuration Recommendations

### Secure CORS Configuration

```yaml
# monster.yaml - Secure CORS configuration
server:
  domain: "deploy.example.com"
  # Explicitly set origins - NEVER use "*" in production
  cors_origins: "https://deploy.example.com,https://admin.deploy.example.com"
  
ingress:
  enable_https: true
  force_https: true
```

### Environment Variable

```bash
# Production - explicit origins only
MONSTER_CORS_ORIGINS="https://deploy.example.com"

# NEVER use in production
# MONSTER_CORS_ORIGINS="*"
```

---

## Compliance Notes

### OWASP CORS Guidelines

The implementation aligns with OWASP recommendations:
- [x] Avoid wildcard origins with credentials
- [x] Validate origins explicitly
- [x] Set appropriate Vary headers
- [x] Limit exposed headers
- [ ] Enforce HTTPS origins (partial)
- [ ] Avoid wildcard in production (relies on admin configuration)

### CORS Specification Compliance

- [x] Proper preflight handling
- [x] Correct Access-Control-Allow-Origin behavior
- [x] Access-Control-Allow-Credentials only with explicit origins
- [x] Vary header for cache safety

---

*Report generated by sc-cors security scanner*  
*End of Report*
