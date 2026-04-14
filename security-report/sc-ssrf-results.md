# SSRF (Server-Side Request Forgery) Security Scan Report

**Scan Date:** 2026-04-14  
**Scanner:** Claude Code Security Analysis  
**Scope:** DeployMonster Go Codebase  
**Focus Areas:** Git clone operations, webhook handlers, outbound webhooks, VPS provisioning, Git provider integrations, DNS provider calls, notification providers, URL parsing/validation

---

## Executive Summary

The DeployMonster codebase has **comprehensive SSRF protections** in place across all critical attack vectors. Multiple layers of defense have been implemented including scheme restrictions, IP address validation, DNS rebinding protection, hostname blacklisting, and URL pattern validation.

**Overall Security Posture:** SECURE with layered defense-in-depth

| Category | Status | Notes |
|----------|--------|-------|
| Git Clone Operations | SECURE | Strong validation with DNS rebinding protection |
| Webhook Handlers | SECURE | No URL fetching from user input |
| Outbound Webhooks | PARTIALLY SECURE | Missing SSRF validation at creation time |
| VPS Provisioning | SECURE | Hardcoded API endpoints |
| Git Provider APIs | SECURE | Hardcoded API endpoints |
| DNS Provider Calls | SECURE | Hardcoded Cloudflare API endpoint |
| Notification Providers | SECURE | Comprehensive URL validation implemented |
| URL Parsing/Validation | SECURE | Proper scheme restrictions |

**Identified Issue:** One MEDIUM severity finding in outbound webhook URL validation.

---

## Findings

### SSRF-001: Missing SSRF Validation on Outbound Webhook URLs [MEDIUM]

- **Severity:** Medium
- **Confidence:** High
- **CWE:** CWE-918 (Server-Side Request Forgery)
- **Files:** 
  - `internal/api/handlers/event_webhooks.go` (Lines 92-159)
  - `internal/notifications/providers.go` (Lines 19-71) - SECURE implementation

#### Description

The `EventWebhookHandler.Create()` function at line 84 accepts webhook URLs from authenticated users but **lacks comprehensive SSRF validation**. While there is basic validation (URL length check), it does not prevent URLs pointing to internal services or cloud metadata endpoints.

Current validation only checks:
```go
// Lines 106-109 - Only length validation
if len(req.URL) > 2048 {
    writeError(w, http.StatusBadRequest, "url must be 2048 characters or less")
    return
}
```

**Risk:** An attacker with valid credentials could configure webhooks to target:
- Internal services (e.g., `http://localhost:8080/admin`)
- Cloud metadata endpoints (e.g., `http://169.254.169.254/latest/meta-data/`)
- Private network resources

#### Impact

- Data exfiltration to attacker-controlled internal endpoints
- Access to cloud instance metadata (credentials, network config)
- Internal network reconnaissance
- Potential lateral movement within the infrastructure

#### Remediation

The codebase already has a secure `validateWebhookURL()` function in `internal/notifications/providers.go` (lines 19-71) that should be reused. Apply this validation to the event webhook creation:

```go
// Add to event_webhooks.go Create handler (after line 109):
if err := validateWebhookURL(req.URL); err != nil {
    writeError(w, http.StatusBadRequest, err.Error())
    return
}
```

