# ADR 0011 — `core.ErrBoltNotFound` sentinel and KV-corruption observability

- **Status:** Accepted
- **Date:** 2026-05-05
- **Deciders:** Ersin KOÇ (project lead)

## Context

DeployMonster keeps short-lived runtime state in BBolt (rate-limit
counters, idempotency cache, session tracking, per-tenant config).
The `core.BoltStorer` interface returns plain `error` values; before
this ADR every implementation produced ad-hoc messages — `"key not
found"`, `"bucket %q not found"`, `"key %q not found"` — and every
caller treated `err != nil` identically: fall through to the
default-or-fresh-state path.

Two field defects forced the issue:

1. **Lockout silently neutered.** `AuthHandler.incrementPerAccountRateLimit`
   guarded the increment with `if err != nil || entry.LockedUntil > 0`.
   The first failed attempt against any account always missed Get
   ("key not found") and that branch returned without writing. A
   credential-stuffing attacker hitting fresh accounts would never
   trip the 5-attempt lockout, because every attempt landed on the
   skip-write side. Conflating "no record yet" with "untrusted
   state" was the root cause.

2. **Dead-code Error logs.** `AuthRateLimiter` and
   `IdempotencyMiddleware` both declared `logger *slog.Logger`
   fields (or closure captures) that were never assigned. The
   `bolt.Set`-failure error logs gated on those loggers were
   silently unreachable in production. Auditing the rate-limit code
   for the lockout fix surfaced both.

Operators had no visibility into BBolt corruption: a damaged entry
on any of the four KV-backed paths (account-RL, auth-IP RL,
tenant-RL, idempotency) silently degraded into the default-or-fresh
behaviour with zero log signal.

## Decision

Three coordinated changes:

### 1. Typed sentinel for KV miss

Add `core.ErrBoltNotFound` (distinct from the existing
`core.ErrNotFound`, which the SQL/PG store layer uses).
`BoltStore.Get` wraps every miss-or-expiry branch with
`fmt.Errorf("...: %w", core.ErrBoltNotFound)`. Unmarshal failures
deliberately stay unwrapped — they signal corruption, not absence.

Callers that need to distinguish "no record yet" from "untrusted
state" match the sentinel via `errors.Is`. Callers that don't care
(read-with-default sites in autoscale, deploy_freeze, basic_auth,
maintenance, app_middleware, etc.) keep their `if err != nil { use
default }` shape unchanged — adoption is opt-in, not viral.

### 2. Constructor-injected logger pattern

For each component that needs to log on KV operations:
- a `logger *slog.Logger` struct field,
- the constructor defaults it to `slog.Default()`,
- a `SetLogger` setter for test injection,
- a `log()` accessor that falls back to `slog.Default()` when the
  field is nil (so direct-struct construction in tests doesn't
  panic).

Applied to `AuthRateLimiter`, `AuthHandler`, `TenantRateLimiter`,
and `IdempotencyMiddleware` (closure-style — no setter needed). All
four now share the same shape.

### 3. Warn-on-corruption convention

On any read whose error is *not* `core.ErrBoltNotFound`:

```go
if err := bolt.Get(...); err != nil && !errors.Is(err, core.ErrBoltNotFound) {
    logger.Warn("<component> read failed; <effect>", "key", k, "error", err)
}
```

Effect text matches what the surrounding code does: `"falling
through to handler"`, `"resetting window"`, `"increment skipped"`.

Behaviour contracts are unchanged. The corrupted-entry path keeps
its existing fail-open (or fail-fresh) treatment so legitimate
users are not wedged. Only operator visibility changes.

## Consequences

**Positive:**

- The lockout bug's class is closed: any future read-modify-write
  pattern can pin "no record yet" via the sentinel without
  conflating it with corruption. The `AuthHandler.incrementPerAccountRateLimit`
  fix is the canonical example.
- Three previously-dead `Error` logs in `AuthRateLimiter` and
  `IdempotencyMiddleware` go live. `bolt.Set` failures that have
  been silently swallowed are now operator-visible.
- Four KV-backed paths (account-RL, auth-IP RL, tenant-RL,
  idempotency) emit a `Warn` on corruption with consistent message
  shape, so dashboards / log alerts can match a single regex.
- The constructor-injected logger pattern gives test code a
  capturing-handler injection point that is uniform across types.

**Negative:**

- Operators may see new alerts on the previously-dead `Error` logs
  if `bolt.Set` has been failing in production. This is strictly
  surfacing existing failures, not introducing new ones, but
  dashboards/alerts should be reviewed at upgrade time.
- The four-component logger pattern is mildly redundant with what
  a future `core.Logger` injection convention would do; the
  duplication is acceptable today because each component already
  manages its own lifecycle and there is no umbrella DI container.
- Sentinel adoption is opt-in: most BBolt callers are pure
  read-with-default and gain nothing from matching the sentinel.
  Future contributors must consciously choose between "I care about
  the distinction" (use `errors.Is`) and "I don't" (keep the
  generic `err != nil`). Documented here as the canonical rule.

**Not done:**

- Test mocks must wrap their not-found returns with
  `core.ErrBoltNotFound` to match production semantics; otherwise
  the new Warn paths fire on legitimate first-request reads. Three
  in-tree mocks already updated (`mockBoltStore` in
  `internal/api/handlers/common_test.go`, `rlBoltStore` in
  `internal/api/middleware/ratelimit_test.go`, `idempBoltStore` in
  `internal/api/middleware/idempotency_test.go`); future mocks must
  follow the same shape.
- An `expired` sentinel could be split out from `ErrBoltNotFound`
  if a caller ever needs to distinguish "never existed" from
  "TTL elapsed". No caller cares today.
- The same logger-injection pattern is not yet applied to other
  handler types (`SessionHandler`, `AdminHandler`, `DatabaseHandler`,
  …). Deferred until a concrete telemetry need surfaces.
