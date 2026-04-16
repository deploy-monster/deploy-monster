# ADR 0010 — Custom `useApi` hook over TanStack Query

- **Status:** Accepted
- **Date:** 2026-04-16
- **Deciders:** Ersin KOÇ (project lead)

## Context

The React UI (`web/`) needs a pattern for talking to the backend
REST API. The space is well-trodden: TanStack Query, SWR, RTK Query,
and Apollo all offer mature data-fetching layers with caching,
deduplication, invalidation, optimistic updates, and background
refetch. Picking one of them is the default recommendation in
"modern React" commentary.

Two things make the default advice a poor fit for DeployMonster
specifically:

1. **The UI is a binary-embedded operator console, not a
   consumer-facing SPA.** ADR 0005 (`embedded-react-ui`) commits the
   React build to ship inside the Go binary via `embed.FS`. Every
   kilobyte of vendor code is shipped to every operator who opens
   the console, and there is no CDN layer to carry the load. The
   bundle budget (ADR-adjacent: `check-bundle-size.mjs` sets 300 KB
   gzipped for the main chunk) is tight enough that a 13 KB gzipped
   dependency for data-fetching is meaningful overhead when the
   entire authenticated main chunk is currently 8 KB gzipped.
2. **The access pattern is navigate-then-read, not
   background-sync.** Pages fetch when mounted, refresh on
   user-visible events (manual reload, post-mutation refetch), and
   throw away their data when the user navigates away. There are no
   cross-page cache coordinations, no windowed subscriptions, no
   infinite-scroll paginations that outlive a single route. A
   fully-featured query client would spend 90% of its complexity on
   features the app doesn't use.

The cost side of the default option is concrete; the benefit side
is speculative ("we might need it later for X"). That asymmetry is
what this ADR resolves.

## Decision

The React UI uses a **custom `useApi<T>` / `useMutation<TInput, TOutput>`
hook family** in `web/src/hooks/useApi.ts` (80 lines total) backed
by a single `api` client in `web/src/api/client.ts` (362 lines
including auth refresh and error handling). No TanStack Query, no
SWR, no RTK Query.

### Shape of the API surface

```ts
// GET with optional auto-refresh interval
const { data, error, loading, refetch } = useApi<App[]>('/apps');

// GET with polling
const { data } = useApi<DeployStatus>(`/apps/${id}/deploy/status`, {
  refreshInterval: 2000,
});

// Mutation with typed input/output
const { mutate, loading } = useMutation<CreateAppInput, App>('post', '/apps');
await mutate({ name: 'my-app', ... });

// Paginated lists
const { data, page, totalPages, setPage } = usePaginatedApi<App>('/apps', 20);
```

The `api` object in `client.ts` provides:

- `get<T>(path)`, `post<T>(path, body)`, `put`, `patch`, `delete`
- Automatic `Authorization: Bearer` header from the auth store
- **Automatic refresh-token rotation on 401** (the non-trivial
  feature that justifies the bespoke client — a single-flight
  refresh guarded by a promise so concurrent 401s don't trigger
  N parallel refresh calls)
- Consistent error shape: `Error` instances with parsed JSON error
  bodies where available, network errors preserved

### Client-side state lives in Zustand

Auth state, theme preference, topology editor state, and deploy
wizard state are in `web/src/stores/`. This is the complement of
the decision: by keeping *client state* in Zustand and *server
state* in `useApi`'s local `useState`, there is no data-fetching
layer that needs to coordinate with a global store — the local hook
state is enough.

## Consequences

**Positive:**

- **Zero external dependency for the dominant use case.** The whole
  data-fetch layer is 442 lines of TypeScript that any contributor
  can read in 15 minutes. There is no "query client" mental model
  to learn, no cache invalidation keys to design, no staleTime /
  cacheTime / gcTime trio to tune per-query.
- **Bundle stays tight.** TanStack Query v5 is ~13 KB gzipped, plus
  a typical `@tanstack/react-query-devtools` dev dependency that
  many teams accidentally ship to prod. The custom hooks add
  roughly zero — they use React primitives (`useState`, `useEffect`,
  `useCallback`) that are already pulled in for everything else.
- **Refetch is explicit.** Every place that needs to refresh after
  a mutation calls `refetch()` directly. Newcomers can grep for
  refetch calls to understand data flow without learning a query-key
  taxonomy.
- **Mocking for tests is trivial.** Stub `api.get` / `api.post` in
  `web/src/api/client.ts`'s module-level export. No need for an
  `msw` server or a `QueryClientProvider` test-wrapper.

**Negative / trade-offs:**

- **No automatic request deduplication.** If three components on
  the same page both call `useApi('/apps')`, that's three network
  requests. Mitigation: lift the call into a parent component and
  pass the result as a prop. This has been the pattern so far and
  the duplication has not shown up as a problem at the scale the
  UI handles (< 100 apps per page, sub-second renders).
- **No background refetch on window focus / reconnect / stale-check.**
  A TanStack-backed UI would automatically refetch when the user
  returns to the tab. Ours does not. For a PaaS operator console
  this is rarely felt — the expected pattern is "click refresh
  when you want fresh data" rather than "trust the UI to know."
  The `refreshInterval` option handles the polling cases that
  legitimately need live updates (deploy status, container stats).
- **Optimistic updates are not built-in.** Every mutation waits for
  the server response before the UI reflects the change. For a
  PaaS tool that's the correct default (an optimistic "app
  created" that fails serverside is worse than a 500 ms wait),
  but any future feature that genuinely needs optimistic UI will
  have to hand-roll it in a single hook.
- **Growing the surface is a linear cost.** A new list pattern
  (infinite scroll, cursor pagination) needs a new hook. TanStack
  would have given it for free. If we ever add 3+ such patterns,
  the per-feature hook count crosses the break-even point with
  adopting a library.

## Alternatives considered

- **TanStack Query** — the incumbent choice. Rejected on bundle cost
  + mental-model surface for a use case that exercises ~20% of the
  library. Not ruled out long-term; see "revisit if" below.
- **SWR** — smaller than TanStack (~4 KB gzipped) but still adds a
  dependency + a cache-key discipline. The advantages over our
  custom hooks are real but marginal; the decision cost / revisit
  cost is higher than the win.
- **RTK Query** — tied to Redux Toolkit, which would also force a
  migration off Zustand. Rejected as scope-creep.
- **Raw `fetch` in every component** — rejected because the
  refresh-token single-flight behavior is non-trivial and deserves
  one owner. That is what `client.ts` is for.

## Revisit if

- **Bundle budget stops being a constraint.** If the UI is ever
  split from the binary (served from a CDN, decoupled update
  cadence), the bundle-size argument weakens. That would also be
  a new ADR.
- **Multi-page cache coordination becomes load-bearing.** Example:
  user edits an app on page A, expects the app list on page B to
  reflect the change without an explicit refetch. Hand-rolling this
  pattern in more than a couple of places is the signal to adopt a
  library.
- **Realtime/subscription data grows beyond the current WebSocket
  surface.** Deploy status and container logs use native WebSockets
  (`web/src/hooks/useDeployProgress.ts`, `useContainerLogs.ts`) and
  are not a data-fetch concern. If the number of such streams grows
  past ~5 and they need to feed into UI state the same way
  REST responses do, the coordination cost would push toward a
  library.
- **Contributors complain about onboarding cost.** The custom hooks
  are simple, but "simple" is a local maximum. If reviewers find
  themselves explaining the hook pattern to every new contributor,
  the industry-standard choice wins on familiarity alone.
