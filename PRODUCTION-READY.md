# DeployMonster — Production Readiness

**Status:** CONDITIONAL GO
**Release candidate:** `fix/verification-test-coverage` (post-test-fix verification branch)
**Report date:** 2026-07-06
**Owner:** Ersin / ECOSTACK TECHNOLOGY OÜ

This file is the executive readiness pointer for the current release candidate.
Detailed historical analysis remains in `docs/archive/`; current release
evidence should be attached to the release issue.

## Verdict

**Ship to self-hosted single-tenant production: GO.**

**Ship to hosted multi-tenant SaaS: CONDITIONAL GO.** The current branch has
local build and grouped-test evidence, but hosted SaaS launch must wait for
real staging proof:

- staging install or upgrade from the release candidate
- authenticated and public smoke checks
- real DNS and TLS validation
- webhook delivery and signature-failure validation
- tenant-isolation spot checks against live data
- backup creation and restore drill
- rollback drill
- load check and short soak
- release artifact and Docker image publication verification

The required procedure is documented in
[`docs/staging-validation.md`](docs/staging-validation.md).

## Current Evidence

The latest repository-local verification is documented in
[`docs/verification-report-2026-07-06.md`](docs/verification-report-2026-07-06.md).
It supports these statements:

| Gate | Status |
|---|---|
| Go production build (`go build ./...`) | Passing |
| Go compile-only tests (`go test -run '^$' ./...`) | Passing |
| Grouped Go package tests (`./cmd/...`, `./internal/...`, `./tests/...`) | Passing |
| Previously failing DB/API/middleware/database-engine packages | Passing in targeted runs |
| Web unit tests (`cd web && pnpm test`) | Passing — 44 files / 405 tests |
| Web production build (`cd web && pnpm build`) | Passing |
| OpenAPI drift (`go run ./cmd/openapi-gen`) | Passing — 236 routes in code and spec |

Caveat: the exact monolithic `go test ./... -timeout 120s` command was
attempted but did not complete before the agent tool timeout. The same package
universe passed when split into groups, so do not claim that exact monolithic
command is green until it completes in a local shell or CI job.

Release artifact and image publication still need to be verified on the
tag-driven release workflow path.

## What Changed Since The April Readiness Review

- Tenant isolation was hardened across app, domain, backup, registry,
  server, certificate, topology, webhook, and deployment-approval paths.
- RBAC route permissions were expanded for non-app operator actions.
- OpenAPI drift, README coverage drift, and project-status drift were
  corrected.
- The current verification branch repaired stale tests and DB/API runtime
  failures left by tenant-scoped interface changes.
- `docs/staging-validation.md` defines the required external proof before a
  hosted SaaS launch.

## Remaining Blockers For Hosted SaaS

1. Staging validation on real infrastructure.
2. Backup/restore and rollback evidence attached to the release issue.
3. Load and short-soak evidence attached to the release issue.
4. Release artifacts and Docker images published from the release workflow.
5. A direct local-shell or CI run of the monolithic Go suite, if that exact
   command is required as release evidence.

## Known Risks

- CI and local verification cannot prove real DNS, ACME, cloud storage,
  webhook-provider, or Docker-host behavior.
- Docker socket access remains an intentionally powerful operational
  capability; follow `docs/docker-socket-hardening.md`.
- The system is not a multi-master HA control plane. Recovery depends on
  backup, restore, and rollback drills.

## References

- **Current status:** `docs/PROJECT_STATUS.md`
- **Latest verification:** `docs/verification-report-2026-07-06.md`
- **Launch plan:** `docs/DEVELOPMENT_LAUNCH_PLAN.md`
- **Staging validation:** `docs/staging-validation.md`
- **Operator runbook:** `docs/runbook.md`
- **Upgrade guide:** `docs/upgrade-guide.md`
- **Security audit:** `docs/security-audit.md`
- **Historical audit archive:** `docs/archive/`
