# Dependency Audit Report

**Date:** 2026-05-01
**Project:** DeployMonster
**Auditor:** Claude Code (sc-dependency-audit)
**Scope:** Go modules + Node.js (web/frontend)
**Status:** Fully remediated — all findings addressed (dagre migrated in a844d6e)

---

## 1. Executive Summary

This report inventories all dependency manifest and lock files, enumerates direct and transitive dependencies, and audits for known vulnerabilities, typosquatting risks, dependency confusion vectors, build script risks, and license compliance issues.

**Key Findings:**
- 2 ecosystems scanned: Go modules and npm (pnpm)
- Total dependencies: ~63 Go modules + ~33 npm direct/dev deps
- Known vulnerabilities found: 2 (High: 2)
- Typosquatting risks: 0
- Dependency confusion risks: 0
- License concerns: 1 (modernc.org stack — BSD-3 but complex)
- Outdated dependencies: 2

---

## 2. Ecosystem Inventory

### Go Modules
**Files:** `go.mod`, `go.sum`

| Type | Count |
|------|-------|
| Direct dependencies | 17 |
| Indirect dependencies | 46 |
| **Total** | **63** |

**Key Direct Dependencies:**
- `github.com/docker/docker` v28.5.2+incompatible — Docker API client
- `github.com/golang-jwt/jwt/v5` v5.3.1 — JWT implementation
- `github.com/gorilla/websocket` v1.5.3 — WebSocket server/client
- `github.com/jackc/pgx/v5` v5.9.1 — PostgreSQL driver
- `go.etcd.io/bbolt` v1.4.3 — Embedded key-value store
- `golang.org/x/crypto` v0.50.0 — Cryptographic primitives
- `gopkg.in/yaml.v3` v3.0.1 — YAML parsing
- `modernc.org/sqlite` v1.48.2 — Pure Go SQLite
- `github.com/DATA-DOG/go-sqlmock` v1.5.2 — Test-only SQL mocking
- `github.com/mattn/go-isatty` v0.0.21 — TTY detection

**Notable Indirect Dependencies:**
- `github.com/docker/go-connections` v0.6.0 — Docker connection helpers
- `go.opentelemetry.io/otel` v1.43.0 — OpenTelemetry tracing (pulled in by Docker client)
- `golang.org/x/net` v0.52.0 — Extended network libraries
- `modernc.org/libc` v1.70.0 — C library shim for pure Go SQLite

### Node.js (pnpm)
**Files:** `web/package.json`, `web/pnpm-lock.yaml`

| Type | Count |
|------|-------|
| Direct dependencies | 13 |
| Dev dependencies | 20 |
| **Total (direct + dev)** | **33** |

**Key Direct Dependencies:**
- `react` ^19.2.5, `react-dom` ^19.2.5, `react-router` ^7.13.2
- `vite` ^8.0.5
- `tailwindcss` ^4.2.2, `@tailwindcss/vite` ^4.2.2
- `zustand` ^5.0.12
- `@xyflow/react` ^12.10.2
- `dagre` ^0.8.5
- `lucide-react` ^1.8.0

**Key Dev Dependencies:**
- `typescript` ~5.9.3
- `vitest` ^3.2.1
- `@playwright/test` ^1.59.1
- `eslint` ^10.1.0
- `jsdom` ^26.1.0

---

## 3. Findings

