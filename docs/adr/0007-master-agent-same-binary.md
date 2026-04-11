# ADR 0007 — Master and agent are the same binary

- **Status:** Accepted
- **Date:** 2026-03-24
- **Deciders:** Ersin KOÇ

## Context

For multi-node deployments, DeployMonster needs a way for a control-plane
node to dispatch work (builds, deploys, log streaming, container exec) to
worker nodes that run the actual user containers.

Options considered:

1. **Separate `deploymonster` and `deploymonster-agent` binaries.** The
   orthodox approach — one build for control plane, one for workers.
2. **Same binary, different mode flag.** `deploymonster serve` vs
   `deploymonster serve --agent --master=<url> --token=<token>`.
3. **Use existing orchestration (Swarm / k8s / Nomad).** Covered by ADR
   0003 — rejected.

## Decision

**The same binary runs in both modes.** A `--agent` flag switches
behavior at `cmd/deploymonster/main.go:109`:

- **Master mode (default):** full platform — API, ingress, build pipeline,
  database, UI, scheduler, everything.
- **Agent mode:** connects to the master over WebSocket, authenticates
  with a join token, receives and executes dispatched jobs via the local
  Docker SDK, streams logs and stats back.

Both modes share the same module system, the same `core.Core`
initialization, and the same lifecycle management. An agent is just a
master with most modules disabled.

## Consequences

**Positive:**

- **One artifact to build, sign, scan, and ship.** CI produces one
  binary per platform. GoReleaser, Docker image, install script — all
  publish exactly one thing.
- **Upgrades are coordinated.** An operator upgrades the master and the
  agents to the same version. There is no "v1.6 master talking to v1.4
  agent" matrix to test because the binary is identical.
- **Code reuse is automatic.** Every helper the master has — logging,
  config loading, retry, circuit breakers, `core.NewHTTPClient` — is
  already available to the agent because they are the same package graph.
- **Operators can toggle modes without re-downloading.** A node running
  as an agent can be promoted to master by restarting with different
  flags. Useful for failover drills and local testing.
- **Testing the protocol is cheap.** Integration tests start two
  `core.Core` instances in one process (or one subprocess each) and wire
  them together via the in-memory or loopback WebSocket dialer.

**Negative / trade-offs:**

- **Agent binaries are bigger than they need to be.** The agent ships
  unused code: the full API router, the React UI, the marketplace
  templates, the whole build pipeline. On a 20 MB binary this is
  acceptable; on a 200 MB one it would force a split.
- **Module gating is necessary.** Some modules must not start in agent
  mode (the API server, the ingress proxy on the master port, etc.).
  This is handled via mode checks in each module's `Start()` plus the
  top-level flag. We've kept this clean so far but it is a source of
  possible bugs.
- **Security surface is larger on agents.** Even though the API server
  is off, the code is still in the binary. A supply-chain compromise of
  one module affects both master and agent nodes. We mitigate with
  go vulncheck in CI and distroless runtime.

## Revisit if

- The binary grows past ~50 MB, at which point agent-only nodes would
  benefit from a slim build.
- Security compliance requires the agent to have a smaller attack
  surface than the master (e.g., a regulated environment where workers
  cannot contain any control-plane code).
- We add a fundamentally different agent type (e.g., a WASM edge runner)
  that cannot share the Go runtime.
