# Project Roadmap

> Based on comprehensive codebase analysis performed on **2026-04-16**
> This roadmap prioritizes work needed to bring the project from its current ≈ 0.1.x state to a genuine 1.0 production release.
> Companion docs: `.project/ANALYSIS.md` (audit findings), `.project/PRODUCTIONREADY.md` (go/no-go verdict).

---

## Current State Assessment

**Where the project stands (honest read).** DeployMonster is feature-rich and architecturally sound, but the gap between *claimed* and *delivered* is material:

- **Test suite is red today.** 4 failing Go tests across `internal/discovery`, `internal/ingress` (×2), `internal/swarm`.
- **1 critical + 12 high-severity security findings** tracked in `security-report/SECURITY-REPORT.md` and not yet remediated (AUTHZ-001 domain hijacking, CORS wildcard with credentials, race conditions in deploy path, JWT refresh alg not pinned, access tokens non-revocable).
- **Version/doc drift.** `VERSION` file says `v0.1.2`; README and `.project/STATUS.md` headline `v0.1.6`. No intermediate tags were cut.
- **Feature-claim drift** *(as found by the original audit; Sprints 1–2 truthed these up in place).* Marketplace templates: audit said 56, real count is **91** (56 curated + 35 net-new from the bulk 100-list after dedup fix — see 2.3). API routes: 205, not the claimed 240. DNS providers: 1 of 3 claimed is real (Cloudflare ships; Route53 + RFC2136 now labelled *planned post-1.0* in SPEC). VPS providers: 5 of 6 claimed are real (Hetzner, DigitalOcean, Vultr, Linode, Custom-SSH all shipping; AWS EC2 moved to *Beyond-1.0*). MongoDB: flagged as "not implemented" by the audit but actually is — engine lives in `internal/database/engines/redis.go` (odd filename, not its own `mongo.go`), covered by 6 tests.
- **E2E suite brittle.** `continue-on-error: true` in CI; last 9 commits are all E2E stabilization work.

**What's working well.**
- Clean 20-module architecture with topologically-ordered lifecycle.
- Strong typing discipline: strict TS with zero `any`/`@ts-ignore`; `go vet` clean; `gofmt` clean.
- Mature CI/CD: 85% coverage gate, OpenAPI drift detection, gitleaks + Trivy (SHA-pinned), multi-platform GoReleaser, scratch Docker image.
- Security fundamentals solid: parameterized SQL, slice-args `exec`, AES-256-GCM secret vault, bcrypt cost 13.
- Graceful shutdown, bounded async event pool, `SafeGo` wrapper with panic recovery.

**Key blockers for a production release** (must all be resolved):
1. Red test suite.
2. Open critical/high security findings.
3. Version/CHANGELOG reconciliation.
4. Either deliver or retract over-claimed features (marketplace, providers, endpoint counts).

**Baseline timeline estimate to 1.0:** 10–14 weeks of focused effort by 1–2 engineers, following the phased plan below.

---

## Phase 1: Critical Fixes (Week 1–2)

**Goal:** red tests → green; critical security findings closed; honest docs.

- [ ] **Fix `TestHealthChecker_CheckAll_TCPUnhealthy`** in `internal/discovery/`. Bug: TCP check on a closed port returns healthy. Review `internal/discovery/health.go` TCP probe — likely missing connect-error classification. *2–4 h.*
- [ ] **Fix `TestReverseProxy_ServeHTTP_BackendConnectionError`** and **`TestReverseProxy_CircuitBreaker_RecordsFailure`**. Both expect 502 but receive 404, and the circuit-breaker stat never increments. `internal/ingress/proxy.go` — error path doesn't distinguish "no backend found" (→ 404) from "backend unreachable" (→ 502). *3–5 h.*
- [ ] **Fix `TestAgentClient_Dial_DefaultPort`**. Error-message format `"master rejected connection: HTTP 200"` not `"... port 8443"`. `internal/swarm/agent.go` dial error construction. *1–2 h.*
- [ ] **AUTHZ-001 — domain hijacking.** Add `requireTenantApp(h.store, claims, req.AppID)` to `internal/api/handlers/domains.go:83–141`. Write a regression test that creates a domain on another tenant's app and asserts 403. *1–2 h.*
- [ ] **CORS-001 — revert wildcard.** Revert the permissive CORS introduced by commit `a72550d`. Implement a proper allowlist: `CORSOrigins` from config, reject `Origin: ""` when `Allow-Credentials: true`, never emit `*` with credentials. Add unit tests covering the three modes (allowlist, empty, wildcard-without-credentials). *3–4 h.*
- [ ] **CORS-002 — WebSocket origin check.** `internal/api/ws/deploy.go:107`: reject empty `Origin` and validate against the same allowlist. *1 h.*
- [ ] **AUTH-001 — pin `alg` on refresh.** `internal/auth/jwt.go:208`: replace `jwt.Parse` with explicit `jwt.ParseWithClaims` + `jwt.WithValidMethods([]string{"HS256"})`. *1 h.*
- [ ] **Version reconciliation.** Either: (a) advance `VERSION` to `v0.1.6`, write missing CHANGELOG entries for 0.1.3–0.1.6, cut a tag; *or* (b) roll README back to `v0.1.2`. Decide and do. *2 h.*
- [ ] **Update `PRODUCTION-READY.md`** to match reality. Remove "100/100" and "zero blockers". If you want the doc to stay, make it list the current production verdict truthfully. *1 h.*

