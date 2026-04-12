# ADR 0001 — Use SQLite as the default database

- **Status:** Accepted
- **Date:** 2026-03-24
- **Deciders:** Ersin KOÇ (project lead)

## Context

DeployMonster needs a persistence layer for tenants, users, apps, deployments,
domains, secrets metadata, audit logs, and ~40 other domain entities. The
target user is a single operator or small team running a self-hosted PaaS on
one VPS (or a small cluster). The product competes with Coolify, Dokploy,
CapRover, and Railway-style tools.

Candidate datastores considered:

| Option | Pros | Cons |
|---|---|---|
| **SQLite** | Zero setup, file-based, embedded, WAL concurrency, mature, single binary distribution, no daemon to manage | Writer-single concurrency, limited horizontal scale |
| **PostgreSQL** | Strong concurrency, rich SQL, horizontal scale | Extra daemon to install/update/secure, adds operational surface |
| **MySQL/MariaDB** | Popular, mature | Same daemon cost as Postgres, no advantage over it for us |
| **BadgerDB/BBolt alone** | Embedded, fast | No SQL, relational queries get painful across ~40 tables |

## Decision

**SQLite is the default database.** PostgreSQL support is provided through
the same `core.Store` interface for users who hit SQLite's write-concurrency
ceiling, but it is explicitly opt-in via `database.driver: postgres`.

Key enabling choices:

- WAL mode on by default (multiple concurrent readers + one writer without
  blocking).
- All data access goes through the `core.Store` interface — no package
  imports `*db.SQLiteDB` directly. This is what makes Postgres a clean
  alternative rather than a rewrite.
- BBolt is used alongside SQLite for KV-shaped state (rate limiter state,
  API keys, webhook secrets, token families, config snapshots) where a
  relational schema adds no value and hurts hot-path latency.

## Consequences

**Positive:**

- Single binary + a file is the whole installation. No `apt install postgres`,
  no connection string, no firewall rule for port 5432.
- Backups are `cp deploymonster.db deploymonster.db.bak` (or the
  `SnapshotBackup` WAL-checkpoint path for consistency).
- Performance is excellent for the target workload: SQLite `GetApp` bench
  is ~41 µs/op, and covering indexes (see
  `internal/db/migrations/0002_add_indexes.sql`) keep it there.
- Tests are trivial — each test gets its own temp file, no fixture server.

**Negative / trade-offs:**

- Single-writer means write-heavy workloads (thousands of concurrent deploys)
  will queue. We mitigate by batching writes via `BoltBatchItem`, keeping
  long-running transactions off the hot path, and offering Postgres for users
  who outgrow SQLite.
- We had to work hard to avoid SQLite-isms leaking into handlers — every
  query goes through the `Store` interface, which is verbose but the right
  abstraction for portability.
- Users expecting a "real" database are occasionally surprised. We address
  this in `docs/getting-started.md` and the README feature list.

## Revisit if

- A single-tenant benchmark shows >20% CPU in SQLite write contention.
- We ship a hosted offering where a shared Postgres cluster is more economical
  than per-tenant SQLite files.
- We add features that genuinely need multi-writer semantics (e.g., global
  rate limiting across multiple nodes — currently handled via BBolt store
  persistence + re-sync).
