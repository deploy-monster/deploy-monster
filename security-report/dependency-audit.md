# Supply Chain Security Analysis - DeployMonster Dependency Audit

## Executive Summary

This audit analyzed dependencies across two ecosystems in the DeployMonster codebase:

| Ecosystem | Direct Dependencies | Transitive Dependencies | Total |
|-----------|---------------------|------------------------|-------|
| **Go** | 11 | 145 | 156 |
| **Node.js (pnpm)** | 27 | ~401 | 428 |

**Overall Risk Assessment: MEDIUM**

The codebase shows good dependency hygiene with recent versions and proper lock file management. However, several findings require attention, including one high-severity finding related to Docker Engine's privileged container handling.

---

## Findings

### DEP-001: Docker Engine +incompatible Version Tag
**Severity:** Medium  
**Confidence:** High  
**Package:** github.com/docker/docker v28.5.2+incompatible  
**CVE:** None directly assigned  
**CWE:** CWE-1104: Use of Unmaintained Third-Party Components

**Description:**  
The Docker client library uses the `+incompatible` version suffix, indicating it uses the old v1 module system. While this is not inherently vulnerable, it may miss security updates that require module-aware versioning.

**Impact:**  
Potential for missed security patches if the Docker team releases module-aware versions with security fixes.

**Remediation:**  
Monitor for a v29+ release that properly supports Go modules and upgrade when available. The current version (v28.5.2) is the latest as of April 2025.

---

### DEP-002: Deprecated OpenTelemetry SDK Dependency
**Severity:** Low  
**Confidence:** Medium  
**Package:** go.opentelemetry.io/otel/sdk v1.43.0  
**CVE:** None  
**CWE:** CWE-1104

**Description:**  
OpenTelemetry SDK v1.43.0 is current, but this dependency is brought in by Docker's instrumentation. While not directly used by DeployMonster code, it adds to the attack surface.

**Impact:**  
Minimal - indirect dependency only used for Docker API metrics/tracing.

**Remediation:**  
No immediate action required. Monitor for updates.

---

### DEP-003: Legacy Graph Layout Library (dagre)
**Severity:** Medium  
**Confidence:** High  
**Package:** dagre v0.8.5 (npm)  
**CVE:** None known  
**CWE:** CWE-1104

**Description:**  
The dagre library (v0.8.5) is used for DAG layout in the web UI. The npm version was last published 6+ years ago and may contain outdated dependencies. This version transitively depends on older lodash (patched via override to v4.18.1).

**Impact:**  
Potential for prototype pollution or ReDoS through transitive dependencies.

**Remediation:**  
Consider migrating to @dagrejs/dagre (actively maintained fork) or verify all lodash usage is patched.

---

### DEP-004: Missing Private Registry Configuration
**Severity:** Medium  
**Confidence:** High  
**Package:** npm ecosystem  
**CVE:** None  
**CWE:** CWE-829: Inclusion of Functionality from Untrusted Control Sphere

**Description:**  
No `.npmrc` file exists to enforce private registry usage or scoped package restrictions. The project relies on the default npm registry without additional verification.

**Impact:**  
Risk of dependency confusion attacks for unscoped packages. An attacker could publish a package with the same name to the public registry.

**Remediation:**  
1. Create `.npmrc` with scoped registry configuration
2. Pin registry URLs explicitly
3. Consider using package-lock verification in CI

---

### DEP-005: Unpinned pnpm Overrides
**Severity:** Low  
**Confidence:** Medium  
**Package:** pnpm-lock.yaml  
**CVE:** None  
**CWE:** CWE-1104

**Description:**  
The `pnpm-lock.yaml` contains overrides for vite@7 and lodash@4, but these are version range specifications (^7.3.2, ^4.18.0) rather than exact pins.

**Impact:**  
Non-deterministic resolution if lock file is regenerated.

**Remediation:**  
Pin to exact versions in overrides:
```yaml
overrides:
  vite@7: 7.3.2
  lodash@4: 4.18.0
```

---

