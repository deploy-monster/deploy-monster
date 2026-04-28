# Security Assessment Report

**Project:** DeployMonster
**Date:** 2026-04-25
**Scanner:** security-check v1.1.0
**Risk Score:** 7.1/10 (High Risk)

---

## Executive Summary

A security assessment was performed on DeployMonster, a self-hosted PaaS (Platform as a Service) built with Go 1.26+ and React 19. The scan analyzed approximately 543,000 lines of code across Go, TypeScript, YAML, CSS, and SQL using 36 automated security skills across 10 vulnerability categories.

DeployMonster demonstrates a strong security foundation with comprehensive authentication, authorization, rate limiting, audit logging, and input validation. However, several medium and high-severity findings require attention, particularly around first-run credential handling, container exec security, and RBAC granularity.

**No Critical vulnerabilities were identified.** All findings are actionable and most have straightforward remediation paths.

### Key Metrics
| Metric | Value |
|--------|-------|
| Total Findings | 26 |
| Critical | 0 |
| High | 3 |
| Medium | 14 |
| Low | 9 |
| Info | 0 |

### Top Risks
1. **Auto-generated super admin credentials written to disk** (High) — First-run setup writes plaintext credentials to `.credentials` file with no automatic cleanup.
2. **Docker client v28.5.2 known vulnerabilities** (High) — AuthZ bypass and plugin privilege escalation in upstream Docker client.
3. **Predictable default super admin email** (High) — Default `admin@deploy.monster` account is a credential stuffing target.

---

## Scan Statistics

| Statistic | Value |
|-----------|-------|
| Files Scanned | ~2,500 |
| Lines of Code | ~543,000 |
| Languages Detected | Go (56.5%), TypeScript (8.2%), YAML (4.5%), CSS (1.2%), SQL (0.4%) |
| Frameworks Detected | React 19, Vite 8, Tailwind CSS 4, Go net/http, Docker client |
| Skills Executed | 36 |
| Findings Before Verification | 42 |
| False Positives Eliminated | 11 |
| Final Verified Findings | 26 |

### Finding Distribution

| Vulnerability Category | Critical | High | Medium | Low | Info |
|-----------------------|----------|------|--------|-----|------|
| Authentication | 0 | 1 | 2 | 0 | 0 |
| Authorization | 0 | 0 | 2 | 0 | 0 |
| Session Management | 0 | 0 | 1 | 2 | 0 |
| Cryptography | 0 | 0 | 0 | 1 | 0 |
| Injection | 0 | 0 | 1 | 0 | 0 |
| Code Execution | 0 | 0 | 1 | 0 | 0 |
| SSRF / Open Redirect | 0 | 0 | 2 | 0 | 0 |
| Client-Side | 0 | 0 | 1 | 0 | 0 |
| Data Exposure | 0 | 0 | 2 | 0 | 0 |
| Infrastructure | 0 | 1 | 1 | 3 | 0 |
| Dependencies | 0 | 1 | 0 | 1 | 0 |
| API Security | 0 | 0 | 0 | 1 | 0 |

---

## Critical Findings

No critical findings identified.

---

## High Findings

### VULN-001: Auto-Generated Super Admin Credentials Written to Unencrypted File

**Severity:** High
**Confidence:** 95/100
**CWE:** CWE-798 — Hardcoded Credentials
**OWASP:** A07:2021 – Identification and Authentication Failures

**Location:** `internal/auth/module.go:126-135`

**Description:**
During first-run setup, if `MONSTER_ADMIN_PASSWORD` is not configured via environment variable, DeployMonster auto-generates a 16-character password and writes it to a `.credentials` file next to the database path with `0600` permissions. The file is never automatically deleted or rotated after the admin first logs in, leaving super admin credentials at rest indefinitely.

**Impact:**
An attacker with filesystem read access (via path traversal, backup extraction, container escape, or host compromise) can obtain the super admin password and gain full platform control.

**Remediation:**
1. Print credentials to stdout/stderr during first-run setup instead of writing to disk.
2. If file-based delivery is necessary, delete the `.credentials` file after the admin first logs in.
3. Best option: Require `MONSTER_ADMIN_PASSWORD` environment variable before first run and refuse to start if unset.

**References:**
- https://cwe.mitre.org/data/definitions/798.html
- https://owasp.org/Top10/A07_2021-Identification_and_Authentication_Failures/

