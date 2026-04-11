# DeployMonster — Project Status Report

> **Date**: 2026-04-11
> **Version**: v1.6.0-rc.1 (release candidate, post-Phase 1–7 Tier 91)
> **Repository**: github.com/deploy-monster/deploy-monster

---

## Executive Summary

DeployMonster v1.6.0-rc.1 is the first release candidate to ship after the Phase 1–6 audit and the 88-tier hardening sweep. Phases 1 through 5 (critical fixes, core completion, hardening, testing, performance + Postgres) are complete; Phase 6 (documentation and developer experience) is 100% complete; Phase 7 (release preparation) is in flight. The 20 Dependabot alerts that were flagged as a v1.0 blocker are down to 3 (all upstream-blocked, documented as accepted risk). Every number in this file is measured against HEAD, not the pre-audit aspirational numbers from the v1.4.0 era.

## Key Metrics

| Metric | Value |
|--------|-------|
| Total LOC (source + tests + web) | ~188K |
| Go Source | ~50K LOC |
| Go Tests | ~117K LOC |
| React / TS / CSS | ~22K LOC |
| API Endpoints | 240 |
| API Handler functions | 222 |
| Modules | 20 |
| Marketplace Templates | 56 (across 16 categories) |
| Test Coverage | 85%+ (CI-enforced gate) |
| Fuzz Targets | 15 |
| Benchmarks | 46 |
| Binary Size | ~24MB stripped, single static binary with embedded UI |
| Repository | github.com/deploy-monster/deploy-monster |

## Test Results (Tier 91, 2026-04-11)

- Go: `go test -short ./...` green across every package
- React: 341 vitest tests pass (38 files)
- Integration: SQLite + Postgres contract suites both green
- Fuzz: 15 targets (see `make bench`)
- Benchmarks: 46 targets
- Coverage gate: 85% in CI (`.github/workflows/ci.yml`), hard fail on regression
- Soak harness: 24h soak runner + 5m CI smoke both green
- Loadtest regression gate: 10% p95 threshold against committed baseline

## What v1.6.0-rc.1 ships

- 88 tiers of lifecycle, context-cancellation, replay, and DoS hardening (see `docs/security-audit.md`)
- `internal/db/migrations/0002_add_indexes.sql` — 30+ indexes on hot query paths
- Argon2id + AES-256-GCM vault with per-install random salt and legacy-salt migration path
- Prometheus runtime-metric block on `/metrics/api`
- Loadtest baseline regression gate + 24-hour soak harness
- OpenAPI drift checker gated in CI (`make openapi-check`)
- Two new ADRs: 0008 (encryption-key strategy) and 0009 (store-interface composition)
- Upgrade guide with per-version compatibility matrix (v0.1.0 → HEAD)
- 17 Dependabot alerts closed (otel 1.42 → 1.43, vite 8.0.3 → 8.0.8, lodash pin, vite@7 transitive pin)

## Outstanding (blocking v1.6.0 final)

- Phase 7.2 — `goreleaser release --snapshot --clean` full-pipeline validation
- Phase 7.3 — smoke-test on fresh Ubuntu 24.04 VM
- Phase 7.4 — CHANGELOG.md from Phase 1–7 delta
- Phase 7.5 — `get.deploy.monster` installer dry-run on fresh VPS
- Phase 7.6 — GHCR image push + scan
- Phase 7.7 — announcement coordination (non-code)

See `.project/ROADMAP.md` Phase 7 for item-level tracking.
