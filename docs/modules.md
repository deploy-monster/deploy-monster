# DeployMonster Module System

This document explains the lifecycle, dependency resolution, and extension contract for DeployMonster modules. It is the authoritative reference for anyone adding a new subsystem or debugging initialization order.

---

## What is a module?

Every major feature in DeployMonster is implemented as a **module**: a Go package under `internal/<name>/` that implements the `core.Module` interface. The binary currently ships 20 modules, ranging from `auth` and `api` to `marketplace` and `swarm`.

A module is **not** a microservice. It is a compile-time boundary inside the same binary. Modules communicate through:

- The `core.Core` dependency container (persistence, config, event bus, services)
- The in-process `EventBus` (pub/sub with prefix matching)
- HTTP routes registered centrally in `internal/api/router.go`

---

## The `core.Module` interface

```go
type Module interface {
    // Identity
    ID() string
    Name() string
    Version() string
    Dependencies() []string // IDs of modules that must start before this one

    // Lifecycle
    Init(ctx context.Context, core *Core) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    // Observability
    Health() HealthStatus

    // Integration
    Routes() []Route
    Events() []EventHandler
}
```

### Identity methods

- `ID()` — unique machine identifier (e.g. `"api"`, `"auth"`, `"deploy"`). Used in dependency declarations and the registry map.
- `Name()` — human-readable name for logs and health readouts.
- `Version()` — module-level version string. This is independent of the binary release version.
- `Dependencies()` — a list of `ID()` strings that must be initialized **before** this module. The registry performs a topological sort on this graph.

### Lifecycle methods

- `Init(ctx, core)` — called once at startup, in dependency order. Receives the fully wired `*core.Core` container. Modules should open DB connections, create buckets, validate config, and attach event subscriptions here. **Init must not block** on long-running work.
- `Start(ctx)` — called after every module has initialized. Background workers, listeners, and goroutines begin here. `Start` may block until `ctx` is cancelled, but modules typically spawn goroutines and return quickly.
- `Stop(ctx)` — called during graceful shutdown, in **reverse** dependency order. Must release resources, drain in-flight work, and return before the 30-second shutdown timeout expires.

### Observability & integration

- `Health() HealthStatus` — returns `HealthOK`, `HealthDegraded`, or `HealthDown`. The `/health/detailed` endpoint aggregates this across all modules.
- `Routes() []Route` — HTTP endpoints this module contributes. Routes are not registered by the module itself; they are collected by the `api` module and wired into the central `http.ServeMux`.
- `Events() []EventHandler` — event subscriptions. Handlers are registered during `Init` via `core.Events.Subscribe(...)`.

---

## Registration

Modules do **not** import `cmd/deploymonster/main.go` directly. Instead, each module package contains an `init()` function that calls `core.RegisterModule(...)`:

```go
// internal/deploy/module.go
package deploy

import "github.com/deploy-monster/deploy-monster/internal/core"

func init() {
    core.RegisterModule(func() core.Module { return NewModule() })
}
```

`cmd/deploymonster/main.go` then blank-imports every module package:

```go
import _ "github.com/deploy-monster/deploy-monster/internal/deploy"
```

This pattern keeps the module list centralized in `main.go` while the module implementation stays decoupled from the orchestrator.

---

## Dependency resolution

The registry builds the initialization graph at runtime:

1. **Collect** — all `init()` calls run during package load, populating `moduleFactories`.
2. **Register** — `registerAllModules(c)` instantiates each factory and inserts it into `core.Registry`.
3. **Resolve** — `Registry.Resolve()` performs a DFS-based topological sort on `Dependencies()`.
4. **Error conditions**:
   - **Circular dependency** → fatal error at startup.
   - **Unknown dependency** → fatal error at startup.
   - **Duplicate ID** → fatal error during registration.

### Example dependency graph

```text
db          →  []
auth        →  [db]
discover    →  [db]
ingress     →  [db]
deploy      →  [db, ingress, discover]
api         →  [auth, deploy, ingress, ...]
```

Because `deploy` declares `ingress` and `discover` as dependencies, the ingress gateway and Docker event watcher are guaranteed to be initialized and started before the deploy engine begins creating containers.

### Reverse shutdown

`Registry.StopAll(ctx)` iterates the resolved order backwards. This ensures that:

- The `api` module stops accepting HTTP requests before `deploy` stops tearing down containers.
- The `ingress` module closes its listeners before the `deploy` module closes Docker connections.
- `db` stops last, so upstream modules can flush state to storage during their `Stop()` calls.

---

## Lifecycle deep-dive

### Init

