# DeployMonster — Production Readiness Assessment

> **Audit date:** 2026-04-11
> **Baseline commit:** `b36fba6` — harden: ws DeployHub Shutdown + concurrent-write mu + dead-client eviction (Tier 77)
> **Auditor role:** Senior software architect / production readiness reviewer
> **Companion documents:** `.project/ANALYSIS.md`, `.project/ROADMAP.md`

This is a **brutally honest** production readiness scorecard. It intentionally does not soften findings to match the project's marketing, its `STATUS.md` self-assessment (which calls the project "production-ready"), or the optimistic tone of the README. Every score is defended with evidence from the code that a reader can verify.

The headline numbers, the verdict, and the go/no-go recommendation are all in §1. The detail is in §2 onward. A reader with 90 seconds should read §1 and §11.

---

## 1. Headline

| | |
|---|---|
| **Overall score** | **64 / 100** |
| **Verdict** | **🟡 NOT PRODUCTION READY — BUT WITHIN REACH** |
| **Go/No-Go** | **NO-GO at HEAD.** Conditional GO in 6-10 weeks after completing `.project/ROADMAP.md` Phase 1-3. |
| **Distance from ship-ready** | ~58-78 engineer-days (see `ROADMAP.md` for decomposition) |
| **Headline strengths** | Excellent architecture, working ingress+proxy, clean Store interface, visible hardening discipline (Tiers 73-77), real VPS providers, real marketplace (116 templates), strong frontend, solid CI/CD |
| **Headline weaknesses** | `go test ./...` is red at HEAD, admin-role middleware missing, SQLite migration rollback deadlocks, AWS SigV4 implemented but never wired into Route53 or S3, Bitbucket webhooks unsigned, build logs discarded, scale_app MCP tool advertised but unimplemented |

The project is **not** what its README claims. It is **also not** the disaster a first-time reader of `go test ./...` output might think it is. It is somewhere in the middle: a well-designed system with real, fixable, specific bugs — and, crucially, a maintainer who has demonstrated (via the Tier 73-77 commits) the exact kind of discipline needed to close them.

---

## 2. Scoring methodology

Each category scores **0-10** on a simple scale:
- **0-2** — broken or missing; blocks production use.
- **3-4** — partial; known gaps users will hit.
- **5-6** — works for the happy path; fails under stress or edge cases.
- **7-8** — production-quality for most deployments; minor hardening remains.
- **9-10** — production-quality across the board; well above industry median.

Scores are multiplied by a weight reflecting how much a real production user cares about that category. Weights sum to 100.

| Category | Weight | Score (0-10) | Weighted |
|---|---:|---:|---:|
| 3. Core functionality | 15 | 7 | 10.5 |
| 4. Reliability | 12 | 5 | 6.0 |
| 5. Security | 15 | 5 | 7.5 |
| 6. Performance | 8 | 7 | 5.6 |
| 7. Testing | 12 | 5 | 6.0 |
| 8. Observability | 8 | 6 | 4.8 |
| 9. Documentation | 8 | 6 | 4.8 |
| 10. Deployment | 12 | 8 | 9.6 |
| 11. Operational maturity | 10 | 7 | 7.0 |
| **Total** | **100** | | **61.8** |

**Rounded: 62/100.** Taking into account the forward-looking trajectory (Tier 73-77 hardening velocity and the correctness of the architecture), I round up to **64/100** for the public headline. The forward-looking bonus is +2 points, not +20 — this is still a NO-GO today.

---

## 3. Core functionality — Score: **7/10** (weight 15)

Can the system do what it claims to do? Mostly yes, with specific gaps.

### What works
- Web UI loads, authenticates, lists apps, creates deploys.
- Git-to-deploy webhook pipeline is real end-to-end for GitHub, GitLab, Gitea (and Bitbucket parsing, though unsigned — see §5).
- 14 language detectors in `internal/build/detectors/`.
- Docker container lifecycle through the Docker SDK works (`internal/deploy/docker.go`).
- Reverse proxy, Let's Encrypt HTTP-01, 5 LB strategies, routing table hot reload — all real.
- Marketplace with **116** real Docker Compose templates, not 25.
- Master/agent WebSocket protocol — real protocol, real handlers, recently hardened.
- VPS provisioning against Hetzner, DigitalOcean, Vultr, Linode.
- Secrets vault with AES-256-GCM + Argon2id.
- Multi-tenancy boundary enforced at the Store layer.

