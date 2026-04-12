# ADR 0003 — Target Docker directly, not Kubernetes

- **Status:** Accepted
- **Date:** 2026-03-24
- **Deciders:** Ersin KOÇ

## Context

The deploy pipeline needs to run user workloads somewhere. The obvious
choices in 2026 are:

1. **Docker (via Docker Engine SDK)** — single daemon on each host,
   widely installed, simple model.
2. **Docker Swarm** — Docker's built-in cluster mode.
3. **Kubernetes** — the industry-standard orchestrator.
4. **Nomad** — HashiCorp's simpler alternative to k8s.
5. **Podman** — Docker-compatible, daemonless.

The target user is a solo developer or small team running 1–10 nodes and
deploying ~10–500 apps. They already have Docker on their VPS.

## Decision

**DeployMonster talks directly to Docker via the Docker Engine SDK.** It
does not require, assume, or integrate with Kubernetes.

Multi-node deployments are handled by the **master/agent architecture**
(see ADR 0007): a master process on the control node dispatches work over
WebSocket to agent processes running on worker nodes. Each agent uses its
local Docker SDK. No Swarm, no kubelet, no kube-proxy.

## Consequences

**Positive:**

- **Onboarding is ~60 seconds.** `curl … | bash`, start the binary, deploy
  an app. No `kubectl`, no `helm`, no etcd, no CNI plugin.
- **Resource footprint fits on a $5 VPS.** Kubernetes control-plane
  overhead alone exceeds 1 GB of RAM. DeployMonster + Docker fits in
  <200 MB.
- **We own the deployment model end-to-end.** Rolling updates, graceful
  shutdown, health checks, volume management, log streaming — all
  implemented directly against the Docker SDK with exactly the semantics we
  want. No fighting with k8s defaults or admission controllers.
- **Debugging is straightforward.** `docker ps`, `docker logs`, and a single
  Go binary are the whole mental model.
- **Tight integration with the build pipeline.** Building an image and
  running it are the same Docker connection; there is no "push to registry,
  pull on node" round-trip unless the user explicitly enables one.

**Negative / trade-offs:**

- **No free k8s ecosystem.** Users who want Prometheus Operator, Istio,
  cert-manager, Argo CD, or any other k8s-native tool will have to run
  DeployMonster alongside k8s, not inside it. We partially compensate via
  the Prometheus exporter, built-in ACME, and built-in reverse proxy.
- **No multi-master HA out of the box.** The control plane is a single
  process. Users who need HA can run a standby master + shared database,
  but it is not a shrink-wrapped feature the way k8s offers.
- **We reinvent pieces of orchestration.** Health probes, restart policies,
  rolling deploys, service discovery, load balancing — all had to be
  written. The circuit breaker, graceful shutdown, and auto-rollback modules
  exist because k8s would have given us equivalents for free.

## Revisit if

- Kubernetes becomes the default deployment environment even for solo
  operators (so far it has not, despite a decade of hype).
- A customer specifically needs to run DeployMonster *inside* k8s as a
  control plane for existing clusters. That is a different product and
  would be a v2.
- The master/agent protocol hits a scalability limit around ~100 nodes.