**Phase 1 total:** ≈ 20–28 h of engineering. Exit criteria: `go test ./...` green; `go vet` clean; 3 critical security fixes merged; docs version-consistent.

---

## Phase 2: Core Completion (Week 3–6)

**Goal:** close feature gaps or prune claims to match. Finish high-severity security remediation. Stabilize E2E.

### 2.1 Security — complete high-severity backlog

- [ ] **RACE-001** deployment trigger race. Replace SELECT-then-INSERT with `INSERT ... ON CONFLICT DO NOTHING` (SQLite) / named advisory lock (Postgres). `internal/api/handlers/deploy_trigger.go:62`. *3–4 h.*
- [ ] **RACE-002** — route all callers of `GetNextDeployVersion` to `AtomicNextDeployVersion`. `internal/api/handlers/deployments.go:115` and any grep hits. *2–3 h.*
- [ ] **SESS-001 access-token revocation.** Denylist in BBolt keyed on JTI with TTL = access-token lifetime. Middleware check before JWT validation. *6–8 h + tests.*
- [ ] **AUTHZ-002 / AUTHZ-003** tenant-scope checks on `internal/api/handlers/ports.go:44` and `healthcheck.go:47`. Same `requireTenantApp` helper as AUTHZ-001. *2–3 h.*
- [ ] Remaining high-severity items (rate-limiter TOCTOU, BBolt metrics race, connection-tracking race, JWT refresh rotation edge case). *6–10 h.*
- [ ] Enable race detector in a nightly CI job (`go test -race ./...`) to catch regressions. *1 h config.*

### 2.2 Feature gaps — implement or prune

**Decide per-feature: implement or retract.** Pruning is legitimate and often better than partial implementation.