### What does not work
- **Build logs are thrown away** (`deploy/pipeline.go:80` uses `io.Discard`). Users cannot see why a build failed. This is the single most user-visible broken feature in the tree.
- **Route53 DNS provider is non-functional** against real AWS. The SigV4 signing helper exists and is tested, but it is not wired into `route53.go:98`. `TestRoute53_SignsRequests` fails at HEAD.
- **S3 backup is non-functional** against authenticated S3. Works against anonymous MinIO. `internal/backup/s3.go:155` has `// Note: Full AWS SigV4 signing would be implemented here.`
- **MCP `scale_app` returns "unknown tool"** even though it is declared in the catalog (`tools.go:54` vs `handler.go:48`).
- **Email notifications do not exist.** No provider, no interface, no SMTP import.
- **DNS-01 ACME challenge missing** — no wildcard certificates possible.
- **Blue-green and canary deploy strategies missing** — spec lists them, only recreate and rolling ship.
- **MongoDB managed DB engine missing** despite README claim.
- **AWS EC2 VPS provider missing.**
- **PostgreSQL store missing** despite the "PostgreSQL ready" README language — the interface is ready, the implementation is not.

**Score rationale:** Core deploy flow works. The stand-out gaps are not in the happy path; they are in the integrations users expect to exist because the docs say so. A user who deploys a Node.js app on GitHub against a Cloudflare DNS provider with local backups will be very happy. A user who deploys on AWS with Bitbucket and expects email alerts will hit three broken features in their first week.

---

## 4. Reliability — Score: **5/10** (weight 12)

Will the system stay up and degrade gracefully?

### Positive signals
- **Tier 73-77 hardening** is visible, recent, and correct. `WaitGroup` drains, `defer recover`, `stopCtx` plumbing, dead-client eviction. This is the exact work a maintainer does when they start pushing load through their own code.
- **EventBus is bounded**: 64-slot semaphore for async handlers prevents goroutine explosions.
- **`SafeGo` wrapper** catches panics in async goroutines and logs them rather than crashing.
- **Graceful shutdown** is topologically ordered (reverse of dependency order) with a 30-second deadline.
- **Automatic rollback on failed deploy** (`internal/deploy/rollback.go`, 176 lines).
- **Docker SDK calls have context plumbing** throughout.
- **Circuit breakers** wrap VPS provider calls.

### Negative signals
- **`TestSQLite_Rollback` deadlocks** at HEAD (`sqlite_test.go:240`). The test first prints "expected tenants table to be dropped after rollback" (i.e., the down migration is broken) and then a `QueryRow` against the same database hangs forever. The goroutine dump shows `database/sql.(*DB).conn` stuck in `select`. **Schema rollback is not safe at HEAD.**
- **Build module has no `Stop()`.** Cancel a deploy mid-build and the `docker build` process stays running until it finishes on its own.
- **Rolling deploy has no SIGTERM grace period.** Old replicas are killed immediately. Apps that need drain time (HTTP request in flight, DB transaction) lose work.
- **No restart-storm test.** After the process is killed and restarted, there is no end-to-end proof that in-flight deploys and builds recover cleanly.
- **No backup restore round-trip test.** Backup creation is tested; full restore-from-zero is not.
- **Frontend API client has no per-request timeout, no retry-with-backoff, unlimited refresh attempts.** A partial backend outage can trigger refresh storms on a single failing tab.
- **WebSocket frame rate limiting missing** on deploy-log, metrics, and swarm-agent endpoints.

**Score rationale:** The architecture is correct (bounded concurrency, graceful shutdown, event-driven hot reload). The execution has known gaps. The SQLite rollback deadlock is the most concerning single finding — not because rollbacks are common, but because a system that can't roll back a schema migration is a system that cannot safely upgrade.

---

## 5. Security — Score: **5/10** (weight 15)

Is the system defensible against a motivated attacker?

### Strong
- **JWT HS256 with key rotation** — correct implementation in `internal/auth/jwt.go`.
- **bcrypt cost 12** for password storage.
- **TOTP 2FA** (RFC 6238) with Google Authenticator compatibility.
- **OAuth SSO** against Google and GitHub with state cookie.
- **API keys** hashed (SHA-256) with `dm_` prefix for easy revocation.
- **Secrets vault AES-256-GCM + Argon2id** — correct primitive choice.
- **HMAC-SHA256 webhook signature verification** for GitHub, GitLab, Gitea, Gogs.
- **Audit log** on every mutating API call.
- **CSRF protection** via `dm_csrf` cookie and `X-CSRF-Token` header.
- **Three-tier rate limiting** (global IP, per-tenant, per-auth-level).
- **Non-root Docker container** with `read_only: true`, `tmpfs`, `no-new-privileges`.
- **SQL queries all parameterized** — grep for `fmt.Sprintf("SELECT`" returns nothing suspicious.

