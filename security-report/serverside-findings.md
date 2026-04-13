# Server-Side Vulnerabilities — Phase 2 HUNT

## Finding 1: SSRF via Unvalidated Git Clone URL

**File**: `internal/build/builder.go`  
**Lines**: 234–256 (`ValidateGitURL`) and 263–298 (`validateResolvedHost`)

```go
parsed, err := url.Parse(raw)
// ... scheme filtering ...
// Line 251:
if parsed.Host != "" && isPrivateOrBlockedIP(parsed.Host) {
    return fmt.Errorf("git URL host %q resolves to a private or blocked IP range", parsed.Host)
}
```

**Issue**: The `ValidateGitURL` function checks for private IPs at validation time via `isPrivateOrBlockedIP`, and `validateResolvedHost` performs DNS lookup to prevent DNS rebinding. However, there is a race condition window: a URL validated at store time could resolve to a clean IP at validation, but later resolve to a private IP at clone time due to TTL expiry or DNS rebinding attack.

The function itself is sound, but the **timing of when validation occurs vs. when the URL is actually used for network access** creates a vulnerability window. Additionally, `validateResolvedHost` is not called from `ValidateGitURL` — it must be called separately by the caller.

**CWE**: CWE-918 (Server-Side Request Forgery)

---

## Finding 2: SSRF via Outbound Webhook URL from User Input

**File**: `internal/api/handlers/event_webhooks.go`  
**Lines**: 85–159 (`Create`)

```go
var req struct {
    URL    string   `json:"url"`
    Secret string   `json:"secret,omitempty"`
    Events []string `json:"events"`
}
// ...
wh := EventWebhookConfig{
    ID:         core.GenerateID(),
    URL:        req.URL,   // <-- user-controlled URL stored directly
    SecretHash: hashSecret(secret),
    Events:     req.Events,
    Active:     true,
    TenantID:   claims.TenantID,
}
```

**Issue**: The `URL` field accepted from the user is stored and later used to make outbound HTTP POST requests to external servers. There is **no URL validation** at creation time (e.g., block `http://` scheme, block private IPs, block internal hostnames like `localhost`, `169.254.169.254`, etc.).

While the `outbound webhook delivery` code was not fully traced to a complete send path in the handlers, the `EventWebhookConfig.URL` is directly user-controlled without sanitization. The webhook URL is returned in the Create response at line 153, confirming it is used as an outbound HTTP target.

**CWE**: CWE-918 (Server-Side Request Forgery)

---

## Finding 3: HTTPS Redirect Using User-Controlled Host Header

**File**: `internal/ingress/module.go`  
**Lines**: 232–240

```go
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    if forceHTTPS {
        w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        target := "https://" + r.Host + r.URL.RequestURI()
        http.Redirect(w, r, target, http.StatusMovedPermanently)
        return
    }
    // ...
})
```

**Issue**: The redirect target uses `r.Host` directly from the HTTP request. An attacker could send a crafted `Host` header (e.g., `Host: evil.com`) to cause a redirect to an external domain. This is an **open redirect** vulnerability.

While the target is always prefixed with `https://`, an attacker could set `Host: evil.com` and the redirect would go to `https://evil.com/original-path`, enabling phishing attacks.

**CWE**: CWE-601 (URL Redirection to Untrusted Site)

---

## Finding 4: Path Traversal in Topology Deployment Work Directory

**File**: `internal/api/handlers/topology.go`  
**Line**: 312

```go
workDir := filepath.Join("/var/lib/deploymonster", "deployments", claims.TenantID, req.ProjectID, req.Environment)
```

**Issue**: `req.ProjectID` and `req.Environment` come from the user-provided request body. If either contains `..` path traversal sequences, the `filepath.Join` will resolve them, potentially escaping the intended deployment directory structure.

Although `filepath.Join` does clean the path, it does not validate that the final resolved path is within the intended base. For example, if `req.ProjectID = "../../../etc"` or `req.Environment = "../../../root"`, the path could escape to sensitive locations.

The fix should use `filepath.Clean` + boundary check similar to `internal/backup/local.go`.

**CWE**: CWE-22 (Path Traversal)

---

## Finding 5: Unsafe HTTP Scheme Allowlist in Import Validation

**File**: `internal/api/handlers/import_export.go`  
**Lines**: 100–107

```go
u, err := url.Parse(m.SourceURL)
if err != nil {
    errors = append(errors, "invalid source_url format")
} else if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "" {
    errors = append(errors, "source_url must use http, https, or ssh scheme")
}
```

**Issue**: The `http` scheme is explicitly allowed for `source_url`. This creates an SSRF vector — an attacker could specify `http://169.254.169.254/latest/meta-data/` (AWS metadata endpoint) or `http://localhost/` to probe internal services.

The builder's `ValidateGitURL` function at `internal/build/builder.go:241` correctly blocks `http` scheme:
```go
case "http":
    return fmt.Errorf("git URL scheme %q is not allowed (use HTTPS)", parsed.Scheme)
```

But the import handler uses a separate, weaker validation that permits `http`. This inconsistency means an imported app manifest could contain a `http://` git URL that bypasses the builder's check.

**CWE**: CWE-918 (Server-Side Request Forgery)

---

## Finding 6: Path Traversal via Import Manifest Branch Field

**File**: `internal/api/handlers/import_export.go`  
**Lines**: 110–114

```go
if m.Branch != "" {
    if strings.Contains(m.Branch, "..") || strings.ContainsAny(m.Branch, ";\n\r") {
        errors = append(errors, "branch contains invalid characters")
    }
}
```

**Issue**: The branch field is validated for `..` path traversal and some special characters, but the validation uses `strings.Contains` for `..` rather than a cleaner check. More importantly, the branch is used in git clone operations that could be vulnerable if the validation is somehow bypassed.

Also, the path traversal check `strings.Contains(m.Branch, "..")` only catches `..` but not other traversal patterns. The validation should reject any path characters entirely (branches are simple names, not paths).

**CWE**: CWE-22 (Path Traversal)

---

## Finding 7: Webhook URL Length Not Validated Against SSRF

**File**: `internal/api/handlers/event_webhooks.go`  
**Lines**: 106–108

```go
if len(req.URL) > 2048 {
    writeError(w, http.StatusBadRequest, "url must be 2048 characters or less")
    return
}
```

**Issue**: The URL length limit of 2048 characters is checked, but there is **no validation of the URL scheme** (allowing `http://`) and **no validation of the hostname** (blocking private IPs, localhost, metadata endpoints).

A 2048-character `http://169.254.169.254/latest/meta-data/` URL would be within the length limit and would pass validation.

**CWE**: CWE-918 (Server-Side Request Forgery)

---

## Summary Table

| # | Category | File | Lines | Severity |
|---|----------|------|-------|----------|
| 1 | SSRF | `internal/build/builder.go` | 234–298 | Medium |
| 2 | SSRF | `internal/api/handlers/event_webhooks.go` | 121–153 | High |
| 3 | Open Redirect | `internal/ingress/module.go` | 237–238 | Medium |
| 4 | Path Traversal | `internal/api/handlers/topology.go` | 312 | Medium |
| 5 | SSRF | `internal/api/handlers/import_export.go` | 101–107 | High |
| 6 | Path Traversal | `internal/api/handlers/import_export.go` | 110–114 | Low |
| 7 | SSRF | `internal/api/handlers/event_webhooks.go` | 106–108 | Medium |