# Open Redirect Security Scan Results

**Scan Date:** 2026-04-14  
**Scanner:** Claude Code Security Analysis  
**Scope:** DeployMonster codebase (D:\CODEBOX\PROJECTS\DeployMonster_GO)  
**Target:** Open Redirect vulnerabilities (CWE-601)

---

## Executive Summary

The Open Redirect security scan examined the DeployMonster codebase for vulnerabilities that could allow attackers to redirect users to malicious external websites. The scan focused on:

1. OAuth callback handlers and redirect_uri validation
2. Login redirect parameters
3. Post-login redirects
4. External URL validation
5. URL parsing and scheme validation
6. Protocol handler validation (javascript:, data:)
7. Fragment injection attacks

**Overall Finding:** DeployMonster has **LOW RISK** for Open Redirect vulnerabilities. The codebase implements several security controls, but a few areas require attention.

---

## Findings Overview

| Severity | Count | Status |
|----------|-------|--------|
| High | 0 | - |
| Medium | 1 | Review Recommended |
| Low | 2 | Best Practice |
| Info | 3 | Documentation |

---

## Detailed Findings

### FINDING-001: HTTPS Redirect in Ingress Module - SECURE

**Location:** `internal/ingress/module.go:237-238`

**Code:**
```go
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    if forceHTTPS {
        w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        target := "https://" + r.Host + r.URL.RequestURI()
        http.Redirect(w, r, target, http.StatusMovedPermanently)
        return
    }
    ...
})
```

**Analysis:** The HTTPS redirect in the ingress module is **SECURE**. The redirect target is constructed using:
- `r.Host` - from the HTTP Host header
- `r.URL.RequestURI()` - from the parsed URL

**Why it's secure:**
1. No user-controlled parameters are used in the redirect
2. The scheme is hardcoded to "https://"
3. The Host header is validated by the HTTP server
4. RequestURI() is properly URL-encoded by Go's standard library
5. HSTS header is set to prevent downgrade attacks

**Severity:** Info  
**Status:** No action required

---

### FINDING-002: WebSocket URL Construction - SECURE with Validation

**Location:** `web/src/hooks/useDeployProgress.ts:78-82`