- [x] **MongoDB managed-DB engine.** *Sprint 2 audit: already implemented.* The engine lives in `internal/database/engines/redis.go` (odd co-location with Redis, not a separate `mongo.go`, but functionally complete). Implements `Name/Versions/DefaultPort/Image/Env/HealthCmd/ConnectionString` for `mongo:7` with root-user init via `MONGO_INITDB_ROOT_{USERNAME,PASSWORD,DATABASE}`, `mongosh ping` health check, and `mongodb://user:pass@host:port/db?authSource=admin` connection strings. Covered by 6 tests in `engine_test.go` + `engine_extra_test.go` (construction, connection-string format, image/version, env vars, health command, exec command). Registered in the engine factory (`engine.go:91`). The roadmap's "implement mongo.go" claim was stale — file name differs but behavior is complete. *Follow-up (low priority): rename `redis.go` → `nosql.go` or split into `redis.go` + `mongo.go` for discoverability.*
- [ ] **Route53 DNS provider.** `internal/dns/providers/route53.go` with IAM-role + static-key auth modes. **Sprint 2 decision: drop from scope.** Only `cloudflare.go` ships today; the SPEC's Route53 references will be truthed-up to "planned" annotations so the marketing footprint matches the binary. Revisit when an actual customer requests it — no speculative implementation. *(Roadmap estimate kept for future: ≈ 10–12 h to implement.)*
- [x] **Linode VPS provider.** *Sprint 2 audit: at parity with other providers.* `internal/vps/providers/linode.go` is 189 lines of real Linode v4 API calls — paginated `ListRegions` / `ListSizes`, `Create` with user-data metadata, `Delete`, `Status`, circuit-breaker + shared `vpsDoRequest` helper. Not a stub. The roadmap's three gripes: (1) "actual API calls" — they exist; (2) "SSH-key attach" — `VPSCreateOpts.SSHKeyID` is defined in `core/interfaces.go:292` but **no provider** (DO, Hetzner, Vultr, or Linode) wires it through to the cloud API — cross-cutting gap, not Linode-specific, split out below; (3) "post-provision validation" — also not done by any other provider, same cross-cutting. Linode is shippable at the same bar as the others.
- [ ] **AWS VPS provider** (EC2). **Deferred to "Beyond 1.0".** No `aws.go` in `internal/vps/providers/`; SPEC references (sections 5, 8, 15) will be truthed-up to "planned / community-contribution welcome". Five other cloud providers plus Custom-SSH cover ~95 % of the user base — AWS adds 16–20 h of vendor-SDK maintenance cost for marginal coverage. Revisit when a paying customer needs EC2.
- [ ] **Cross-provider SSH-key wiring** (split out from Linode item). `VPSCreateOpts.SSHKeyID` is plumbed through the interface but **no VPS provider** (DigitalOcean, Hetzner, Vultr, Linode) passes it to the cloud API. Wire each provider's `Create` payload to include SSH key IDs so provisioned boxes are login-ready without embedding a root password. *≈ 2 h total (four providers × ~30 min + tests).*
- [x] **Weighted load-balancer strategy.** `internal/ingress/lb/weighted.go` landed in Sprint 2 with Nginx-style smooth weighted round-robin, zero-weight drain, negative-weight clamping, atomic SetWeights, and a `Canary` wrapper that implements stable/canary % splits on top of Weighted. Wired through the string factory (`lb.New("weighted")`) and covered by 15 behavioral tests (default-1 weights, exact distribution over a cycle, smoothness proof, drain, factory wire-up, canary ramp 0→50→100, empty-canary fallback, per-pool weights, clamping).
- [x] **Finish or gate 26 `StatusNotImplemented` handlers.** *Sprint 2 audit closed this with zero findings.* `rg StatusNotImplemented`, `rg 'http\.StatusNotImplemented'`, `rg 'writeError.*501'`, `rg 'w\.WriteHeader\(501\)'` against `internal/` all returned zero matches. Only `"not implemented"` hits are intentional test-mock panics (`internal/api/handlers/common_test.go`, `internal/deploy/mock_test.go`) used to fail fast when a test accidentally exercises an uncovered store method — correct testing hygiene, not production stubs. The roadmap's "26 handlers" count was stale (same pattern as Sprint 1's pre-fixed AUTHZ-001/CORS-002 audit findings).

### 2.3 Marketplace truth-up

- [ ] **Deduplicate `internal/marketplace/builtins_*.go`.** Consolidate into a single sorted table; remove duplicate Slug entries; report the real count. *3–4 h.*
- [x] **Decide target count.** Picked (a) in v0.1.7: README.md and `.project/SPECIFICATION.md` now read "91 built-in templates, community-contributed growing" instead of "150+". The 56 figure originally cited in this roadmap (and in `.project/ANALYSIS.md`) counted only `LoadBuiltins()` output and missed the second load path inside `marketplace.Module.Init` that merges the `moreTemplates100` bulk list. Subsequent audit work in Sprint 2 (commit-to-follow) exposed that second load path and the silent-overwrite bug it carried — the true at-startup registry size is 91, not 56. `.project/BRANDING.md` still carries the aspirational 150+ copy because it is marketing/website material tracked in a separate project per ownership decision — revisit when that copy is reconciled with the published marketplace count.
- [x] Raise `internal/marketplace/` test coverage from 69% → 85%. *Sprint 2: 70.1% → 96.0%.* The 26-percentage-point jump came from a new `sources_test.go` that pinned the four previously-uncovered public methods on the remote-source path (`AddSource`, `SetUpdateInterval`, `UpdateTemplates`, `updateLoop`) plus `TemplateRegistry.Update` and the conditional branches inside `Start/Stop`. Tests use a local `fakeSource` (no HTTP, no timers beyond a 10ms ticker for the one goroutine test) so the suite stays fast (~50ms) and hermetic. Key behavioural guarantees now pinned: nil-source ignored, concurrent `AddSource` is race-free (16×50 goroutine stress), a zero-template upstream never empties the registry, one failing source does not abort the others, validation-rejected templates are counted but do not block valid ones, `Stop` is idempotent (second close-of-channel panic guarded by `sync.Once`), and the background loop is *not* spawned when sources or interval are unset (cheap default path).

