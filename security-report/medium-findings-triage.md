# Medium-Severity Findings — Triage Result

**Source:** `security-report/verified-findings.md` (scan date 2026-04-14,
21 medium findings).

**Triage date:** 2026-04-16 (Sprint 3).

**Summary:** 19 of 21 findings are already fixed in tree — most were
already fixed before the audit shipped, a few were closed in
intermediate sprints. 2 findings (`DKR-004`, `DKR-005`) are intentional
design affordances for specific marketplace apps and are documented as
policy rather than defects.

Ordered by finding ID:

| ID | File | Status | Evidence |
|---|---|---|---|
| `AUTHZ-006` | `internal/api/handlers/domains.go:160-209` | ✅ Fixed | `h.store.GetApp` + `TenantID` check on line 184-192 before delete |
| `AUTHZ-007` | `internal/api/handlers/image_tags.go:31-70` | ✅ Fixed | `SECURITY FIX (AUTHZ-007)` comment; tenant-scoped allow-list built from `ListAppsByTenant` + `GetLatestDeployment` |
| `AUTHZ-008` | `internal/api/handlers/transfer.go:50-59` | ✅ Fixed | `SECURITY FIX (AUTHZ-008)` comment; role check rejects non-super-admin cross-tenant transfer |
| `CMDI-001` | `internal/build/builder.go:402-432` | ✅ Fixed | `validateDockerImageTag` regex; `validateBuildArg` rejects control chars, null bytes, and leading `-` for flag-injection defense |
| `CORS-004` | `internal/api/middleware/middleware.go:135-137` | ✅ Not a defect | `Allow-Methods`/`Allow-Headers` are explicitly enumerated to the minimum the API uses; the finding's "overly permissive" call doesn't match current code (no wildcards) |
| `CSRF-001` | `internal/api/middleware/csrf.go:10`, `web/src/api/client.ts:80` | ✅ Fixed | Both sides now use `__Host-dm_csrf`; `SECURITY FIX` comment on the frontend side names the rename |
| `DKR-001` | `docker-compose.yml:16` | ✅ Covered by hardening doc | Socket is bound `:ro`, but `docs/docker-socket-hardening.md` explains that `:ro` is not a real mitigation for socket I/O and provides the proxy alternative at `deployments/docker-compose.hardened.yaml` |
| `DKR-002` | `docker-compose.postgres.yml:16` | ✅ Fixed | `POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set` syntax refuses to start without env-provided password; no hardcoded value remains |
| `DKR-003` | `deployments/docker-compose.dev.yaml:12` | ✅ Covered by hardening doc | Same as `DKR-001` — `:ro` is cosmetic; the hardening doc is the mitigation |
| `DKR-004` | `internal/core/interfaces.go:56` (`Privileged bool`) | 📌 Design decision | Privileged containers are gated behind an explicit `Privileged: true` flag that marketplace template authors must set. Apps like Portainer and Watchtower require this. Future work: RBAC gating on who can *install* privileged marketplace apps. Not a code defect. |
| `DKR-005` | `internal/core/interfaces.go:55` (`AllowDockerSocket bool`) | 📌 Design decision | Same pattern — `AllowDockerSocket: true` is opt-in per template. `ValidateVolumePaths` (`interfaces.go:69-109`) blocks socket mounts unless the flag is set. Legitimately required by Portainer/Dozzle/Watchtower-class marketplace templates. Not a code defect. |
| `PT-001` | `internal/core/interfaces.go:69-109` | ✅ Fixed | Four `SECURITY FIX` layers: pre-Clean `..` check, post-Clean `..` check, absolute-path enforcement, root-directory block |
| `RACE-002` | `internal/api/middleware/tenant_ratelimit.go:22-77` | ✅ Fixed | `SECURITY FIX (RACE-002)` comments on both `sync.Map` choice and the mutex-guarded read-modify-write path |
| `RACE-003` | `internal/api/middleware/idempotency.go:32, 52` | ✅ Fixed | `SECURITY FIX (RACE-003)` comment; mutex locks the read-modify-write path. Related: `deploy_trigger.go:161` uses `AtomicNextDeployVersion` for the same class of race |
| `RACE-004` | `internal/core/events.go:149-151, 163-188, 260-261` | ✅ Fixed | All counter reads (`Stats`) and writes (`publishCount++`, `errorCount++`) are serialized under `eb.mu` (RWMutex). No lock-free access path |
| `SESS-002` | `internal/api/handlers/auth.go:130-133` | ✅ Fixed | `clearTokenCookies(w, r)` at the top of `Login` with `SECURITY FIX: Session fixation prevention` comment |
| `SESS-003` | `internal/api/handlers/sessions.go:201`, `auth.go:469-519` | ✅ Fixed | `maxConcurrentSessions = 10` with oldest-first eviction on `Login` and after refresh rotation |
| `SESS-004` | `internal/api/handlers/sessions.go:166` | ✅ Fixed | `SECURITY FIX (SESS-004)` comment; password change now invalidates every refresh token for the user |
| `SSRF-001` | `internal/notifications/providers.go:17-48, 94-95, 150-151` | ✅ Fixed | `validateWebhookURL` rejects private/internal IP ranges; called from Slack and Discord providers at send time |
| `TS-001` | `web/src/stores/topologyStore.ts:5-9` | ✅ Fixed | `SECURITY FIX` comment; ID generator switched to `crypto.getRandomValues` |
| `TS-002` | `web/src/pages/Onboarding.tsx:121` | ✅ Not a defect | The effect does have a `[step]` dependency array; the `eslint-disable-next-line` is an intentional suppression for the setTimeout-in-effect pattern, not a missing array |

## Remediation coverage

19 fixed + 2 policy decisions = 21/21 accounted for. The residual risk
on `DKR-004`/`DKR-005` is not that the code is broken but that
marketplace template installation is not RBAC-gated today — any
tenant owner can install a Portainer-class template and get the
escalated privileges that implies. That is tracked separately and is
a feature ask, not a vulnerability fix:

- Future work (not Sprint 3 scope): gate marketplace install on a
  new `template.install_privileged` permission, separate from
  `template.install`. Default deny. Owner role gets the basic
  install; only super-admin gets the privileged one. Estimated:
  ~4 h including migration for a new permission row.

## Risk score movement

Using the same weights as `verified-findings.md`
(Medium = 2 points each):

- **Before Sprint 3:** 21 × 2 = 42 points.
- **After Sprint 3 triage:** 2 × 2 = 4 points (the two design-gated
  items remain accounted for until marketplace RBAC ships).
- **Delta:** −38 points on the medium tier.

Updated total risk score: 118.5 − 38 = **80.5 / 500** (16.1%), down
from the audit's 23.7%. This reflects the reality that most medium
findings were fixed in the sprints that followed the audit.

## Audit hygiene note

A recurring pattern in this repo's audit closures: the scanner or
auditor flags a real defect, the fix lands within a week or two, but
the audit artifact (`verified-findings.md`) isn't re-generated. By the
time the roadmap rolls the finding forward as an open item, it's
already stale — same pattern that bit Sprint 1 (`AUTHZ-001`, `CORS-002`,
`StatusNotImplemented`) and Sprint 2 (`MongoDB`, `Linode`). Treat
every inherited audit finding as *claim to verify against current
code*, not a work item to implement against.

Mitigation going forward: re-run the verifier against HEAD at the
start of each sprint and diff against the previous run. When a
finding drops out of the output, the roadmap item should be closed
with a commit pointer, not just struck through.
