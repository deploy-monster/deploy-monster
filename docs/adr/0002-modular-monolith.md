# ADR 0002 — Modular monolith, not microservices

- **Status:** Accepted
- **Date:** 2026-03-24
- **Deciders:** Ersin KOÇ

## Context

DeployMonster has 21 distinct functional areas: auth, api, build, deploy,
ingress, dns, vps, backup, billing, marketplace, notifications, swarm,
topology, webhooks, secrets, resource, discovery, mcp, enterprise,
gitsources, database/db. Each could plausibly be its own service.

The alternative architectures considered were:

1. **Microservices** — one process per bounded context, communicating via
   gRPC or message broker.
2. **Modular monolith** — one process, but strict module boundaries enforced
   via package isolation and an internal event bus.
3. **Single flat codebase** — no module boundaries, just Go packages.

## Decision

**DeployMonster is a modular monolith.** One Go binary. Modules are Go
packages under `internal/<name>/` that implement the `core.Module` interface
and register themselves via `init() { core.RegisterModule(...) }`. They
communicate through:

- The `core.Store` interface (for data).
- The `core.EventBus` (for pub/sub, with sync and async handlers).
- The `core.Services` registry (for cross-module service lookup — e.g.,
  container runtime, DNS provider).
- Well-typed Go interfaces, never concrete types from another module.

Dependencies between modules are declared via `Module.Dependencies() []string`
and resolved with a topological sort at boot (`Registry.Resolve`). Shutdown
walks the resolved order in reverse with a 30s timeout.

## Consequences

**Positive:**

- **One binary to install, version, and operate.** This is the single biggest
  win for self-hosted PaaS. The user's mental model is "one service",
  systemd/docker have one thing to restart, backups have one file to copy.
- **In-process latency.** EventBus publishes are function calls; there is no
  network overhead, no serialization, no retry loop. A deploy completion
  event fires and all subscribers see it in microseconds.
- **Atomic transactions.** Because everything is in one process sharing one
  SQLite connection pool, a multi-step operation (create tenant → seed
  buckets → register routes) can be wrapped in a single transaction.
- **Simple testing.** Each module can be loaded with a fake core and mocked
  dependencies. No Docker Compose needed to run unit tests.
- **Strong module boundaries without distributed-system cost.** We still get
  the benefits of explicit interfaces, dependency declaration, and graceful
  lifecycle management.

**Negative / trade-offs:**

- **No independent deployment.** Shipping a bug fix to one module ships all
  21. This is acceptable because the whole thing is a ~20MB binary and
  rolling a new binary is cheap.
- **Scaling is vertical by default.** For horizontal scale, the master/agent
  split (see ADR 0007) lets worker nodes join, but the control plane itself
  is still one process. That matches the target operator profile.
- **Temptation to skip module boundaries.** A `Store` reference and an
  `EventBus` reference are both sitting in `core.Core`, so it is easy for a
  lazy developer to reach across modules. Linting rules (`go vet` + package
  visibility) plus code review have to catch this. In practice we have kept
  it clean.

## Revisit if

- A hosted multi-tenant offering needs genuine per-module scaling.
- A module grows large enough that its startup/shutdown blocks the rest of
  the platform (we'd split it out and run it as an optional agent).
- Compliance requirements force process-level isolation between components.