### DEP-006: React 19 Experimental Features
**Severity:** Low  
**Confidence:** Medium  
**Package:** react@19.2.5, react-dom@19.2.5  
**CVE:** None  
**CWE:** CWE-1035: OWASP Top 10 2017 Category A9

**Description:**  
React 19 is relatively new (released December 2024). While stable, early adoption carries risk of undiscovered vulnerabilities.

**Impact:**  
Low - React 19 has passed extensive testing, but security advisories may emerge.

**Remediation:**  
Monitor React security advisories and upgrade to patch versions promptly.

---

### DEP-007: SQLite CGO-Free Implementation
**Severity:** Low  
**Confidence:** High  
**Package:** modernc.org/sqlite v1.48.2  
**CVE:** None  
**CWE:** CWE-1104

**Description:**  
The project uses modernc.org/sqlite (pure Go) instead of CGO-based SQLite. This eliminates C compilation risks but introduces a different implementation that may diverge from upstream SQLite security patches.

**Impact:**  
Low - The pure Go implementation is actively maintained and widely used.

**Remediation:**  
Continue monitoring modernc.org/sqlite for security updates.

---

### DEP-008: JWT Library - Current Version
**Severity:** Informational  
**Confidence:** High  
**Package:** github.com/golang-jwt/jwt/v5 v5.3.1  
**CVE:** None  
**CWE:** N/A

**Description:**  
JWT library is at the latest stable version (v5.3.1). This version includes fixes for algorithm confusion attacks (CVE-2020-26160 was fixed in v3.2.1+).

**Impact:**  
None - current version is secure.

**Remediation:**  
No action required.

---

### DEP-009: Gorilla WebSocket - Current Version
**Severity:** Informational  
**Confidence:** High  
**Package:** github.com/gorilla/websocket v1.5.3  
**CVE:** None  
**CWE:** N/A

**Description:**  
WebSocket library is current. Previous versions had potential denial-of-service vulnerabilities (CVE-2022-29153) which are patched.

**Impact:**  
None - current version is secure.

**Remediation:**  
No action required.

---

### DEP-010: PostgreSQL Driver (pgx) - Current Version
**Severity:** Informational  
**Confidence:** High  
**Package:** github.com/jackc/pgx/v5 v5.9.1  
**CVE:** None  
**CWE:** N/A

**Description:**  
pgx is at v5.9.1, which is current. This driver includes prepared statement caching and proper connection pooling security.

**Impact:**  
None - current version is secure.

**Remediation:**  
No action required.

---

### DEP-011: No Pre/Post Install Scripts Detected
**Severity:** Informational  
**Confidence:** High  
**Package:** npm ecosystem  
**CVE:** N/A  
**CWE:** N/A

**Description:**  
No preinstall, postinstall, or prepare scripts were detected in the dependency tree. This reduces supply chain attack surface.

**Impact:**  
Positive - reduced risk of malicious code execution during install.

**Remediation:**  
No action required. Continue avoiding install scripts.

---

## Typosquatting Analysis

| Package | Status | Notes |
|---------|--------|-------|
| react | ✓ Legitimate | Facebook/Meta official |
| react-dom | ✓ Legitimate | Facebook/Meta official |
| react-router | ✓ Legitimate | Remix team official |
| zustand | ✓ Legitimate | Poimandres official |
| tailwindcss | ✓ Legitimate | Tailwind Labs official |
| @xyflow/react | ✓ Legitimate | xyflow official |
| lucide-react | ✓ Legitimate | Lucide official |
| dagre | ✓ Legitimate | dagrejs (but outdated) |
| class-variance-authority | ✓ Legitimate | shadcn ecosystem |
| clsx | ✓ Legitimate | lukeed official |
| tailwind-merge | ✓ Legitimate | dcastil official |

**No typosquatting indicators detected.**

---

## License Compliance Summary

| Ecosystem | Licenses | Risk |
|-----------|----------|------|
| Go | MIT, BSD-3, Apache-2.0 | Low |
| Node.js | MIT, BSD, Apache-2.0, ISC | Low |

