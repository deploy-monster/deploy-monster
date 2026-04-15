const API_BASE = '/api/v1';

// Phase 3.4.8: every fetch must be bounded. A hung backend used to
// wedge the UI indefinitely because client.ts had no AbortController
// plumbing — callers could only wait for the browser's own very long
// network timeout. 30s is generous enough for slow DB queries but
// tight enough that the user sees a real error instead of a stuck
// spinner. tryRefresh gets a shorter cap since the refresh endpoint
// is lightweight and a slow refresh blocks every retry afterwards.
const DEFAULT_TIMEOUT_MS = 30_000;
const REFRESH_TIMEOUT_MS = 10_000;

// Phase 3.4.9: transparent retry for gateway-class server errors.
// 502/503/504 strongly imply the upstream never processed the
// request, so retrying is safe for all HTTP verbs. 500 is explicitly
// excluded because it often means "I saw your request and blew up",
// which makes retrying non-idempotent writes dangerous. MAX_RETRIES
// is the number of EXTRA attempts after the first — total calls =
// attempts + 1. Backoff is exponential with jitter to avoid a
// synchronised retry storm when a backend briefly flaps.
const MAX_RETRIES = 2;
const RETRY_BASE_DELAY_MS = 300;
const RETRY_MAX_DELAY_MS = 2_000;
const RETRIABLE_STATUSES = new Set([502, 503, 504]);

// Phase 3.4.10: tryRefresh used to loop. A 401 would kick off a
// refresh, the refresh would succeed, the retry would 401 again
// (because the server REALLY doesn't like this user), and the retry
// would kick off ANOTHER refresh. Worse: N parallel requests all
// getting 401 at once would fire N refreshes. The fix has three
// parts:
//
//  1. Per-request cap: a recursive retry after a successful refresh
//     sets _noRefresh=true, so a second 401 in the same call chain
//     hands off to /login instead of refreshing again.
//  2. Concurrent coalescing: if a refresh is already in-flight, new
//     callers await the same promise.
//  3. Failure cooldown: after a refresh failure, wait 30s before
//     hitting /auth/refresh again — the session is dead and the
//     user has to log in anyway.
const REFRESH_COOLDOWN_MS = 30_000;
let inFlightRefresh: Promise<boolean> | null = null;
let lastRefreshFailureAt = 0;

interface RequestOptions {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
  /** Per-request timeout in milliseconds. Defaults to DEFAULT_TIMEOUT_MS. */
  timeout?: number;
  /** Caller-supplied abort signal merged with the timeout signal. */
  signal?: AbortSignal;
  /** Extra retries on 502/503/504 or transient network error. Default MAX_RETRIES. */
  retries?: number;
  /**
   * Internal flag set by the 401-retry path to prevent refresh loops.
   * When true, a 401 response skips tryRefresh and hands off to /login.
   */
  _noRefresh?: boolean;
}

/** Options accepted by every public verb on `api`. */
export interface CallOptions {
  timeout?: number;
  signal?: AbortSignal;
  retries?: number;
}

class APIError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'APIError';
    this.status = status;
  }
}

function getCSRFToken(): string {
  // SECURITY FIX: Backend sets cookie as __Host-dm_csrf, frontend was looking for dm_csrf
  const match = document.cookie.match(/(?:^|;\s*)__Host-dm_csrf=([^;]*)/);
  return match ? match[1] : '';
}

// sleep never rejects — on abort it short-circuits and returns. The
// retry loop then proceeds to the next doFetch, which sees the
// aborted signal and throws APIError(0) from its abort branch,
// producing a single consistent cancellation path.
function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal?.aborted) {
      resolve();
      return;
    }
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort);
      resolve();
    }, ms);
    const onAbort = () => {
      clearTimeout(timer);
      resolve();
    };
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}