---

### VULN-002: Docker Client v28.5.2 Known AuthZ and Plugin Privilege Issues

**Severity:** High
**Confidence:** 90/100
**CWE:** CWE-863 — Incorrect Authorization
**OWASP:** A06:2021 – Vulnerable and Outdated Components

**Location:** `go.mod:9`

**Description:**
The project uses `github.com/docker/docker` v28.5.2, which has known AuthZ bypass and plugin privilege escalation vulnerabilities. The `go.mod` itself contains a security comment acknowledging these issues. DeployMonster does not use AuthZ plugins, reducing practical impact, but the dependency should be upgraded.

**Impact:**
If an attacker compromises the Docker daemon or an AuthZ plugin is enabled, privilege escalation or authorization bypass could occur.

**Remediation:**
Upgrade to `github.com/docker/docker` v29+ when available. Monitor Docker security advisories.

**References:**
- https://cwe.mitre.org/data/definitions/863.html
- Docker security advisories

---

### VULN-003: Predictable Default Super Admin Email

**Severity:** High
**Confidence:** 85/100
**CWE:** CWE-521 — Weak Password Requirements
**OWASP:** A07:2021 – Identification and Authentication Failures

**Location:** `internal/auth/module.go:99`

**Description:**
The default super admin email is `admin@deploy.monster` if `MONSTER_ADMIN_EMAIL` is not set. This predictable account name makes the super admin a target for credential stuffing and brute-force attacks.

**Impact:**
Successful credential stuffing against the predictable super admin account leads to full platform compromise.

**Remediation:**
Force mandatory `MONSTER_ADMIN_EMAIL` environment variable. Refuse to start if unset.

**References:**
- https://cwe.mitre.org/data/definitions/521.html

---

## Medium Findings

### VULN-004: No Multi-Factor Authentication (MFA/TOTP)
- **Severity:** Medium | **Confidence:** 90/100 | **CWE:** CWE-308
- **Location:** `internal/api/handlers/auth.go`
- **Description:** Login accepts only email and password. The `users` table has a `totp_enabled` column but no TOTP implementation.
- **Remediation:** Implement TOTP enrollment, verification, and backup codes.

### VULN-005: Weak Password Policy (8 chars, no special char requirement)
- **Severity:** Medium | **Confidence:** 85/100 | **CWE:** CWE-521
- **Location:** `internal/auth/password.go:27-50`
- **Description:** Password validation requires only 8 chars with one uppercase, one lowercase, and one digit.
- **Remediation:** Increase minimum to 12 chars. Add dictionary check (Have I Been Pwned). Consider zxcvbn.

### VULN-006: No Fine-Grained RBAC on App Mutations Beyond Tenant Scope
- **Severity:** Medium | **Confidence:** 80/100 | **CWE:** CWE-862
- **Location:** `internal/api/router.go` (multiple handlers)
- **Description:** App mutation endpoints check tenant ownership but not user role. A viewer can delete/restart apps.
- **Remediation:** Add `requirePermission(perm string)` middleware for destructive endpoints.

### VULN-007: Team Invite Creation Lacks Role Escalation Validation
- **Severity:** Medium | **Confidence:** 75/100 | **CWE:** CWE-269
- **Location:** `internal/api/handlers/invites.go:30-99`
- **Description:** Invite handler does not validate that inviter's role is higher than invited role.
- **Remediation:** Enforce role hierarchy check in invite creation.

### VULN-008: SameSite=None on Authentication Cookies
- **Severity:** Medium | **Confidence:** 85/100 | **CWE:** CWE-1275
- **Location:** `internal/api/handlers/auth.go:49-76`
- **Description:** Cookies use `SameSite=None` when HTTPS is enabled, increasing CSRF surface area.
- **Remediation:** Default to `SameSite=Lax` unless cross-site API is explicitly required.

### VULN-009: Command Blocklist Bypass Potential in Container Exec
- **Severity:** Medium | **Confidence:** 75/100 | **CWE:** CWE-78
- **Location:** `internal/api/handlers/exec.go:18-31`, `internal/api/ws/terminal.go:36-40`
- **Description:** `isCommandSafe` uses a substring blocklist that can be bypassed with creative variants (`/bin/rm -rf /`, `wget -O- | bash`).
- **Remediation:** Replace blocklist with an allowlist of safe commands or use a restricted shell.

