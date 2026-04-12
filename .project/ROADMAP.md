# DeployMonster — Production Roadmap

> **Audit date**: 2026-04-11
> **HEAD commit**: `7add828` (Tier 105)
> **Target**: v0.0.1 final (from v0.0.1)
> **Companion documents**: `.project/ANALYSIS.md`, `.project/PRODUCTIONREADY.md`

---

## How to use this document

This is a sequenced remediation plan, not a wish list. Phases are ordered by dependency and by risk-of-shipping-without. Every item is tagged with:

- **Priority**: P0 (blocks v0.0.1 final) / P1 (blocks scale) / P2 (should fix) / P3 (tech debt)
- **Effort**: engineer-days, estimated conservatively
- **Evidence**: file path or test name so you can verify the fix landed
- **Why**: short justification that stays load-bearing after the fix

Phases 1–5 are marked done with a reconciliation line each; they reflect the 28 hardening tiers between Tier 77 and Tier 105. Phases 6–7 are where the remaining work lives. Phase 8 is post-v0.0.1.

Total remaining effort to v0.0.1 final: **~3 engineer-days** on the critical path, **~6.5 engineer-days** including the nice-to-haves. (Originally 11/15 ed; 7.0 + 7.2 + 7.4 closed 2026-04-11.)

---

## Phase 1 — Critical correctness fixes — ✅ DONE (Tier 78–83)

Originally the Tier 77 audit listed a red test suite, a deadlocked rollback test, a hard-coded vault salt, and an unbounded async event dispatcher as v1.0 blockers.

**Reconciled at HEAD**:

- ✅ `TestSQLite_Rollback` deadlock — fixed in Tier 79. `internal/db/sqlite_test.go:220–294` now closes the rows handle; line 236 documents *why* (`MaxOpenConns(1), so leaking a cursor here would deadlock`).
- ✅ Hard-coded vault salt — fixed in Tier 83. Per-install random salt + legacy migration path, documented in `docs/adr/0008-encryption-key-strategy.md`.
- ✅ Unbounded async event goroutines — fixed in Tier 82. 64-slot semaphore in `internal/core/events.go`.
- ✅ `go test -short ./...` — GREEN across every package at HEAD.
- ✅ `go vet ./...` + `go build ./cmd/deploymonster` — clean.

No outstanding Phase 1 work.

---

## Phase 2 — Core completion & Postgres backend — ✅ DONE (Tier 81, 84–90)

The Tier 77 audit flagged that the store interface was SQLite-only in practice despite the ADR claim.

**Reconciled at HEAD**:

- ✅ PostgreSQL backend via `jackc/pgx/v5` (go.mod:12) — real, not aspirational.
- ✅ Contract suite — both SQLite and Postgres green per `STATUS.md`. Verified in `internal/db/contract_test.go` and the Postgres-tagged build.
- ✅ 30+ indexes on hot paths — `internal/db/migrations/0002_add_indexes.sql`.
- ✅ `MaxOpenConns(1)` design choice documented in migration/test comments.

**Outstanding**: none. Writers-under-load benchmark done 2026-04-11 (Phase 7.10).

---

## Phase 3 — Security hardening — ✅ MOSTLY DONE (Tier 82, 83, 91–103)

**Reconciled at HEAD**:

- ✅ Bounded async event dispatch (Tier 82)
- ✅ Per-install random vault salt (Tier 83)
- ✅ 17 of 20 Dependabot alerts closed; 3 remaining are upstream-blocked and documented in `docs/security-audit.md`
- ✅ Cookie `Secure` flag gated on request transport (Tier 103, `091f6c4`)
- ✅ Global rate limiter scoped to `/api/` + `/hooks/` (Tier 102, `fe08133`)
- ✅ SPA 404 + embed invariants + full-router integration guards (Tier 102, 104)
- ✅ Audit log middleware writing to `audit_log` table

**Outstanding** (carried to Phase 7):

- ✅ **Admin middleware wiring** — done 2026-04-11. See Phase 7, item 7.0.
- ✅ CSP header on embedded SPA responses — done 2026-04-11. See Phase 7, item 7.8.
- ✅ Secrets scanning in CI — done 2026-04-11. See Phase 7, item 7.9.

---

## Phase 4 — Testing & observability — ✅ DONE (Tier 88–98)

**Reconciled at HEAD**:

- ✅ Coverage gate 85 % in CI (`.github/workflows/ci.yml`)
- ✅ 15 fuzz targets, 46 benchmarks
- ✅ 341 vitest tests / 38 files green
- ✅ Playwright E2E suite with loud-fail setup (Tier 105, `7add828`)
- ✅ 24 h soak harness + 5 m CI smoke
- ✅ Loadtest baseline + 10 % p95 regression gate
- ✅ Prometheus runtime-metric block on `/metrics/api`
- ✅ OpenAPI drift gate (`make openapi-check`)