### Weak
- **Admin-role middleware does not exist.** `RequireAdmin` / `RequireSuperAdmin` / `RequireOwnerOrAbove` are tested in `admin_test.go` but unimplemented. Every admin endpoint relies on handler-local `if claims.RoleID == "role_super_admin"` checks. One forgotten check = privilege escalation. **This is the most serious security finding in the audit.**
- **Bitbucket webhook signature verification missing.** `VerifySignature` falls through to `default: return true`. Anyone who can guess a webhook ID can trigger deployments on a Bitbucket-connected app.
- **Master encryption key in config file or env var**, not HSM-backed, not KMS-fetched.
- **Hardcoded Argon2id salt** (`"deploymonster-vault-salt-v1"`). An attacker who compromises one DeployMonster deployment can precompute rainbow tables usable against any other deployment.
- **TOTP recovery codes stored plaintext** in the database. They are secondary passwords; they should be hashed.
- **OAuth has no PKCE.** Not fatal for confidential clients, but free to add.
- **No WebSocket frame rate limiting** on any WS endpoint.
- **TLS not enforced by default** — HTTPS is opt-in, not opt-out.
- **API keys hashed with SHA-256, not bcrypt/Argon2id.** Cheap to brute-force if the hash table leaks.

### Neutral
- **`docs/security-audit.md` exists (untracked)** — promising signal but cannot be scored because content is not reviewed as part of this audit; it is in-progress.

**Score rationale:** The cryptographic primitives are correct. The operational gaps are real and two of them (admin middleware, Bitbucket webhooks) are latent authentication/authorization bypasses. A security review by a third party is strongly recommended before production use. Score cannot exceed 5 while admin middleware is missing.

---

## 6. Performance — Score: **7/10** (weight 8)

Does the system scale beyond the happy path?

### Strong
- **Binary is ~22 MB stripped.** Small.
- **SQLite with WAL mode + PRAGMA tuning** — correct for a single-node control plane.
- **Reverse proxy uses `httputil.ReverseProxy`** — battle-tested, zero-allocation hot path when the routing table isn't being written.
- **LB strategies** use atomic counters and `fnv.New64a` — no allocation per request.
- **Vite 8 frontend** with 5 manual chunks and 21 lazy-loaded pages → first-paint is fast.
- **EventBus async is bounded (64 slots)** — cannot runaway under event floods.

### Weak
- **Routing table uses `sync.RWMutex`.** Readers acquire the read lock on every request. Under 200+ concurrent connections with thousands of apps, this becomes a measurable hotspot. An `atomic.Value` swap would fix it.
- **Per-tenant build queue does not exist.** Global `max_concurrent_builds` means tenant A can starve tenant B.
- **No load test baseline committed.** `tests/loadtest` target exists in the Makefile but there is no "this is our current baseline" artifact in the repo.
- **No soak test.** 24-hour runs are not part of CI. Goroutine leaks and memory climb are not detected until a user runs the system for a week.
- **Host-level `/proc` metrics missing on Linux** — the monitoring dashboard reports zero for host CPU/mem/disk.

**Score rationale:** Nothing in the hot path is obviously bad. The system has not been pushed hard enough for anyone to know where it actually breaks. Score 7 reflects "architecturally fine, operationally unproven".

---

## 7. Testing — Score: **5/10** (weight 12)

Are we confident in what ships?

### Strong
- **194 `_test.go` files, ~47,000 lines of tests** — extensive surface area.
- **Test-to-source ratio of 1.74:1** — objectively high for Go.
- **7 fuzz tests** in auth, core, ingress.
- **38 benchmarks.**
- **9 Playwright E2E suites** covering auth, dashboard, apps, deploy-flow, domain-setup, marketplace, navigation, team-management, topology-editor.
- **~38 Vitest unit test files.**
- **CI runs test + test-react + test-e2e + lint + build matrix + docker + release** on every push.
- **Race detector enabled** in `make test`.

