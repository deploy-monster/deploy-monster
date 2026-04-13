# API Security & Rate Limiting Findings

**Audit Phase**: 2 - HUNT
**Date**: 2026-04-13
**Scope**: `internal/api/router.go`, `internal/api/middleware/`, `internal/api/handlers/`, `internal/api/ws/`

---

## Finding 1: GlobalRateLimiter Trusts X-Forwarded-For Without Validation

**File**: `internal/api/middleware/global_ratelimit.go:138`

```go
func (rl *GlobalRateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // ...
        ip := realIP(r)
```

**Code** (middleware.go:266-274):
```go
func realIP(r *http.Request) string {
    if ip := r.Header.Get("X-Real-IP"); ip != "" {
        return ip  // Returns unsanitized X-Real-IP
    }
    if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
        return ip  // Returns unsanitized X-Forwarded-For
    }
    return r.RemoteAddr
}
```

**Why It's a Vulnerability**:
- `realIP()` returns `X-Real-IP` and `X-Forwarded-For` headers without validation
- An attacker can spoof these headers to bypass per-IP rate limits
- The in-memory rate limiter keys off this IP, so spoofing allows unlimited requests

**Contrast with AuthRateLimiter**:
The `AuthRateLimiter` (ratelimit.go:58-98) properly validates IPs:
```go
func safeClientIP(r *http.Request, trustXFF bool) string {
    if !trustXFF {
        return stripPort(r.RemoteAddr)
    }
    // Validates via validateIP() which rejects private/loopback/link-local
    if ip := r.Header.Get("X-Real-IP"); ip != "" {
        if validated := validateIP(ip); validated != "" {
            return validated
        }
    }
    // ...
}
```

**CWE**: CWE-346 (Origin Validation Error), CWE-799 (Improper Restriction of Interaction Frequency)

**Severity**: Medium

---

## Finding 2: Verbose Health Endpoint Exposes Internal System State

**File**: `internal/api/handlers/health_detailed.go:29-115`

```go
func (h *DetailedHealthHandler) DetailedHealth(w http.ResponseWriter, r *http.Request) {
    // ...
    checks["database"] = map[string]any{"healthy": dbOK, "driver": h.core.Config.Database.Driver}
    checks["docker"] = map[string]any{"healthy": dockerOK}
    checks["events"] = map[string]any{
        "healthy":       true,
        "published":     evStats.PublishCount,
        "errors":        evStats.ErrorCount,
        "subscriptions": evStats.SubscriptionCount,
    }
    checks["runtime"] = map[string]any{
        "healthy":    true,
        "goroutines": runtime.NumGoroutine(),  // Internal goroutine count
        "alloc_mb":   mem.Alloc / 1024 / 1024, // Memory allocation
        "sys_mb":     mem.Sys / 1024 / 1024,   // System memory
        "gc_runs":    mem.NumGC,               // GC statistics
    }
    writeJSON(w, httpStatus, map[string]any{
        "status":   status,
        "version":  h.core.Build.Version,      // Version disclosure
        "checks":   checks,
        "duration": time.Since(start).String(),
    })
}
```

**Route** (`router.go:117-118`):
```go
detailedH := handlers.NewDetailedHealthHandler(r.core)
r.mux.HandleFunc("GET /health/detailed", detailedH.DetailedHealth)
```

**Why It's a Vulnerability**:
- `/health/detailed` is unauthenticated (no middleware wrapper shown)
- Exposes: version number, database driver, event bus statistics, goroutine count, memory stats, GC runs
- Version disclosure helps attackers target known vulnerabilities
- Goroutine/memory stats help identify load patterns and potential DoS vectors

**CWE**: CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)

**Severity**: Medium

---

## Finding 3: EventStreamer SSE Endpoint Exposes Internal Event Bus Data

**File**: `internal/api/ws/logs.go:95-143`

```go
func (es *EventStreamer) StreamEvents(w http.ResponseWriter, r *http.Request) {
    typeFilter := r.URL.Query().Get("type")
    // ...
    subID := es.events.SubscribeAsync(typeFilter, func(_ context.Context, event core.Event) error {
        // ...
        data := event.DebugString()
        _, _ = w.Write([]byte("event: " + event.Type + "\ndata: " + data + "\n\n"))
        // ...
    })
}
```

**Route** (`router.go:649`):
```go
r.mux.Handle("GET /api/v1/events/stream", protected(http.HandlerFunc(eventStreamer.StreamEvents)))
```

**Why It's a Vulnerability**:
- While the route is `protected` (requires auth), the event stream exposes internal system events
- Event names and payloads could reveal business logic, deployment workflows, and system architecture
- A compromised account could monitor all system events in real-time

**CWE**: CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)

**Severity**: Low (requires authentication)

---

## Finding 4: WebSocket Origin Validation Configurable to Wildcard

**File**: `internal/api/ws/deploy.go:105-126`

```go
func (h *DeployHub) upgrader() websocket.Upgrader {
    return websocket.Upgrader{
        CheckOrigin: func(r *http.Request) bool {
            if h.allowedOrigins == "*" {
                return true  // Accepts any origin if configured
            }
            // ...
        },
    }
}
```

**Configuration** (`router.go:652`):
```go
ws.GetDeployHub().SetAllowedOrigins(r.core.Config.Server.CORSOrigins)
```

**Why It's a Vulnerability**:
- If `CORSOrigins` is set to `*` in configuration, WebSocket origin checks are bypassed
- This allows cross-site WebSocket hijacking
- Attacker can open WebSocket connections from any origin if misconfigured

**CWE**: CWE-346 (Origin Validation Error), CWE-20 (Improper Input Validation)

**Severity**: Medium (depends on configuration)

---

