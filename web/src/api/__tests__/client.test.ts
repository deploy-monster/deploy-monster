import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { api, APIError } from '../client';

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
});
