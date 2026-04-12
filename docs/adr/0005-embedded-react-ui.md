# ADR 0005 — Embed the React UI into the Go binary

- **Status:** Accepted
- **Date:** 2026-03-24
- **Deciders:** Ersin KOÇ

## Context

DeployMonster ships a React 19 + Vite 8 + TypeScript SPA with 17 pages. We
have to decide how the UI gets delivered to users. Options:

1. **Embed into the Go binary** via `embed.FS`. One artifact to ship.
2. **Serve from a CDN** and point the Go server at it. Smaller binary, but
   adds an external dependency.
3. **Separate nginx container** serving static files. Traditional, but
   adds another moving part to self-host.
4. **Install UI files separately** on the host filesystem. Painful for
   users to keep in sync with the binary version.

## Decision

**Embed the built React UI into the Go binary via `embed.FS`.** The build
pipeline (`scripts/build.sh`) runs `pnpm run build`, copies `web/dist/` into
`internal/api/static/`, and then `go build` picks up the embedded files via
an `//go:embed` directive. At runtime the API server serves static files
directly from the embedded FS and falls back to `index.html` for SPA
routes.

The entire UI (JS bundle, CSS, images, fonts) lives inside the ~20 MB
binary.

## Consequences

**Positive:**

- **Version skew is impossible.** The UI files that ship with
  `deploymonster v1.7.0` are *exactly* the ones built from the
  corresponding commit. A user cannot run an outdated UI against a newer
  backend.
- **One thing to install.** The installer drops one binary. There is no
  `www/` directory to maintain, no nginx config, no CDN cache to invalidate
  on release.
- **No CORS headaches.** The UI and API are served from the same origin
  (`/api/v1` relative to the host serving the SPA), so cross-origin
  preconnect, CORS preflight, and cookie `SameSite` complications
  disappear.
- **Offline-friendly.** Air-gapped installs work without any network
  access to a CDN.
- **Easier PR review.** A change to the UI and its backing endpoint lands
  in one commit and ships atomically.

**Negative / trade-offs:**

- **Binary size.** The JS bundle adds several MB. With manualChunks
  vendor splitting in `web/vite.config.ts`, lazy-loading all 17 pages via
  `React.lazy`, and gzip on the HTTP response, the on-wire footprint is
  small, but the binary is bigger than a cgo-free Go server would be by
  default.
- **Rebuilding the binary for UI-only changes.** A CSS tweak triggers
  `go build`. In practice this is fast enough (<30 seconds with the build
  cache) that it has not been a problem, and `scripts/build.sh` handles
  the pipeline automatically.
- **No separate UI dev server in production.** Developers still get HMR
  via `pnpm run dev` in `web/`, which proxies API calls to a running
  `deploymonster serve` — the embedded bundle is only used for production
  builds.

## Revisit if

- The UI bundle grows past ~20 MB uncompressed (would signal a
  dependency-bloat problem worth fixing at source).
- We ship a desktop or mobile client that reuses the React source outside
  the Go binary context.
