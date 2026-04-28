# Operator Runbook

This runbook indexes the step-by-step procedures for the scenarios an
on-call operator is most likely to hit. Every scenario points at the
authoritative doc for the mechanics; this page is the "which doc do I
need right now?" entry point.

> If you are upgrading on a tight maintenance window, jump to the
> pre-flight checklist in [`upgrade-guide.md`](upgrade-guide.md#pre-flight-before-stopping-the-server).
> It is designed to be followed top-to-bottom without reading anything
> else first.

## Scenario index

| Scenario | Severity | Where to start |
|---|---|---|
| Routine upgrade (patch or minor) | Scheduled | [Upgrade guide § General upgrade procedure](upgrade-guide.md#general-upgrade-procedure) |
| Upgrade across a major version | Scheduled | [Upgrade guide § Per-version compatibility matrix](upgrade-guide.md#per-version-compatibility-matrix) |
| Rollback a failed upgrade | P1 | [Upgrade guide § Rollback procedure](upgrade-guide.md#rollback-procedure) |
| DB migration failed mid-upgrade | P1 | [§ DB migration failure](#db-migration-failure) below |
| JWT secret needs rotation (routine) | Scheduled | [Secret rotation runbook](secret-rotation.md#standard-rotation) |
| JWT secret compromise (emergency) | P0 | [Secret rotation runbook § Emergency compromise](secret-rotation.md#emergency-compromise-response) |
| Full host loss — restore from scratch | P0 | [§ Disaster recovery](#disaster-recovery) below |
| Scheduled backup verification | Scheduled | [Upgrade guide § Backup and restore](upgrade-guide.md#backup-and-restore) |
| Docker socket exposure hardening | Scheduled | [Docker socket hardening](docker-socket-hardening.md) |
| Agent fleet upgrade | Scheduled | [Upgrade guide § Upgrading agents](upgrade-guide.md#upgrading-agents) |
| Config-validation failure at boot | P2 | Run `deploymonster config --config /etc/deploymonster/monster.yaml` and fix reported errors before starting the service |
| Latency / throughput alert against published SLA | P2 | [SLA targets](sla.md) — compare current `make loadtest-check` output to `tests/loadtest/baselines/http.json`; a 10 %+ regression is a release blocker |

## DB migration failure

If `journalctl -u deploymonster -f` shows `migration failed` during a
boot, **do not restart the service repeatedly**. The transaction that
applied the migration rolled back, but downstream state may be
inconsistent.

1. `sudo systemctl stop deploymonster` — stop cleanly if it hasn't
   crashed on its own.
2. Copy the current database aside for analysis:
   ```bash
   sudo cp /var/lib/deploymonster/deploymonster.db /tmp/dm-migfail-$(date +%FT%H-%M).db
   sudo cp /var/lib/deploymonster/deploymonster.bolt /tmp/dm-migfail-$(date +%FT%H-%M).bolt
   ```
3. Restore the pre-upgrade backup from the upgrade's step-2 snapshot:
   ```bash
   sudo cp /var/lib/deploymonster/deploymonster.db.bak /var/lib/deploymonster/deploymonster.db
   sudo cp /var/lib/deploymonster/deploymonster.bolt.bak /var/lib/deploymonster/deploymonster.bolt
   ```
4. Put the previous binary back (you kept it, per the upgrade
   checklist):
   ```bash
   sudo mv /usr/local/bin/deploymonster.previous /usr/local/bin/deploymonster
   sudo systemctl start deploymonster
   ```
5. Open an issue with: the migration name from the log, the
   `journalctl` extract showing `migration failed`, the `CHANGELOG`
   entry for the target version, and the copy of the database from
   step 2. Do not attempt to hand-apply the migration — file the bug
   so the migration itself can be fixed forward.

The programmatic rollback path (`sqliteDB.Rollback(1)`) exists for
development but is **not the recommended production recovery path**
— restore-from-backup is faster and preserves data integrity. A
future release will expose Rollback as a CLI flag with explicit
operator prompts.

## Disaster recovery

Scenario: the host running DeployMonster is irrecoverably lost (cloud
instance destroyed, disk failed without a filesystem backup, host
compromised and wiped). You have an S3-backed daily backup. Target:
serve again on a new host with all apps and tenants intact.

**Prerequisites:**

- Backups configured to S3 (or equivalent object store) per
  [deployment-guide.md § S3 storage](deployment-guide.md#s3-storage).
- The S3 credentials are stored outside DeployMonster (if the only
  copy lived in its own vault, you cannot decrypt them without it).
- The vault master secret (`MONSTER_SECRET` / `secret_key` in
  `monster.yaml`) is recorded in your organisation's password manager
  or HSM — **not only inside the failed DeployMonster**. Without
  this, encrypted secrets cannot be decrypted; there is no backdoor.

**Procedure:**

1. Stand up a fresh host with the target OS and Docker. Install the
   same DeployMonster version the backup came from — crossing a major
   version during disaster recovery adds risk. Follow
   [deployment-guide.md § Installation](deployment-guide.md#installation).
2. Stop the service before it initialises a fresh DB:
   ```bash
   sudo systemctl stop deploymonster
   ```
3. Restore the config from your offsite copy:
   ```bash
   sudo cp /path/to/monster.yaml /etc/deploymonster/monster.yaml
   ```
4. Download the most recent S3 backup archive into the data dir:
   ```bash
   aws s3 cp s3://<your-bucket>/deploymonster-<latest>.tar.gz /tmp/
   sudo tar xzf /tmp/deploymonster-<latest>.tar.gz -C /var/lib/deploymonster/
   sudo chown -R deploymonster:deploymonster /var/lib/deploymonster
   ```
5. Verify the master secret the backup was encrypted with is in place
   (either via the `MONSTER_SECRET` env var or the `secret_key` field
   in `monster.yaml`). Mismatched keys silently fail to decrypt; you
   will only see errors when users try to view secrets.
6. Start the service:
   ```bash
   sudo systemctl start deploymonster
   sudo journalctl -u deploymonster -f
   ```
   Watch for `deploymonster ready version=...` and confirm the module
   list matches the previous host's setup.
7. Verify:
   - `deploymonster health` → OK.
   - Log into the UI with an existing admin account. If the session
     cookies from the old host were in-browser, they are invalid
     against the new JWT secret; log in fresh.
   - Spot-check: apps list, recent deploys, secrets (decrypt one to
     confirm vault works), domains.
8. Re-point DNS at the new host's IP. Let's Encrypt will issue fresh
   certificates on the first HTTPS request — plan for a ~30-second
   first-connection delay per domain.
9. **Agents:** every existing agent will fail to reconnect because
   the agent-registration secret is per-host. Re-enrol each agent
   via **Servers → Add Server → Connect existing** or re-run the
   agent bootstrap; the agent binary does not need to be replaced.
10. Open a post-mortem ticket covering root cause, MTTR, any data loss
    (window between last backup and failure), and action items to
    close the gap that caused the outage.

**What you cannot recover without the master secret:**

- Any encrypted secret (API tokens, DB passwords, webhook secrets).
- The legacy salt-free vault (installations from before the Phase 2
  secret-vault refactor). The migration runs on first post-upgrade
  boot; a disaster before that migration completes means you still
  have legacy-keyed data.

This is why the master secret belongs in your password manager, not
only in the host's config.

## Emergency contacts

- **Security disclosures:** `security@ecostack.ee`.
- **Project issues:** https://github.com/deploy-monster/deploy-monster/issues
- **Status page / incidents:** maintain your own per-deployment; this
  project does not host one.

## Post-incident checklist

After any P0 or P1 event, capture:

- Detection time, acknowledge time, resolution time.
- Was the relevant runbook section correct and complete? If not,
  update it as part of the follow-up PR — the runbook you did not
  have is the most expensive kind of documentation debt.
- Any permanent mitigation that should land in the codebase (new
  alert, new test, new migration, new gate). Add to
  `.project/ROADMAP.md` so it gets sized and scheduled.
