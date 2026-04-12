# ADR 0009 — Store interface composition

- **Status:** Accepted
- **Date:** 2026-04-11
- **Deciders:** Ersin KOÇ (project lead)

## Context

ADR 0001 ("Use SQLite as the default database") committed
DeployMonster to supporting a second database (PostgreSQL) through
the same abstraction, on the grounds that every path goes through a
single `core.Store` interface and no module imports the concrete
`internal/db` package. That commitment only holds if the abstraction
itself is shaped in a way that discourages leakage.

Two failure modes were in scope:

1. **A single giant interface.** If `core.Store` exposed 150 methods
   in one flat type, every mock would need to stub 150 methods, every
   test would fake the whole surface, and small modules would depend
   on capabilities they do not need. Go's interface-by-consumption
   idiom exists precisely to avoid this.
2. **Driver-specific types bleeding through the interface.** A method
   returning `*sql.Rows` or `pq.Error` would technically satisfy "no
   one imports `internal/db`" but would still bind callers to the SQL
   package and break a future non-SQL backend.

Both have to be prevented *by the shape of the interface*, not by
documentation or code review.

## Decision

`core.Store` is a composite interface built by embedding **12
domain-specific sub-interfaces** plus two utility methods
(`internal/core/store.go:18-33`). Each sub-interface covers exactly
one entity family:

| Sub-interface | Scope | Declared in store.go |
|---|---|---|
| **TenantStore** | tenant CRUD, slug lookup | lines 36–42 |
| **UserStore** | user CRUD, password, last-login, bulk membership | lines 45–54 |
| **AppStore** | application CRUD, list-by-tenant/project, status | lines 57–66 |
| **DeploymentStore** | deployment records, version tracking, history | lines 69–74 |
| **DomainStore** | domain CRUD, DNS sync/verify, bulk | lines 77–84 |
| **ProjectStore** | project CRUD, tenant defaults init | lines 87–93 |
| **RoleStore** | role lookup, team membership | lines 96–100 |
| **AuditStore** | audit log insert + paginated read | lines 103–106 |
| **SecretStore** | versioned secrets, encrypted metadata, scope/name lookup | lines 109–117 |
| **InviteStore** | invitation management, tenant listing | lines 120–124 |
| **UsageRecordStore** | billing usage records + aggregation | lines 310–313 |
| **BackupStore** | backup metadata + status tracking | lines 334–338 |

Plus `Close() error` and `Ping(ctx) error` for lifecycle management.

### Composition, not inheritance

A module that only needs to read apps can accept an `AppStore`
parameter and tests can stub just that interface. A module that needs
cross-cutting access takes the full `core.Store`. This is the same
pattern `io.Reader` / `io.ReadWriter` uses — the sub-interface is the
common case, the composite is the convenience.

### No driver types in the interface

Every method signature in `internal/core/store.go` uses:

- Standard library types (`context.Context`, `error`, `int`, `[]T`,
  `time.Time`)
- `core`-package domain models (`*core.Tenant`, `*core.User`,
  `*core.Deployment`, …)

A grep for `sql.` or `sqlite.` in `internal/core/store.go` yields
zero matches. No `*sql.Tx`, no `*sql.Rows`, no `sqlite3.Error`. The
abstraction is hermetically sealed from the driver layer.

### Contract enforced at compile time

Both concrete implementations declare a compile-time check:

```go
// internal/db/sqlite.go:17
var _ core.Store = (*SQLiteDB)(nil)

// internal/db/postgres.go:22
var _ core.Store = (*PostgresDB)(nil)
```

Adding a method to any sub-interface breaks the build in both
implementations simultaneously. You cannot ship an incomplete
backend.

### Contract enforced at runtime

`internal/db/store_contract_test.go` defines `runStoreContract()` — a
backend-agnostic integration suite that exercises the full interface
against whichever implementation is passed in. It is invoked from:

- `internal/db/sqlite_integration_test.go` (build tag: `integration`)
- `internal/db/postgres_integration_test.go` (build tag: `pgintegration`)

So every `core.Store` method is exercised against both SQLite and
Postgres in CI, with identical assertions. A SQLite-only behavior
would fail the Postgres run and vice versa.

### Import discipline enforced by the codebase

A search for `"deploy-monster/deploy-monster/internal/db"` imports
outside `internal/db/` itself returns only the module wiring in
`cmd/deploymonster/main.go:24` (a blank `_` import for
auto-registration via `init()`) and the shared DTOs in
`internal/db/models/`. **No module imports the concrete DB package
for its types.** Every handler, every test, every background worker
goes through `core.Store`.

## Consequences

**Positive:**

- **Tests stay small.** A test that only reads apps mocks `AppStore`,
  not a 200-method behemoth. See `internal/deploy/mock_test.go` for
  the pattern — each mock implements only what the test under
  observation needs.
- **Postgres is a real option, not a bolt-on.** The Postgres backend
  was added without touching a single module outside `internal/db/`,
  because every caller was already talking to `core.Store`.
  Integration tests prove both backends are behaviorally
  indistinguishable.
- **Mechanical refactors are safe.** Renaming a method or adding a
  parameter breaks the build in both backends, which means you get a
  compile error list instead of a runtime surprise.
- **Adding a new backend is a constrained problem.** Want a MySQL
  backend? Implement 12 sub-interfaces, write the compile-time
  assertion, run the contract test. No core-code edits.

**Negative / trade-offs:**

- **12 sub-interfaces are a lot to scan.** A new contributor takes
  longer to get a mental model than they would from a "one file per
  table" shape. ADR 0009 (this file) exists partly to mitigate that.
- **Method signatures are verbose.** `core.Store` can only express
  what the weakest sub-interface supports, which means
  Postgres-specific features (JSONB operators, listen/notify,
  advisory locks) cannot leak into the interface. They stay buried
  inside `internal/db/postgres.go` and are invisible to callers.
  That's the right tradeoff but it does mean a "Postgres power user"
  gets no extra speed boost from the abstraction.
- **Adding a new entity requires a decision.** Is this big enough to
  be its own sub-interface (`WebhookStore`, `IntegrationStore`), or
  does it fit inside an existing one? Ad-hoc additions drift the
  composition model. We accept the small coordination cost.

## Revisit if

- A Postgres-specific feature becomes load-bearing (e.g., listen/notify
  for live updates) and the abstraction forces us to reinvent it on
  SQLite at serious cost.
- A third backend (MySQL, CockroachDB, planetscale, etc.) is seriously
  considered — revisit the contract-test harness to make sure it
  parameterizes cleanly.
- The number of sub-interfaces grows past ~18. At that point it is
  worth grouping them into a second composition tier (`CoreStore`,
  `BillingStore`, `OperationsStore`) to keep the mental model
  manageable.
- We need an ORM-like batch/transaction API (`store.InTx(func(tx Store) error)`).
  That is deliberately not in the current interface because both
  backends serialize writes at the database level already, but a
  future multi-statement atomic requirement would push us there.
