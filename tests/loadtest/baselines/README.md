# Load-test baselines

This directory holds committed baseline results used by the regression
gate in `tests/loadtest/main.go`. Each JSON file is the output of a
single load-test run against a production-shaped DeployMonster instance
and represents the throughput + p95 latency the codebase is expected
to meet or beat.

Phase 5.3.5 of the roadmap requires that every CI run compare against
a committed baseline and fail on a >=10% regression on either axis,
per endpoint.

## Files

- `http.json` — canonical baseline for the default public read-only
  endpoint mix (`GET /health`, `GET /api/v1/health`,
  `GET /api/v1/marketplace`, `GET /api/v1/openapi.json`,
  `GET /login`). Regenerate when the production-shaped config
  changes, hardware changes, or after a deliberate perf win you want
  to lock in.

## How to regenerate a baseline

Baselines must be captured against a **production-shaped** server —
TLS on, ACME cache warm, the full marketplace registry loaded, real
DB, and no `-short` test paths. Running against `make dev` with an
empty SQLite file gives numbers that are not representative.

1. Start the server with the production-shaped config:

   ```bash
   ./bin/deploymonster -config monster.production.yaml
   ```

2. Wait for the system to warm up (ACME caches, marketplace index,
   first-pass DB statistics). 60 seconds is usually enough.

3. Run the load-test driver with `-save-baseline`:

   ```bash
   go run ./tests/loadtest \
       -url https://localhost:8443 \
       -duration 60s \
       -concurrency 50 \
       -save-baseline tests/loadtest/baselines/http.json
   ```

4. Inspect the resulting JSON. Sanity check: error rate should be
   zero, RPS on `/health` should be thousands, p95 on `/marketplace`
   should be tens of ms.

5. Commit the updated baseline with a descriptive message that
   explains *why* it is being regenerated. The commit message is the
   audit trail for every perf movement — "regenerate baseline: N+1
   query fix lands" or "regenerate baseline: upgraded CI runners to
   c7g.xlarge".

## How CI uses the baseline

The `loadtest-check` Makefile target runs the driver in comparison
mode:

```bash
make loadtest-check
# Equivalent to:
go run ./tests/loadtest \
    -url http://localhost:8443 \
    -duration 30s \
    -concurrency 20 \
    -baseline tests/loadtest/baselines/http.json
```

On pass, it prints `Baseline check PASSED` and exits 0. On regression,
it prints a per-endpoint breakdown and exits 2. Exit code 2 is
distinct from 1 so CI can distinguish a regression from an
infrastructure failure (server failed to start, network error, etc).

## Regression semantics

For each endpoint in the baseline, the current run fails if either:

- **Throughput regression:** `current_rps < baseline_rps * (1 - 0.10)`
- **Latency regression:** `current_p95 > baseline_p95 * (1 + 0.10)`

Exact-threshold runs (current == baseline * (1 ± threshold)) are a
**pass**, not a fail — this guarantees that a run reproducing the
baseline to the last microsecond never flaps. The threshold is
configurable via `-regression-threshold` if you need to tighten or
loosen it for a specific run.

Endpoints in the baseline that are missing from the current run are
treated as regressions (production surface area went dark). Endpoints
in the current run that are missing from the baseline are ignored —
they are new surface area the baseline has not yet seen.

## Hardware note

A baseline is only meaningful against the hardware that produced it.
Comparing a number captured on an M2 MacBook to a run on a
Hetzner CPX21 is noise. The CI workflow pins baselines to a specific
runner class (see `.github/workflows/loadtest.yml`) and regenerating
on a new runner class is one of the explicit triggers for a fresh
baseline commit.

## The committed baseline

`http.json` in this directory was captured on a Windows dev machine
running Go 1.26.1 against the server started from the repo root with
the default `monster.yaml` (ingress moved to ports 18080/18443 to
avoid the privileged-port requirement). It is a **seed baseline** —
useful for proving the regression-gate machinery works end-to-end,
but not authoritative for any other environment.

Before relying on the gate in your own CI, regenerate the baseline
on the target runner with `make loadtest-baseline` and commit the
result. Run-to-run variance on an unconstrained dev laptop is
commonly in the 10–15% range because of background processes and GC
noise, so the 10% default threshold may flap there. On a dedicated
CI runner (or a production-shaped VM with CPU pinning) that variance
typically drops to under 5% and the 10% gate becomes a reliable
indicator of real regressions.
