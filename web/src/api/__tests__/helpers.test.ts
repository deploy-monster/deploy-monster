import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { adminAPI } from '../admin';
import { backupsAPI } from '../backups';
import { gitSourcesAPI } from '../git-sources';
import { secretsAPI } from '../secrets';
import { teamAPI } from '../team';

describe('API helper response contracts', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  function mockJSON(body: unknown) {
    vi.mocked(globalThis.fetch).mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: () => Promise.resolve(body),
      headers: new Headers(),
    } as Response);
  }

  it('preserves admin API key list metadata', async () => {
    const envelope = {
      data: [{ prefix: 'dm_live', type: 'platform', created_by: 'u1', created_at: '2026-01-01T00:00:00Z' }],
      total: 1,
    };
    mockJSON(envelope);

    await expect(adminAPI.apiKeys()).resolves.toEqual(envelope);
  });

  it('preserves team member list metadata', async () => {
    const envelope = {
      data: [{
        id: 'tm1',
        name: 'Alice',
        email: 'alice@example.com',
        role: 'role_admin',
        joined_at: '2026-01-01T00:00:00Z',
      }],
      total: 1,
    };
    mockJSON(envelope);

    await expect(teamAPI.members()).resolves.toEqual(envelope);
  });

  it('preserves backup list metadata', async () => {
    const envelope = {
      data: [{ key: 'tenant/full.tar.gz', size: 1024, type: 'full', status: 'completed', created_at: 1767225600 }],
      total: 1,
    };
    mockJSON(envelope);

    await expect(backupsAPI.list()).resolves.toEqual(envelope);
  });

  it('preserves secret list metadata', async () => {
    const envelope = {
      data: [{ id: 's1', name: 'DB_PASSWORD', scope: 'tenant', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' }],
      total: 1,
    };
    mockJSON(envelope);

    await expect(secretsAPI.list()).resolves.toEqual(envelope);
  });

  it('unwraps bare data-only helper responses', async () => {
    const providers = [{ id: 'github', name: 'GitHub', type: 'github', connected: false, repo_count: 0 }];
    mockJSON({ data: providers });

    await expect(gitSourcesAPI.list()).resolves.toEqual(providers);
  });
});
