import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { api, APIError, __resetRefreshStateForTests } from '../client';

describe('API client', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
    // Clear cookies
    document.cookie = 'dm_csrf=; max-age=0';
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  function mockFetch(status: number, body: unknown, ok = status < 400) {
    vi.mocked(globalThis.fetch).mockResolvedValue({
      ok,
      status,
      statusText: 'OK',
      json: () => Promise.resolve(body),
      headers: new Headers(),
    } as Response);
  }

  describe('api.get', () => {
    it('sends GET request with credentials', async () => {
      mockFetch(200, { id: '1', name: 'App' });

      const result = await api.get<{ id: string; name: string }>('/apps/1');

      expect(result).toEqual({ id: '1', name: 'App' });
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/apps/1',
        expect.objectContaining({
          method: 'GET',
          credentials: 'include',
        })
      );
    });

    it('unwraps {data: ...} responses', async () => {
      mockFetch(200, { data: [{ id: '1' }, { id: '2' }] });

      const result = await api.get<Array<{ id: string }>>('/apps');

      expect(result).toEqual([{ id: '1' }, { id: '2' }]);
    });

    it('returns undefined for 204 No Content', async () => {
      vi.mocked(globalThis.fetch).mockResolvedValue({
        ok: true,
        status: 204,
        statusText: 'No Content',
        json: () => Promise.reject(new Error('no body')),
        headers: new Headers(),
      } as Response);

      const result = await api.get('/some/endpoint');
      expect(result).toBeUndefined();
    });
  });

  describe('api.post', () => {
    it('sends POST with JSON body', async () => {
      mockFetch(200, { id: 'new' });

      await api.post('/apps', { name: 'MyApp' });

      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/apps',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ name: 'MyApp' }),
        })
      );
    });

    it('includes CSRF token from cookie', async () => {
      document.cookie = 'dm_csrf=test-csrf-token';
      mockFetch(200, {});

      await api.post('/apps', { name: 'App' });

      const headers = vi.mocked(globalThis.fetch).mock.calls[0][1]?.headers as Record<string, string>;
      expect(headers['X-CSRF-Token']).toBe('test-csrf-token');
    });
  });

  describe('error handling', () => {
    it('throws APIError on non-ok response', async () => {
      mockFetch(400, { error: 'Bad request' }, false);

      await expect(api.get('/bad')).rejects.toThrow(APIError);
      await expect(api.get('/bad')).rejects.toThrow('Bad request');
    });

    it('APIError includes status code', async () => {
      mockFetch(404, { error: 'Not found' }, false);

      try {
        await api.get('/missing');
        expect.fail('should throw');
      } catch (err) {
        expect(err).toBeInstanceOf(APIError);
        expect((err as APIError).status).toBe(404);
      }
    });
  });

  describe('other methods', () => {
    it('api.put sends PUT', async () => {
      mockFetch(200, {});
      await api.put('/apps/1', { name: 'Updated' });
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/apps/1',
        expect.objectContaining({ method: 'PUT' })
      );
    });

    it('api.patch sends PATCH', async () => {
      mockFetch(200, {});
      await api.patch('/apps/1', { name: 'Patched' });
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/apps/1',
        expect.objectContaining({ method: 'PATCH' })
      );
    });

    it('api.delete sends DELETE', async () => {
      mockFetch(200, {});
      await api.delete('/apps/1');
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/apps/1',
        expect.objectContaining({ method: 'DELETE' })
      );
    });
  });

  describe('timeout (Phase 3.4.8)', () => {
    it('attaches an AbortSignal to every request', async () => {
      const captured: AbortSignal[] = [];
      vi.mocked(globalThis.fetch).mockImplementation(async (_url, init) => {
        const sig = (init as RequestInit).signal;
        if (sig) captured.push(sig as AbortSignal);
        return {
          ok: true,
          status: 200,
          statusText: 'OK',
          json: () => Promise.resolve({}),
          headers: new Headers(),
        } as Response;
      });

      await api.get('/foo');
      await api.post('/foo', { x: 1 });

      expect(captured.length).toBe(2);
      expect(captured[0]).toBeInstanceOf(AbortSignal);
      expect(captured[1]).toBeInstanceOf(AbortSignal);
    });

    it('aborts after per-call timeout and throws APIError 408', async () => {
      vi.useFakeTimers();
      try {
        vi.mocked(globalThis.fetch).mockImplementation(
          (_url, init) =>
            new Promise((_resolve, reject) => {
              const signal = (init as RequestInit).signal as AbortSignal | undefined;
              if (!signal) return;
              signal.addEventListener('abort', () => {
                reject(new DOMException('aborted', 'AbortError'));
              });
            })
        );

        const promise = api.get('/slow', { timeout: 50 }).catch((e) => e);
        await vi.advanceTimersByTimeAsync(60);
        const err = await promise;

        expect(err).toBeInstanceOf(APIError);
        expect((err as APIError).status).toBe(408);
        expect((err as APIError).message).toContain('50ms');
      } finally {
        vi.useRealTimers();
      }
    });

    it('classifies caller-signal cancels as APIError 0, not 408', async () => {
      const abortable = new AbortController();
      vi.mocked(globalThis.fetch).mockImplementation(
        (_url, init) =>
          new Promise((_resolve, reject) => {
            const signal = (init as RequestInit).signal as AbortSignal | undefined;
            if (!signal) return;
            signal.addEventListener('abort', () => {
              reject(new DOMException('aborted', 'AbortError'));
            });
          })
      );

      const promise = api.get('/never', { signal: abortable.signal }).catch((e) => e);
      abortable.abort();
      const err = await promise;

      expect(err).toBeInstanceOf(APIError);
      expect((err as APIError).status).toBe(0);
      expect((err as APIError).message).toBe('request cancelled');
    });

    it('rejects immediately when caller signal already aborted', async () => {
      const pre = new AbortController();
      pre.abort();
      vi.mocked(globalThis.fetch).mockImplementation(
        (_url, init) =>
          new Promise((_resolve, reject) => {
            const signal = (init as RequestInit).signal as AbortSignal | undefined;
            if (signal?.aborted) {
              reject(new DOMException('aborted', 'AbortError'));
              return;
            }
            signal?.addEventListener('abort', () => {
              reject(new DOMException('aborted', 'AbortError'));
            });
          })
      );

      const err = await api.get('/nope', { signal: pre.signal }).catch((e) => e);
      expect(err).toBeInstanceOf(APIError);
      expect((err as APIError).status).toBe(0);
    });
  });

  describe('retry on 5xx (Phase 3.4.9)', () => {
    function jsonResp(status: number, body: unknown, ok = status < 400): Response {
      return {
        ok,
        status,
        statusText: 'x',
        json: () => Promise.resolve(body),
        headers: new Headers(),
      } as Response;
    }

    it('retries 503 and succeeds on 2nd attempt', async () => {
      vi.useFakeTimers();
      try {
        let call = 0;
        vi.mocked(globalThis.fetch).mockImplementation(async () => {
          call++;
          if (call === 1) return jsonResp(503, { error: 'down' }, false);
          return jsonResp(200, { ok: true });
        });

        const promise = api.get<{ ok: boolean }>('/thing');
        await vi.advanceTimersByTimeAsync(5_000);
        const result = await promise;

        expect(result).toEqual({ ok: true });
        expect(globalThis.fetch).toHaveBeenCalledTimes(2);
      } finally {
        vi.useRealTimers();
      }
    });

    it('retries up to 2 times on 502 then surfaces final APIError', async () => {
      vi.useFakeTimers();
      try {
        vi.mocked(globalThis.fetch).mockResolvedValue(
          jsonResp(502, { error: 'upstream dead' }, false),
        );

        const promise = api.get('/thing').catch((e) => e);
        await vi.advanceTimersByTimeAsync(10_000);
        const err = await promise;

        expect(globalThis.fetch).toHaveBeenCalledTimes(3);
        expect(err).toBeInstanceOf(APIError);
        expect((err as APIError).status).toBe(502);
      } finally {
        vi.useRealTimers();
      }
    });

    it('retries on 504 Gateway Timeout', async () => {
      vi.useFakeTimers();
      try {
        let call = 0;
        vi.mocked(globalThis.fetch).mockImplementation(async () => {
          call++;
          if (call < 3) return jsonResp(504, { error: 'gateway' }, false);
          return jsonResp(200, { recovered: true });
        });

        const promise = api.get<{ recovered: boolean }>('/thing');
        await vi.advanceTimersByTimeAsync(10_000);
        const result = await promise;

        expect(result).toEqual({ recovered: true });
        expect(globalThis.fetch).toHaveBeenCalledTimes(3);
      } finally {
        vi.useRealTimers();
      }
    });

    it('does NOT retry on 500 (non-gateway server error)', async () => {
      vi.mocked(globalThis.fetch).mockResolvedValue(
        jsonResp(500, { error: 'boom' }, false),
      );

      await expect(api.get('/thing')).rejects.toThrow(APIError);
      expect(globalThis.fetch).toHaveBeenCalledTimes(1);
    });

    it('does NOT retry on 4xx', async () => {
      vi.mocked(globalThis.fetch).mockResolvedValue(
        jsonResp(404, { error: 'not found' }, false),
      );

      await expect(api.get('/thing')).rejects.toThrow(APIError);
      expect(globalThis.fetch).toHaveBeenCalledTimes(1);
    });

    it('honors retries: 0 to disable retries entirely', async () => {
      vi.mocked(globalThis.fetch).mockResolvedValue(
        jsonResp(503, { error: 'down' }, false),
      );

      await expect(api.get('/thing', { retries: 0 })).rejects.toThrow(APIError);
      expect(globalThis.fetch).toHaveBeenCalledTimes(1);
    });

    it('retries on TypeError (network failure)', async () => {
      vi.useFakeTimers();
      try {
        let call = 0;
        vi.mocked(globalThis.fetch).mockImplementation(async () => {
          call++;
          if (call === 1) throw new TypeError('network down');
          return jsonResp(200, { recovered: true });
        });

        const promise = api.get<{ recovered: boolean }>('/thing');
        await vi.advanceTimersByTimeAsync(5_000);
        const result = await promise;

        expect(result).toEqual({ recovered: true });
        expect(globalThis.fetch).toHaveBeenCalledTimes(2);
      } finally {
        vi.useRealTimers();
      }
    });

    it('does NOT retry after a 408 timeout — AbortError is terminal', async () => {
      vi.useFakeTimers();
      try {
        vi.mocked(globalThis.fetch).mockImplementation(
          (_url, init) =>
            new Promise((_resolve, reject) => {
              const signal = (init as RequestInit).signal as AbortSignal | undefined;
              signal?.addEventListener('abort', () => {
                reject(new DOMException('aborted', 'AbortError'));
              });
            }),
        );

        const promise = api.get('/slow', { timeout: 10 }).catch((e) => e);
        await vi.advanceTimersByTimeAsync(50);
        const err = await promise;

        expect(err).toBeInstanceOf(APIError);
        expect((err as APIError).status).toBe(408);
        expect(globalThis.fetch).toHaveBeenCalledTimes(1);
      } finally {
        vi.useRealTimers();
      }
    });

    it('caller abort during backoff short-circuits to APIError 0', async () => {
      vi.useFakeTimers();
      try {
        const controller = new AbortController();
        let call = 0;
        vi.mocked(globalThis.fetch).mockImplementation(
          (_url, init) => {
            call++;
            const signal = (init as RequestInit).signal as AbortSignal | undefined;
            if (signal?.aborted) {
              return Promise.reject(new DOMException('aborted', 'AbortError'));
            }
            if (call === 1) {
              return Promise.resolve(jsonResp(503, { error: 'down' }, false));
            }
            return new Promise((_resolve, reject) => {
              signal?.addEventListener('abort', () => {
                reject(new DOMException('aborted', 'AbortError'));
              });
            });
          },
        );

        const promise = api.get('/thing', { signal: controller.signal }).catch((e) => e);
        // Let the first fetch resolve and the backoff timer arm.
        await Promise.resolve();
        controller.abort();
        await vi.advanceTimersByTimeAsync(5_000);
        const err = await promise;

        expect(err).toBeInstanceOf(APIError);
        expect((err as APIError).status).toBe(0);
      } finally {
        vi.useRealTimers();
      }
    });
  });

  describe('refresh cap (Phase 3.4.10)', () => {
    const originalLocation = window.location;
    let locationHref = '';

    function installLocationStub() {
      locationHref = '';
      Object.defineProperty(window, 'location', {
        configurable: true,
        writable: true,
        value: {
          get href() {
            return locationHref;
          },
          set href(val: string) {
            locationHref = val;
          },
        },
      });
    }

    beforeEach(() => {
      __resetRefreshStateForTests();
      installLocationStub();
    });

    afterEach(() => {
      Object.defineProperty(window, 'location', {
        configurable: true,
        writable: true,
        value: originalLocation,
      });
    });

    function jsonResp(status: number, body: unknown, ok = status < 400): Response {
      return {
        ok,
        status,
        statusText: 'x',
        json: () => Promise.resolve(body),
        headers: new Headers(),
      } as Response;
    }

    it('refreshes once per 401 chain and then hands off to /login on second 401', async () => {
      const calls: string[] = [];
      vi.mocked(globalThis.fetch).mockImplementation(async (url) => {
        const u = String(url);
        calls.push(u);
        if (u.endsWith('/auth/refresh')) return jsonResp(200, {});
        return jsonResp(401, { error: 'unauth' }, false);
      });

      const err = await api.get('/thing').catch((e) => e);

      expect(err).toBeInstanceOf(APIError);
      expect((err as APIError).status).toBe(401);
      expect(locationHref).toBe('/login');
      // exactly one /auth/refresh, no matter how many 401s came back
      const refreshCalls = calls.filter((c) => c.endsWith('/auth/refresh'));
      expect(refreshCalls.length).toBe(1);
    });

    it('coalesces concurrent 401s into a single /auth/refresh call', async () => {
      const pathCalls: Record<string, number> = {};
      vi.mocked(globalThis.fetch).mockImplementation(async (url) => {
        const u = String(url);
        if (u.endsWith('/auth/refresh')) return jsonResp(200, {});
        pathCalls[u] = (pathCalls[u] ?? 0) + 1;
        if (pathCalls[u] === 1) return jsonResp(401, { error: 'unauth' }, false);
        return jsonResp(200, { path: u });
      });

      const results = await Promise.all([
        api.get<{ path: string }>('/a'),
        api.get<{ path: string }>('/b'),
        api.get<{ path: string }>('/c'),
      ]);

      expect(results.map((r) => r.path)).toEqual(['/api/v1/a', '/api/v1/b', '/api/v1/c']);
      const allCalls = vi.mocked(globalThis.fetch).mock.calls.map((c) => String(c[0]));
      const refreshCalls = allCalls.filter((c) => c.endsWith('/auth/refresh'));
      expect(refreshCalls.length).toBe(1);
    });

    it('enters 30s cooldown after a failed refresh — second 401 skips /auth/refresh', async () => {
      vi.mocked(globalThis.fetch).mockImplementation(async (url) => {
        const u = String(url);
        if (u.endsWith('/auth/refresh')) return jsonResp(500, {}, false);
        return jsonResp(401, { error: 'unauth' }, false);
      });

      const first = await api.get('/a').catch((e) => e);
      expect(first).toBeInstanceOf(APIError);
      expect(locationHref).toBe('/login');

      // Reset href so the second failure still surfaces cleanly.
      locationHref = '';

      const second = await api.get('/b').catch((e) => e);
      expect(second).toBeInstanceOf(APIError);
      expect(locationHref).toBe('/login');

      const allCalls = vi.mocked(globalThis.fetch).mock.calls.map((c) => String(c[0]));
      const refreshCalls = allCalls.filter((c) => c.endsWith('/auth/refresh'));
      // Only the first 401 hit /auth/refresh; the second was inside
      // the cooldown window so it skipped straight to /login.
      expect(refreshCalls.length).toBe(1);
    });

    it('cooldown expires after 30s and the next 401 refreshes again', async () => {
      vi.useFakeTimers();
      try {
        vi.mocked(globalThis.fetch).mockImplementation(async (url) => {
          const u = String(url);
          if (u.endsWith('/auth/refresh')) return jsonResp(500, {}, false);
          return jsonResp(401, { error: 'unauth' }, false);
        });

        const p1 = api.get('/a').catch((e) => e);
        await vi.advanceTimersByTimeAsync(100);
        await p1;

        // Past the 30s cooldown window.
        await vi.advanceTimersByTimeAsync(31_000);

        const p2 = api.get('/b').catch((e) => e);
        await vi.advanceTimersByTimeAsync(100);
        await p2;

        const allCalls = vi.mocked(globalThis.fetch).mock.calls.map((c) => String(c[0]));
        const refreshCalls = allCalls.filter((c) => c.endsWith('/auth/refresh'));
        expect(refreshCalls.length).toBe(2);
      } finally {
        vi.useRealTimers();
      }
    });

    it('successful refresh on 1st 401 then immediate 2nd 401 does NOT re-refresh', async () => {
      const pathCalls: Record<string, number> = {};
      vi.mocked(globalThis.fetch).mockImplementation(async (url) => {
        const u = String(url);
        if (u.endsWith('/auth/refresh')) return jsonResp(200, {});
        pathCalls[u] = (pathCalls[u] ?? 0) + 1;
        return jsonResp(401, { error: 'still unauth' }, false);
      });

      const err = await api.get('/protected').catch((e) => e);
      expect(err).toBeInstanceOf(APIError);
      expect((err as APIError).status).toBe(401);
      expect(locationHref).toBe('/login');

      const allCalls = vi.mocked(globalThis.fetch).mock.calls.map((c) => String(c[0]));
      const refreshCalls = allCalls.filter((c) => c.endsWith('/auth/refresh'));
      expect(refreshCalls.length).toBe(1);
      // Two hits on /protected: initial + one retry after successful refresh.
      expect(pathCalls['/api/v1/protected']).toBe(2);
    });
  });
});