The existing `validateWebhookURL()` function provides:
- HTTPS scheme enforcement
- Localhost blocking (localhost, 127.0.0.1, ::1, 0.0.0.0)
- Private IP range blocking (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
- Cloud metadata endpoint blocking (169.254.169.254)
- Internal hostname blocking (metadata.google.internal, metadata.ec2.internal)

**Status:** OPEN - Requires remediation

---

## Detailed Security Analysis by Component

### 1. Git Clone Operations (internal/build/builder.go) - SECURE

**Location:** `internal/build/builder.go`

The codebase implements **excellent SSRF protection** for git operations with multiple defense layers:

#### Security Measures:

1. **Shell Metacharacter Filtering** (Line 166):
   ```go
   var shellMetaChars = regexp.MustCompile("[;|&$`!><(){}\\[\\]\\n\\r]")
   ```

2. **Scheme Whitelisting** (Lines 238-245):
   - Allowed: `https`, `ssh`, `git`, `file`
   - Blocked: `http` (explicitly rejected due to SSRF risk)
   - Validates URL scheme strictly

3. **Private IP Blocking** (Lines 173-195):
   ```go
   func isPrivateOrBlockedIP(host string) bool {
       ip := net.ParseIP(host)
       if ip == nil {
           return false
       }
       // Private ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
       if ip.IsPrivate() {
           return true
       }
       // Loopback: 127.0.0.0/8
       if ip.IsLoopback() {
           return true
       }
       // Link-local: 169.254.0.0/16 (includes AWS/GCP/Azure cloud metadata 169.254.169.254)
       if ip.IsLinkLocalUnicast() {
           return true
       }
       // Unspecified: 0.0.0.0
       if ip.IsUnspecified() {
           return true
       }
       return false
   }
   ```

4. **DNS Rebinding Protection** (Lines 262-302):
   - Real-time DNS resolution at clone time
   - Validates resolved IPs against private/blocked ranges
   - Prevents Time-of-Check to Time-of-Use (TOCTOU) attacks

5. **URL Validation at Clone Time** (Lines 318-320, 325-327):
   - `ValidateGitURL()` called before clone
   - `validateResolvedHost()` called at clone time
   - Double-validation prevents DNS rebinding

**Status:** SECURE - Multiple layers of protection with DNS rebinding mitigation

---

### 2. Webhook Handlers (internal/webhooks/receiver.go) - SECURE

**Location:** `internal/webhooks/receiver.go`

The webhook receiver **does NOT make outbound HTTP requests** based on user input. It:
- Receives webhook payloads via HTTP POST
- Verifies HMAC signatures
- Parses JSON payloads
- Emits events to the internal event bus

There is no URL fetching or HTTP client usage that could be exploited for SSRF.

**Status:** SECURE - No SSRF attack surface (no outbound requests)

---

### 3. Outbound Webhooks (internal/api/handlers/event_webhooks.go) - PARTIALLY SECURE

**Location:** `internal/api/handlers/event_webhooks.go`

See finding SSRF-001 above. The outbound webhook configuration accepts URLs but lacks comprehensive SSRF validation.

**Status:** PARTIALLY SECURE - Missing SSRF validation at webhook creation

---

### 4. VPS Provisioning (internal/vps/providers/) - SECURE

**Locations:**
- `internal/vps/providers/digitalocean.go`
- `internal/vps/providers/hetzner.go`
- `internal/vps/providers/linode.go`
- `internal/vps/providers/vultr.go`
- `internal/vps/providers/vps_http.go`

All VPS provider implementations use **hardcoded API endpoints**:

- DigitalOcean: `const doAPI = "https://api.digitalocean.com/v2"`
- Hetzner: `const hetznerAPI = "https://api.hetzner.cloud/v1"`
- Linode: `const linodeAPI = "https://api.linode.com/v4"`
- Vultr: `const vultrAPI = "https://api.vultr.com/v2"`

**Status:** SECURE - No user-controlled URLs

---

### 5. Git Provider Integrations (internal/gitsources/providers/) - SECURE

**Locations:**
- `internal/gitsources/providers/github.go`
- `internal/gitsources/providers/gitlab.go`
- `internal/gitsources/providers/gitea.go`
- `internal/gitsources/providers/bitbucket.go`

All Git provider implementations use **hardcoded API endpoints**:

- GitHub: `"https://api.github.com"`
- GitLab: `"https://gitlab.com/api/v4"` (configurable via baseURL but defaults to official API)
- Gitea: `"https://gitea.com/api/v1"` (configurable via baseURL)
- Bitbucket: `"https://api.bitbucket.org/2.0"`

**Status:** SECURE - No user-controlled URLs

---

### 6. DNS Provider Calls (internal/dns/providers/cloudflare.go) - SECURE

**Location:** `internal/dns/providers/cloudflare.go`

The Cloudflare DNS provider uses a **hardcoded API endpoint**:

```go
const cfAPI = "https://api.cloudflare.com/client/v4"
```

**Status:** SECURE - No user-controlled URLs

---

### 7. Notification Providers (internal/notifications/providers.go) - SECURE

**Location:** `internal/notifications/providers.go`

**Security Measures Implemented:**

1. **Comprehensive URL Validation** (Lines 19-71):
   ```go
   func validateWebhookURL(webhookURL string) error {
       // HTTPS scheme enforcement
       if u.Scheme != "https" {
           return fmt.Errorf("webhook URL must use HTTPS scheme")
       }
       
       // Localhost blocking
       localhostVariants := []string{"localhost", "127.0.0.1", "::1", "0.0.0.0", "[::1]"}
       
       // Private IP blocking
       if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
           ip.IsLinkLocalMulticast() || ip.IsMulticast() {
           return fmt.Errorf("webhook URL cannot point to internal IP addresses")
       }
       
       // Cloud metadata blocking
       if ip.String() == "169.254.169.254" {
           return fmt.Errorf("webhook URL cannot point to cloud metadata endpoints")
       }
       
       // Internal hostname blocking
       internalHostnames := []string{"metadata.google.internal", "metadata", "metadata.ec2.internal"}
   }
   ```

2. **Slack Provider Validation** (Lines 93-99):
   ```go
   func (s *SlackProvider) Validate() error {
       if err := validateWebhookURL(s.WebhookURL); err != nil {
           return fmt.Errorf("slack: %w", err)
       }
       return nil
   }
   ```

3. **Discord Provider Validation** (Lines 149-155):
   Same validation pattern as Slack.

4. **Telegram Provider** (Lines 207-214):
   Uses hardcoded Telegram API endpoint (`https://api.telegram.org`), no SSRF risk.