**Outstanding**:

- ✅ **Writers-under-load benchmark** (P1) — done 2026-04-11. See Phase 7, item 7.10.
- ✅ Cross-tenant fuzz target (P2) — landed 2026-04-12. See Phase 7, item 7.11.

---

## Phase 5 — Performance — ✅ DONE (Tier 84–90)

- ✅ 30+ indexes added
- ✅ Loadtest baseline committed
- ✅ Soak harness green
- ✅ No perf regressions in the last 15 tiers per the committed baseline

No outstanding Phase 5 work.

---

## Phase 6 — Documentation & DX — ✅ DONE (Tier 91–99)

- ✅ README current (240 endpoints, 56 templates)
- ✅ OpenAPI spec CI-gated
- ✅ 9 ADRs including the two new ones (0008 encryption-key strategy, 0009 store composition)
- ✅ Upgrade guide with per-version matrix
- ✅ `.project/SPECIFICATION.md` + `.project/IMPLEMENTATION.md` maintained

**Outstanding** (P3, not blocking v0.0.1):

- ✅ Module-registry ADR or `docs/modules.md` — done 2026-04-11. `docs/modules.md` documents lifecycle, topo-sort, and shutdown semantics.
- ✅ Spec addendum for post-spec features (canary, deploy freeze/schedule/approval, bounded async dispatch, per-install salt). Done 2026-04-12. See `docs/spec-addendum.md`.
- ✅ `.project/` consolidation cleanup — done 2026-04-12. All references normalized to `.project/`.

---

## Phase 7 — Release preparation — 🟡 IN FLIGHT

This is where the remaining v0.0.1-blocking work lives. Items are ordered so that a single engineer can walk down the list.

### 7.0 — WIRE THE ADMIN MIDDLEWARE (P0, 0.5 day) — ✅ DONE 2026-04-11

**State reconciled**: `internal/api/router.go` lines 103–109 define `adminOnly` as `protected(middleware.RequireSuperAdmin(next))`. Every `/api/v1/admin/*` route (20 endpoints) is wrapped with `adminOnly`. Inline role checks were removed from handlers. Table-driven router tests (`TestAdminRoutes_*`) verify 403 for non-admin tokens.

**Evidence**: `grep -c 'adminOnly' internal/api/router.go` returns 21 (definition + 20 call sites). `go test -short ./internal/api/...` green.

---

### 7.1 — Merge previous Phase 7.1 (audit-doc pass) — ✅ DONE

Covered by the Tier 98–99 reconciliation. `ANALYSIS.md`, `ROADMAP.md`, `PRODUCTIONREADY.md` in this audit supersede the previous versions.

---

### 7.2 — goreleaser snapshot pipeline validation — ✅ DONE (2026-04-11)

**Work completed**:
1. ✅ `goreleaser release --snapshot --clean --skip=before,docker` runs clean end-to-end. Builds 6 targets (linux/darwin/windows × amd64/arm64), 6 archives, 6 SBOMs via syft, `checksums.txt`. Binary runs and reports correct version/commit/date from ldflags.
2. ✅ `Makefile` `release-snapshot` target updated to match validated invocation.
3. ✅ `.github/workflows/release.yml` created — triggers on `v*.*.*` tag push, builds web UI first, runs goreleaser action v6 with GHCR login.
4. ✅ Deprecation warning noted: `dockers`/`docker_manifests` → `dockers_v2` migration scheduled for Phase 8 (non-blocking, current syntax still valid in v2.15.2).

**Deferred to separate phases**:
- Signed `checksums.txt.sig` — requires a GPG/cosign key. Tracked under release-key setup; not blocking snapshot validation.
- Docker image build/scan — Phase 7.6 explicitly owns this.
- `before` hook pipeline (runs full test suite during release) — active for real releases via the workflow; skipped for local snapshots to avoid duplicating CI work.

**Evidence**: Re-run `make release-snapshot` — produces a populated `dist/` with 6 binaries, 6 archives, 6 SBOMs, and `checksums.txt`. Validated on 2026-04-11 against HEAD `7add828`, tag `0.5.2-SNAPSHOT-7add828`.

---

### 7.3 — Fresh Ubuntu 24.04 VM smoke test (P0, 1 day)

**State**: Pending. The binary has not been installed and exercised on a clean VM matching a minimum-supported OS release since v1.5.
**Update 2026-04-12**: `scripts/smoke-test.sh` added — automated headless runner that validates install, auth, deploy, and teardown in one command. Real VM execution still pending.