## Finding 5: Terminal Command Blocklist is Pattern-Based (Bypass Potential)

**File**: `internal/api/ws/terminal.go:36-51`

```go
var blockedPatterns = []string{
    "rm -rf /", "rm -rf /*", ":(){ :|:& };:", "mkfs",
    "dd if=/dev/zero", "> /dev/sd", "chmod -R 777 /", "chown -R",
    "curl | sh", "wget | sh", "curl | bash", "wget | bash",
}

func isCommandSafe(cmd string) bool {
    cmdLower := strings.ToLower(cmd)
    for _, pattern := range blockedPatterns {
        if strings.Contains(cmdLower, strings.ToLower(pattern)) {
            return false
        }
    }
    return true
}
```

**Why It's a Vulnerability**:
- Blocklist approach is inherently weak — can be bypassed with encoding, whitespace, or novel patterns
- e.g., `rm -rf / /*` (extra space), `curl\x20|sh`, `$'rm -rf /'`, etc.
- No shell injection prevention when commands reach `/bin/sh`
- Pattern matching on lowercase only catches ASCII

**CWE**: CWE-20 (Improper Input Validation), CWE-78 (OS Command Injection)

**Severity**: Medium (mitigated by RequireAuth and RequireSuperAdmin)

---

## Finding 6: Bulk Operations Error Messages Include Raw Error Strings

**File**: `internal/api/handlers/bulk.go:78-100`

```go
case "start":
    if err := h.store.UpdateAppStatus(r.Context(), appID, "running"); err != nil {
        results[i].Status = "error"
        results[i].Error = err.Error()  // Raw error leaked to client
    }
```

**Why It's a Vulnerability**:
- Internal error messages (e.g., database errors, constraint violations) exposed to client
- Could reveal internal implementation details, table names, query structure
- Helps attackers craft targeted attacks against the database

**CWE**: CWE-209 (Information Exposure Through Error Message)

**Severity**: Low

---

## Finding 7: CORS Wildcard Not Guaranteed to Be Disabled in Production

**File**: `internal/api/middleware/middleware.go:120-123`

```go
func CORS(allowedOrigins string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        // ...
        if allowedOrigins == "*" {
            w.Header().Set("Access-Control-Allow-Origin", "*")
            // ...
        }
```

**Configuration Source** (`router.go:89`):
```go
middleware.CORS(r.core.Config.Server.CORSOrigins),
```

**Why It's a Vulnerability**:
- If `CORSOrigins` defaults to `*` in configuration, all origins are allowed
- Combined with `Access-Control-Allow-Credentials: true` (which only happens on explicit match), still risky
- Wildcard CORS prevents proper origin validation in browsers

**Note**: Code shows `Access-Control-Allow-Credentials` only set when `originMatched` is true (line 143), which correctly prevents wildcard+credentials. However, wildcard origin still allows any site to make requests.

**CWE**: CWE-346 (Origin Validation Error)

**Severity**: Medium (depends on configuration)

---

## Finding 8: CSRF Double-Submit Cookie Without SameSite Attribute

**File**: `internal/api/middleware/csrf.go:62-70`

```go
func SetCSRFCookie(w http.ResponseWriter, r *http.Request) {
    // ...
    http.SetCookie(w, &http.Cookie{
        Name:     csrfCookieName,
        Value:    token,
        Path:     "/",
        MaxAge:   86400,
        HttpOnly: false,
        Secure:   secure,
        SameSite: http.SameSiteLaxMode,  // Lax, not Strict
    })
}
```

**Why It's a Vulnerability**:
- `SameSite: http.SameSiteLaxMode` allows cross-site GET requests to send the cookie
- Combined with CSRF protection, but Lax mode means navigation GETs (links) will include the cookie
- Attackers could potentially exploit this for CSRF in certain browser configurations

**CWE**: CWE-344 (Origin Equality Error - SameSite cookie attribute not set properly)

**Severity**: Low (CSRF token still provides protection)

---

## Positive Security Findings (Not Vulnerabilities)

### Rate Limiting Present
- Global rate limiter: 120 req/min per IP for `/api/` and `/hooks/` paths
- Auth rate limiters: 5/min login, 3/min register, 5/min refresh
- Tenant rate limiter: 100 req/min per tenant (after auth)
- WebSocket frame rate limiter: 100 frames/sec with 200 burst

### Security Headers Present
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 0`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Content-Security-Policy` with strict directives
- HSTS header for TLS connections

### CSRF Protection Present
- Double-submit cookie pattern implemented
- Only enforced for cookie-authenticated requests (not Bearer/API key)

### Clickjacking Protection
- `X-Frame-Options: DENY` prevents framing

### Auth Middleware Comprehensive
- JWT validation (Authorization header and dm_access cookie)
- API key validation (X-API-Key header)
- Requires proper `dm_` prefix and 12+ character length
- SHA-256 hash comparison for API keys
- Expiration checking for API keys

### Admin Routes Protected
- `adminOnly` wrapper uses `RequireSuperAdmin` on top of `protected`
- Router test walks all admin routes and asserts 403

### Recovery Middleware
- Panic recovery prevents server crashes on handler panics

---

## Recommendations

1. **Fix GlobalRateLimiter.realIP**: Apply the same IP validation from AuthRateLimiter
2. **Add auth check to /health/detailed**: Require authentication or remove sensitive fields
3. **Remove event streaming or add additional filtering**: Don't expose all event types
4. **Log warning if CORSOrigins is "*"**: Alert operators to misconfiguration
5. **Prefer allowlist for terminal commands**: Use exec.Command with arg slicing instead of blocklist
6. **Set SameSite=Strict for CSRF cookie**: If session cookies are SameSite=Strict
7. **Sanitize bulk operation error messages**: Don't leak internal errors to clients