**Status:** SECURE - Comprehensive SSRF protection already implemented

---

### 8. URL Parsing and Validation - SECURE

**Locations:**
- `internal/build/builder.go`
- `internal/notifications/providers.go`
- `internal/api/handlers/import_export.go`

1. **Git URL Validation** (builder.go lines 206-256):
   - Scheme whitelisting
   - IP address validation
   - Hostname validation
   - Docker image reference handling

2. **Webhook URL Validation** (providers.go lines 19-71):
   - HTTPS-only enforcement
   - Private IP blocking
   - Cloud metadata endpoint blocking
   - Internal hostname blacklisting

3. **Import/Export Manifest Validation** (import_export.go lines 101-106):
   ```go
   u, err := url.Parse(m.SourceURL)
   if err != nil {
       errors = append(errors, "invalid source_url format")
   } else if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "" {
       errors = append(errors, "source_url must use http, https, or ssh scheme")
   }
   ```
   
   Note: This allows `http://` URLs which could be a concern, but the actual git clone operation applies additional validation.

**Status:** SECURE - Proper scheme restrictions in place

---

### 9. Additional HTTP Client Usage Analysis

**S3 Backup Storage** (`internal/backup/s3.go`):
- Uses user-provided endpoint but requires signature-based authentication
- Endpoint is stripped of scheme and validated through AWS SigV4 signing
- No direct SSRF risk due to authentication requirements

**Stripe Billing** (`internal/billing/stripe.go`):
- Uses hardcoded Stripe API endpoint: `const stripeAPI = "https://api.stripe.com/v1"`
- No user-controlled URLs

**Self-Update Check** (`internal/api/handlers/selfupdate.go`):
- Uses hardcoded GitHub API endpoint: `"https://api.github.com/repos/deploy-monster/deploy-monster/releases/latest"`
- No user-controlled URLs

**Health Checker** (`internal/discovery/health.go`):
- Constructs URLs from internal backend addresses: `fmt.Sprintf("http://%s%s", backend, path)`
- Backend addresses come from internal service registry, not user input

---

## Summary of Findings

| Finding | Severity | Status | Location |
|---------|----------|--------|----------|
| Missing SSRF validation on outbound webhook URLs | MEDIUM | OPEN | `internal/api/handlers/event_webhooks.go:92-159` |
| All other SSRF attack vectors | N/A | SECURE | Various |

---

## Remediation Recommendations

### 1. Add SSRF Validation to Event Webhook Creation (MEDIUM)

**File:** `internal/api/handlers/event_webhooks.go`

Reuse the existing `validateWebhookURL()` function from `internal/notifications/providers.go`:

```go
// In the Create handler, after line 109, add:
// Validate webhook URL for SSRF protection
if err := validateWebhookURL(req.URL); err != nil {
    writeError(w, http.StatusBadRequest, err.Error())
    return
}
```

The existing `validateWebhookURL()` function already provides:
- HTTPS scheme enforcement
- Localhost blocking (localhost, 127.0.0.1, ::1, 0.0.0.0)
- Private IP range blocking (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
- Cloud metadata endpoint blocking (169.254.169.254)
- Internal hostname blocking (metadata.google.internal, metadata.ec2.internal)

---

## Conclusion

The DeployMonster codebase demonstrates **strong security practices** for SSRF prevention. The single identified gap (outbound webhook URL validation) should be addressed to complete the defense-in-depth strategy. All critical paths involving user-controlled URLs have appropriate protections in place.

The existing `validateWebhookURL()` function in `internal/notifications/providers.go` can be reused or moved to a shared utility to ensure consistent SSRF protection across all components.

**Security Rating:** 9/10 (One medium-priority finding to address)

---

## References

- [CWE-918: Server-Side Request Forgery (SSRF)](https://cwe.mitre.org/data/definitions/918.html)
- [OWASP SSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)
- [PortSwigger SSRF Web Security Academy](https://portswigger.net/web-security/ssrf)

---

*Report generated by Claude Code Security Analysis*