// redirectToLogin navigates to the login page without a hard reload
// when the user is already on /login or /register. This prevents the
// infinite refresh loop: /login → initialize() → 401 on /auth/me →
// window.location.href='/login' → React re-mounts → initialize() again.
function redirectToLogin(): void {
  const path = window.location.pathname;
  if (path === '/login' || path === '/register') {
    // Already on an auth page — no point reloading. The auth store
    // will have already set isAuthenticated=false, so ProtectedRoute
    // won't interfere, and the login/register forms remain visible.
    return;
  }
  window.location.href = '/login';
}

function backoffDelay(attempt: number): number {
  // attempt is 0-indexed: first retry uses attempt=0, second uses 1.
  // exponential + full jitter: cap*(2^attempt) ± random(0..base).
  const exp = RETRY_BASE_DELAY_MS * Math.pow(2, attempt);
  const jitter = Math.random() * RETRY_BASE_DELAY_MS;
  return Math.min(RETRY_MAX_DELAY_MS, exp + jitter);
}

// doFetch issues a single bounded HTTP request. It encapsulates the
// AbortController plumbing so the retry loop above does not need to
// worry about signal lifecycle. Returns the Response or throws an
// APIError (0 = caller cancel, 408 = deadline) or re-throws whatever
// other error fetch produced (TypeError for network failures).
async function doFetch(
  path: string,
  method: string,
  headers: Record<string, string>,
  body: unknown,
  timeout: number,
  callerSignal: AbortSignal | undefined,
): Promise<Response> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeout);
  const onCallerAbort = () => controller.abort();
  if (callerSignal) {
    if (callerSignal.aborted) {
      controller.abort();
    } else {
      callerSignal.addEventListener('abort', onCallerAbort, { once: true });
    }
  }

  const config: RequestInit = {
    method,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...headers,
    },
    signal: controller.signal,
  };
  if (body !== undefined && body !== null) {
    config.body = JSON.stringify(body);
  }

  try {
    return await fetch(`${API_BASE}${path}`, config);
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      if (callerSignal?.aborted) {
        throw new APIError(0, 'request cancelled');
      }
      throw new APIError(408, `request timed out after ${timeout}ms`);
    }
    throw err;
  } finally {
    clearTimeout(timer);
    if (callerSignal) {
      callerSignal.removeEventListener('abort', onCallerAbort);
    }
  }
}

// Only transient errors get retried. APIError 0 (caller cancel) and
// 408 (deadline) are explicitly NOT retried — the caller asked us to
// stop, and re-trying a deadline with the same budget just wedges for
// another full timeout.
function isTransientNetworkError(err: unknown): boolean {
  if (err instanceof APIError) return false;
  // TypeError from fetch indicates a generic network failure —
  // DNS miss, TCP reset, offline, TLS handshake failure. Retrying
  // these is safe and usually succeeds on a blip.
  return err instanceof TypeError;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const {
    method = 'GET',
    body,
    headers = {},
    timeout = DEFAULT_TIMEOUT_MS,
    signal: callerSignal,
    retries = MAX_RETRIES,
  } = options;

  // Add CSRF token for mutating requests
  if (method !== 'GET' && method !== 'HEAD') {
    const csrf = getCSRFToken();
    if (csrf) {
      headers['X-CSRF-Token'] = csrf;
    }
  }

  let response: Response | undefined;
  for (let attempt = 0; ; attempt++) {
    try {
      response = await doFetch(path, method, headers, body, timeout, callerSignal);
    } catch (err) {
      if (attempt < retries && isTransientNetworkError(err)) {
        await sleep(backoffDelay(attempt), callerSignal);
        continue;
      }
      throw err;
    }

    // Gateway-class errors: retry if attempts remain, otherwise fall
    // through to the normal error handling path so the caller sees
    // the final status code on the last response.
    if (attempt < retries && RETRIABLE_STATUSES.has(response.status)) {
      await sleep(backoffDelay(attempt), callerSignal);
      continue;
    }
    break;
  }

  if (!response) {
    // Unreachable — loop either returns response or throws. Belt and
    // braces so TypeScript's narrowing is happy.
    throw new APIError(0, 'request failed');
  }

  // Handle 401 - try refresh exactly once per logical request.
  if (response.status === 401) {
    if (options._noRefresh) {
      // We already refreshed once in this chain and STILL got 401 —
      // the session is genuinely dead. Hand off to login.
      redirectToLogin();
      throw new APIError(401, 'Session expired');
    }
    const refreshed = await tryRefresh();
    if (refreshed) {
      return request<T>(path, { ...options, _noRefresh: true });
    }
    redirectToLogin();
    throw new APIError(401, 'Session expired');
  }

  if (!response.ok) {
    const data = await response.json().catch(() => ({ error: response.statusText }));
    // SECURITY FIX: Sanitize error message to prevent potential XSS through error messages
    const rawError = data.error || response.statusText;
    // Remove potential HTML/script tags from error messages
    const sanitizedError = typeof rawError === 'string'
      ? rawError.replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '[script removed]')
                .replace(/<[^>]*>/g, '') // Remove all HTML tags
      : 'An error occurred';
    throw new APIError(response.status, sanitizedError);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const json = await response.json();

  // Unwrap {data: ...} responses — but only when it's a simple list wrapper.
  // Common pattern: {"data": [...], "total": N} → unwrap to the array.
  // Marketplace pattern: {"data": [...], "categories": [...], "total": N} →
  //   keep as-is (caller needs all keys).  We detect this by checking if the
  //   response has more than 2 keys — the common wrapper has only "data" and
  //   optionally "total".
  if (json && typeof json === 'object' && 'data' in json && !('error' in json)) {
    const keys = Object.keys(json);
    const isSimpleWrapper = keys.length <= 2; // {"data": ...} or {"data": ..., "total": N}
    if (isSimpleWrapper) {
      return json.data as T;
    }
  }

  return json as T;
}

