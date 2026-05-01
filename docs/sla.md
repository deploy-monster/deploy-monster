# Service-Level Agreements

This document commits DeployMonster to specific, measurable performance
and availability targets for the **single-master / single-tenant**
production topology. It exists because Phase 5 of the roadmap asked
for published numbers to optimize *against* — "can't optimize without
a target." The numbers below are the first publication; subsequent
revisions are tracked in `CHANGELOG.md` under a **Performance**
section.

> **Scope.** These SLAs apply to the **self-hosted single-binary**
> install described in `README.md`. Multi-master HA and managed-SaaS
> SLAs are explicitly out of scope until Post-1.0 (see § Out of scope).

---

## Authority

When this document disagrees with:

- **The measured baseline in `tests/loadtest/baselines/http.json`** —
  the baseline wins. SLAs target what the code *will* hit on
  production hardware, not what the current dev-box run produced;
  revise the doc, not the baseline.
- **The roadmap's "SLA" item** — this document is the fulfilment of
  that item.
- **A marketing page** — this document wins. The marketing page
  tracks the spec, not the other way round.

---

## How to read the targets

Each target is expressed as a **p95 objective** (the 95th-percentile
latency or throughput must be at or better than the target under the
stated load). p99 is reported for context but is **not** a pass/fail
gate — tail-latency is dominated by GC pauses and scheduler noise
that should be surfaced as advisory, not as a block on releases.

Targets are baseline-relative: the regression gate (
`tests/loadtest/main.go`, invoked as `make loadtest-check`) fails CI
when current run is >= 10% worse on either RPS or p95 versus the
committed baseline, per endpoint. The numbers in this doc are the
*absolute* floor; the baseline is the *relative* floor. Breaking
either fails.

---

## Measurement environment

All numbers below are expressed against the reference hardware
profile:

- **CPU:** 4 vCPU (Intel/AMD Zen 3-class or better, or Apple M-series)
- **Memory:** 4 GB RAM minimum, 8 GB recommended for >50 apps
- **Disk:** Local SSD/NVMe (not network-attached block storage)
- **OS:** Linux x86_64 kernel 5.10+ (the single-binary target)
- **Deployment shape:** Single master, SQLite store, ACME cache warm,
  marketplace registry loaded (91 built-ins), no agents

This matches a 4-vCPU Hetzner CPX31 / Digital Ocean c-4 / Vultr
Cloud-Compute 4-vCPU — in other words, the mid-tier droplet a small
team would pick for their first production install.

Numbers captured on the repo's seed baseline (Windows dev machine,
Go 1.26.1, `http.json`) are higher than the targets because the dev
machine is unconstrained. The targets are what reference hardware
must sustain; the dev-box baseline is what the code *can* do on a
hot machine — use it as a ceiling, not a floor.

---

## HTTP API — read path

Measured under 20 concurrent clients, 60-second run, public
read-only endpoint mix (`GET /health`, `GET /api/v1/health`,
`GET /api/v1/marketplace`, `GET /api/v1/openapi.json`,
`GET /login`).

| Endpoint | Target p95 | Target RPS | Seed p95 | Seed RPS |
|---|---:|---:|---:|---:|
| `GET /health` | **< 50 ms** | **>= 150 req/s** | 27.8 ms | 331.6 |
| `GET /api/v1/health` | **< 50 ms** | **>= 150 req/s** | 27.4 ms | 331.5 |
| `GET /api/v1/marketplace` | **< 100 ms** | **>= 150 req/s** | 24.5 ms | 331.5 |
| `GET /api/v1/openapi.json` | **< 100 ms** | **>= 150 req/s** | 24.1 ms | 331.4 |
| `GET /login` | **< 100 ms** | **>= 150 req/s** | 22.9 ms | 331.4 |
| **Aggregate** | — | **>= 750 req/s** | — | 1657.4 |

- **Target ceiling** is set at roughly **half** the dev-box seed
  baseline so reference hardware (smaller, noisier) has headroom
  without the test being vacuous. A 4-vCPU VM under load is well
  inside the target.
- **Target floor RPS** is the aggregate each endpoint must deliver
  at concurrency=20. Failing this on reference hardware is a release
  blocker.

---

## HTTP API — mutation path

No committed baseline yet — the seed harness runs reads only.
Targets are derived from the 30 s request timeout (`BodyLimit(10 MB)`
+ `Timeout(30 s)` middleware) and the observed cost of the hot
mutation endpoints in the unit-test suite.

| Operation | Target p95 |
|---|---:|
| `POST /api/v1/apps` (small app create, no build) | **< 500 ms** |
| `PUT /api/v1/apps/{id}/env` (env-var write) | **< 200 ms** |
| `POST /api/v1/deploy` (webhook-to-queue enqueue, not build) | **< 300 ms** |
| `POST /api/v1/auth/login` (incl. bcrypt cost 13) | **< 800 ms** |
| `POST /api/v1/auth/refresh` | **< 100 ms** |

