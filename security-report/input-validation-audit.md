# Input-Validation Audit

**Date:** 2026-04-16 (Sprint 3).
**Scope:** Every `POST`/`PUT`/`PATCH` handler under
`internal/api/handlers/` (36 mutation endpoints).
**Method:** Static walk of each handler's request-decode block, paired
with the existing `FieldError` / length-cap / enum-check patterns.

## Summary

DeployMonster's input-validation posture is *baseline adequate* and
*consistent about the one thing that actually drives DoS*: every
request goes through `middleware.BodyLimit(10 MB)` and
`middleware.Timeout(30 s)` before it reaches a handler. No handler
can be made to process more than 10 MB of body or run longer than
30 s regardless of what's missing below.

Inside that envelope, the handler-level validation is uneven:

- **9 of 36 handlers** use the canonical `FieldError` /
  `writeValidationErrors` helper pattern. These get per-field,
  multi-error responses and the tightest-checked inputs. Examples:
  `apps.go:Create`, `databases.go:Create`, `domains.go:Create`,
  `auth.go:Login`, `servers.go:Create`, `sessions.go:UpdateProfile`.
- **~20 handlers** use ad-hoc inline checks (`len(x) > N`,
  enum-map lookups, range checks) without the helper. Validation is
  still present and correct; the pattern just isn't uniform. Examples:
  `announcements.go:Create`, `redirects.go:Create`,
  `event_webhooks.go:Create`.
- **~7 handlers** had real gaps before this sprint: unbounded int
  fields, missing enum checks, uncapped arrays, and the Set-Cookie
  header-splitting risk on `sticky_sessions.go`. Sprint 3 fixes the
  three highest-value ones (see below); the rest are listed as
  follow-up items.

## Fixed in this sprint

| Handler | Gap | Fix |
|---|---|---|
| `ports.go:Update` | `HostPort` range, protocol enum, array length cap | `0..65535` range, `tcp`/`udp` enum, 100-mapping cap; regression tests `TestPortHandler_Update_HostPortOutOfRange`, `_UnknownProtocol`, `_TooManyMappings` |
| `healthcheck.go:Update` | Unbounded `Interval`/`Timeout`/`Retries`, missing `Path` length, missing `Port` range | 3600s/300s/100 caps, 2048-char `Path`, `0..65535` `Port`; regression tests for each |
| `sticky_sessions.go:Update` | Missing cookie-name validation (Set-Cookie header-splitting risk), missing `SameSite` enum, unbounded `MaxAge` | RFC 6265 token regex for cookie name, `lax`/`strict`/`none` enum, 1-year `MaxAge` cap; regression tests including the header-splitting exploit payload |

Each fix is pinned by at least one test that fails if the validation
regresses. The `sticky_sessions` header-splitting test is the
highest-value one — it pins a real exploit, not a hygiene rule.

## Follow-up items (not closed this sprint)

These were triaged but deferred. None are urgent:

- `envvars.go:Update` — env var keys/values uncapped in length;
  allowing multi-MB strings inside the 10 MB envelope. Low risk
  because the worker writes env vars into Docker `--env` where
  Docker itself has a limit.
- `redirects.go:Create` — `StatusCode` not validated against the
  HTTP redirect range (301/302/307/308). A user could set 418; the
  proxy would emit a nonsense status but nothing breaks.
- `response_headers.go:Update` — header names not checked for
  CRLF or non-token characters. Similar header-splitting class to
  `sticky_sessions`. Not exploited because the reverse proxy
  normalizes header names before emitting, but belts-and-braces
  says we should validate at ingress too. ~30 min fix.
- `labels.go:Update` — label keys/values uncapped. Kubernetes-style
  label limits (63 chars per key, 253 chars per value) would be a
  sensible cap.
- `dns_records.go:Create` — record `Content` (value) not length-
  capped by handler; relies on the DNS provider to reject oversized
  payloads. Providers do, but validating at ingress gives cleaner
  error responses.
- `log_retention.go:Update` — retention days not upper-bounded. A
  user could set 10^9 days; the garbage collector would never fire.
  Cap at 3650 (10 years) is sensible.
- `error_pages.go:Update` — HTML body uncapped below the 10 MB
  envelope. 1 MB per error page is generous.

Each item above is ~15–30 min of work and zero risk. Batch them into
one PR when someone picks up the polish pass.

## Pattern observations (for future handlers)

What the good handlers do that new handlers should copy:

1. **Check tenant ownership before reading the body.**
   `requireTenantApp(w, r, h.store)` early-returns if the app isn't
   in the caller's tenant. Saves a JSON decode and rejects cross-
   tenant probes with a `404`, not a `403` (a `403` would leak that
   the app exists).
2. **Cap strings, cap arrays, cap ints.** Three categories covers
   nearly every DoS vector inside the 10 MB body envelope. Missing
   any one of these is the most common gap found in this audit.
3. **Reject enums by allow-list, not by formatting the value back
   into an error message.** `map[string]bool{"lax": true, ...}` is
   cheaper and safer than `fmt.Errorf("invalid value %q", v)`.
4. **When a field participates in a security-critical header (Cookie
   name, Content-Disposition filename, Redirect target, CORS
   Origin), validate *at ingress* even if the downstream sanitizes.**
   Defense in depth is cheap and the downstream can change.
5. **Regression-test the exploit payload, not just the happy path.**
   A test that POSTs `foo; Path=/` and asserts 400 is worth ten
   tests that POST valid values and assert 200.

## Relationship to the security-report findings

This audit is the open end of the Phase 3 roadmap item "Input
validation audit. Spot-check every POST/PUT handler for missing JSON
schema / length / range validation." It is distinct from the
medium-severity findings walk in
`security-report/medium-findings-triage.md` — that walk closes out
the 21 audit findings from 2026-04-14, most of which were already
fixed. This audit is a forward-looking spot-check that uncovered new
gaps (the three fixed above) in handlers the original scan didn't
flag.

Going forward: input-validation gaps should be caught by the same
process used for security findings — a periodic re-scan, with the
delta filed as a PR, not as a roadmap entry. The five patterns
above should be codified as a handler-template / reviewer checklist.
