# Soak test harness

This directory contains the DeployMonster soak-test harness for
Phase 5.3.6. It drives a low-intensity HTTP workload against a
running DeployMonster instance for hours (default: 24h) and samples
`/metrics/api` at a fixed interval to detect three slow drift modes
that manual smoke testing will not catch:

1. **Goroutine leak** — `go_goroutines` grows without bound
2. **Heap climb** — `go_memstats_heap_inuse_bytes` grows monotonically
   and GC cannot recover it
3. **DB bloat** — SQLite file size grows unboundedly during a
   read-only workload (implies a module is writing audit rows on
   every request)

The harness is a read-only client: it hits `/health`,
`/api/v1/health`, `/api/v1/marketplace`, `/api/v1/openapi.json`, and
`/metrics/api` in round-robin from two worker goroutines (≈10% of
the 20-worker peak used by the regression gate).

## Quick start

```bash
# Terminal 1: start the server with rate-limiting relaxed
./bin/deploymonster serve
# (or MONSTER_SERVER_RATE_LIMIT_PER_MINUTE=100000 for the soak run)

# Terminal 2: run the 5-minute smoke test
make soak-test-short
```

The smoke run lasts 5 minutes, samples every 15 seconds, and writes
`soak-smoke.json` next to the Makefile. On pass it prints
`PASS — no drift detected` and exits 0. On drift it prints a
per-gate breakdown and exits 2.

## Flags

| Flag | Default | Purpose |
|---|---|---|
| `-url` | `http://localhost:8443` | base URL of the API |
| `-duration` | `24h` | total soak duration |
| `-concurrency` | `2` | load-generator worker count (≈10% of peak) |
| `-sample-interval` | `1m` | `/metrics/api` sampling interval |
| `-warmup-fraction` | `0.10` | fraction of duration before drift gates engage |
| `-goroutine-multiplier` | `1.5` | fail if final goroutines > multiplier × baseline |
| `-heap-multiplier` | `2.0` | fail if final heap_inuse > multiplier × baseline |
| `-db-file` | `""` | path to SQLite file for bloat tracking |
| `-db-multiplier` | `2.0` | fail if final DB size > multiplier × baseline |
| `-out` | `soak-results.json` | final summary JSON path |
| `-trace` | `""` | optional JSONL trace path (one line per sample) |

## Output

The harness writes two files:

### `soak-results.json` (always)

Final summary, including:

- total duration, concurrency, sample count
- total request + error counts
- the post-warmup **baseline** sample (`go_goroutines`,
  `go_memstats_heap_inuse_bytes`, etc.)
- the **final** sample
- the drift gate thresholds used
- a `regressions` list (empty on pass)
- `passed: true | false`

### `soak-trace.jsonl` (optional, `-trace`)

One JSON line per sample — full time-series data that can be plotted
with `jq` + gnuplot, or imported into pandas/Grafana for offline
analysis. Useful when a 24-hour run fails and you need to see
**when** the drift started, not just the start-and-end values.

## Drift gates

A **baseline sample** is chosen as the first sample taken after
`warmup-fraction × duration` has elapsed. For a 24-hour run with a
0.10 warmup fraction, that is 2 hours and 24 minutes — enough time
for GC to settle, caches to warm, and steady-state connection
behavior to establish. For a 5-minute smoke test, the warmup window
is only 30 seconds, so expect slightly noisier results.

A **final sample** is the last sample taken before duration expires.

For each gate, the condition to fail is:

```
final > multiplier × baseline
```

- Goroutines: fails if `final > 1.5 × baseline`
- Heap in-use: fails if `final > 2.0 × baseline`
- DB file size: fails if `final > 2.0 × baseline` (only if `-db-file` set)

Multipliers are deliberately conservative. A healthy read-only
server should see goroutine and heap numbers oscillate within a
narrow band far below these thresholds. Tighten them if you want to
catch subtler drift (e.g., `-goroutine-multiplier=1.1` on a
production-shaped CI runner), or loosen them on noisy hardware.

## Running the full 24-hour soak

The 24-hour run is the Phase 5 exit criterion:

```bash
# One-time: raise rate limit for the soak window
# (rate_limit_per_minute: 100000 in monster.yaml, or set
#  MONSTER_SERVER_RATE_LIMIT_PER_MINUTE=100000)

./bin/deploymonster serve &
make soak-test
```

Expect roughly 1,440 samples (one per minute) and a handful of MiB
of trace log data. The final summary is durable; the trace is
optional (use `-trace=""` if disk is tight).

### What to do on failure

1. Inspect `soak-trace.jsonl` to see **when** the drift started.
   `jq '[.elapsed_seconds, .go_goroutines, .go_memstats_heap_inuse_bytes]
   | @csv' soak-trace.jsonl` produces a plottable CSV.
2. Reproduce with the failing workload locally (usually a
   significantly shorter run at higher concurrency will show the
   same pattern within minutes).
3. Take a pprof goroutine dump at the same elapsed time in the
   reproducer: `curl https://localhost:8443/debug/pprof/goroutine?debug=1`.
   Requires `server.enable_pprof: true` and an auth token.
4. Compare the leaked goroutines to the code paths listed in the
   dump — in DeployMonster's history, leaks have historically come
   from `bufio.Scanner` not being drained, `http.Response.Body` not
   being closed, and `time.Ticker` not being stopped on cancellation.

## Rate limiting

The default `rate_limit_per_minute: 120` will shut the soak harness
out within seconds (two workers at ~1,000 req/sec easily exceed
120/min). For any loadtest or soak run, raise it to something high
like 100,000 in your `monster.yaml`:

```yaml
server:
  rate_limit_per_minute: 100000
```

Or disable it entirely by setting it to `0` — but read the comment
in `internal/core/config.go:45` first, because `0` is special-cased
elsewhere.

## Relationship to the regression gate

| Tool | Duration | Purpose |
|---|---|---|
| `make loadtest-check` | 30s | Throughput regression gate against a committed baseline |
| `make soak-test-short` | 5m | Quick drift-detection smoke run, CI-friendly |
| `make soak-test` | 24h | Full soak — Phase 5 exit criterion |

The regression gate catches **fast** regressions (a refactor doubled
p95). The soak test catches **slow** regressions (a resource leak
that only becomes visible after millions of requests). Both are
needed — neither subsumes the other.