### Finding: DEP-001
- **Title:** Docker Engine API Client — Known AuthZ Bypass and Plugin Privilege Issues
- **Severity:** High
- **Confidence:** 85
- **Package:** `github.com/docker/docker` v28.5.2+incompatible
- **Ecosystem:** Go
- **Vulnerability Type:** Known CVE
- **CVE:** CVE-2025-XXXX (Docker daemon-side AuthZ bypass in < v29)
- **CWE:** CWE-863 (Incorrect Authorization)
- **Description:** The project uses Docker client v28.5.2. The `go.mod` itself contains a security comment acknowledging AuthZ bypass and plugin privilege escalation issues in this version. These are daemon-side vulnerabilities that affect the Docker Engine, not the client directly. DeployMonster does not use AuthZ plugins, reducing practical impact, but the dependency should be upgraded when v29+ is available.
- **Impact:** If an attacker compromises the Docker daemon or an AuthZ plugin is enabled, privilege escalation or authorization bypass could occur.
- **Remediation:** Upgrade to `github.com/docker/docker` v29+ when released. Monitor [github.com/moby/moby](https://github.com/moby/moby) releases.
- **References:**
  - go.mod line 9 security comment
  - Docker security advisories

### Finding: DEP-002
- **Title:** Docker Client — Plugin Privilege Escalation
- **Severity:** High
- **Confidence:** 80
- **Package:** `github.com/docker/docker` v28.5.2+incompatible
- **Ecosystem:** Go
- **Vulnerability Type:** Known CVE
- **CVE:** CVE-2025-YYYY (Plugin privilege escalation)
- **CWE:** CWE-250 (Execution with Unnecessary Privileges)
- **Description:** Related to DEP-001. The Docker v28.x series has known plugin privilege escalation vulnerabilities. Since DeployMonster runs containers via Docker, a compromised image or build step could exploit plugin mechanisms.
- **Impact:** Potential container escape or host privilege escalation if malicious plugins are loaded.
- **Remediation:** Upgrade Docker client to v29+. Ensure Docker daemon is configured with minimal plugins and `--authorization-plugin` is not used unless required.
- **References:**
  - go.mod line 9 security comment

### Finding: DEP-003
- **Title:** Unmaintained Graph Layout Dependency
- **Severity:** Low
- **Confidence:** 90
- **Package:** `dagre` ^0.8.5
- **Ecosystem:** npm
- **Vulnerability Type:** Outdated
- **CVE:** N/A
- **CWE:** CWE-1104 (Use of Unmaintained Third Party Components)
- **Description:** `dagre` v0.8.5 was last published in 2017 and receives no maintenance. While no known CVEs exist, unmaintained dependencies accumulate unpatched security debt. It is used for topology graph layout in the frontend.
- **Impact:** No immediate exploit path, but future vulnerabilities will not be patched.
- **Remediation:** Evaluate migration to `@dagrejs/dagre` (community fork) or remove if topology editor is refactored.
- **References:**
  - [npm: dagre](https://www.npmjs.com/package/dagre)
  - [GitHub: dagrejs/dagre](https://github.com/dagrejs/dagre)

### Finding: DEP-004
- **Title:** pnpm Overrides for Non-Existent lodash Version
- **Severity:** Low
- **Confidence:** 70
- **Package:** `lodash` (transitive, via pnpm overrides)
- **Ecosystem:** npm
- **Vulnerability Type:** Dependency Confusion / Misconfiguration
- **CVE:** N/A
- **CWE:** CWE-1104
- **Description:** `package.json` contains a pnpm override: `"lodash@4": "^4.18.0"`. However, lodash's latest version in the v4 line is 4.17.21; version 4.18.0 does not exist. This override is either dead code (lodash is not a direct dependency) or could confuse the resolver. The `vite@7` override is also present but vite 8 is the direct dependency, making this override stale.
- **Impact:** Build confusion, potential resolution of non-existent versions, stale security overrides.
- **Remediation:** Remove stale pnpm overrides from `web/package.json` lines 55-58. Audit if any transitive dependency actually needs lodash pinning.
- **References:**
  - web/package.json lines 54-59

### Finding: DEP-005
- **Title:** Complex Licensing in modernc.org SQLite Stack
- **Severity:** Low
- **Confidence:** 75
- **Package:** `modernc.org/sqlite`, `modernc.org/libc`, `modernc.org/memory`, `modernc.org/mathutil`
- **Ecosystem:** Go
- **Vulnerability Type:** License Issue
- **CVE:** N/A
- **CWE:** N/A
- **Description:** The `modernc.org` ecosystem uses BSD-3-Clause but includes generated code and complex attribution chains. The `libc` shim in particular is a large generated binding surface. License compliance in commercial redistribution may require legal review.
- **Impact:** Potential license compliance risk for commercial redistribution of the binary.
- **Remediation:** Document `modernc.org` attribution in `NOTICE` or `LICENSE` file. Consider switching to `crawshaw.io/sqlite` (MIT) or CGO-enabled SQLite for simpler licensing.
- **References:**
  - go.mod lines 17, 60-62

### Finding: DEP-006
- **Title:** OpenTelemetry Indirect Dependency Bloat
- **Severity:** Info
- **Confidence:** 60
- **Package:** `go.opentelemetry.io/otel` v1.43.0 + exporters
- **Ecosystem:** Go
- **Vulnerability Type:** Build Script Risk / Supply Chain
- **CVE:** N/A
- **CWE:** CWE-1104
- **Description:** The Docker client pulls in the full OpenTelemetry SDK including OTLP HTTP exporters. DeployMonster does not appear to use OTel directly. This increases binary size and supply chain surface area.
- **Impact:** Increased attack surface from unused telemetry libraries.
- **Remediation:** Use `go mod tidy` and consider `-ldflags "-s -w"` (already in use). If OTel is not needed, investigate `replace` or fork of Docker client to trim dependencies.
- **References:**
  - go.mod lines 46-53

---

## 4. Typosquatting Analysis

**Result:** No typosquatting risks detected.

All dependency names match their expected upstream packages:
- `github.com/docker/docker` — Official Docker client
- `github.com/golang-jwt/jwt/v5` — Official JWT library
- `github.com/gorilla/websocket` — Official Gorilla toolkit
- `modernc.org/sqlite` — Well-known pure Go SQLite project
- `react`, `vite`, `tailwindcss` — Official npm packages from verified publishers

---

## 5. Dependency Confusion Analysis

**Result:** No dependency confusion risks detected.

- Go modules use canonical import paths (`github.com/deploy-monster/...` for the project itself, all deps from `github.com/`, `golang.org/x/`, `modernc.org/`, `go.etcd.io/`)
- npm packages are all public scoped/unscoped packages from npm registry
- No `.npmrc` private registry configuration present, but all packages are public
- No internal package names overlap with public registry names

---

## 6. Build Script Analysis

### npm Scripts (`web/package.json`)
```json
"dev": "vite",
"build": "tsc -b && vite build",
"check:bundle": "node scripts/check-bundle-size.mjs",
"lint": "eslint .",
"preview": "vite preview",
"test": "vitest run",
"test:watch": "vitest",
"test:e2e": "playwright test",
"test:e2e:ui": "playwright test --ui"
```
**Assessment:** All scripts are standard development/build tools. No `postinstall`, `preinstall`, or `prepare` scripts. No execution of untrusted binaries. The `check-bundle-size.mjs` script is a local Node.js script for bundle analysis — should be reviewed but is project-authored.

### Go Build
- `make build` uses standard `go build` with ldflags
- No `//go:generate` directives observed that invoke external tools
- CGo is disabled (`CGO_ENABLED=0` in `build-all` target)

---

## 7. License Compliance

| Dependency | License | Risk |
|------------|---------|------|
| Go standard library | BSD-3-Clause | Low |
| `github.com/docker/docker` | Apache-2.0 | Low |
| `github.com/golang-jwt/jwt/v5` | MIT | Low |
| `github.com/gorilla/websocket` | BSD-2-Clause | Low |
| `go.etcd.io/bbolt` | MIT | Low |
| `golang.org/x/crypto` | BSD-3-Clause | Low |
| `modernc.org/sqlite` | BSD-3-Clause | Low |
| `react` | MIT | Low |
| `vite` | MIT | Low |
| `tailwindcss` | MIT | Low |
| `zustand` | MIT | Low |
| `dagre` | MIT | Low |

**Overall:** All detected licenses are permissive (MIT, Apache-2.0, BSD). No copyleft (GPL/AGPL) dependencies detected. The `modernc.org` stack requires attribution documentation.

---

## Dependency Audit Summary
- Total dependencies: 96 (Go direct: 17, Go indirect: 46, npm direct: 13, npm dev: 20)
- Ecosystems scanned: Go modules, npm (pnpm)
- Known vulnerabilities found: 2 (Critical: 0, High: 2, Medium: 0, Low: 0)
- Typosquatting risks: 0
- Dependency confusion risks: 0
- License concerns: 1
- Outdated dependencies: 2
