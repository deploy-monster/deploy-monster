# DeployMonster — Production Readiness

**Status:** CONDITIONAL GO
**Version:** v0.1.8 candidate
**Report date:** 2026-05-09
**Owner:** Ersin / ECOSTACK TECHNOLOGY OÜ

This file is the executive readiness pointer for the current branch.
Detailed historical analysis remains in `docs/archive/`; current release
evidence should be attached to the PR or release issue.

## Verdict

**Ship to self-hosted single-tenant production: GO after review and
merge.**

**Ship to hosted multi-tenant SaaS: CONDITIONAL GO.** The code and CI
state are ready for review, but hosted SaaS launch must wait for real
staging proof:

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

PR #43 (`codex/launch-hardening`) is ready for review, mergeable, and
green on current-head CI:

| Gate | Status |
|---|---|
| Go race / coverage test | Passing |
| React typecheck, lint, Vitest, build, bundle budget | Passing |
| Go vet, Go build, OpenAPI drift | Passing |
| SQLite integration tests | Passing |
| Postgres integration tests | Passing |
| Secrets scan | Passing |
| Playwright E2E | Passing |
| Cross-platform release build matrix | Passing |

The Docker job is skipped on the PR path by workflow condition. Docker
image publication must still be verified on the release path.

## What Changed Since The April Readiness Review

- Tenant isolation was hardened across app, domain, backup, registry,
  server, certificate, topology, webhook, and deployment-approval paths.
- RBAC route permissions were expanded for non-app operator actions.
- OpenAPI drift, README coverage drift, and project-status drift were
  corrected.
- Frontend, backend, integration, E2E, secrets, and build-matrix gates
  are green on GitHub.
- `docs/staging-validation.md` now defines the required external proof
  before a hosted SaaS launch.

## Remaining Blockers For Hosted SaaS

1. Human review and merge of PR #43.
2. Staging validation on real infrastructure.
3. Backup/restore and rollback evidence attached to the release issue.
4. Load and short-soak evidence attached to the release issue.
5. Release artifacts and Docker images published from the release
   workflow.

## Known Risks

- The PR is large and still needs human review.
- CI cannot prove real DNS, ACME, cloud storage, webhook-provider, or
  Docker-host behavior.
- Docker socket access remains an intentionally powerful operational
  capability; follow `docs/docker-socket-hardening.md`.
- The system is not a multi-master HA control plane. Recovery depends on
  backup, restore, and rollback drills.

## References

- **Current status:** `docs/PROJECT_STATUS.md`
- **Launch plan:** `docs/DEVELOPMENT_LAUNCH_PLAN.md`
- **Staging validation:** `docs/staging-validation.md`
- **Operator runbook:** `docs/runbook.md`
- **Upgrade guide:** `docs/upgrade-guide.md`
- **Security audit:** `docs/security-audit.md`
- **Historical audit archive:** `docs/archive/`