### Weak
- **`go test ./...` is RED at HEAD.** 3 packages fail to build, 1 package has a test failure, 1 package deadlocks. No responsible release engineer ships from a red tree.
- **97% coverage claim is unverifiable** at HEAD because of the above. Realistic measurement is 85-90% on the packages that compile.
- **Several `*_coverage_test.go` files** look generated or backfilled to push coverage numbers — useful for regression but not behavior-driven.
- **No master/agent end-to-end integration test.** Protocol is unit-tested at the wire level.
- **No Let's Encrypt staging integration test.**
- **No backup restore round-trip test.**
- **No SIGHUP reload integration test.**
- **No Stripe webhook replay test.**
- **No tenant-starvation adversarial test.**

**Score rationale:** Abundant tests, some of them buggy, with meaningful gaps in integration coverage. The red tree at HEAD prevents any score higher than 5 — and arguably should force a score lower. The +1 bonus is for the volume and shape of what works.

---

## 8. Observability — Score: **6/10** (weight 8)

Can operators see what is happening?

### Strong
- **Structured logging via `log/slog`** with `"module"` key on every entry.
- **Audit log** on every mutating call with IP, timestamp, actor.
- **Prometheus `/metrics` endpoint** (claimed by README; not verified in depth but the middleware chain includes `APIMetrics`).
- **Request ID middleware** in the API chain.
- **SSE / WebSocket streaming endpoints** exist for deploy logs and metrics (even though deploy logs are currently piped to `io.Discard`).
- **Tier 73-77 hardening commits** add logger statements in the right places.

### Weak
- **Build logs discarded.** The single biggest observability hole. A failed build is effectively a silent failure to the user.
- **Host metrics not implemented on Linux** (`/proc` parsing). Dashboard reads zero.
- **No distributed tracing.** OpenTelemetry is not wired in. For a single-node master/agent system this is acceptable; for the "1000 tenants" end of the roadmap it will become a gap.
- **Metrics drift between what Prometheus scrapes and what the dashboard shows** — not verified in this audit, but with 20 modules this is a latent risk.
- **No `debug/pprof` endpoint** — at least not verified in the API surface.

**Score rationale:** The chassis for observability is there. A couple of specific holes (build logs, host metrics) keep it from being production-grade.

---

## 9. Documentation — Score: **6/10** (weight 8)

Can someone run this system without the author?

### Strong
- **7 ADRs in `docs/adr/`** covering the non-obvious architectural decisions (SQLite default, modular monolith, no Kubernetes, pure Go SQLite, embedded React, in-process event bus, master/agent same binary).
- **`docs/architecture.md`** (~62 KB) exists.
- **`docs/openapi.yaml`** (~57 KB) exists.
- **`docs/api-reference.md`**, **`docs/deployment-guide.md`**, **`docs/getting-started.md`**, **`docs/configuration.md`**, **`docs/troubleshooting.md`** all present.
- **`·project/SPECIFICATION.md`** is ~220 KB and genuinely comprehensive.
- **`Makefile` has `make help`** and every target is documented.
- **`scripts/build.sh`** is a real end-to-end pipeline.
- **`monster.yaml` init scaffold** via `deploymonster init`.

### Weak
- **README overstates reality** in at least three places: "97% test coverage", "9 MCP tools", "25+ marketplace templates", and (in CLAUDE.md) "TanStack React Query".
- **`docs/openapi.yaml` is hand-maintained** and almost certainly drifted from the 240 registered routes in `internal/api/router.go`. CI does not validate the drift.
- **`docs/upgrade-guide.md` and `docs/security-audit.md` are untracked**. They exist as work in progress, not as committed documentation.
- **No "gotchas" document** for operators — e.g., "don't run two masters against the same SQLite file", "the Argon2id salt is currently deployment-global", "agent port is hardcoded to 8443".
- **No ADR for the master encryption key strategy** — a load-bearing operational decision with no captured rationale.
- **`·project/` directory uses middle-dot U+00B7** — unfriendly to tooling, CI, and Windows shells without care.

**Score rationale:** The documentation surface area is wide but some of the user-facing claims are wrong. 6/10 reflects "enough to get started, not enough to trust without verification".

---

## 10. Deployment — Score: **8/10** (weight 12)

Can this be installed, upgraded, and rolled back?

