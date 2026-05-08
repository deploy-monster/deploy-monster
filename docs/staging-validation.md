# Staging Validation Runbook

Use this runbook before marking a release candidate production-ready.
It assumes the branch already passes CI and that you have a disposable
staging host with Docker, DNS control, and a smoke-test admin account.

## Inputs

Record these values in the release issue before starting:

| Field | Value |
|---|---|
| Git ref / commit |  |
| DeployMonster version |  |
| Staging base URL |  |
| Staging host IP |  |
| Admin smoke account |  |
| Backup target | local / S3 / R2 / MinIO |
| DNS provider |  |
| Start time |  |
| Operator |  |

Do not paste secrets into the issue. Store passwords, API tokens, S3
keys, and `MONSTER_SECRET` in the team password manager.

## Pre-flight

1. Confirm the candidate commit has a green CI run:
   ```bash
   gh pr checks <PR_NUMBER>
   ```
2. Build the release-shaped binary locally from a clean checkout:
   ```bash
   scripts/build.sh
   ./bin/deploymonster version
   ./bin/deploymonster help >/dev/null
   ```
3. Confirm the staging host has enough space for a local restore drill:
   ```bash
   ssh <host> 'df -h /var/lib/deploymonster || df -h /'
   ```
   Require at least three times the current data directory size.
4. Confirm DNS TTLs are short enough for cutover testing:
   ```bash
   dig +short <staging-domain>
   dig +short <test-app-domain>
   ```

## Deploy To Staging

1. Install or update the candidate binary on the staging host.
2. Run setup with staging-safe values:
   ```bash
   deploymonster setup
   sudo systemctl restart deploymonster
   sudo journalctl -u deploymonster -n 200 --no-pager
   ```
3. Confirm boot health:
   ```bash
   curl -fsS http://<staging-host>:8443/health
   curl -fsS http://<staging-host>:8443/api/v1/health
   ```

## Smoke Checks

Run the repository smoke script against the live host:

```bash
STAGING_BASE_URL=https://staging.example.com \
DM_SMOKE_EMAIL=<admin-email> \
DM_SMOKE_PASSWORD=<admin-password> \
./scripts/staging-smoke.sh
```

For a public-only smoke pass, use:

```bash
STAGING_BASE_URL=https://staging.example.com \
DM_SMOKE_PUBLIC_ONLY=1 \
./scripts/staging-smoke.sh
```

Pass criteria:

- `/health`, `/api/v1/health`, `/api/v1/openapi.json`, and
  `/api/v1/marketplace` return HTTP 200.
- Authenticated login returns an access token.
- `/api/v1/auth/me` and `/api/v1/apps` return HTTP 200 for the smoke
  account.

## Real-World Flow Checks

These checks intentionally exercise external dependencies that CI
mocks out.

1. **Domain and SSL**
   - Create a test app or use an existing staging app.
   - Add `test-<timestamp>.<staging-domain>`.
   - Point DNS at the staging host.
   - Verify HTTP and HTTPS:
     ```bash
     curl -I http://test-<timestamp>.<staging-domain>
     curl -I https://test-<timestamp>.<staging-domain>
     ```
   - If using Let's Encrypt staging, verify the staging issuer is
     expected. Do not treat an untrusted staging certificate as a
     production TLS failure.

2. **Webhook**
   - Connect a throwaway repository.
   - Push a harmless commit.
   - Confirm one deployment is created and the webhook delivery has a
     success status.
   - Confirm a request with a bad signature is rejected.

3. **Tenant Isolation**
   - Create two tenants and one app in each tenant.
   - Log in as a tenant-scoped admin for tenant A.
   - Try to access tenant B's app, domain, backup, server, and registry
     resources through the UI and API.
   - Expected result: read attempts return not found or forbidden;
     mutation attempts return forbidden or not found; no cross-tenant
     state changes occur.

4. **Backup**
   - Trigger a manual backup from the UI or API:
     ```bash
     curl -fsS -X POST \
       -H "Authorization: Bearer $TOKEN" \
       https://staging.example.com/api/v1/backups
     ```
   - Confirm the backup appears in the list endpoint and in the
     configured storage target.
   - Record backup key, size, checksum if available, and completion
     timestamp in the release issue.

5. **Restore Drill**
   - Stop the service.
   - Copy the current data directory aside.
   - Restore the selected backup into `/var/lib/deploymonster`.
   - Start the service.
   - Re-run `scripts/staging-smoke.sh`.
   - Spot-check one app, one secret decrypt, one domain, and one recent
     deployment.
   - Restore the pre-drill data directory if the drill was destructive.

6. **Rollback Drill**
   - Keep the previous known-good binary on the host.
   - Stop the service.
   - Swap back to the previous binary.
   - Restore the pre-upgrade database snapshot if the candidate ran
     migrations.
   - Start the service and run `scripts/staging-smoke.sh`.
   - Swap forward again to the candidate and repeat the smoke script.

## Performance And Soak

Run these after functional smoke passes:

```bash
make loadtest-check
make soak-test-short
```

Pass criteria:

- No load-test regression beyond the committed threshold.
- No growing goroutine or heap trend during the short soak window.
- No repeated module health flaps in the service logs.

## Evidence To Attach

Attach or link the following in the release issue:

- CI run URL for the exact commit.
- `scripts/staging-smoke.sh` output.
- Load-test summary.
- Short-soak summary.
- Backup key and restore-drill result.
- Rollback-drill result.
- Any known exceptions accepted for this release.

## Go / No-Go

Mark the release candidate **go** only when all of these are true:

- CI is green on the release commit.
- Staging smoke, external flow checks, backup restore, rollback, load
  test, and short soak passed.
- No open P0/P1 bugs remain.
- Any P2 bug accepted for release has an owner, issue, and rollback
  plan.

If any item fails, keep the PR or release candidate open, file the
blocking issue, and link the failed evidence.