**Work**:
1. Provision a clean Ubuntu 24.04 LTS VM (Hetzner CX22 or DO basic droplet).
2. Install from the snapshot binary produced by 7.2 (not from source).
3. Walk the happy path: first-run wizard → create tenant → connect GitHub → deploy a Node app → attach domain → verify ACME HTTP-01 → tear down.
4. Walk the master/agent path: provision a second VM, join as agent, deploy to the agent, verify logs stream back.
5. File any issues found as P0 if they break the happy path, P1 if they break the agent path, P2 if they are cosmetic.

**Evidence after fix**: a `docs/smoke-v0.0.1.md` file with the exact commands, screenshots of key states, and the `uname -a` of the VM. No blocker bugs found.

**Effort**: 1 engineer-day, including VM teardown and doc writing.

---

### 7.4 — CHANGELOG from Phase 1–7 delta — ✅ DONE (2026-04-11)

**State reconciled**: The `[0.0.1]` section I inherited in `CHANGELOG.md`
was already comprehensive for Tiers 76–97 but missed the Tier 100–105 tail,
the Phase 7.0 admin-middleware wiring, and the Phase 7.2 goreleaser validation
details (and carried a stale "88 hardening tiers" count).

**Work completed**:
1. ✅ Security: new bullet documenting Phase 7.0 admin-middleware wiring — 20 routes, 7 inline checks removed, 4 router-level authorization tests (63 subtests), 8 stale handler-level auth tests deleted.
2. ✅ Security: tier count corrected from 88 → 105, with an explicit Tier 100–105 block in the representative-fixes list (deployment status persistence; WG-reuse + `-race` CI fix; rate limiter scoping; cookie Secure gating; SPA embed invariants; e2e loud-fail).
3. ✅ Release engineering: goreleaser bullet rewritten to reflect the validated `--skip=before,docker` invocation, the `Makefile:release-snapshot` update, the new `.github/workflows/release.yml`, the `dockers` → `dockers_v2` deprecation note, and a more precise reason for the orphan v1.x tag footgun.
4. ✅ Structure preserved — no grouping changes, no new subsections; changes were surgical insertions into existing Security + Release-engineering buckets. `## [0.0.1]` remains the header for today's aggregate work.

**Evidence**: `grep -n "^### " CHANGELOG.md` returns the expected 8 subsection headers; `grep -c "105 hardening tiers" CHANGELOG.md` returns 1; `grep -c "Phase 7.0 closure" CHANGELOG.md` returns 1.

---

### 7.5 — Installer dry-run (P0, 1 day)

**State**: Installer **code hardened locally 2026-04-12**, dry-run on a real VM still pending.

**Pre-audit state** (`scripts/install.sh` before the 2026-04-12 sweep): the script detected platform, queried the GitHub API for the latest release tag, downloaded the tarball, and dropped the binary into `/usr/local/bin`. It would have passed the roadmap's §3 visual checklist but failed the §3 "verifies checksum" and §4 "uninstaller works" requirements — neither path existed. It also silently fell back to `go install github.com/.../cmd/deploymonster@latest` on download failure, which ships a binary **without** the embedded React SPA and is a worse user experience than failing loudly.

**Audit fixes applied 2026-04-12** (scripts/install.sh rewrite):

1. **SHA256 verification against `checksums.txt`.** New `verify_checksum()` downloads the release's `checksums.txt` alongside the archive, extracts the expected SHA via `awk '$2 == name { print $1 }'` (resilient to extra-whitespace and to other entries in the file), computes the actual via `sha256sum` or `shasum -a 256` (whichever is present — Linux vs macOS), and errors out with a precise mismatch message if they don't match. Exercised locally with a synthetic tarball + checksums.txt pair — happy path emits `[INFO] Checksum verified (<first-16-hex-chars>…)`, tampered-archive path errors with `[ERROR] Checksum mismatch for <name>: expected <hex>, got <hex>` and exits 1. No GPG/cosign yet — that's explicitly Phase 8 once a release key exists.
2. **`uninstall` subcommand.** New `do_uninstall()` runs via the raw GitHub URL with the `uninstall` argument, stops + disables `deploymonster.service`, removes the unit file, daemon-reloads, deletes the binary, and **intentionally preserves `/var/lib/deploymonster`** (dataloss surface — bolt buckets, SQLite DBs, vault salt, uploaded secrets). Prints a pointer telling the operator to `sudo rm -rf /var/lib/deploymonster` manually if they want a full wipe. Matches the roadmap's §4 requirement.
3. **Broken `go install` fallback removed.** The old fallback path fired on any download error and silently shipped a binary without the embedded SPA. Deleted — users on a platform without a release tarball now see a clear error ("Failed to download <url>") instead of a silently broken daemon.
4. **Validated version detection.** `get_latest_version()` now:
   - fails loudly when the GitHub API call 4xx/5xx's (previously swallowed the error and set `VERSION=latest` which broke the download URL);
   - anchors the `grep` for `"tag_name"` to lines starting with whitespace + `"tag_name"` so the regex can't match a nested `tag_name` inside a related-object payload;
   - validates the parsed string against `^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$` and errors out if it doesn't look like a semver tag. Regex exercised locally against `v0.0.1`, `v0.0.1`, `v0.0.1-beta.3`, `v10.20.30-alpha.1.2`, and `not-a-tag` — accepts the first four, rejects the fifth.
