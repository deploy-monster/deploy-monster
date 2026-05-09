# DeployMonster Development and Launch Plan

Last updated: 2026-05-09

This plan is the working path for taking DeployMonster from the current
repository state to a shippable self-hosted product. It is ordered by
dependency: do not add major features until the earlier gates are green.

## Current Status

- PR #43 is merged to `master`, and post-merge GitHub CI is green as of
  2026-05-09.
- Sprint 0 through Sprint 7 in-repo readiness work is complete on `master`. The
  remaining launch gate is external staging proof, not additional local code
  cleanup.
- Critical mutating API routes now use permission-aware middleware,
  viewer-role regression coverage was added, and non-app resource operations
  now have dedicated permission names for network, volume, registry, backup,
  git, marketplace, topology, webhook, deploy-freeze, and deploy-approval
  actions.
- Topology save/deploy now verifies that `project_id` belongs to the caller's
  tenant before writing topology state or compiling deployment output.
- Registry connections and credentials are tenant-scoped in BBolt, and backup
  list/restore/download operations are constrained to the caller's tenant
  prefix.
- Pending deployment approvals are now listed only for the caller's tenant,
  with regression coverage for unauthenticated and cross-tenant list attempts.
- Outbound webhook delivery logs now carry tenant/user context and the app-level
  webhook log API filters delivery history to the caller's tenant.
- Server delete now rejects cross-tenant and shared/platform server deletion
  from tenant scope, while list coverage confirms tenant filtering.
- Certificate upload now requires the referenced domain to belong to the caller's
  tenant, and wildcard certificate requests are stored under tenant-scoped keys.
- Domain verification now resolves the stored domain first, rejects cross-tenant
  or mismatched FQDN verification, and batch verification only accepts FQDNs
  already attached to the caller's tenant.
- Remaining launch work: execute `docs/staging-validation.md` on real
  infrastructure, attach evidence, then run the release workflow.

## Target State

- The working tree is clean and release-ready.
- Backend, frontend, OpenAPI, docs, and embedded UI agree with each other.
- Local CI and GitHub CI are green.
- A new operator can install DeployMonster, complete setup, and deploy the
  first app without manual code-level intervention.
- Tenant isolation, RBAC, backups, deployment recovery, and release artifacts
  are covered by automated tests.
- Documentation is honest about what is production-ready, what is conditional,
  and what is intentionally deferred.

## Sprint 0: Restore Green Gates

Goal: remove current blockers before changing product behavior.

Tasks:

- Run `gofmt` on all changed Go files that fail formatting.
- Fix the frontend ESLint failure in `web/src/pages/AppDetail.tsx`.
- Close OpenAPI drift for:
  - `GET /api/v1/setup/checks`
  - `DELETE /api/v1/servers/{id}`
- Keep runtime/generated backups out of commits.
- Fix the pre-commit hook stderr typo in `scripts/setup-git-hooks.sh`.
- Add frontend ESLint to GitHub CI so local and remote gates match.

Validation:

```bash
go vet ./...
go test -short ./...
go run ./cmd/openapi-gen
cd web && pnpm run lint && pnpm test && pnpm run build && pnpm run check:bundle
```

Exit criteria:

- All commands above pass.
- `git status` contains only intentional source/doc changes.
- No generated runtime backups are staged.

## Sprint 1: RBAC and Tenant Isolation

Goal: make every state-changing endpoint enforce the right permission.

Tasks:

- Audit every `POST`, `PUT`, `PATCH`, and `DELETE` route in `internal/api/router.go`.
- Replace plain `protected(...)` wrappers with `protectedPerm(...)` or `adminOnly(...)`
  where the endpoint changes state or crosses tenant boundaries.
- Add route-level regression tests for the permission matrix.
- Extend cross-tenant tests beyond apps to servers, backups, registries,
  webhooks, topology, and deploy approvals.
- Confirm all resource-scoped handlers use tenant ownership checks equivalent
  to `requireTenantApp`.

Validation:

```bash
go test -run 'TestRouter|FuzzRouter|CrossTenant|Permission' ./internal/api/...
go test -short ./...
```

Exit criteria:

- No mutating route is protected only by authentication unless it is explicitly
  documented as safe.
- Non-admin users cannot call admin or cross-tenant endpoints.
- Cross-tenant mutation tests cover the main resource families.

## Sprint 2: First-Run Product Flow

Goal: a new user can install, configure, and deploy without guessing.

Tasks:

- Verify `deploymonster setup` end to end.
- Align the UI onboarding page with `GET /api/v1/setup/checks`.
- Add setup checks for Docker access, database writability, ports, domain,
  admin account, and secret length.
- Make first deploy paths complete:
  - Git source deploy
  - marketplace deploy
  - Docker image deploy
  - compose stack deploy
- Improve user-facing error messages with cause and recovery guidance.

Validation:

```bash
scripts/build.sh
./bin/deploymonster setup
./bin/deploymonster serve
cd web && pnpm test:e2e
```

Exit criteria:

- A clean VM install reaches the dashboard.
- An admin can deploy at least one app through the UI.
- Failure states are actionable to an operator.

## Sprint 3: Data Layer Reliability

Goal: make storage behavior predictable under migration, backup, and load.

Tasks:

- Keep SQLite and PostgreSQL implementations aligned with the `core.Store`
  contract.
