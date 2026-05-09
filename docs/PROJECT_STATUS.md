# DeployMonster Project Status

**Last updated:** 2026-05-09
**Current branch under review:** `codex/launch-hardening` / PR #43
**Readiness verdict:** conditional go

DeployMonster is ready for self-hosted, single-tenant operation after
the launch-hardening work in PR #43. Multi-tenant SaaS launch remains
conditional until the staging validation, backup/restore drill,
rollback drill, and release artifact publication are completed on real
infrastructure.

This page is a short status pointer. For detailed evidence, use:

- [`README.md`](../README.md) for current feature counts and quick-start
  positioning.
- [`PRODUCTION-READY.md`](../PRODUCTION-READY.md) for the executive
  readiness verdict.
- [`docs/DEVELOPMENT_LAUNCH_PLAN.md`](DEVELOPMENT_LAUNCH_PLAN.md) for
  the sprint-by-sprint implementation plan.
- [`docs/staging-validation.md`](staging-validation.md) for the required
  pre-release staging proof.
- GitHub PR #43 for current CI evidence.

## Current Evidence

PR #43 current-head CI is green:

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

The Docker publish job is skipped on this PR path by workflow
condition; image publication still needs to be verified on the release
path.

## What Is Ready

- Single-binary build with embedded React UI.
- Modular monolith lifecycle and dependency ordering.
- Tenant isolation hardening on app, domain, backup, registry, server,
  and deployment-sensitive paths covered by regression tests.
- RBAC route permissions expanded and aligned with operator actions.
- OpenAPI drift gate and generated API documentation.
- Frontend unit tests and Playwright E2E coverage for core workflows.
- Operational docs for install, upgrade, secret rotation, Docker socket
  hardening, incident response, and staging validation.

## What Is Conditional

- Multi-tenant SaaS operation depends on successful staging proof with
  real DNS, SSL, webhook, backup, restore, rollback, load, and soak
  checks.
- Release artifacts and Docker images still need to be built and
  published from the release workflow.
- Any launch candidate must attach evidence from
  [`docs/staging-validation.md`](staging-validation.md) before being
  called production-ready for hosted SaaS.

## Known Limitations

1. **No multi-master HA.** DeployMonster is still a single control-plane
   process; use host backups and restore drills for recovery.
2. **Docker socket exposure is powerful by design.** Operators must
   follow [`docker-socket-hardening.md`](docker-socket-hardening.md).
3. **Kubernetes is intentionally out of scope.** See
   [`adr/0003-no-kubernetes.md`](adr/0003-no-kubernetes.md).
4. **Release Docker publish is not proven by PR CI.** It is skipped
   outside the release path.
5. **Staging validation is not optional.** CI proves code paths; it does
   not prove real DNS, ACME, cloud storage, or provider integrations.

## Next Required Actions

1. Review and merge PR #43.
2. Execute [`docs/staging-validation.md`](staging-validation.md) on a
   disposable staging host.
3. Attach staging smoke, backup/restore, rollback, load, and soak
   evidence to the release issue.
4. Build and publish release artifacts/images from the release workflow.
5. Cut the release only after the go/no-go checklist is complete.

## License

AGPL-3.0. Commercial licensing terms are handled outside this
repository.