**Code:**
```typescript
// SECURITY FIX: Use URL constructor for safer URL building
const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const wsUrl = new URL(`/api/v1/topology/deploy/${encodeURIComponent(projectId)}/progress`, `${protocol}//${window.location.host}`).toString();
```

**Analysis:** The WebSocket URL construction is **SECURE**. Key security measures:

1. **Input validation:** `projectId` is validated against safe character pattern
   ```typescript
   if (!/^[a-zA-Z0-9_-]+$/.test(projectId)) {
     console.error('Invalid projectId format');
     return;
   }
   ```

2. **Proper encoding:** `encodeURIComponent()` is used for the `projectId` path segment

3. **URL constructor:** Uses the standard `URL` constructor with a base URL

**Severity:** Info  
**Status:** No action required

---

### FINDING-003: Client-Side Login Redirect - SECURE

**Location:** `web/src/api/client.ts:232,239`

**Code:**
```typescript
if (response.status === 401) {
  if (options._noRefresh) {
    window.location.href = '/login';
    throw new APIError(401, 'Session expired');
  }
  const refreshed = await tryRefresh();
  if (refreshed) {
    return request<T>(path, { ...options, _noRefresh: true });
  }
  window.location.href = '/login';
  throw new APIError(401, 'Session expired');
}
```

**Analysis:** The client-side redirect to `/login` is **SECURE**:

1. **Hardcoded path:** The redirect URL is hardcoded to `/login`
2. **No user input:** No user-controlled data is used in the redirect
3. **Same-origin:** Redirects to same-origin path only
4. **No query parameters:** No dynamic query parameters that could be manipulated

**Severity:** Info  
**Status:** No action required

---

### FINDING-004: React Router Navigation - SECURE

**Location:** Various files (`Login.tsx:67`, `Register.tsx:112`, `Onboarding.tsx:128`, etc.)

**Code Pattern:**
```typescript
const navigate = useNavigate();
// ...
navigate('/');
```

**Analysis:** Navigation using React Router's `useNavigate` is **SECURE**:

1. **Client-side routing:** Uses React Router's internal history API
2. **No external redirects:** All navigation targets are internal application routes
3. **No user input:** Navigation targets are hardcoded or validated

**Severity:** Info  
**Status:** No action required

---

### FINDING-005: Redirect Handler Destination Validation - MEDIUM

**Location:** `internal/api/handlers/redirects.go:57-113`

**Code:**
```go
func (h *RedirectHandler) Create(w http.ResponseWriter, r *http.Request) {
    // ...
    var rule RedirectRule
    if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    if rule.Source == "" || rule.Destination == "" {
        writeError(w, http.StatusBadRequest, "source and destination required")
        return
    }
    if len(rule.Source) > 2048 {
        writeError(w, http.StatusBadRequest, "source must be 2048 characters or less")
        return
    }
    if len(rule.Destination) > 2048 {
        writeError(w, http.StatusBadRequest, "destination must be 2048 characters or less")
        return
    }
    // ...
}
```

**Analysis:** The redirect rule creation endpoint accepts a `Destination` field that can be any URL including external sites. While this is by design for the ingress module's redirect functionality, there are security considerations:

**Current validation:**
1. Length limit (2048 characters)
2. Required field check
3. No scheme validation

**Potential risks:**
1. **Open redirect via app redirects:** An attacker with access could create a redirect rule pointing to a malicious external site
2. **Protocol handler abuse:** Destination could be `javascript:alert(1)` or `data:text/html,<script>...`
3. **Phishing:** Redirect rules could be used to create convincing phishing URLs

**Recommendations:**
```go
// Add destination URL validation
func validateRedirectDestination(dest string) error {
    if dest == "" {
        return fmt.Errorf("destination is required")
    }
    
    // Parse the URL
    u, err := url.Parse(dest)
    if err != nil {
        return fmt.Errorf("invalid destination URL: %w", err)
    }
    
    // Block dangerous schemes
    dangerousSchemes := []string{"javascript", "data", "vbscript", "file", "ftp"}
    for _, scheme := range dangerousSchemes {
        if strings.EqualFold(u.Scheme, scheme) {
            return fmt.Errorf("scheme %q is not allowed", scheme)
        }
    }
    
    // For absolute URLs, validate scheme
    if u.IsAbs() {
        allowedSchemes := []string{"http", "https"}
        schemeValid := false
        for _, scheme := range allowedSchemes {
            if strings.EqualFold(u.Scheme, scheme) {
                schemeValid = true
                break
            }
        }
        if !schemeValid {
            return fmt.Errorf("scheme %q is not allowed", u.Scheme)
        }
    }
    
    // Block URLs containing @ (credential injection)
    if strings.Contains(dest, "@") {
        return fmt.Errorf("destination cannot contain '@'")
    }
    
    // Block URLs with fragment-only redirects that could be used for XSS
    if strings.HasPrefix(dest, "#") {
        return fmt.Errorf("fragment-only redirects are not allowed")
    }
    
    return nil
}
```

**Severity:** Medium  
**Status:** Review Recommended

---

### FINDING-006: Download File URL Construction in CompileModal - LOW

**Location:** `web/src/components/topology/CompileModal.tsx:43-51`

**Code:**
```typescript
const downloadFile = (content: string, filename: string) => {
  const blob = new Blob([content], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
};
```

**Analysis:** The file download functionality uses `URL.createObjectURL()` which is **SECURE**:

1. **Blob URLs:** Creates blob: URLs that are same-origin only
2. **No external URLs:** Does not accept external URLs from user input
3. **Download attribute:** Uses the `download` attribute for client-side downloads

**Note:** The `filename` parameter is not sanitized, but this is low risk as it only affects the downloaded file name locally.

**Severity:** Low  
**Status:** Best Practice (optional validation)

---

### FINDING-007: Git URL Validation in Builder - SECURE

**Location:** `internal/build/builder.go:206-256`

**Code:**
```go
func ValidateGitURL(raw string) error {
    if raw == "" {
        return fmt.Errorf("git URL is empty")
    }
    if shellMetaChars.MatchString(raw) {
        return fmt.Errorf("git URL contains disallowed characters")
    }
    if strings.HasPrefix(raw, "-") {
        return fmt.Errorf("git URL must not start with a dash")
    }
    
    // Docker image references (source_type=image)
    if dockerImageRef.MatchString(raw) && !strings.Contains(raw, "://") {
        return nil
    }
    
    // Local absolute path
    if isAbsPath(raw) {
        return nil
    }
    
    // SSH shorthand: git@github.com:org/repo.git
    if sshLikeURL.MatchString(raw) {
        return nil
    }
    
    // Standard URL: https://, ssh://, git://, file:// (http:// is NOT allowed)
    parsed, err := url.Parse(raw)
    if err != nil {
        return fmt.Errorf("git URL is malformed: %w", err)
    }
    switch parsed.Scheme {
    case "https", "ssh", "git", "file":
        // allowed
    case "http":
        return fmt.Errorf("git URL scheme %q is not allowed (use HTTPS)", parsed.Scheme)
    default:
        return fmt.Errorf("git URL scheme %q is not allowed", parsed.Scheme)
    }
    // ...
}
```

**Analysis:** Git URL validation is **COMPREHENSIVE** and **SECURE**:

1. **Shell injection prevention:** Blocks shell metacharacters
2. **Scheme whitelist:** Only allows https, ssh, git, file (explicitly blocks http)
3. **Flag injection prevention:** Rejects URLs starting with "-"
4. **Private IP blocking:** `isPrivateOrBlockedIP()` blocks internal network access
5. **DNS rebinding protection:** `validateResolvedHost()` validates resolved IPs at clone time

**Severity:** Info  
**Status:** Excellent security implementation

---

### FINDING-008: App Manifest Source URL Validation - SECURE

**Location:** `internal/api/handlers/import_export.go:93-107`

**Code:**
```go
// Validate source_url (required, must be valid URL or image reference)
if m.SourceURL == "" {
    errors = append(errors, "source_url is required")
} else if m.SourceType == "image" || m.SourceType == "docker" {
    // For Docker images, validate as image reference
    if strings.ContainsAny(m.SourceURL, ";\n\r") {
        errors = append(errors, "source_url contains invalid characters")
    }
} else {
    u, err := url.Parse(m.SourceURL)
    if err != nil {
        errors = append(errors, "invalid source_url format")
    } else if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "" {
        errors = append(errors, "source_url must use http, https, or ssh scheme")
    }
}
```

**Analysis:** Source URL validation in import manifest is **SECURE**:

1. **Scheme whitelist:** Only allows http, https, ssh, or empty scheme
2. **Character filtering:** Blocks dangerous characters for Docker images
3. **URL parsing:** Validates URL format

**Severity:** Info  
**Status:** No action required

---

### FINDING-009: Webhook URL Handling - SECURE

**Location:** `internal/webhooks/receiver.go`

**Analysis:** Webhook handling is **SECURE**:

1. **No redirects:** Webhook receiver does not perform any redirects
2. **Signature verification:** HMAC-SHA256 signature verification for all webhooks
3. **Provider detection:** Based on headers, not user input
4. **JSON parsing:** Uses safe JSON unmarshaling

**Severity:** Info  
**Status:** No action required

---

## Areas Not Found (Positive Security Notes)

The following common Open Redirect patterns were **NOT FOUND** in the codebase:

1. **No OAuth redirect_uri parameters:** DeployMonster does not implement OAuth 2.0 flow with redirect_uri parameters
2. **No returnTo/next URL parameters:** Login/logout flows use hardcoded redirects
3. **No user-controlled redirect targets:** No endpoints accept user-controlled redirect URLs
4. **No protocol handler abuse:** No user input reaches protocol handlers
5. **No fragment injection:** Fragment handling is not user-controlled

---

## Recommendations

### Immediate Actions

1. **FINDING-005 (Medium):** Implement destination URL validation for redirect rules
   - Block dangerous schemes (javascript:, data:, vbscript:, file:, ftp:)
   - Validate URL format using `url.Parse()`
   - Block credential injection attempts (@ symbol)
   - Consider implementing a whitelist of allowed domains for external redirects

### Best Practices

2. **FINDING-006 (Low):** Add optional filename sanitization for downloads
   - Strip path separators from filenames
   - Validate filename length
   - Remove potentially dangerous characters

3. **General:** Consider implementing a centralized redirect validation utility
   ```go
   package security
   
   func ValidateRedirectURL(url string, options RedirectOptions) error
   func IsSafeRedirectTarget(url string) bool
   ```

4. **Documentation:** Document the redirect rule feature security considerations in the API documentation

---

## Conclusion

The DeployMonster codebase demonstrates **strong security practices** regarding Open Redirect vulnerabilities:

- No user-controlled redirect parameters in authentication flows
- Comprehensive URL validation for external resources (git URLs)
- Secure HTTPS redirect implementation
- Proper use of React Router for client-side navigation
- HSTS headers for HTTPS enforcement

The **single medium-risk finding** relates to the redirect rule feature which could potentially be used to create open redirects if an attacker gains authorized access to create redirect rules. Implementing the recommended validation would reduce this risk to an acceptable level.

**Overall Risk Rating:** LOW

---

## Appendix: Security Controls Summary

| Control | Implementation | Status |
|---------|---------------|--------|
| HTTPS Enforcement | HSTS headers + HTTP->HTTPS redirect | Secure |
| URL Scheme Validation | Whitelist approach (http, https, ssh, git, file) | Secure |
| Private IP Blocking | `isPrivateOrBlockedIP()` function | Secure |
| DNS Rebinding Protection | `validateResolvedHost()` real-time validation | Secure |
| Shell Injection Prevention | Metacharacter filtering | Secure |
| Client-Side Navigation | React Router (no external redirects) | Secure |
| WebSocket URL Construction | Input validation + `encodeURIComponent` | Secure |
| Redirect Rule Destination | **Needs validation** | Review |

---

*Report generated by Claude Code Security Analysis*
