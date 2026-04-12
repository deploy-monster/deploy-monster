# ADR 0006 — In-process pub/sub, not a message broker

- **Status:** Accepted
- **Date:** 2026-03-24
- **Deciders:** Ersin KOÇ

## Context

Modules need to react to each other's work: when `deploy` finishes, the
`notifications` module should fire a webhook, the `audit` module should
log an entry, the `backup` module may trigger a snapshot, and the
`resource` module updates metrics. The naive alternative is direct
function calls between modules, which couples everyone to everyone.

The classic solutions:

1. **External message broker** — NATS, Redis Streams, RabbitMQ, Kafka.
2. **In-process event bus** — a `sync.Map` of subscribers, direct
   function dispatch.
3. **Hand-rolled observer pattern per module** — each module exposes its
   own `Subscribe` method.

## Decision

**Use an in-process event bus (`internal/core/events.go`).** One
`EventBus` is created at boot and attached to `core.Core`. Modules
subscribe via `Subscribe(eventType, handler)` for synchronous delivery
or `SubscribeAsync(eventType, handler)` for fire-and-forget. Publishers
call `Publish(ctx, event)` or the convenience helpers
`Emit(type, source, data)` / `EmitWithTenant(...)`.

Event type strings follow `{domain}.{action}` convention (e.g.
`app.deployed`, `build.started`, `container.died`). Matching supports:

- Exact match: `"app.created"`
- Prefix wildcard: `"app.*"`
- Global wildcard: `"*"`

No external broker is required or supported for internal events.

## Consequences

**Positive:**

- **Zero operational dependencies.** There is no broker to install,
  secure, monitor, or restart. Users who have never heard of NATS can
  still run DeployMonster.
- **Microsecond latency.** Publishes are direct function calls. A deploy
  completion event reaches the audit, metrics, and notifications modules
  in microseconds, not milliseconds.
- **Transactional cohesion.** Because event handlers share the database
  connection pool, a handler can commit its side effects in the same
  transaction as the event that triggered it.
- **Testing is trivial.** Tests inject a fake bus or subscribe a
  collector function directly. No broker container, no network setup.
- **Structured panic recovery.** `SafeGo` wraps every async handler so a
  misbehaving subscriber cannot crash the server.

**Negative / trade-offs:**

- **No persistence.** If the server crashes mid-handler, the event is
  lost. We mitigate this for durability-critical actions by:
  - Persisting to SQLite/BBolt *before* publishing the event, so a crash
    replay can recover state.
  - Using `core.Retry` with exponential backoff for external webhook
    deliveries (`internal/webhooks/outbound.go`).
  - Idempotency keys on sensitive handlers.
- **No cross-process delivery.** Events stay inside one
  `deploymonster` process. The master/agent protocol (ADR 0007) explicitly
  handles cross-node dispatch over WebSocket for operations that need it
  — the event bus is not a substitute.
- **Subscribers can block publishers.** Synchronous handlers run on the
  publisher's goroutine. Long-running handlers must use
  `SubscribeAsync`. We document this and lint for obvious mistakes, but
  a sloppy `Subscribe` with a slow handler is a footgun.

## Revisit if

- We need cross-process event delivery beyond what the master/agent
  protocol covers (e.g., a true HA multi-master control plane).
- A future audit-log requirement demands guaranteed delivery with replay.
- We hit a subscriber-count scale limit (unlikely — the bus handles
  thousands of subscribers fine).
