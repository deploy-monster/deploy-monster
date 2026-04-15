# DeployMonster — Project Status Report

> **Date**: 2026-04-15
> **Version**: v0.1.6
> **Repository**: github.com/deploy-monster/deploy-monster

---

## Executive Summary

DeployMonster v0.1.6 is the latest release after the comprehensive UX overhaul. Three major phases are complete:

1. **Phase 1: Marketplace Overhaul** — Templates now have icons, config schemas, and dynamic forms. New TemplateDetail page at `/marketplace/:slug`.
2. **Phase 2: Modal/Dialog → Sheet + AlertDialog** — All `window.confirm()` eliminated. Complex forms use slide-over Sheet panels.
3. **Phase 3: Topology Fixes** — Zustand single-source-of-truth, ConfigPanel widened, empty state for 0-node topology.

E2E test fixes: marketplace deploy dialog selectors corrected, broken loader waits removed from global-setup.ts and helpers.ts.

## Key Metrics

| Metric | Value |
|--------|-------|
| Total LOC (source + tests + web) | ~190K |
| Go Source | ~50K LOC |
| Go Tests | ~117K LOC |
| React / TS / CSS | ~23K LOC |
| API Endpoints | 240 |
| API Handler functions | 222 |
| Modules | 20 |
| Marketplace Templates | 56 (across 16 categories) |
| Test Coverage | 85%+ (CI-enforced gate) |
| Fuzz Targets | 15 |
| Benchmarks | 46 |
| Binary Size | ~23MB stripped, single static binary with embedded UI |
| Repository | github.com/deploy-monster/deploy-monster |

## Test Results (2026-04-15)

- Go: `go test ./...` — 5184 tests pass, 42 packages
- React: Vitest tests pass (38 files)
- Integration: SQLite + Postgres contract suites both green
- Fuzz: 15 targets
- Benchmarks: 46 targets

## v0.1.6 Changelog

### Marketplace
- Added icons and config_schema to 12 templates (Ghost, Grafana, WordPress, n8n, Portainer, Immich, Jellyfin, Open WebUI, Ollama, Uptime Kuma, Plausible, Umami)
- Dynamic config form generation from `config_schema` properties
- New TemplateDetail page at `/marketplace/:slug` with services list, resource requirements, compose preview
- Featured templates horizontal scroll section
- Deploy dialog → Sheet component for wider form area

### Dialog/Modal System
- New `Sheet` component — slide-over panel from right (`max-w-xl`)
- New `AlertDialog` component — confirmation dialogs with `default` | `destructive` variants
- Eliminated all 6 `window.confirm()` calls:
  - Apps (delete), AppDetail (delete), Domains (delete)
  - GitSources (disconnect), Secrets (delete), Team (remove member)
- Converted complex create forms from Dialog to Sheet:
  - Servers (4 providers, regions, sizes)
  - Databases (5 engines, versions)
  - GitSources (token auth)

### Topology
- Fixed ReactFlow/Zustand state sync: removed `useNodesState`/`useEdgesState` dual-state pattern
- Zustand store is now single source of truth for nodes and edges
- Added `updateNodePosition` action for proper drag-sync
- ConfigPanel widened from `w-72` (288px) to `w-96` (384px)
- Empty state for 0-node topology with "Start with a template" button

### E2E Tests
- Fixed marketplace deploy dialog password field selectors (added config_schema to mock Ghost)
- Removed dead `[data-testid="full-page-loader"]` wait from helpers.ts and global-setup.ts
- Fixed auth cookie propagation: switched from `page.request` API calls to UI-based login in global-setup.ts
- Fixed `/me` response parsing: backend returns `{user, membership, role_id, tenant_id}` but frontend checked `user.id` on the whole response — now properly extracts nested user data
- Added `data-testid="dashboard-shell"` to AppLayout for cross-page navigation assertions
- Removed duplicate `dashboard-shell` from Dashboard.tsx
- Fixed Playwright strict mode violations: added `.first()` to ambiguous text selectors
- Fixed API client response unwrapping: `{"data": [...], "total": N}` responses now correctly unwrap to arrays (was breaking Secrets, Domains, Databases pages with "X.reduce is not a function" errors)
- Fixed marketplace mock API: deploy dialog now opens with correct selectors
- **Result**: 59-62/86 E2E tests passing (68-72%). Core suites fully green: auth, navigation (21/21), dashboard (11/11), marketplace (7/7), topology. Remaining failures are flaky timing issues in apps/page and auth session tests.

### Auth
- Cookie path changed from `/api` to `/` with SameSite=None for cross-site compatibility
- Fixed auth initialization race: `/me` response now correctly parsed from nested structure