### Strong
- **Dockerfile is multi-stage**, uses `alpine:3.21`, runs as non-root, has a `HEALTHCHECK`, `CGO_ENABLED=0`.
- **`docker-compose.prod.yml`** is well-tightened: `read_only: true`, `tmpfs /tmp:100M`, `no-new-privileges`, memory and CPU caps.
- **`.goreleaser.yml`** (85 lines) covers pre-release hooks (tidy + vet + test), multi-arch (linux/darwin/windows × amd64/arm64), SBOM per archive, SHA256 checksums.
- **CI pipeline is 5-stage** (test / test-react / test-e2e / lint / build) with a docker-push and release stage.
- **Binary-in-one philosophy** — `deploymonster` is the whole thing.
- **`deployments/deploymonster.service`** (untracked but present) — systemd unit.
- **`deploymonster init`** generates a starter config.
- **`deploymonster rotate-keys`** provides a real key-rotation CLI command.
- **SIGHUP config reload** is implemented in `main.go:122-131`.

### Weak
- **Upgrade rollback is not safe.** `TestSQLite_Rollback` deadlocks. A failed upgrade has no clean back-out path.
- **The `get.deploy.monster` installer** is a separate project not verified in this audit.
- **No per-OS install test matrix** — Ubuntu 24.04 is the assumed target but older LTS versions are not verified.
- **GHCR-only image distribution** (per user memory) — fine, but no mirror.

**Score rationale:** Deployment is one of the strongest parts of the project. The single failing piece is the migration rollback, which is a Phase 1 fix.

---

## 11. Operational maturity — Score: **7/10** (weight 10)

Is the team operating this like a production product?

### Strong
- **Tier 73-77 hardening commits** — five consecutive commits addressing `WaitGroup` drains, `defer recover`, `stopCtx`, dead-client eviction, concurrent-write mu. This is the signature of a maintainer who reads their own code under load. Easily the most important positive signal in the audit.
- **CI runs on every push** with race detection and a coverage gate.
- **SBOM generation on release.**
- **Automatic rollback on failed deploy.**
- **Graceful shutdown order is reversed dependency order with a 30-second deadline.**
- **Circuit breakers on external calls.**
- **Rate limiting at three tiers.**
- **Audit log on every mutating API call.**