### 2.4 E2E stabilization

- [ ] **Audit all 11 Playwright specs** against current UI. The last 9 commits tell the story — selectors are drifting with each UI iteration. *1–2 days.*
- [ ] **Harden auth setup** — the `login` project already uses per-test contexts to dodge rate limits, but the user-creation step in `global-setup.ts` is race-prone. *4–6 h.*
- [ ] **Flip `continue-on-error: false`** on `test-e2e` job once suite is green for 5 consecutive runs. Then the CI gate actually means something.
- [x] **Add `axe-playwright`** for basic accessibility checks. *Sprint 2: `@axe-core/playwright@4.11.1` added as a devDep, with `web/e2e/a11y.ts` wrapping `AxeBuilder` behind a `scanA11y(page, opts?)` helper that disables `color-contrast` (tailwind zinc-on-zinc is just-under 4.5:1, design-token question) and `region` (single-form pages don't need a landmark) by default, and fails tests only on `serious`/`critical` violations. Lower-severity counts are JSON-logged so trend data shows up in CI without merge-blocking on trivia. `web/e2e/a11y.spec.ts` smoke-scans seven authenticated routes: `/`, `/apps`, `/marketplace`, `/domains`, `/projects`, `/settings`, plus the deploy-wizard entry at `/apps/new`. Runs under the existing `chromium` project against the authed storageState — no extra setup. Public-page scans (`/login`, `/register`) intentionally deferred until we wire a project that starts from empty auth state. Run with `pnpm test:e2e --grep a11y` once `make dev` is up on `:8443`.

**Phase 2 total:** ≈ 100–150 h (≈ 3–4 weeks with one engineer). Exit criteria: no open critical/high security items; every feature in README is implemented or retracted; E2E gate is blocking and green.

---

## Phase 3: Hardening (Week 7–8)

**Goal:** finish medium-severity items; tighten operational defaults; strengthen CI signal.

- [x] **Close 21 medium-severity security findings.** *Sprint 3: 19 stale / 2 design decisions.* Per-finding triage in `security-report/medium-findings-triage.md`. 19 were already fixed in tree before this roadmap pass (the same audit-hygiene pattern that bit Sprint 1's `AUTHZ-001`/`CORS-002`/`StatusNotImplemented` and Sprint 2's `MongoDB`/`Linode`): `AUTHZ-006/007/008` have explicit `SECURITY FIX (AUTHZ-00X)` comments with tenant-ownership checks, `CMDI-001` has `validateDockerImageTag` + `validateBuildArg` regex + control-char + flag-injection defense, `CSRF-001` has the `__Host-dm_csrf` rename on both sides, `PT-001` has four-layer traversal defense (pre-Clean, post-Clean, absolute-only, root-block), `RACE-002/003/004` are all mutex-guarded with matching `SECURITY FIX` comments, `SESS-002/003/004` have `clearTokenCookies` + `maxConcurrentSessions = 10` + password-change session invalidation, `SSRF-001` has `validateWebhookURL` rejecting private IP ranges, `TS-001` swapped to `crypto.getRandomValues`, `DKR-002` requires env-provided postgres password via `${VAR:?must be set}` syntax, `DKR-001/003` are now covered by the Sprint-3 hardening doc. Two findings (`DKR-004` privileged containers, `DKR-005` socket mount for marketplace apps) are not defects — they're opt-in `Privileged`/`AllowDockerSocket` flags that marketplace templates like Portainer/Watchtower legitimately require, gated by `ValidateVolumePaths` at deploy time. Remaining work on that pair is a separate **Beyond-Sprint-3** ask: RBAC-gate *who can install privileged marketplace templates* (~4 h, new permission + migration), which doesn't block the medium-tier closure. Risk-score delta: 42 → 4 points on medium tier, total 118.5 → 80.5 / 500 (23.7% → 16.1%).
- [x] **Upgrade API-key hashing to match password storage.** *Sprint 2: partially stale, finished.* The "SHA-256 → bcrypt" switch was already shipped as CRYPTO-001 before this roadmap was written (see commit history) — `internal/auth/apikey.go` has been using `bcrypt.GenerateFromPassword` for a while. What *was* true is that it ran at `bcrypt.DefaultCost` (10) while passwords ran at cost 13 (`internal/auth/password.go`), which meant an attacker dumping the DB would find API keys ~8× cheaper to crack than user passwords. Sprint 2 closes the asymmetry by pinning a new `apiKeyBcryptCost = 13` constant and adding two regression tests: one that fails if the cost drifts away from `bcryptCost` again, one that asserts legacy cost-10 hashes still verify (so no migration is needed — bcrypt encodes the cost into the hash, `CompareHashAndPassword` reads it back, existing keys keep working until next rotation).
- [ ] **`db-gate` baseline on GH Actions runners.** Re-capture the writers-under-load baseline on the 2-vCPU GH runner (not the 16-core dev box). Commit, remove `continue-on-error`. *2 h.*
- [x] **`loadtest-check` in CI.** *Sprint 3: wired as a new nightly workflow.* `.github/workflows/loadtest-nightly.yml` boots a real DeployMonster instance on a GH runner (same pattern as `test-e2e` — build UI, embed, build Go, start server, wait for `/health`), runs a 10s warm-up at concurrency=5 to let the runtime settle past JIT/GC warm-up, then a measured 30s run at concurrency=20 against the five public-read endpoints. Results are uploaded as `loadtest-results-${run_id}.json` with 30-day retention so operators can compare trends across nights. Cron at `47 3 * * *` staggers it 30 min after `race-nightly` so they don't collide for runner quota. `continue-on-error: true` on the measured-run step for the same reason as `db-gate`: the committed dev-box baseline (1657 req/s, p95 ≈ 25 ms) is 3-5× faster than GH runners, so a `-baseline` flag would false-positive every run. Workflow exposes a `capture_baseline` dispatch input that writes to a distinct artifact path — follow-up PR moves it to `tests/loadtest/baselines/http-ghrunner.json` and flips the flag off.
- [x] **Input validation audit.** *Sprint 3: spot-check complete, three gap-fixes shipped, remaining items cataloged.* Full audit in `security-report/input-validation-audit.md`. Baseline posture is adequate — every request goes through `BodyLimit(10 MB)` + `Timeout(30 s)` middleware so no handler can be forced to process unbounded input regardless of handler-local gaps. Inside that envelope: 9 of 36 handlers use the canonical `FieldError`/`writeValidationErrors` helper, ~20 use consistent inline checks, ~7 had real gaps. Sprint 3 fixes the three highest-value ones: `ports.go:Update` (host-port range, protocol enum `tcp`/`udp`, 100-mapping cap), `healthcheck.go:Update` (interval/timeout/retries upper bounds, path length cap, port range check), and `sticky_sessions.go:Update` (RFC-6265 cookie-name regex, SameSite enum, MaxAge bounds). The sticky-sessions fix is the one with a real exploit if it regresses — a cookie name of `foo; Path=/; Set-Cookie:` would split the Set-Cookie header when the reverse proxy writes it. Regression tests in `internal/api/handlers/validation_test.go` pin each rule including the header-splitting exploit payload. Remaining 7 follow-up items (envvars length caps, redirect status-code enum, response-headers CRLF check, labels key/value caps, dns-records content cap, log-retention upper bound, error-pages body cap) are 15–30 min each and cataloged in the audit doc — deferred as a batched polish PR.
- [x] **Secret rotation tool.** *Sprint 3: documentation shipped.* The code side (`RotationGracePeriod = 20 * time.Minute`, `NewJWTService(secret, previousSecrets...)`, `AddPreviousKey`, `Server.PreviousSecretKeys` config field, `MONSTER_PREVIOUS_SECRET_KEYS` env override) was already wired before this roadmap was written — the gap was purely documentation. `docs/secret-rotation.md` now covers the standard rotation procedure (move active key into `previous_secret_keys`, generate new via `openssl rand -hex 32`, restart, wait ~35 min for every in-flight access token to expire, then clean up the list on the next config edit), the emergency path (leave `previous_secret_keys` empty to force-logout every user immediately after a confirmed compromise), the 32-char / 256-bit minimum secret length (enforced by `jwt.go` panic on startup), and a staging rehearsal smoke script that captures a token, rotates, and asserts the grace-period cutover behaviour end-to-end.
- [x] **Docker socket hardening.** *Sprint 3: docs + reference compose shipped.* `docs/docker-socket-hardening.md` explains why `:ro` on `/var/run/docker.sock` isn't a real mitigation (Docker API is bidirectional socket I/O — the mount's ro flag doesn't apply), maps the Docker API endpoints DeployMonster actually uses (containers/images/networks/volumes/info/version/ping — no Swarm, no Services) to the Tecnativa proxy env-var toggles, and calls out the tradeoff: the proxy reduces the blast radius of a compromised DeployMonster container but doesn't eliminate it (an attacker with `/containers/create` can still request a privileged sibling). `deployments/docker-compose.hardened.yaml` is the ready-to-run reference — `MONSTER_DOCKER_HOST=tcp://docker-proxy:2375` on a `networks.dm-internal.internal: true` bridge so the proxy endpoint is never published to the host. No code changes needed: `MONSTER_DOCKER_HOST` / `docker.host` was already a config knob.

**Phase 3 total:** ≈ 50–70 h. Exit criteria: security-report shows only low-severity items remaining; performance regression gates are active and blocking.

---

## Phase 4: Testing (Week 9–10)

**Goal:** test depth and reliability, not just coverage percentage.

- [ ] **Implement fuzz tests** for input-parsing boundaries. `PRODUCTION-READY.md` claimed 15 fuzz targets; grep finds zero. Good candidates: webhook payload parsing, `${SECRET:…}` resolver, YAML config, compose-file validator, HMAC verifier, JWT parser. Target **8–10 fuzz targets**. *12–16 h.*
- [x] **Lift `internal/marketplace/` coverage to 85%** — *done in Phase 2.3, final measurement 96.0%.* Duplicate tracker in this phase is closed by reference; see the Phase 2.3 entry for the test inventory.
- [ ] **Lift `internal/auth/` coverage to 85%** (currently 78.7%). Focus on refresh-token rotation edge cases. *4–6 h.*
- [ ] **Lift `internal/db/` coverage to 85%** (currently 78.9%). Transaction-rollback paths are under-tested. *4–6 h.*
- [ ] **Add integration test: multi-tenant authorization matrix.** A single table-driven test that enumerates (tenant A token, tenant B resource, endpoint) and asserts 403. Prevents authorization regressions systemically. *6–8 h.*
- [ ] **Add integration test: webhook-to-deploy end-to-end.** Simulated GitHub webhook → build → deploy → container running. Uses dockertest or testcontainers. *8–12 h.*
- [ ] **Frontend: one test per lazy-loaded page** — smoke render + happy-path click. Cheap insurance. *1 day.*
- [ ] **Chaos test.** Kill the DB mid-deploy; verify recovery. *4–6 h.*

**Phase 4 total:** ≈ 50–70 h. Exit criteria: every package > 80% coverage; fuzz targets running in CI; authorization matrix test in place.

---

## Phase 5: Performance & Optimization (Week 11–12)

**Goal:** the system should hold its published SLAs under realistic load.

- [ ] **Publish SLAs.** Write down the target numbers first (e.g. 300 req/s per master, p95 < 200 ms, 50 concurrent builds). Can't optimize without a target.
- [ ] **Re-capture `loadtest-baseline` on a production-representative VM.** Document the topology. *4 h.*
- [ ] **Run the 24-h soak test (`make soak-test`)** and review `soak-results.json`. Address any leak, latency creep, or cold-cache anomaly found. *24 h clock time + 8 h analysis.*
- [ ] **Profile with pprof** against a synthetic workload. Typical wins: Docker SDK client reuse (already done), JSON encoding hot path, bcrypt cost re-examined. *1–2 days.*
- [ ] **DB indexes audit.** SQLite + Postgres migrations both need an EXPLAIN-QUERY-PLAN sweep over the top 20 query patterns. *6–8 h.*
- [ ] **Bundle-size budget.** Add a Vite CI check that fails if the main chunk grows > 300 KB gzip. *2 h.*
- [ ] **Lazy-load topology editor vendor bundle.** `@xyflow/react` + `dagre` are heavy; ensure they're not in the main chunk. *2 h.*
- [ ] **BBolt batching.** Hot paths that call `Set` in a loop should use `BatchSet` where available. Audit for N>1 set loops. *4 h.*

**Phase 5 total:** ≈ 60–80 h + soak clock time. Exit criteria: published SLAs met under soak; no identified hot-path regressions.

---

## Phase 6: Documentation & DX (Week 13–14)

**Goal:** every claim in the repo should be true; every getting-started path should work for a first-time contributor.

- [ ] **Rewrite README.** Remove aspirational counts. Add a "Known Limitations" section. *4 h.*
- [ ] **Regenerate `.project/SPECIFICATION.md`** as an honest v1.0 scope doc: what's in, what's explicitly out. Link from README. *1–2 days.*
- [ ] **Rewrite `.project/TASKS.md`** or retire it. It's no longer a useful artifact. *2 h.*
- [ ] **Write 4–6 ADRs** under `docs/adr/` for the big decisions: SQLite-default, custom-API-client vs Query, embedded UI, modular-monolith, master/agent over full distributed. *1 day.*
- [ ] **Run the quickstart cold.** Fresh VM → `curl | bash` install → first app deployed. Fix everything that broke. *1 day.*
- [ ] **API docs.** Either (a) reduce the router surface to what's in OpenAPI; (b) generate OpenAPI from router.go annotations (the `make openapi-check` tool already does the comparison — invert it into a generator). *1–2 days.*
- [ ] **Contributor guide.** Reality-check `CONTRIBUTING.md` — is `make test-integration` actually runnable? Is the local-DB setup documented? *4 h.*
- [ ] **Deploy runbook.** Upgrade procedure, backup/restore, DB migration rollback, JWT-secret rotation, disaster recovery. *1 day.*

**Phase 6 total:** ≈ 50–70 h. Exit criteria: README is honest; cold install works; ADRs capture why the system looks the way it does; runbook exists.

---

## Phase 7: Release Preparation (Week 15–16)

**Goal:** cut 1.0 with confidence.

- [ ] **Verify `make release-snapshot`** produces exactly the artifacts you want (tar.gz + SBOM + SHA256 + GHCR image).
- [ ] **Cut `v0.9.0-rc1`** and do a closed-beta install on 3 representative VPS topologies (2-vCPU Hetzner, 4-vCPU DO, 8-vCPU dedicated).
- [ ] **SBOM audit** — SPDX JSON generated by GoReleaser; Trivy-scan attached to the GitHub Release. Subscribe to CVE feeds for direct deps.
- [ ] **Release notes.** CHANGELOG entries for everything since v0.1.2 (the last tagged version). Focus on user-facing changes — security fixes called out explicitly.
- [ ] **Binary signing.** Evaluate cosign keyless signing of GHCR images and release archives. *4–6 h.*
- [ ] **Cut `v1.0.0`** once beta feedback is clean for ≥ 7 days.
- [ ] **Zero-downtime upgrade verified.** Fresh master + pre-existing DB → new version → all existing apps still reachable.
- [ ] **Rollback drill.** Install v1.0, then downgrade to v0.9-rc1 cleanly. Document it.

**Phase 7 total:** ≈ 40–60 h + beta clock time. Exit criteria: v1.0.0 tag cut; upgrade + rollback paths validated.

---

## Beyond v1.0: Future Enhancements

- [ ] Additional marketplace templates toward 100+ verified.
- [ ] **Postgres-backed HA** story (Litestream or logical-replica failover). Documented, tested, runbook.
- [ ] **Distributed tracing.** The OpenTelemetry auto-SDK is already pulled in transitively — wire an OTLP exporter and emit spans from the middleware chain + module lifecycle.
- [ ] **AWS VPS provider.**
- [ ] **RFC2136 DNS provider** (generic/BIND).
- [ ] **Redis-backed rate limiter** for multi-master deployments.
- [ ] **Multi-region agent orchestration.**
- [ ] **Admin audit-log export** to S3 / SIEM.
- [ ] **`react-hook-form` + `zod`** migration as forms grow past the current handful.
- [ ] **Server-side rendering** for status pages (SEO / faster first-paint for the marketing surface). Only if a user need materialises.
- [ ] **Plugin system** for third-party builders / notifiers. Currently everything is a first-party module.
- [ ] **Kubernetes agent mode.** Long-tail ask from users who "already have k8s."

---

## Effort Summary

| Phase | Description | Estimated Hours | Priority | Dependencies |
|---|---|---:|---|---|
| 1 | Critical fixes (red tests, critical security, version reconcile) | 20–28 | **CRITICAL** | — |
| 2 | Core completion (feature/claim alignment, high-sev security, E2E) | 100–150 | **HIGH** | Phase 1 |
| 3 | Hardening (medium-sev, perf gates, validation) | 50–70 | HIGH | Phase 2 |
| 4 | Testing (fuzz, authz matrix, coverage laggards, chaos) | 50–70 | HIGH | Phase 2 |
| 5 | Performance & optimization (SLA, soak, profiling) | 60–80 + soak | MEDIUM | Phase 3 |
| 6 | Documentation & DX (README truth-up, ADRs, runbook) | 50–70 | MEDIUM | Phase 2 |
| 7 | Release prep (RC, beta, v1.0 cut) | 40–60 + beta | MEDIUM | Phases 3–6 |
| — | **Total to v1.0** | **≈ 370–530 h** | | |

At 1 engineer × 30 productive hours/week → **12–18 weeks**.
At 2 engineers × 30 h/week in parallel where possible → **7–10 weeks**.

---

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| Feature-claim debt erodes user trust | **High** | High | Phase 1 doc reconciliation; Phase 6 README rewrite. Ship less, mean more. |
| Security finding exploited in the wild before remediation | Medium | **Critical** | Phase 1 closes critical; Phase 2 closes high. Publish `SECURITY.md` with disclosure policy. |
| E2E suite never stabilizes → no real CI signal for UI | Medium | High | Dedicate a single engineer to E2E for 1 sprint; flip `continue-on-error: false` as soon as green. |
| SQLite scale ceiling hit earlier than expected | Medium | High | Postgres is already wired. Document "switch to Postgres when X" heuristic in the runbook. Finish the Postgres integration test matrix. |
| Docker SDK CVEs require disruptive upgrades | Medium | Medium | Already SHA-pinning Trivy/gitleaks; subscribe to `docker/docker` GHSA feed; pin to a minor line with backports. |
| Drift between master and agent protocols after 1.0 | Low | High | Version-negotiate the WebSocket handshake now; add protocol-version handshake test to `make test-integration`. |
| Baseline-gated tests flake in CI and get bypassed | Medium | Medium | Capture baselines on the same hardware CI actually runs on; make re-baselining a deliberate, reviewed commit. |
| Contributor onboarding too painful | Medium | Medium | Phase 6 cold-start drill; minimal Docker-compose dev environment. |
| Scope creep during Phase 2 (feature-gap work keeps expanding) | **High** | Medium | Decide per-feature in the first week of Phase 2 and *write it down*: implement vs prune. Don't revisit. |
| Release automation breaks on tag day | Low | High | `make release-snapshot` dry-run must pass a week before the real cut. |

---

## Exit Criteria for 1.0.0

A single concrete checklist — everything here must be true on the day the 1.0 tag is cut:

- [ ] `go test -race ./...` green, including all integration tags.
- [ ] `make openapi-check` clean without allowlist exemptions for any route that is advertised in the README.
- [ ] `docker/docker`, `pgx`, `jwt` upgraded within 30 days of the release date.
- [ ] Zero critical, zero high-severity findings in `security-report/SECURITY-REPORT.md`.
- [ ] E2E suite blocking (`continue-on-error: false`), green for 5 consecutive runs.
- [ ] `make db-gate` and `make loadtest-check` blocking in CI, baselines captured on representative hardware.
- [ ] README feature claims verified; no aspirational counts.
- [ ] `CHANGELOG.md` entries for every release from v0.1.2 forward.
- [ ] Upgrade and rollback paths tested on 3 VPS topologies.
- [ ] Every advertised DNS / VPS / DB provider either works or is removed from advertising.
- [ ] A cold-start install from scratch has been timed and succeeds in < 5 minutes.
