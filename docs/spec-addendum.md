# Specification Addendum — Post-Specification Features (v0.0.1)

This document records features that were **not in the original base specification** but were hardened and shipped as part of the v0.0.1 release stabilization pass (Tiers 76–105 / Phase 7).

---

## 1. Canary Deployments

### Motivation
The base spec defined `recreate` and `rolling` strategies. For risk-sensitive production workloads, a gradual traffic-shift strategy was needed to reduce blast radius.

### Behavior
- A new container is started alongside the old one.
- Traffic is split in configurable phases (default: 10 % → 50 % → 100 %).
- Each phase has a **dwell time** (default 2 min). If health checks fail at any phase, traffic is reverted to 0 % and the new container is removed.
- On success, the old container is drained and removed after the final phase.

### API / UI Impact
- `DeployStrategy` enum gained `canary`.
- `AppConfig.Canary` holds phases, dwell, and health-check parameters.
- UI deployment dialog shows canary progress bars and per-phase health status.

### Technical Notes
- `internal/deploy/graceful/canary.go` implements `CanaryController`.
- Weight adjustment is delegated to the ingress layer (`ingress/lb/weighted.go`) so the strategy does not need to know the load-balancer internals.
- Context cancellation during a dwell period triggers an automatic rollback (0 % weight + cleanup).

---

## 2. Deploy Freeze, Schedule & Approval

### 2.1 Deploy Freeze

**Motivation**: Prevent accidental Friday-night or holiday-window deploys.

**Behavior**:
- Tenant-level freeze windows (`DeployFreeze` record) block all new deployments with a clear `423 Locked` error.
- Freeze rules support recurring windows (e.g. "Fridays 18:00–23:59 UTC") or one-off blackouts.
- Does **not** affect running containers — only new `POST /apps/{id}/deploy` calls.

**Files**: `internal/deploy/freeze.go`, `internal/db/freeze_store.go`.

### 2.2 Scheduled Deploy

**Motivation**: Time-zone-aware teams need to queue a deploy for off-peak hours.

**Behavior**:
- `Deployment.ScheduledAt` stores the target time.
- `core.Scheduler` polls the pending queue every minute and promotes due items to the build queue.
- If a freeze window overlaps with `ScheduledAt`, the deployment is rejected at promotion time (not at scheduling time).

**Files**: `internal/core/scheduler.go`, `internal/deploy/scheduler_consumer.go`.

### 2.3 Approval Workflow

**Motivation**: Enterprise tenants want a second pair of eyes before production changes.

**Behavior**:
- `Tenant.Settings.DeployApprovalRequired` gates every deploy request.
- When enabled, a deploy creates an `ApprovalRequest` record in `pending` state.
- Admins approve or reject via `POST /api/v1/admin/approvals/{id}/{action}`.
- Only after approval does the deployment enter the scheduler/build queue.
- Rejection is logged to the audit trail and surfaced in the deploy timeline UI.

**Files**: `internal/api/handlers/approval.go`, `internal/deploy/approval_gate.go`, `internal/db/approval_store.go`.

---

## 3. Bounded Async Event Dispatch

### Motivation
The original event bus spawned an unbounded goroutine per async event. Under high load (e.g. 10 000 container health-check flaps) this could OOM the process.

### Behavior
- `EventBus.SubscribeAsync` handlers are now executed through a **64-slot worker pool**.
- If all workers are busy, new async events are queued in a bounded channel (depth = 1024).
- When the queue is full, the oldest event is dropped and a `dropped_async_event` metric is incremented.
- Synchronous handlers (`Subscribe`) are unchanged and still block the publisher.

### API / UI Impact
- No user-facing change.
- `/api/v1/health` surfaces `event_bus_dropped_total` when > 0, returning `HealthDegraded` instead of `HealthOK`.

### Technical Notes
- `internal/core/events.go` — `asyncSem` (chan struct{}, 64) + `asyncQueue` (chan Event, 1024).
- Worker goroutines are started lazily on the first `SubscribeAsync` call and stopped gracefully during module shutdown.

---

## 4. Per-Install Vault Salt

### Motivation
The initial implementation used a hard-coded fallback salt for the secrets vault. A stolen database backup could therefore be brute-forced offline by anyone who read the source code.

### Behavior
- On first boot, `secrets.Module` generates a 32-byte random salt and persists it in BBolt (`_config` bucket, key `vault.salt`).
- The salt is combined with the operator-supplied `MONSTER_SECRET_KEY` via HKDF-SHA256 to derive the vault KEK.
- Legacy installations that lack a persisted salt are transparently migrated: the old hard-coded salt is used to decrypt existing secrets, then a new random salt is generated and all secrets are re-encrypted.
- If `MONSTER_SECRET_KEY` is rotated, the operator must run `deploymonster vault rotate --old-key=... --new-key=...` manually; there is no automatic rotation on config change.

### API / UI Impact
- No direct API change.
- First-run wizard warns the operator if `MONSTER_SECRET_KEY` is the default placeholder (`changeme`).

### Technical Notes
- ADR-0008 (`docs/adr/0008-encryption-key-strategy.md`) documents the full threat model and migration path.
- Files: `internal/secrets/vault.go`, `internal/secrets/rotation.go`, `internal/secrets/migrate_salt.go`.

---

## Verification Checklist

| Feature | Test Evidence |
|---------|---------------|
| Canary | `internal/deploy/strategies/strategy_test.go` — canary phase transition + rollback tests |
| Freeze | `internal/deploy/freeze_test.go` — window overlap + API rejection tests |
| Schedule | `internal/core/scheduler_test.go` — due-time promotion tests |
| Approval | `internal/api/handlers/approval_test.go` — admin approve/reject flow |
| Bounded async | `internal/core/events_test.go` — 64-worker saturation + drop tests |
| Per-install salt | `internal/secrets/vault_test.go` — legacy migration + KEK derivation tests |