### VULN-010: Build Pipeline Executes User-Controlled Git Clone and Docker Build
- **Severity:** Medium | **Confidence:** 80/100 | **CWE:** CWE-78
- **Location:** `internal/build/builder.go:59-156`
- **Description:** Build pipeline runs `git clone` and `docker build` with user-controlled inputs. A URL validation bypass would lead to RCE.
- **Remediation:** Run builds in isolated environments (gVisor/Firecracker). Drop Docker capabilities.

### VULN-011: Git Clone URL Validation Bypass Risk (SSRF)
- **Severity:** Medium | **Confidence:** 70/100 | **CWE:** CWE-918
- **Location:** `internal/build/builder.go:206-258`
- **Description:** `ValidateGitURL` allows `git://` and `ssh://` schemes. `git://` is unencrypted and can redirect to local protocols.
- **Remediation:** Disable `git://` scheme. Restrict local paths to whitelist in production.

### VULN-012: Outbound Webhook Deliveries May Reach Internal Networks
- **Severity:** Medium | **Confidence:** 65/100 | **CWE:** CWE-918
- **Location:** `internal/webhooks/` (sender/delivery)
- **Description:** Outbound webhook URLs are user-configured. If private IPs are not blocked, webhooks could attack internal services.
- **Remediation:** Validate webhook URLs against private/blocked IP ranges before delivery.

### VULN-013: No X-Frame-Options or CSP frame-ancestors
- **Severity:** Medium | **Confidence:** 75/100 | **CWE:** CWE-1021
- **Location:** `internal/api/router.go:76-94`
- **Description:** Security headers middleware does not include clickjacking protection.
- **Remediation:** Add `X-Frame-Options: DENY` and `CSP: frame-ancestors 'self'`.

### VULN-014: File Browser Endpoint May Allow Path Traversal
- **Severity:** Medium | **Confidence:** 65/100 | **CWE:** CWE-22
- **Location:** `internal/api/router.go:366`
- **Description:** `GET /api/v1/apps/{id}/files` may allow path traversal if path validation is insufficient.
- **Remediation:** Validate paths against container root. Reject `..` and absolute paths.

### VULN-015: PATCH Handlers May Allow Mass Assignment
- **Severity:** Medium | **Confidence:** 60/100 | **CWE:** CWE-915
- **Location:** `internal/api/router.go:140`
- **Description:** PATCH endpoints may update unintended fields without explicit whitelisting.
- **Remediation:** Implement field whitelisting for all PATCH/PUT handlers.

### VULN-016: Certificate Upload Without File Type Validation
- **Severity:** Medium | **Confidence:** 65/100 | **CWE:** CWE-434
- **Location:** `internal/api/router.go:431-432`
- **Description:** Certificate upload may accept non-certificate files.
- **Remediation:** Validate MIME type, extension, and parse PEM/DER before storage.

### VULN-017: App Import Accepts Arbitrary Archives
- **Severity:** Medium | **Confidence:** 60/100 | **CWE:** CWE-22
- **Location:** `internal/api/router.go:165`
- **Description:** App import may accept archives. Zip slip or path traversal could exist in extraction.
- **Remediation:** Validate extracted paths. Reject `..`. Whitelist file types.

---

## Low Findings

| ID | Title | CWE | Location | Remediation |
|----|-------|-----|----------|-------------|
| VULN-018 | Missing Audience and Issuer in JWT Claims | CWE-345 | `internal/auth/jwt.go` | Add `aud` and `iss` claims |
| VULN-019 | Key Rotation Grace Period Could Be Exploited | CWE-347 | `internal/auth/jwt.go` | Add emergency key revocation API |
| VULN-020 | No Absolute Session Timeout for Refresh Tokens | CWE-613 | `internal/auth/jwt.go` | Implement 30-day absolute timeout |
| VULN-021 | TLS 1.2 Minimum — TLS 1.3 Not Enforced | CWE-326 | `internal/ingress/module.go` | Upgrade to `tls.VersionTLS13` |
| VULN-022 | Generous Login/Register Rate Limits | CWE-307 | `internal/api/router.go` | Add per-account rate limiting |
| VULN-023 | GitHub Actions Permissions Not Restricted | CWE-250 | `.github/workflows/` | Add explicit `permissions:` blocks |
| VULN-024 | Dockerfiles Generated Without USER Directive | CWE-250 | `internal/build/dockerfiles.go` | Add non-root `USER` to templates |
| VULN-025 | Panic on Short JWT Secret at Startup | CWE-391 | `internal/auth/jwt.go:54` | Return error instead of panic |
| VULN-026 | Unmaintained dagre Dependency | CWE-1104 | `web/package.json:22` | Migrate to `@dagrejs/dagre` |