### Weak
- **`STATUS.md` claims "production-ready, all 251 tasks complete"** while this audit finds a red test tree and 15+ P0/P1 gaps. The gap between self-assessment and reality is the single biggest operational maturity concern. A team that cannot accurately grade its own work is a team that will ship with confidence into a wall.
- **No incident postmortem template**, no on-call runbook, no staging environment referenced in docs.
- **No disaster recovery drill** documented (the backup restore round-trip test doesn't exist; see §7).
- **No security review** by a third party (internal `docs/security-audit.md` is WIP).
- **No SLO/SLI definition** — the project has Prometheus metrics but no "here's what we promise" targets.

**Score rationale:** The craft (Tier 73-77 hardening, CI discipline, SBOM, audit log) is high. The self-assessment accuracy is low. Net 7/10 with a note that the gap between marketing and reality has to close before v1.0.0.

---

## 12. Category summary table

| # | Category | Weight | Score | Weighted | Gap to 8/10 | Priority |
|---|---|---:|---:|---:|---|---|
| 3 | Core functionality | 15 | 7 | 10.5 | Wire SigV4, implement missing providers, restore build logs | HIGH |
| 4 | Reliability | 12 | 5 | 6.0 | Fix rollback deadlock, add restart-storm and restore round-trip tests | CRITICAL |
| 5 | Security | 15 | 5 | 7.5 | Admin middleware, Bitbucket signing, hash recovery codes, per-deployment salt | CRITICAL |
| 6 | Performance | 8 | 7 | 5.6 | Commit load-test baseline, add soak test, lock-free routing | MEDIUM |
| 7 | Testing | 12 | 5 | 6.0 | Fix red tree, add integration tests, honest coverage measurement | CRITICAL |
| 8 | Observability | 8 | 6 | 4.8 | Stream build logs, add Linux host metrics, pprof endpoint | HIGH |
| 9 | Documentation | 8 | 6 | 4.8 | Correct README/CLAUDE.md drift, generate OpenAPI from code, finish security-audit.md | MEDIUM |
| 10 | Deployment | 12 | 8 | 9.6 | Safe migration rollback; per-OS install matrix | MEDIUM |
| 11 | Operational maturity | 10 | 7 | 7.0 | Runbook, SLO/SLI, third-party review | LOW-MEDIUM |

Ranked by weighted gap-to-target (8 is the "production-quality" bar), the highest-leverage work is:
1. **Security** (gap 3 × weight 15 = 45) — admin middleware + Bitbucket + vault salt
2. **Testing** (gap 3 × weight 12 = 36) — fix the red tree + add integration tests
3. **Reliability** (gap 3 × weight 12 = 36) — rollback deadlock + restart-storm
4. **Core functionality** (gap 1 × weight 15 = 15) — wire SigV4, restore build logs, implement missing MCP tool
5. **Observability** (gap 2 × weight 8 = 16) — stream build logs, host metrics

Notice that the top five leverage areas are **all addressed by Phase 1-3 of `.project/ROADMAP.md`**. This is not a coincidence — the roadmap was built from this scorecard's gaps.

---

## 13. Go / No-Go Decision

### NO-GO at HEAD. Here is why, in one paragraph:

At HEAD (`b36fba6`), the test suite does not compile cleanly, a core test (`TestSQLite_Rollback`) deadlocks rather than failing fast, admin-role middleware is absent while claiming to enforce admin-only endpoints, AWS Route53 DNS provider does not sign its requests, S3 backup does not sign its uploads, Bitbucket webhooks are unauthenticated, and build logs are silently discarded before reaching the user. Any one of these is a "wait until it's fixed" finding on its own; the cluster of seven together is unambiguous. Shipping from HEAD would produce a product that demonstrably fails against named, common deployment targets (AWS, Bitbucket, anything requiring clean upgrade/rollback), and a product whose user experience on its most common workflow (push code, watch build, see failure) is broken because the failure is invisible.

### CONDITIONAL GO after Phase 1-3 of the Roadmap. Here is why:

Phase 1 (8-11 days) restores test correctness and closes every P0 bypass. Phase 2 (14-18 days) fills every spec-vs-code gap that an attentive reader of the README would be disappointed to miss. Phase 3 (10-14 days) hardens the lifecycle, data durability, and multi-tenant contention paths. After Phase 3, every finding in §§3-11 that currently scores below 7 will score at 7 or above, putting the weighted total at ~78-82/100 — solidly in "production-quality for most deployments, minor hardening remains" territory. At that point, conditional on a third-party security review catching nothing beyond what this audit found, the project should receive a GO.

### The conditions

To upgrade from NO-GO to GO, the project must satisfy **all** of the following:

1. **`go test -race -timeout 180s ./...`** exits 0 across three consecutive runs on CI.
2. **`TestSQLite_Rollback`** passes without timeout and the migration rollback machinery has a dedicated "clean up partial rollback" test.
3. **`TestRoute53_SignsRequests`** passes and a manual end-to-end test against a real AWS account creates, lists, and deletes a Route53 record.
4. **Admin middleware** (`RequireAdmin` / `RequireSuperAdmin` / `RequireOwnerOrAbove`) exists and is applied at the router level to every endpoint previously relying on handler-local role checks. A linter check or a route manifest test prevents regressions.
5. **Bitbucket webhook signature verification** is implemented and unknown-provider webhooks are rejected, not accepted.
6. **Build logs stream to the UI** — end-to-end verified by a Playwright test.
7. **`scale_app` MCP tool** either works end-to-end or is removed from `tools.go`.
8. **README and CLAUDE.md** contain no factually inaccurate claim about test coverage, library usage, feature count, or supported integrations.
9. **A third-party security review** covers the auth, secrets, and multi-tenant isolation subsystems. Any CRITICAL or HIGH finding must be closed before GO.
10. **A fresh-VPS installer smoke test** — one-line install from `get.deploy.monster`, followed by a marketplace app deploy with automatic TLS, succeeds on Ubuntu 22.04 and 24.04.

---

## 14. The final word

DeployMonster is a project with **an unusual amount of the right stuff under the hood** and **an unusual amount of marketing that runs ahead of the code**. The fix is not to throw anything away. The fix is to spend 6-8 calendar weeks doing disciplined bug-fix work against a specific, short, achievable list — and to stop claiming 97% test coverage until someone can actually measure it.

The Tier 73-77 hardening commits in the last week are the most important positive signal in this audit. They tell me the maintainer knows what correct production code looks like. They also tell me the maintainer has been doing the work quietly. The audit's job is to make the remaining work visible so the same discipline can be applied to it.

If the team executes `.project/ROADMAP.md` Phase 1-3, v1.0.0 will be shippable. If the team tries to ship v1.0.0 from HEAD, at least three named user personas hit broken features on day one.

**My recommendation: hold the release, execute Phase 1-3, re-audit, ship.**

---

*Prepared 2026-04-11 from HEAD `b36fba6`. No code was modified during the audit. Every score is defensible from the file:line evidence cited in `.project/ANALYSIS.md` and `.project/ROADMAP.md`.*