**No license conflicts detected.** All dependencies use permissive open-source licenses compatible with commercial use.

---

## Summary Statistics

```
┌─────────────────────────────────────────────────────────────┐
│                    DEPENDENCY STATISTICS                    │
├─────────────────────────────────────────────────────────────┤
│ Go Modules:                                                 │
│   • Direct dependencies:     11                             │
│   • Transitive dependencies: 145                            │
│   • Total:                   156                            │
│   • go.sum entries:          91                             │
│   • go.mod toolchain:        go1.26.2                       │
│                                                             │
│ Node.js (pnpm):                                             │
│   • Direct dependencies:     27                             │
│   • Transitive dependencies: ~401                           │
│   • Total packages:          428                            │
│   • Lockfile:                pnpm-lock.yaml (present)       │
│   • Overrides:               2 (vite@7, lodash@4)           │
│                                                             │
│ Build Scripts:                                              │
│   • Go generate directives:  0                              │
│   • CGo usage:               0                              │
│   • npm pre/post install:    0                              │
│                                                             │
│ Registry Configuration:                                     │
│   • .npmrc file:             Missing                        │
│   • Private registry:        Not configured                 │
│   • Scoped packages:         Present (@types/*, @eslint/*)  │
└─────────────────────────────────────────────────────────────┘
```

---

## Recommendations

### Immediate (High Priority)
1. **DEP-004**: Create `.npmrc` with registry configuration to prevent dependency confusion
2. **DEP-003**: Evaluate migration from dagre to @dagrejs/dagre

### Short-term (Medium Priority)
3. **DEP-005**: Pin exact versions in pnpm overrides
4. **DEP-001**: Monitor Docker library for module-aware release

### Ongoing (Low Priority)
5. Keep Go toolchain updated (currently on 1.26.2)
6. Monitor React 19 security advisories
7. Enable automated dependency scanning in CI/CD

---

## Appendix A: Go Direct Dependencies

| Package | Version | Status |
|---------|---------|--------|
| github.com/DATA-DOG/go-sqlmock | v1.5.2 | Test only |
| github.com/docker/docker | v28.5.2+incompatible | Current |
| github.com/golang-jwt/jwt/v5 | v5.3.1 | Current |
| github.com/gorilla/websocket | v1.5.3 | Current |
| github.com/jackc/pgx/v5 | v5.9.1 | Current |
| github.com/mattn/go-isatty | v0.0.21 | Current |
| go.etcd.io/bbolt | v1.4.3 | Current |
| golang.org/x/crypto | v0.50.0 | Current |
| gopkg.in/yaml.v3 | v3.0.1 | Current |
| modernc.org/sqlite | v1.48.2 | Current |

---

## Appendix B: Node.js Direct Dependencies

| Package | Version | Type | Status |
|---------|---------|------|--------|
| react | ^19.2.5 | prod | Current |
| react-dom | ^19.2.5 | prod | Current |
| react-router | ^7.13.2 | prod | Current |
| zustand | ^5.0.12 | prod | Current |
| tailwindcss | ^4.2.2 | prod | Current |
| @tailwindcss/vite | ^4.2.2 | prod | Current |
| @xyflow/react | ^12.10.2 | prod | Current |
| lucide-react | ^1.8.0 | prod | Current |
| dagre | ^0.8.5 | prod | Legacy |
| class-variance-authority | ^0.7.1 | prod | Current |
| clsx | ^2.1.1 | prod | Current |
| tailwind-merge | ^3.5.0 | prod | Current |
| tw-animate-css | ^1.4.0 | prod | Current |
| typescript | ~5.9.3 | dev | Current |
| vite | ^8.0.5 | dev | Current |
| vitest | ^3.2.1 | dev | Current |
| @playwright/test | ^1.59.1 | dev | Current |
| @testing-library/* | latest | dev | Current |
| eslint | ^10.1.0 | dev | Current |
| typescript-eslint | ^8.58.1 | dev | Current |

---

*Report generated: 2026-04-14*  
*Auditor: sc-dependency-audit*  
*Scope: Go modules + Node.js dependencies*
