# DeployMonster Project Status

**Last updated:** 2026-07-06
**Current working branch:** `fix/verification-test-coverage`
**Readiness verdict:** conditional go

DeployMonster remains suitable for self-hosted, single-tenant operation when
operators follow the deployment and backup guidance. Hosted multi-tenant SaaS
launch remains conditional until staging validation, backup/restore, rollback,
load, soak, and release-artifact evidence are completed on real infrastructure.

This page is a short status pointer. For detailed evidence, use:

- [`README.md`](../README.md) for current feature counts and quick-start
  positioning.
- [`PRODUCTION-READY.md`](../PRODUCTION-READY.md) for the executive readiness
  verdict.
- [`docs/verification-report-2026-07-06.md`](verification-report-2026-07-06.md)
  for the latest repository-local build/test evidence.
- [`docs/DEVELOPMENT_LAUNCH_PLAN.md`](DEVELOPMENT_LAUNCH_PLAN.md) for the
  sprint-by-sprint implementation plan.
- [`docs/staging-validation.md`](staging-validation.md) for the required
  pre-release staging proof.

## Current Evidence

The current repository-local verification supports:

| Gate | Status |
|---|---|
| Go production build (`go build ./...`) | Passing |
| Go compile-only tests (`go test -run '^$' ./...`) | Passing |
| DB package tests (`go test ./internal/db`) | Passing |
| API handlers full package (`go test ./internal/api/handlers -timeout 60s`) | Passing |
| API middleware package (`go test ./internal/api/middleware`) | Passing |
| Database engines package (`go test ./internal/database/engines`) | Passing |
| Grouped Go package tests (`./cmd/...`, `./internal/...`, `./tests/...`) | Passing |
| Web unit tests (`cd web && pnpm test`) | Passing — 44 files / 405 tests |
| Web production build (`cd web && pnpm build`) | Passing |
| OpenAPI drift (`go run ./cmd/openapi-gen`) | Passing — 236 routes in code and spec |

Caveat: the exact monolithic `go test ./... -timeout 120s` command was attempted
but did not complete before the agent tool timeout. The same package universe
passed when split into groups. Do not claim the monolithic command is green
until it completes in a local shell or CI environment.

## What Is Ready

- Single-binary build with embedded React UI.
- Modular monolith lifecycle and dependency ordering.
- Tenant isolation hardening on app, domain, backup, registry, server, and
  deployment-sensitive paths covered by regression tests.
- RBAC route permissions expanded and aligned with operator actions.
- OpenAPI drift gate and generated API documentation.
- Frontend unit-test and build evidence for the current branch.
- Operational docs for install, upgrade, secret rotation, Docker socket
  hardening, incident response, and staging validation.

## What Is Conditional

- Multi-tenant SaaS operation depends on successful staging proof with real DNS,
  SSL, webhook, backup, restore, rollback, load, and soak checks.
- Release artifacts and Docker images still need to be built and published from
  the release workflow.
- Any launch candidate must attach evidence from
  [`docs/staging-validation.md`](staging-validation.md) before being called
  production-ready for hosted SaaS.
- A direct local-shell or CI run of the monolithic Go test command is still
  recommended if that exact command is required as release evidence.

## Known Limitations

1. **No multi-master HA.** DeployMonster is still a single control-plane
   process; use host backups and restore drills for recovery.
2. **Docker socket exposure is powerful by design.** Operators must follow
   [`docker-socket-hardening.md`](docker-socket-hardening.md).
3. **Kubernetes is intentionally out of scope.** See
   [`adr/0003-no-kubernetes.md`](adr/0003-no-kubernetes.md).
4. **Release publication is not complete.** Build/test evidence does not prove
   the tag-driven release workflow has published artifacts and the GHCR image.
5. **Staging validation is not optional.** Local and CI checks prove code paths;
   they do not prove real DNS, ACME, cloud storage, or provider integrations.

## Next Required Actions

1. Execute [`docs/staging-validation.md`](staging-validation.md) on a disposable
   staging host.
2. Attach staging smoke, backup/restore, rollback, load, and soak evidence to
   the release issue.
3. Build and publish release artifacts/images from the release workflow.
4. Run the monolithic Go suite directly outside the agent tool timeout if that
   exact command is a release gate.
5. Cut the release only after the go/no-go checklist is complete.

## License

AGPL-3.0. Commercial licensing terms are handled outside this repository.
