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

## Follow-up items (closed in Sprint 3 polish batch, 2026-04-17)

All 7 items from the original deferred list are now closed. Each fix
is pinned by a regression test in `validation_test.go`. Stale-claim
note: `envvars.go` already had per-key / per-value / total-payload
caps; the gap was the missing *array length* cap, addressed by
`maxVars = 500`.

| Handler | Fix | Regression test |
|---|---|---|
| `envvars.go:Update` | `maxVars = 500` array-length cap on top of the existing 256-char key / 64 KB value / 512 KB total caps | `TestEnvVarHandler_Update_TooManyVars` |
| `redirects.go:Create` | `StatusCode` enum-checked against `{301, 302, 307, 308}` | `TestRedirectHandler_Create_UnknownStatusCode`, `TestRedirectHandler_Create_ValidStatusCodes` |
| `response_headers.go:Update` | Custom-header names validated against RFC 7230 token grammar; values checked for CR/LF and 4 KB length cap; 50-header count cap | `TestResponseHeadersHandler_Update_HeaderNameInjection`, `TestResponseHeadersHandler_Update_HeaderValueCRLF` |
| `labels.go:Update` | Kubernetes-style caps: 63-char key, 253-char value, 64-label count cap | `TestLabelsHandler_Update_KeyTooLong`, `TestLabelsHandler_Update_ValueTooLong` |
| `dns_records.go:Create` | `Name` capped at 253 chars (FQDN max), `Value` capped at 2048 chars | `TestDNSRecordHandler_Create_ValueTooLong` |
| `log_retention.go:Update` | `MaxSizeMB ≤ 10240` (10 GB), `MaxFiles ≤ 100`, `Driver` enum checked against `{json-file, local, syslog}`. (Audit item mentioned "retention days" but the actual field is `MaxSizeMB` / `MaxFiles` — upper-bounding those is the equivalent guard.) | `TestLogRetentionHandler_Update_MaxSizeTooLarge`, `TestLogRetentionHandler_Update_UnknownDriver` |
| `error_pages.go:Update` | 1 MB cap per page (502 / 503 / 504 / maintenance) inside the 10 MB body envelope | `TestErrorPageHandler_Update_PageTooLarge` |

Closure criteria met:
- Each fix is behind a handler check that returns 400 before the
  payload reaches storage or downstream systems.
- Each fix is pinned by at least one regression test that fails if
  the validation is removed.
- All 11 new tests pass; no existing tests regressed.

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