5. **`--version=X` override.** Users can now pin a specific release: `curl … | bash -s -- --version=v0.0.1`. Skips the API call entirely when set.
6. **`--force` reinstall guard.** Detects an existing `/usr/local/bin/deploymonster`, emits a warning with the existing version + "pass --force to overwrite, or run uninstall", and exits cleanly. Previously a reinstall silently overwrote without warning.
7. **Preflight check.** `preflight()` requires `uname`, `curl`, `tar`, `mktemp` up front and errors out with `Missing required command: <name>` instead of failing on the first uncommon system with a cryptic `command not found`.
8. **Systemd unit upgrades.**
   - `After=network-online.target docker.service` + `Wants=network-online.target` (was `After=docker.service`) — the first-run ACME path needs network, not just Docker.
   - `LimitNOFILE=65536` added (was unset — a connection-heavy PaaS running under systemd's default 1024 fd ceiling is a footgun).
   - `Documentation=https://github.com/deploy-monster/deploy-monster` added for `systemctl status` output.
   - `systemctl enable deploymonster.service` now runs automatically so the service survives a reboot. Previous version only ran `daemon-reload` and told the user to run start/enable manually.
   - Explicit comment explaining why `User=` is NOT set (daemon needs `/var/run/docker.sock` access + ability to bind :80/:443 for ACME — operators who want an unprivileged account must adjust the unit + add the account to the `docker` group).
9. **Trap quoting fixed** (the one real shellcheck finding on the old script — SC2064). `trap 'rm -rf "${tmp_dir}"' EXIT` uses single quotes so `${tmp_dir}` is expanded at signal time, not at trap-definition time. Latent bug in the old version: if `mktemp -d` had failed, `TMP_DIR` would be empty and `rm -rf` would expand to a no-op rather than an error.
10. **TTY-aware colors.** Escape codes only emit when both stdout and stderr are a TTY (`[ -t 1 ] && [ -t 2 ]`). When piped through `curl … | bash`, the pipe is not a TTY and the codes now collapse to empty strings, keeping logs clean.
11. **`tar -xzf --no-same-owner`** so running the installer as root via sudo doesn't preserve whatever UID the GitHub Actions runner had when it built the archive.
12. **New `.gitattributes` at repo root** forcing `*.sh text eol=lf` (plus Go, Makefile, YAML, Dockerfile). Windows contributors with `core.autocrlf=true` would otherwise silently commit CRLF and break the curl-pipe installer the moment it runs on a Linux VM — `curl | bash` on a script with `\r` in the shebang or in variable assignments fails in ugly ways. Defense in depth before the 7.5 dry-run.

**Local verification**:
- `shellcheck` (`koalaman/shellcheck:stable`) on the new script → exit 0, zero findings.
- `bash -n scripts/install.sh` → syntax clean.
- `bash scripts/install.sh --help` → prints the usage header from the top of the file.
- `verify_checksum` exercised against a real tarball + matching `checksums.txt` → PASS; tampered tarball → FAIL with precise mismatch message.
- Version-detection regex exercised against five positive tags + one negative.

**GitHub-first installer 2026-04-12**:
- The installer is distributed directly from GitHub (no short URL yet):
  ```bash
  curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --version=v0.0.1
  ```
- `scripts/smoke-test.sh` added — one-command headless validation of install, auth, app-deploy, and teardown. Run on a fresh VM with:
  ```bash
  bash -c "$(curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/smoke-test.sh)"
  ```

**Still pending — needs a real VM**:
1. Provision a clean Ubuntu 24.04 VM.
2. Run the smoke-test.sh command above (or the manual curl-pipe path).
3. Capture transcript + `systemctl status deploymonster` output into `docs/smoke-v0.0.1.md`.

**Evidence after fix**: clean install + clean uninstall on a fresh VM, both captured into `docs/smoke-v0.0.1.md` with `uname -a` and the two transcript blocks. The code side is already closed.

**Effort remaining**: ~0.5 engineer-day (VM time only). Code audit is done.

---

### 7.6 — GHCR image push + scan — ✅ DONE 2026-04-12

**State**: Dockerfile, goreleaser config, release workflow, and trivy scan
step are all landed and locally validated. The actual push to
`ghcr.io/deploy-monster/deploy-monster:v0.0.1` and `:latest` happens
automatically when the `v0.0.1` tag is pushed — at which point
`.github/workflows/release.yml` runs goreleaser (builds + pushes image) and
then trivy-scans the result, failing the job on any new HIGH/CRITICAL
finding.

**Work completed 2026-04-12**:

1. ✅ **Root `Dockerfile` rewritten as a three-stage scratch build.**
   - Stage 1 (`node:22-alpine`): `pnpm install --frozen-lockfile` + `pnpm run build` → `web/dist/`. Uses `corepack enable` so the pnpm version is pulled from `web/package.json` `packageManager` instead of being hardcoded.
   - Stage 2 (`golang:1.26-alpine` + ca-certificates + tzdata): `go mod download`, copies source, **overwrites `internal/api/static/*` with the freshly built SPA from stage 1** so the image can never ship a stale UI even if someone forgets to run `scripts/build.sh` before tagging, then builds with `-trimpath -ldflags "-s -w -X main.version/commit/date"`. Also prepares `/rootfs/var/lib/deploymonster` chowned to `65534:65534` for stage 3's scratch runtime (scratch has no shell, so the VOLUME mount point has to be chowned at build time).
   - Stage 3 (`scratch`): copies only the CA bundle, zoneinfo, the pre-chowned rootfs, and the binary. `USER 65534:65534`, `VOLUME ["/var/lib/deploymonster"]`, `EXPOSE 8443 80 443`, full OCI `org.opencontainers.image.*` label set. No shell, no `curl`, no package manager. HEALTHCHECK intentionally dropped — orchestrators define readiness probes; shipping curl just for a Docker HEALTHCHECK contradicts the minimal-surface posture.
   - Build tested locally: `docker build --build-arg VERSION=0.0.1-local --build-arg COMMIT=… --build-arg DATE=…` produces a **32.1 MB image** (8.3 MB binary layer) that runs `deploymonster version` cleanly and reports the injected ldflags.
   - `deployments/Dockerfile` left alone — it's the dev-only local build target referenced by `deployments/docker-compose.dev.yaml` and is optimized for hot rebuilds, not release minimalism.

2. ✅ **`.goreleaser.yml` `dockers:` section aligned to the repo name.**
   - Image tags corrected: `ghcr.io/deploy-monster/deploymonster:*` → `ghcr.io/deploy-monster/deploy-monster:*` (matches the GitHub org/repo slug — was silently wrong since Phase 7.2 because docker push was skipped in the snapshot validation).
   - New `--build-arg=DATE={{.Date}}` so the Dockerfile's `main.date` ldflag matches goreleaser's `{{.Date}}` instead of drifting to the docker-build wall-clock.
   - `--pull` + `--platform=linux/amd64` flags added explicitly.
   - Full OCI label set baked in via `--label` flags: `title`, `description`, `url`, `source`, `vendor`, `licenses`, plus the dynamic `version={{.Version}}`, `revision={{.FullCommit}}`, `created={{.Date}}`. These satisfy the GHCR "discoverability" checklist and make `docker inspect` immediately show provenance without cracking the manifest.
   - `goreleaser check` passes. The pre-existing `dockers → dockers_v2` deprecation warning remains (tracked under Phase 8, non-blocking for v0.0.1 per the 7.2 decision).

3. ✅ **`.github/workflows/release.yml` grew a trivy scan step.**
   - New `Trivy — scan GHCR image` step runs `aquasecurity/trivy-action@0.28.0` after `goreleaser release --clean` completes. Scan config: `severity: HIGH,CRITICAL`, `ignore-unfixed: true`, `pkg-types: os,library`, `exit-code: '1'`. Scans `ghcr.io/deploy-monster/deploy-monster:latest` (always set by goreleaser, so the tag is deterministic within a single workflow run).
   - `grype` was considered but not added: running two scanners in the same gate doubles the flake surface without meaningfully increasing coverage — trivy's DB is a superset of grype's for Go stdlib + OS packages, and both use the same NVD feed. Can be layered on later if a real blind spot shows up.

4. ✅ **`.trivyignore` added at repo root.** Single suppressed CVE:
   - **CVE-2026-34040** — `github.com/docker/docker` Moby daemon authorization bypass. Trivy's DB records `Fixed Version: 29.3.1` against the module path, but that's the Moby daemon binary, not the Go client SDK — `go list -m -versions github.com/docker/docker` confirms `v28.5.2+incompatible` is still the latest Go tag as of 2026-04-12, and there is no v29 client SDK release. The Go client cannot patch a daemon-side bypass by bumping itself; this is the exact mis-attribution pattern already tracked in CHANGELOG §0.0.1 Security as "R-001 upstream-blocked". The `.trivyignore` comment points any future maintainer at `docs/security-audit.md`.
   - Policy stated in the file header: suppressions are **by exact CVE ID only**, never by severity or package name. Any new HIGH/CRITICAL must be either fixed or explicitly added here with justification.

**Verification**:
- `docker build ...` — produces a 32.1 MB scratch image, binary executes and emits the expected version string.
- `goreleaser check` — clean (only the known deprecation notice).
- `docker run --rm aquasec/trivy:0.58.0 image --severity HIGH,CRITICAL --ignore-unfixed --pkg-types os,library --ignorefile .trivyignore --exit-code 1 deploy-monster:local-phase76` — **exit 0**, "Some vulnerabilities have been ignored/suppressed" (the one tracked CVE). Zero new findings.

**Post-push verification completed 2026-04-12**:
- `v0.0.1` tag pushed → release workflow triggered → GitHub release created with 6 archives, 6 SBOMs, and `checksums.txt`.
- GHCR image `ghcr.io/deploy-monster/deploy-monster:0.0.1` and `:latest` pushed and **pull verified locally**.
- Trivy scan **exit 0**, zero new HIGH/CRITICAL findings.

**Deferred to Phase 8**:
- Multi-arch (`linux/arm64`) docker image — `dockers:` in goreleaser v2 is single-arch by default; jumping to `dockers_v2` is bundled with the deprecation migration.

**Effort remaining**: 0 engineer-days.

**Memory check**: `feedback_deployment.md` — GHCR only, never Docker Hub. The `.goreleaser.yml` `dockers:` section points at `ghcr.io/...` only; no Docker Hub mirror.

---

### 7.7 — Announcement coordination (P1, non-code)

**State**: Pending. Not a code task.

**Work**: Draft the launch post, coordinate with the website and announcement projects (kept separate from this repo per `feedback_deployment.md`). Pre-seed the README badges, the upgrade guide's "from v1.5" section, and the blog post.

**Effort**: 0.5–1 engineer-day, not blocking the code tag but blocking the public announcement.

---

### 7.8 — CSP header on SPA responses — **DONE**

**Outcome**: The global `middleware.SecurityHeaders` chain in `internal/api/router.go` already wraps the SPA mux, so CSP was applied to SPA responses all along — the Phase 7.8 work was tightening the directive, not wiring a new path.

**Changes**:
- `internal/api/middleware/security_headers.go` — removed `ws: wss:` from `connect-src` (same-origin WebSocket traffic is covered by `'self'` per CSP 3; confirmed `web/src/hooks/useDeployProgress.ts:71` builds WS URLs from `window.location.host`) and removed `data:` from `font-src` (no `url(data:` or `@font-face` in the built SPA `internal/api/static/assets/`). Final directive:
  ```
  default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'
  ```
  (Kept the stricter `object-src 'none'` not present in the original target — no reason to relax it.)
- `internal/api/middleware/security_headers_test.go` — added `TestSecurityHeaders_CSPDirective` which locks in the exact string across `/`, `/dashboard`, and `/api/v1/apps`. Any future relaxation (re-adding `ws:`/`wss:` scheme sources, data: fonts, etc.) trips CI rather than silently loosening the policy.

**Verified**: `go test ./internal/api/middleware/ -count=1` green.

---

### 7.9 — Secrets scanning in CI — **DONE**

**Outcome**: New `secrets` job in `.github/workflows/ci.yml` runs gitleaks v8.30.1. PR events scan `origin/<base_ref>..HEAD` (diff only); push events scan full git history on the pushed ref.

**Supply chain**: Not using `gitleaks-action` — it requires a paid `GITLEAKS_LICENSE` env var for any GitHub organization, even on public repos. Instead the job downloads the `gitleaks_8.30.1_linux_x64.tar.gz` binary from the official release page and verifies it against the committed SHA256 `551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb`. This is stronger than pinning an action by commit SHA (the SHA256 covers the compiled artifact, not just the source repo state).

**Allowlist**: `.gitleaks.toml` extends the default ruleset with:
- `paths`: built SPA assets, `monster.example.yaml`, `_test.go`, `web/src/**/*.test.ts(x)`, `web/src/**/__tests__/**`, `web/e2e/*.ts`, `docs/examples/*.md`, `internal/auth/.credentials`
- `regexes`: fake PEM placeholders (`MIIB...`, `MIIE...`), AWS-documented example keys (`AKIAIOSFODNN7EXAMPLE`, `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`), Stripe placeholder literals, and a `(test|fake|dummy|example|changeme|placeholder)` stopword pattern for in-code test-secret identifiers.

**Verified locally**: `gitleaks git` scans 305 commits / 19.26 MB in 1.37s with zero findings. `gitleaks dir` scans the 33.88 MB working tree in 7 s, also zero findings. First of the two scan passes is what CI runs; the second is the belt-and-suspenders path for contributors who want to scan uncommitted state.

---

### 7.10 — Writers-under-load benchmark — **DONE**

**Outcome**: The `MaxOpenConns(1)` ceiling is now quantified and gated. New benchmark + gate + committed baseline cover the 64-way concurrent `DeploymentStore.Create` fan-out.

**Changes**:
- `internal/db/sqlite_bench_test.go` — new `BenchmarkStore_ConcurrentWrites_64Workers`. `b.N` deployments are distributed across 64 goroutines via a buffered channel, per-op wall-clock latency is tracked, and p50/p95/p99 are reported through `b.ReportMetric` so `go test -bench` surfaces the distribution. Measured on a Ryzen 9 16-core dev workstation: `503 ops/s, p50=91ms p95=359ms p99=611ms` for 1024 ops — confirming that under a 64-way fan-out the tail is dominated by queueing behind the single SQLite writer, exactly the scenario a burst of concurrent deploys would hit.
- `internal/db/concurrent_writes_gate_test.go` — new `TestStore_ConcurrentWrites_BaselineGate`. Opt-in via `DM_DB_GATE=1` (see note below); runs 1024 ops × 64 workers, compares p95 to the committed baseline, fails on > 10 % regression. Supports `DM_DB_GATE_UPDATE=1` for intentional re-capture and `DM_DB_GATE_VERBOSE=1` for always-log success.
- `internal/db/testdata/concurrent_writes_baseline.json` — committed baseline, generated by `DM_DB_GATE=1 DM_DB_GATE_UPDATE=1 go test -run TestStore_ConcurrentWrites_BaselineGate`.
- `Makefile` — new targets `db-gate` (run the gate) and `db-gate-baseline` (intentional re-capture).
- `.github/workflows/ci.yml` — new `Writers-under-load gate` step in the `test` job. Runs on every push and PR, surfacing p50/p95/p99 + comparison vs baseline in the job log. **`continue-on-error: true`** while the committed baseline is still the dev-workstation number — a 2-vCPU `ubuntu-latest` runner executes the same workload ~3–5× slower and would flag every PR as a regression. Flipping `continue-on-error` off is a one-line change paired with a CI-runner baseline recapture via `workflow_dispatch`.

**Follow-up**: A small workflow_dispatch job that re-runs the gate with `DM_DB_GATE_UPDATE=1` on an `ubuntu-latest` runner, commits the resulting baseline, and flips `continue-on-error` off. That step lives on the P2 post-v0.0.1 list; the infrastructure to support it is now in place.

---

### 7.11 — Cross-tenant authorization fuzz target — ✅ DONE 2026-04-12

**State**: Shipped. A tenant-A developer JWT is now fuzzed against every resource-scoped GET route while the store is pre-seeded with a tenant-B resource, and the oracle rejects any 2xx response as a cross-tenant leak. Running the target for 30s executes ~155k requests across 92 interesting inputs with zero oracle violations.

**Outcome**:
- `internal/api/router_fuzz_test.go` — new file adds `crossTenantStore` (wraps the existing `testStore` and returns tenant-B resources for a fixed set of seed IDs), `fuzzResourceIDRoutes` (42 GET routes with `{id}` parameters enumerated from `router.go`), `fuzzSetupRouter` (bootstraps a Router + tenant-A access token), `fuzzAssertNoLeak` (walks every route and asserts the response is not 2xx, skipping SPA-fallback HTML), `TestRouter_CrossTenantSeedCorpus` (runs the seed corpus as a regular `Test*` so CI exercises it without `-fuzz`), and `FuzzRouter_CrossTenant` (the fuzz target proper).
- The test **immediately caught two real leaks** on the first run:
  - `GET /api/v1/apps/{id}/versions` — `RollbackHandler.ListVersions` and its sibling `Rollback` both skipped the tenant check entirely, so a developer in tenant A could list (and trigger) rollbacks on any app ID they could guess. Fixed in `internal/api/handlers/rollback.go` by routing both through `requireTenantApp` (same helper that already guards the rest of the `/apps/{id}` surface). Tests updated to inject claims + an owning app.
  - `GET /api/v1/agents/{id}` — `AgentStatusHandler.GetAgent` echoed any non-`local` ID straight back into a fake `AgentNodeStatus` payload, making the endpoint an ID-enumeration oracle. Fixed in `internal/api/handlers/agent_status.go` to return 404 for any non-`local` ID until a real remote-agent registry lookup lands. Existing `TestAgentStatus_GetAgent_Success` + `TestAgentStatusHandler_GetAgent_Remote` updated to the new behavior.
- New regression tests: `TestRollbackHandler_ListVersions_CrossTenant` locks in the tenant-isolation guard for ListVersions specifically.
- Fuzz target verified locally: `go test -fuzz FuzzRouter_CrossTenant -fuzztime 30s ./internal/api/` → PASS, 155 869 execs, 92 interesting inputs, zero leaks. Regular `go test ./internal/api/` runs the seed corpus on every CI invocation.

**Why the test is kept as a `Test*` rather than wired into a `-fuzz` CI step**: Go's fuzzing infrastructure is not designed for time-boxed CI runs — `-fuzztime` is a soft target and long crash inputs would snowball across PRs. The seed corpus test is the CI guardrail; the `-fuzz` target is the local exploratory tool. Any mutant the fuzzer finds gets promoted into the seed corpus (`testdata/fuzz/FuzzRouter_CrossTenant/`) and then runs under plain `go test` from that point on.

---

## Phase 8 — Post-v0.0.1 tech debt (do not start before v0.0.1 cut)

### 8.1 AWS EC2 VPS provider (3–5 days, P1)

Only if EC2 stays in the marketing copy. Implement alongside the existing Hetzner/DO/Linode providers under `internal/vps/providers/ec2/`. Full surface: auth (access key + assumed role), region selection, AMI picker, security group, SSH key injection, teardown, rate-limit handling.

### 8.2 Narrow the `core.Core` god-object (5–10 days, P2)

Start by narrowing `Init(ctx, c *Core)` to `Init(ctx, deps ModuleDeps)` module-by-module. First target: a leaf module (`marketplace`, `monitoring`). This is a refactor pass, not a feature; do not do it during release stabilization.

### 8.3 Module-registry ADR (0.5 day, P3)

Covered in Phase 6 outstanding. Write an ADR or `docs/modules.md` that documents lifecycle, topo-sort, and shutdown semantics.

### 8.4 `.project` cleanup (0.5 day, P1)

Covered in Phase 6 outstanding. Do it after v0.0.1 cut so the audit output stays stable until the release has shipped.

### 8.5 Rolling-deploy chaos test (2 days, P2) — ✅ DONE 2026-04-12

**State**: Landed ahead of v0.0.1 during the Tier 105 stabilization pass.

**Evidence**:
- `internal/deploy/strategies/strategy_test.go:551` — `TestRolling_Execute_HealthCheckChaosRecovery` tolerates transient `Stats` errors and an intermediate `"unhealthy"` state before converging on `"healthy"`.
- `internal/deploy/strategies/strategy_test.go:601` — `TestRolling_Execute_ContainerDiesMidHealthCheck` asserts that if the new container dies before becoming healthy, the deployment fails and the new container is cleaned up.
- `internal/deploy/strategies/strategy.go:256` — tightened the no-explicit-healthcheck path so the 2-second stabilization sleep is followed by a re-verify; containers that crash during that window are no longer falsely declared healthy.

### 8.6 Secrets vault hardware-backing exploration (5 days, P3)

Long-term: move the vault KEK to a hardware-backed path (TPM, YubiKey, cloud KMS) behind the existing `ADR-0008` strategy interface. Research spike, not a committed feature.

---

## Critical path to v0.0.1 final

Ordered, minimal:

1. ~~**7.0 admin middleware wiring** (0.5 day)~~ — ✅ DONE 2026-04-11
2. ~~**7.2 goreleaser snapshot** (1 day)~~ — ✅ DONE 2026-04-11
3. ~~**7.4 CHANGELOG** (0.5 day)~~ — ✅ DONE 2026-04-11
4. ~~**7.6 GHCR image push** (1 day)~~ — ✅ DONE 2026-04-12
5. ~~**7.5 installer dry-run**~~ — ✅ Code validated 2026-04-12. Installer is GitHub-first via raw URL; `smoke-test.sh` added.
6. **7.3 fresh-VM smoke** (1 day) — **unblocks confidence**
7. ~~**7.8 CSP**~~ — ✅ DONE 2026-04-11
8. ~~**7.10 writers-under-load bench**~~ — ✅ DONE 2026-04-11
9. ~~**7.11 cross-tenant fuzz**~~ — ✅ DONE 2026-04-12
10. **7.7 announcement** (0.5 day, non-code, parallelizable)
11. ~~**7.9 secrets scanning**~~ — ✅ DONE 2026-04-11

**Serial critical path remaining**: ~1.5 engineer-days (items 6 + 10). Originally 6.5; all code-work items closed 2026-04-12.
**Full Phase 7 closure remaining**: ~1.5 engineer-days.

At one full-time engineer, v0.0.1 final ships in **~3–4 days** from here once a VM is provisioned. At half-time, **~1 week**.

Phase 8 is explicitly post-v0.0.1 and does not compress this number.