---

## Remediation Roadmap

### Phase 1: Immediate (1-3 days)
Address all High findings.

| # | Finding | Effort | Impact |
|---|---------|--------|--------|
| 1 | VULN-001: Auto-generated credentials written to disk | Low | High |
| 2 | VULN-003: Predictable default admin email | Low | High |
| 3 | VULN-002: Docker client v28.5.2 vulnerabilities | Medium | High |

### Phase 2: Short-Term (1-2 weeks)
Address Medium findings and quick-win Low findings.

| # | Finding | Effort | Impact |
|---|---------|--------|--------|
| 4 | VULN-013: Add X-Frame-Options and CSP | Low | Medium |
| 5 | VULN-008: Change cookie SameSite to Lax | Low | Medium |
| 6 | VULN-004: Implement MFA/TOTP | High | Medium |
| 7 | VULN-005: Strengthen password policy | Low | Medium |
| 8 | VULN-006: Add RBAC permission checks | Medium | Medium |
| 9 | VULN-009: Harden container exec blocklist | Medium | Medium |
| 10 | VULN-021: Enforce TLS 1.3 | Low | Low |
| 11 | VULN-025: Return error instead of panic | Low | Low |

### Phase 3: Medium-Term (1-2 months)
Address remaining Medium findings and infrastructure hardening.

| # | Finding | Effort | Impact |
|---|---------|--------|--------|
| 12 | VULN-010: Isolate build pipeline | High | Medium |
| 13 | VULN-011: Harden git URL validation | Low | Medium |
| 14 | VULN-012: Validate webhook URLs | Low | Medium |
| 15 | VULN-007: Role hierarchy for invites | Low | Medium |
| 16 | VULN-014: Harden file browser | Medium | Medium |
| 17 | VULN-015: Field whitelisting for PATCH | Medium | Medium |
| 18 | VULN-016: Validate certificate uploads | Low | Medium |
| 19 | VULN-017: Harden app import | Medium | Medium |
| 20 | VULN-022: Per-account rate limiting | Medium | Low |
| 21 | VULN-024: Non-root USER in Dockerfiles | Low | Low |

### Phase 4: Hardening (Ongoing)

| # | Recommendation | Effort | Impact |
|---|---------------|--------|--------|
| 22 | VULN-018: Add JWT aud/iss claims | Low | Low |
| 23 | VULN-019: Emergency key revocation API | Low | Low |
| 24 | VULN-020: Absolute session timeout | Low | Low |
| 25 | VULN-023: Pin GitHub Action SHAs | Low | Low |
| 26 | VULN-026: Replace unmaintained dagre | Medium | Low |

---

## Methodology

This assessment was performed using security-check, an AI-powered static analysis suite that uses large language model reasoning to detect security vulnerabilities.

### Pipeline Phases
1. **Reconnaissance** — Automated codebase architecture mapping and technology detection
2. **Vulnerability Hunting** — 36 specialized skills scanned for 10 vulnerability categories
3. **Verification** — False positive elimination with confidence scoring (0-100)
4. **Reporting** — CVSS-aligned severity classification and remediation prioritization

### Limitations
- Static analysis only — no runtime testing or dynamic analysis performed
- AI-based reasoning may miss vulnerabilities requiring deep domain knowledge
- Confidence scores are estimates, not guarantees
- Custom business logic flaws may require manual review

---

## Disclaimer

This security assessment was performed using automated AI-powered static analysis. It does not constitute a comprehensive penetration test or security audit. The findings represent potential vulnerabilities identified through code pattern analysis and LLM reasoning. False positives and false negatives are possible.

This report should be used as a starting point for security remediation, not as a definitive statement of the application's security posture. A professional security audit by qualified security engineers is recommended for production applications handling sensitive data.

Generated by security-check — github.com/ersinkoc/security-check
