# DeployMonster — Production Readiness

**Status:** CONDITIONAL GO
**Version:** v0.1.7
**Report date:** 2026-04-16
**Owner:** Ersin / ECOSTACK TECHNOLOGY OÜ

This file is an executive pointer. The **authoritative** readiness review
lives at `docs/archive/PRODUCTIONREADY.md` — it is rewritten against HEAD at
each sprint close. The `docs/archive/` triad is the source of truth:

- `docs/archive/ANALYSIS.md` — brutally honest feature / security /
  code-quality snapshot
- `docs/archive/ROADMAP.md` — three-path decision framework and sprint plan
- `docs/archive/PRODUCTIONREADY.md` — go / no-go verdict with blocker list

---

## Verdict (2026-04-16)

**Ship to self-hosted single-tenant production: GO.**
**Ship to multi-tenant SaaS: conditional on the Sprint 1–3 residual items
being closed.**

Sprint 1 (this release, v0.1.7) shipped:

- Four previously-red tests turned green via portable ephemeral-port
  fixtures (discovery, ingress ×2, swarm).
- `CORS-001` regression from v0.1.4 reverted. Two-mode contract restored:
  public wildcard (no credentials) vs strict allowlist (with credentials).
- `AUTH-001` JWT alg-pinning hardened with the canonical
  `jwt.WithValidMethods` option alongside the existing post-parse guard.
- Version / CHANGELOG drift closed: `VERSION` file, git tags, and
  CHANGELOG now agree at `v0.1.7`; v0.1.3 through v0.1.6 backfilled.
- Three "open" items from the security audit were discovered to be
  already-fixed in commit `7d69c5e` (`AUTHZ-001`, `CORS-002`, partial
  `SESS-001`). Report was stale, not the code.

Remaining blockers for multi-tenant SaaS (tracked in
`docs/archive/ROADMAP.md`):

- Feature-prune decision on MongoDB / Route53 / Linode / AWS adapters
  (implement or move to "Beyond 1.0").
- Marketplace claim: truth-up to "56 built-in, growing" or commit to
  100.
- PostgreSQL store contract enforcement in CI (compile-time assertion
  exists; integration suite gated behind `pgintegration` build tag).

---

## Test + build state at tag

- `go test -short -count=1 ./...` → **40/40 packages PASS**
- `go vet ./...` → clean
- `gofmt -l .` → empty
- `make build` → single binary, embedded SPA, ldflags populated

---

## Why this file exists

Earlier revisions asserted **Production Readiness Score: 100/100** after
closing a 13-finding security audit. That claim was accurate for the
scope of that audit but **did not survive contact** with the follow-up
audit recorded in `docs/archive/ANALYSIS.md`: four tests were red, a CORS
regression had silently disabled the allowlist, JWT alg-pinning was only
post-parse, and the security report's file line numbers no longer
matched HEAD (fixes had already landed in `7d69c5e` but the report was
never re-generated).

The 100/100 framing is retired. This file now tracks the conditional-go
state honestly, and defers the long-form numbers to `docs/archive/`.

---

## References

- **Current sprint log:** `CHANGELOG.md` (the v0.1.7 entry)
- **Honest feature/security audit:** `docs/archive/ANALYSIS.md`
- **Go/no-go with blocker list:** `docs/archive/PRODUCTIONREADY.md`
- **Roadmap + sprint plan:** `docs/archive/ROADMAP.md`
- **Upgrade guide:** `docs/upgrade-guide.md`
- **Security audit (historical):** `docs/security-audit.md`
