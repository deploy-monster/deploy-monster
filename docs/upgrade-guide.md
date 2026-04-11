# Upgrade Guide

This guide covers upgrading DeployMonster between releases. Follow it in
order — the steps are deliberate and the order matters.

> **Always take a backup before upgrading.** DeployMonster ships with
> built-in backups; the `SnapshotBackup` path produces a
> consistent SQLite snapshot via WAL checkpoint + `VACUUM INTO`. See
> [Backup and restore](#backup-and-restore) below.

## Compatibility guarantees

- **Patch releases** (`x.y.Z`): drop-in. No config changes, no migrations,
  no API changes. Safe to upgrade in place.
- **Minor releases** (`x.Y.z`): backwards-compatible. May add new config
  fields with safe defaults. May add new DB migrations (forward-only by
  default; a matching `.down.sql` exists for supported rollbacks).
- **Major releases** (`X.y.z`): may break config, API, or DB schema.
  Read the CHANGELOG carefully. A dedicated upgrade section will be
  added to this guide for every major release.

The `core.Store` interface is stable across minor releases. Custom
modules that depend on it do not need recompilation for patch upgrades
but do need a rebuild for minor/major releases because they ship inside
the same binary.

## General upgrade procedure

Works for any release type.

### 1. Check the CHANGELOG

```bash
# What's in the new release
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/main/CHANGELOG.md \
  | less
```

Look for:

- Breaking changes (marked explicitly in major releases).
- New required config fields.
- Migration notes.
- Security advisories.

### 2. Back up the database and secrets vault

```bash
# Stop any in-flight backups first so the snapshot is consistent
sudo systemctl stop deploymonster

# Copy the database file and the secrets vault
sudo cp /var/lib/deploymonster/deploymonster.db /var/lib/deploymonster/deploymonster.db.bak
sudo cp /var/lib/deploymonster/deploymonster.bolt /var/lib/deploymonster/deploymonster.bolt.bak

# (Optional) keep a copy offsite
tar czf deploymonster-backup-$(date +%F).tar.gz \
  /var/lib/deploymonster/*.db \
  /var/lib/deploymonster/*.bolt \
  /etc/deploymonster/monster.yaml
```

Do **not** skip this step. Migrations are designed to be safe, but a
failed upgrade is much easier to recover from with a cold copy of the
database.

### 3. Dry-run the new version

```bash
# Download the new binary to a scratch location
wget https://github.com/deploy-monster/deploy-monster/releases/download/vX.Y.Z/deploymonster_linux_amd64.tar.gz
tar xzf deploymonster_linux_amd64.tar.gz -C /tmp

# Validate the config against the new version
/tmp/deploymonster config --config /etc/deploymonster/monster.yaml
```

If `config` reports validation errors, fix them *before* swapping the
binary. Common causes: a removed field, a renamed key, or a new
required setting.

### 4. Swap the binary

```bash
# Service is already stopped from step 2
sudo mv /tmp/deploymonster /usr/local/bin/deploymonster
sudo chmod 0755 /usr/local/bin/deploymonster
sudo chown root:root /usr/local/bin/deploymonster

sudo systemctl start deploymonster
sudo systemctl status deploymonster
```

The first startup after an upgrade runs any pending DB migrations. Watch
the logs:

```bash
sudo journalctl -u deploymonster -f
```

A successful upgrade looks like:

```
level=INFO msg="running migrations" module=db from=0005 to=0008
level=INFO msg="migration applied" module=db name=0006_add_tenant_invites
level=INFO msg="migration applied" module=db name=0007_backup_verification
level=INFO msg="migration applied" module=db name=0008_rate_limit_store
level=INFO msg="deploymonster ready" version=vX.Y.Z
```

### 5. Verify

```bash
# Health check
curl -fsS https://localhost:8443/api/v1/health

# Or via the binary itself (works with the distroless image too)
deploymonster health

# Version check
deploymonster version
```

Log in via the UI and spot-check: dashboard, apps list, a recent deploy
history, domain list.

## Upgrading the Docker image

```bash
# Pull the new tag
docker pull ghcr.io/deploy-monster/deploymonster:vX.Y.Z

# Stop the running container (volume data persists)
docker stop deploymonster
docker rm deploymonster

# Start the new version pointing at the same volume
docker run -d \
  --name deploymonster \
  -p 8443:8443 -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dm-data:/var/lib/deploymonster \
  ghcr.io/deploy-monster/deploymonster:vX.Y.Z
```

Or with `docker-compose.prod.yml`:

```bash
# Pin the new version
sed -i 's|ghcr.io/deploy-monster/deploymonster:.*|ghcr.io/deploy-monster/deploymonster:vX.Y.Z|' \
  docker-compose.prod.yml

docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

## Rollback procedure

If the new version fails health checks or the UI is broken, roll back to
the previous binary.

### Binary install

```bash
sudo systemctl stop deploymonster

# Restore the database from your step-2 backup
sudo cp /var/lib/deploymonster/deploymonster.db.bak /var/lib/deploymonster/deploymonster.db
sudo cp /var/lib/deploymonster/deploymonster.bolt.bak /var/lib/deploymonster/deploymonster.bolt

# Put the old binary back
sudo mv /usr/local/bin/deploymonster.previous /usr/local/bin/deploymonster
sudo systemctl start deploymonster
```

> **Tip:** Before step 4 of the upgrade, copy the current binary aside:
> `sudo cp /usr/local/bin/deploymonster /usr/local/bin/deploymonster.previous`.
> This makes rollback a one-command restore.

### Docker install

```bash
docker stop deploymonster && docker rm deploymonster
docker run -d --name deploymonster \
  -p 8443:8443 -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dm-data:/var/lib/deploymonster \
  ghcr.io/deploy-monster/deploymonster:<PREVIOUS_VERSION>
```

Volume data from the failed upgrade stays in place. If the new version
ran migrations that the old version does not understand, restore the
database file from the backup *before* starting the old binary:

```bash
docker run --rm -v dm-data:/data -v $(pwd):/backup alpine \
  cp /backup/deploymonster.db.bak /data/deploymonster.db
```

### Migration-level rollback

DB migrations ship with `.up.sql` + `.down.sql` pairs in
`internal/db/migrations/`. If a migration caused the problem, the
programmatic rollback path is:

```go
// Not exposed as a CLI command today — contact an operator or use the
// backup/restore flow above for now.
sqliteDB.Rollback(1) // roll back the most recent migration
```

For production, the recommended recovery path is **restore from backup**
rather than partial migration rollback — it is faster, less error-prone,
and preserves data integrity.

## Backup and restore

### Create a consistent snapshot while the server is running

DeployMonster can produce a consistent backup without stopping the
service. The backup scheduler (`internal/backup/scheduler.go`) runs
this daily, but you can trigger one manually via the admin API:

```bash
curl -X POST https://localhost:8443/api/v1/admin/backups/snapshot \
  -H "Authorization: Bearer $TOKEN"
```

This performs WAL checkpoint → `VACUUM INTO` → checksum verification
and emits `backup.completed` (or `backup.failed`) on the event bus.

### Cold copy (server stopped)

```bash
sudo systemctl stop deploymonster
sudo tar czf deploymonster-$(date +%F).tar.gz \
  -C /var/lib/deploymonster .
sudo systemctl start deploymonster
```

### Restore

```bash
sudo systemctl stop deploymonster
sudo tar xzf deploymonster-YYYY-MM-DD.tar.gz -C /var/lib/deploymonster/
sudo chown -R deploymonster:deploymonster /var/lib/deploymonster
sudo systemctl start deploymonster
```

## Upgrading agents

Agents run the same binary as the master (see
[ADR 0007](adr/0007-master-agent-same-binary.md)) and speak a versioned
WebSocket protocol to the control plane. The supported matrix is:

| Master version | Compatible agent versions |
|---|---|
| `x.y.z` | `x.y-1.*` through `x.y.z` (one minor back) |

Rolling upgrade procedure:

1. Upgrade the master first.
2. Verify the master is healthy.
3. Upgrade agents one at a time. The master reroutes jobs to healthy
   agents while one is restarting, so there is no downtime.
4. Once all agents report the new version (`GET /api/v1/admin/agents`
   shows `version: vX.Y.Z` for each), the upgrade is complete.

Do **not** run an agent more than one minor version behind the master.
The protocol is forwards-compatible for one minor release but breaks
silently on larger gaps.

## Version-specific notes

> Each future major release will add a subsection here with breaking
> changes, required config migrations, and verification steps. Minor
> releases are covered by the generic procedure above unless otherwise
> noted in the CHANGELOG.

### Unreleased

- No breaking changes vs. 1.6.0. 60 hardening tiers, Prometheus coverage
  expansion, distroless Docker image, production systemd unit. All
  existing configs continue to work unchanged.
- New optional metrics exported — scraper rules may need updates to
  take advantage of `deploymonster_build_queue_*` and
  `deploymonster_db_connections_*`.
- Docker healthcheck now uses `deploymonster health` instead of `curl`.
  If you have custom Dockerfiles derived from the old image, update the
  `HEALTHCHECK` line.

### 1.6.0 → 1.7 (future)

_Placeholder — update when cutting the next release._