async function tryRefresh(): Promise<boolean> {
  // Cooldown: if a refresh failed recently, short-circuit so a
  // burst of 401s doesn't pummel /auth/refresh on every request.
  if (Date.now() - lastRefreshFailureAt < REFRESH_COOLDOWN_MS) {
    return false;
  }

  // Coalesce concurrent refreshes — every parallel caller awaits
  // the same in-flight promise so only ONE /auth/refresh hits the
  // server regardless of how many requests got 401 simultaneously.
  if (inFlightRefresh) {
    return inFlightRefresh;
  }

  inFlightRefresh = (async () => {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), REFRESH_TIMEOUT_MS);
    try {
      const response = await fetch(`${API_BASE}/auth/refresh`, {
        method: 'POST',
        credentials: 'include', // sends dm_refresh cookie
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
        signal: controller.signal,
      });
      if (!response.ok) {
        lastRefreshFailureAt = Date.now();
        return false;
      }
      // Cookies are set by the server — nothing to store client-side
      return true;
    } catch {
      lastRefreshFailureAt = Date.now();
      return false;
    } finally {
      clearTimeout(timer);
      inFlightRefresh = null;
    }
  })();

  return inFlightRefresh;
}

/**
 * Test-only reset for the refresh-coalescing singleton state.
 * Not re-exported from the public `api` surface; vitest imports it
 * directly from this module.
 */
export function __resetRefreshStateForTests(): void {
  inFlightRefresh = null;
  lastRefreshFailureAt = 0;
}

export const api = {
  get: <T>(path: string, opts?: CallOptions) =>
    request<T>(path, { ...opts }),
  post: <T>(path: string, body?: unknown, opts?: CallOptions) =>
    request<T>(path, { method: 'POST', body, ...opts }),
  put: <T>(path: string, body?: unknown, opts?: CallOptions) =>
    request<T>(path, { method: 'PUT', body, ...opts }),
  patch: <T>(path: string, body?: unknown, opts?: CallOptions) =>
    request<T>(path, { method: 'PATCH', body, ...opts }),
  delete: <T>(path: string, opts?: CallOptions) =>
    request<T>(path, { method: 'DELETE', ...opts }),
};

export { APIError };