Login latency is dominated by bcrypt cost 13 (intentional — see
ADR 0008). The 800 ms ceiling assumes reference-hardware CPU; on
slower hardware the cost scales linearly and the target is relaxed
by the CPU ratio, not the absolute number. Do not lower bcrypt
cost to meet a latency target — adjust the target instead.

---

## Deploy pipeline

Deploys are inherently variable (clone size, build time, image
pull). The SLA is on **overhead**, not on total wall-clock time.

| Stage | Target overhead |
|---|---:|
| Webhook received → job queued | **< 500 ms p95** |
| Job queued → clone started | **< 1 s p95** |
| Clone complete → builder running | **< 2 s p95** |
| Build complete → container running | **< 3 s p95** |
| **Control-plane overhead total** (excluding clone + build + pull) | **< 8 s p95** |

Build and image-pull time itself is not SLA-bound — those are
user-controlled (pinned base images, cache layers, registry
geography) and can range from 10 s to 20 min. What DeployMonster
owns is the time between user-controlled stages.

---

## Concurrency

| Axis | Target |
|---|---:|
| Concurrent in-flight deploys | **>= 10** |
| Concurrent HTTP clients sustained | **>= 50** |
| Apps running per master | **>= 100** |
| Tenants per master | **>= 50** |
| Agents connected per master | **>= 20** |

These are *capacity* targets, not *throughput*. A master meeting
this concurrency ceiling with the HTTP-API RPS floor above is the
shape of a healthy single-master install at the 4-vCPU tier.

---

## Availability

| Metric | Target (rolling 30 d) |
|---|---:|
| Master uptime (process alive, `/health` returns 200) | **>= 99.5 %** |
| Master MTTR after unplanned restart | **< 30 s** |
| Zero-downtime upgrade (planned) | **100 %** — no 5xx to users during rolling restart |

99.5 % on 30 days = 3 h 36 m of permissible downtime per month.
This is a conservative target for a single-binary single-master
install without HA. Operators wanting higher availability run
a front-door load balancer with DNS failover to a standby master
(documented in `docs/runbook.md § Disaster recovery`); that
configuration is not bound by this SLA.

---

## What triggers a re-baseline

Any of the following forces a fresh `tests/loadtest/baselines/http.json`
capture and a revisit of this document:

1. Reference-hardware CPU/memory spec change.
2. Go toolchain minor-version bump (1.N → 1.N+1).
3. Any middleware addition that runs on every request.
4. Store-interface implementation change (SQLite → Postgres).
5. A deliberate perf win you want to lock in ("regenerate baseline:
   N+1 query fix lands" — see `baselines/README.md`).

Cosmetic changes (new endpoint added, new tenant field, new log line)
do **not** trigger a re-baseline.

---

## Verification

Run the regression gate locally:

```bash
make loadtest-check
# Equivalent to:
go run ./tests/loadtest \
    -url http://localhost:8443 \
    -duration 30s \
    -concurrency 20 \
    -baseline tests/loadtest/baselines/http.json
```

Exit code 0 = pass; 2 = regression (10 %+ off baseline on RPS or
p95); 1 = infrastructure failure (server failed to start, network
error, etc). The distinction matters for CI — only exit-2 is a
release blocker.

Nightly CI run: `.github/workflows/loadtest-nightly.yml`. Daily run
at `47 3 * * *` captures 30-day trend data as artifacts with 30-day
retention. This is the source of truth for "are we at SLA over
time" questions.

---

## Out of scope (Post-1.0)

- **Multi-master HA SLA.** Requires Postgres-backed store + shared
  session cache + front-door LB; tracked as a Beyond-1.0 roadmap
  item.
- **Managed-SaaS SLA.** Cloud-operated DeployMonster is a separate
  product with its own uptime commitment; this doc is for
  self-hosted installs.
- **Build-throughput SLA.** The builder layer is resource-bounded
  by the host and the user's build scripts; DeployMonster does not
  commit to build duration.
- **Agent-fleet SLA at scale beyond 20 agents per master.** Current
  ceiling is a soft limit — the WebSocket pool is healthy up to
  ~50 agents, but the agent-enrolment + heartbeat-timeout tuning
  was only validated up to 20.

Each of these graduates to a tracked SLA when the underlying
capability ships, not before.

---

## History

| Date | Change | Authoring commit |
|---|---|---|
| 2026-04-17 | First publication — sprint(3) roadmap close of Phase 5 "Publish SLAs". Numbers derived from `tests/loadtest/baselines/http.json` seed plus conservative reference-hardware ceilings. | _see git log_ |

When this table grows past ~10 rows, move entries older than two
majors into `CHANGELOG.md` under the relevant Performance section
and link here.
