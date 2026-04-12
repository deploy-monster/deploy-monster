# ADR 0004 — Use the pure-Go SQLite driver (modernc.org/sqlite)

- **Status:** Accepted
- **Date:** 2026-03-24
- **Deciders:** Ersin KOÇ

## Context

Go has two well-known SQLite drivers:

1. **`github.com/mattn/go-sqlite3`** — the classic choice. C bindings via
   cgo, links against the real `sqlite3.c`.
2. **`modernc.org/sqlite`** — pure Go, transpiled from the C source of
   SQLite using the `ccgo` tool. No cgo.

We need to pick one for the `database/sql` layer behind
`internal/db/sqlite.go`.

## Decision

**Use `modernc.org/sqlite`.** No cgo anywhere in the build chain.

## Consequences

**Positive:**

- **Cross-compilation is trivial.** `GOOS=linux GOARCH=arm64 go build` just
  works. No `CC=aarch64-linux-gnu-gcc`, no sysroot fiddling, no cross
  toolchain. This matters a lot for the multi-arch Docker image
  (amd64 + arm64) and for GoReleaser publishing Linux, macOS, and Windows
  binaries from one CI runner.
- **The Docker image can use distroless static.** With cgo we would need
  glibc (or at minimum musl) at runtime, which means alpine or
  debian-slim. `modernc.org/sqlite` lets us target
  `gcr.io/distroless/static-debian12:nonroot`, which ships nothing but
  ca-certificates and the binary (see ADR-compatible Dockerfile).
- **Build reproducibility.** Pure-Go builds with `-trimpath` and
  `-buildid=` are byte-reproducible across machines. A cgo build depends
  on the host C compiler version and libc, which breaks reproducibility.
- **Single binary distribution.** Users don't need libsqlite3 installed.
  `deploymonster` runs on a minimal VPS with nothing else.
- **go test ./... is faster.** No C compilation in the test loop.

**Negative / trade-offs:**

- **Slightly slower than the C driver.** Benchmarks we've run show
  `modernc.org/sqlite` is within 10–20% of `mattn/go-sqlite3` on our
  workload. For our `GetApp` at 41 µs/op, this is not material. For a
  write-saturated workload it would be, but SQLite's single-writer
  constraint dominates well before the driver overhead matters.
- **Larger binary.** The pure-Go driver adds ~3 MB because it ships the
  transpiled SQLite source. For a 20 MB binary, acceptable.
- **Less mainstream.** The cgo driver has more Stack Overflow answers.
  `modernc.org/sqlite` is maintained and mature, but newer contributors
  occasionally expect the cgo one.

## Revisit if

- The modernc driver falls behind SQLite upstream by more than one minor
  version for a feature we need.
- A performance regression shows the driver is the bottleneck (not SQLite
  itself).
- The ecosystem shifts to another pure-Go alternative that is strictly
  better.