- Run real PostgreSQL integration tests in CI and locally when available.
- Expand tests around `ServerStore`, migrations, backup snapshot consistency,
  WAL checkpoint behavior, and transaction rollback.
- Standardize query timeout handling across store methods.
- Document the SQLite/Postgres migration-pair rule.

Validation:

```bash
go test ./internal/db/...
TEST_POSTGRES_DSN='postgres://deploymonster:deploymonster@localhost:5432/deploymonster_test?sslmode=disable' \
  go test -tags pgintegration -v ./internal/db/...
make db-gate
```

Exit criteria:

- Store contract tests pass for SQLite and PostgreSQL.
- Migration drift is covered by tests.
- Writers-under-load gate remains within threshold.

## Sprint 4: Deploy Runtime and Agent Mode

Goal: deployments recover cleanly from failures and agent mode is credible.

Tasks:

- Define and test the deployment state machine:
  - `queued`
  - `building`
  - `deploying`
  - `healthy`
  - `failed`
  - `rolled_back`
- Verify stale deployment reclamation after crash.
- Expand restart-storm, rollback, orphan cleanup, and Docker timeout tests.
- Validate master/agent protocol versioning and compatibility.
- Make Docker unavailable mode clear in API and UI.

Validation:

```bash
go test ./internal/deploy/... ./internal/swarm/...
go test -tags integration -run TestMasterAgent_Integration -v ./internal/swarm/...
```

Exit criteria:

- A crashed deployment is not left permanently in-flight.
- Agent connection and command flow are covered.
- Runtime failures produce consistent status and logs.

## Sprint 5: Frontend Product Quality

Goal: make the UI maintainable and reliable for daily operations.

Tasks:

- Split large pages, starting with `web/src/pages/AppDetail.tsx`, into focused
  components for settings, env vars, logs, metrics, and actions.
- Standardize API error presentation across pages.
- Add E2E coverage for:
  - expired auth
  - permission denied
  - failed deploy
  - Docker unavailable
  - empty states
- Keep accessibility tests blocking.
- Check core layouts on desktop, tablet, and mobile viewports.

Validation:

```bash
cd web
pnpm run lint
pnpm test
pnpm test:e2e
pnpm run build
```

Exit criteria:

- Page components are small enough to review safely.
- Common failure states are tested.
- No lint, type, test, or bundle-size regressions.

## Sprint 6: Documentation Truth Sync

Goal: all public docs describe the current product, not stale targets.

Tasks:

- Update route counts from `go run ./cmd/openapi-gen`.
- Update module counts from `core.RegisterModule` registrations.
- Update marketplace counts from registry tests.
- Update coverage claims from the CI-filtered coverage profile.
- Reconcile:
  - `README.md`
  - `docs/PROJECT_STATUS.md`
  - `docs/architecture.md`
  - `CLAUDE.md`
  - `AGENTS.md`
  - `PRODUCTION-READY.md`
- Replace broad "production-ready" claims with scoped, verifiable statements.

Validation:

```bash
go run ./cmd/openapi-gen
COVERAGE_PROFILE=coverage.out ./scripts/check-readme-coverage.sh
```

Exit criteria:

- Docs and code agree on counts and feature status.
- Known limitations are explicit.
- Quickstart commands have been tested on a clean machine.

## Sprint 7: Release Pipeline

Goal: releases are repeatable and verifiable.

Tasks:

- Install local release tooling before running release targets:
  - `goreleaser`
  - `syft` (used by GoReleaser SBOM generation)
- Verify `scripts/build.sh` produces the expected embedded UI binary.
- Test release Docker image and development Docker image.
- Smoke-test CLI commands:
  - `deploymonster version`
  - `deploymonster help`
  - `deploymonster config`
  - `deploymonster health`
- Verify SBOM and Trivy release gates.
- Ensure version, tag, changelog, and release artifact metadata match.

Validation:

```bash
scripts/build.sh
scripts/release-snapshot.sh
make docker
make release-snapshot
```

Exit criteria:

- A release can be built from a clean checkout.
- Artifacts include the correct version and embedded frontend.
- Image scan has no fixable high or critical findings.

## Sprint 8: Launch Readiness

Goal: prove the product survives real operation.

Tasks:

- Deploy to staging.
- Run staging smoke checks against public and authenticated endpoints.
- Run load test and compare against the committed runner baseline.
- Run a short soak test, then schedule the long soak test.
- Test backup and restore on staging.
- Write and verify operational runbooks:
  - install
  - upgrade
  - backup
  - restore
  - secret rotation
  - Docker socket hardening
  - rollback

Validation:

```bash
scripts/staging-smoke.sh
make loadtest-check
make soak-test-short
```

Exit criteria:

- Staging can be rebuilt from scratch.
- Backup and restore are proven.
- Launch checklist is fully checked off.

## Launch Checklist

- CI green.
- OpenAPI drift check green.
- Frontend lint, tests, E2E, and bundle-size gates green.
- Backend race suite green in CI.
- Integration tests green.
- Secrets scan green.
- Release image scan green.
- Docs synced.
- Staging smoke green.
- Backup and restore verified.
- Rollback plan documented.

## Working Rules

- Keep changes small and reviewable.
- Do not mix unrelated product work with CI repair.
- Every new API route must update OpenAPI in the same change.
- Every new mutating route must state its permission requirement.
- Every generated artifact must have a clear reason to be committed.
- Documentation claims must be backed by an executable check or a named source.