```go
func (m *Module) Init(ctx context.Context, c *core.Core) error {
    m.store = c.Store
    m.events = c.Events
    m.runtime = c.Services.Container

    // Subscribe to domain events
    c.Events.Subscribe("app.deployed", m.onAppDeployed)

    // Pre-validate config
    if m.runtime == nil {
        return fmt.Errorf("container runtime not configured")
    }
    return nil
}
```

**Rules of Init:**
- Do not start background goroutines that outlive the function.
- Do not bind to network ports (do that in `Start`).
- Return a terminal error if prerequisites are missing — the binary will fail fast.

### Start

```go
func (m *Module) Start(ctx context.Context) error {
    go m.backgroundWorker(ctx)
    go m.healthTicker(ctx)
    return nil
}
```

**Rules of Start:**
- Every goroutine spawned here must respect `ctx.Done()`.
- Use a module-scoped `stopCtx` + `sync.WaitGroup` if `Stop()` needs to wait for goroutine exit.
- Return quickly; heavy initialization belongs in `Init`.

### Stop

```go
func (m *Module) Stop(ctx context.Context) error {
    close(m.stopCh)      // signal goroutines to exit
    m.wg.Wait()          // drain background work
    return m.db.Close()  // release resources
}
```

**Rules of Stop:**
- Respect the passed `ctx` timeout. `Registry.StopAll` uses a 30-second deadline.
- If a goroutine performs a blocking RPC (e.g. Docker build), add a `select` on `ctx.Done()` so shutdown isn't hostage to a slow call.
- Return the first error encountered, but attempt to stop every module regardless.

---

## Adding a new module

1. **Create the package** under `internal/<name>/`.
2. **Define the module struct** and implement `core.Module`.
3. **Add `module.go`** with an `init()` that calls `core.RegisterModule(...)`.
4. **Blank-import** the package in `cmd/deploymonster/main.go`.
5. **Declare dependencies** accurately. If your module talks to Docker, depend on `deploy` or `discovery`, not the other way around.
6. **Return routes** from `Routes()`; the `api` module will surface them (or wire them directly if you own the router mux).

### Minimal template

```go
// internal/demo/module.go
package demo

import (
    "context"
    "github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
    core.RegisterModule(func() core.Module { return &Module{} })
}

type Module struct{}

func (m *Module) ID() string   { return "demo" }
func (m *Module) Name() string { return "Demo Module" }
func (m *Module) Version() string { return "1.0.0" }
func (m *Module) Dependencies() []string { return []string{"db"} }

func (m *Module) Init(ctx context.Context, c *core.Core) error  { return nil }
func (m *Module) Start(ctx context.Context) error               { return nil }
func (m *Module) Stop(ctx context.Context) error                { return nil }
func (m *Module) Health() core.HealthStatus                     { return core.HealthOK }
func (m *Module) Routes() []core.Route                          { return nil }
func (m *Module) Events() []core.EventHandler                   { return nil }
```

---

## Common pitfalls

| Pitfall | Why it happens | Fix |
|---------|---------------|-----|
| **Init deadlock** | `Init` calls a store method that depends on another module that hasn't initialized yet. | Only use interfaces that were injected in your own `Init`. If two modules need each other, refactor so one publishes events and the other subscribes. |
| **Startup race** | `Start` assumes another module's goroutine is already running. | `Start` runs in dependency order; if the race persists, move the coordination to `Init` (e.g. register a callback) rather than polling. |
| **Shutdown leak** | `Stop` does not wait for a goroutine to exit. | Add a `stopCtx` + `sync.WaitGroup`. See `internal/discovery/module.go` for a canonical example. |
| **Circular health dependency** | `Health()` calls another module's `Health()` via a shared object. | `Health()` should inspect only local state. Cross-module degradation is handled by the detailed health endpoint, not by modules calling each other. |
| **Route auth drift** | A new route forgets to set `Auth` on the `core.Route` struct. | Add a table-driven test in `internal/api/router_test.go` that walks your routes with unauthenticated, developer, and admin tokens. |

---

## Diagnostics

### List modules at runtime

```bash
curl -s https://localhost:8443/api/v1/admin/system | jq '.modules'
```

### Inspect the resolved init order

There is no CLI flag for this today, but you can add a temporary log line in `core.Registry.Resolve()`:

```go
slog.Info("module init order", "order", r.order)
```

### Health per module

```bash
curl -s https://localhost:8443/health/detailed | jq '.modules'
```

---

## See also

- `internal/core/module.go` — interface definition and auth levels
- `internal/core/registry.go` — topological sort and lifecycle orchestration
- `internal/core/app.go` — `Core.Run()` and graceful shutdown logic
- `cmd/deploymonster/main.go` — module imports and entry point
