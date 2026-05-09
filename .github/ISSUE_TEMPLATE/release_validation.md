---
name: Release validation
about: Track staging proof before cutting a release
title: '[Release] vX.Y.Z staging validation'
labels: release, validation
assignees: ''
---

## Candidate

| Field | Value |
|---|---|
| Git ref / commit |  |
| DeployMonster version |  |
| CI run URL |  |
| Staging base URL |  |
| Staging host IP |  |
| Admin smoke account |  |
| Backup target | local / S3 / R2 / MinIO |
| DNS provider |  |
| Start time |  |
| Operator |  |

Do not paste secrets. Store passwords, API tokens, S3 keys, and
`MONSTER_SECRET` in the team password manager.

## Pre-Flight

- [ ] Candidate commit has green CI.
- [ ] Release-shaped binary was built from a clean checkout.
- [ ] `deploymonster version` output was recorded.
- [ ] `deploymonster help` succeeded.
- [ ] `scripts/staging-preflight.sh` passed.
- [ ] Staging host disk capacity is at least 3x current data size.
- [ ] DNS TTLs are low enough for cutover testing.

## Staging Deployment

- [ ] Candidate binary installed or upgraded on staging.
- [ ] `deploymonster setup` completed with staging-safe values.
- [ ] Service restarted cleanly.
- [ ] Recent service logs reviewed.
- [ ] `/health` returns HTTP 200.
- [ ] `/api/v1/health` returns HTTP 200.

## Smoke Evidence

Attach `scripts/staging-smoke.sh` output.

- [ ] Public smoke passed.
- [ ] Authenticated smoke passed.
- [ ] `/api/v1/openapi.json` returned HTTP 200.
- [ ] `/api/v1/marketplace` returned HTTP 200.
- [ ] `/api/v1/auth/me` returned HTTP 200.
- [ ] `/api/v1/apps` returned HTTP 200.

## External Flow Evidence

- [ ] Domain and HTTP routing verified.
- [ ] HTTPS/TLS verified.
- [ ] Webhook delivery success verified.
- [ ] Bad webhook signature rejected.
- [ ] Tenant isolation spot checks passed.

## Data Safety Evidence

- [ ] Manual backup created.
- [ ] Backup appeared in configured storage.
- [ ] Backup key, size, checksum, and timestamp recorded.
- [ ] Restore drill passed.
- [ ] Secret decrypt spot check passed after restore.
- [ ] Rollback drill passed.

## Performance Evidence

- [ ] Load-test summary attached.
- [ ] Short-soak summary attached.
- [ ] No repeated module health flaps observed.
- [ ] No growing goroutine or heap trend observed.

## Release Publication

- [ ] Release workflow completed successfully.
- [ ] Release archive smoke passed.
- [ ] SBOMs attached to release artifacts.
- [ ] Checksums attached and verified.
- [ ] GHCR image published.
- [ ] Trivy HIGH/CRITICAL fixable vulnerability scan passed.

## Go / No-Go

- [ ] No open P0/P1 bugs remain.
- [ ] Accepted P2 issues have owner, issue link, and rollback plan.
- [ ] All exceptions are listed below.
- [ ] Release is approved to cut.

## Exceptions

List any accepted exceptions, owner, follow-up issue, and rollback plan.
