import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock API modules before importing the store
vi.mock('../../api/auth', () => ({
  authAPI: {
    login: vi.fn(),
    register: vi.fn(),
  },
}));

vi.mock('../../api/client', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

import { useAuthStore, __resetInitStateForTests } from '../auth';
import { authAPI } from '../../api/auth';
import { api } from '../../api/client';

// Helper: create a fake JWT token with the given payload
function fakeTokenPair(payload: Record<string, string>) {
  const header = btoa(JSON.stringify({ alg: 'HS256' }));
  const body = btoa(JSON.stringify(payload));
  return {
    access_token: `${header}.${body}.signature`,
    refresh_token: 'refresh-token',
    expires_in: 900,
    token_type: 'Bearer',
  };
}

describe('authStore', () => {
  beforeEach(() => {
    __resetInitStateForTests();
    useAuthStore.setState({
      user: null,
      isAuthenticated: false,
      isLoading: true,
    });
    vi.clearAllMocks();
  });

  it('starts unauthenticated', () => {
    const state = useAuthStore.getState();
    expect(state.user).toBeNull();
    expect(state.isAuthenticated).toBe(false);
    expect(state.isLoading).toBe(true);
  });

  describe('login', () => {
    it('sets user from /auth/me on success', async () => {
      vi.mocked(authAPI.login).mockResolvedValue(fakeTokenPair({}));
      vi.mocked(api.get).mockResolvedValue({
        user: { id: 'u1', email: 'test@example.com', name: 'Test User' },
        membership: { role_id: 'admin', tenant_id: 't1' },
        role_id: 'admin',
        tenant_id: 't1',
      });

      await useAuthStore.getState().login('test@example.com', 'password');

      const state = useAuthStore.getState();
      expect(state.isAuthenticated).toBe(true);
      expect(state.user?.id).toBe('u1');
      expect(state.user?.email).toBe('test@example.com');
      expect(state.user?.tenant_id).toBe('t1');
    });

    it('propagates API errors', async () => {
      vi.mocked(authAPI.login).mockRejectedValue(new Error('Invalid credentials'));

      await expect(
        useAuthStore.getState().login('bad@example.com', 'wrong')
      ).rejects.toThrow('Invalid credentials');

      expect(useAuthStore.getState().isAuthenticated).toBe(false);
    });
  });

  describe('register', () => {
    it('sets user from /auth/me on success', async () => {
      vi.mocked(authAPI.register).mockResolvedValue(fakeTokenPair({}));
      vi.mocked(api.get).mockResolvedValue({
        user: { id: 'u2', email: 'new@example.com', name: 'New User' },
        membership: { role_id: 'member', tenant_id: 't2' },
        role_id: 'member',
        tenant_id: 't2',
      });

      await useAuthStore.getState().register('new@example.com', 'password', 'New User');

      const state = useAuthStore.getState();
      expect(state.isAuthenticated).toBe(true);
      expect(state.user?.id).toBe('u2');
    });
  });

  describe('logout', () => {
    it('clears user state and calls API', () => {
      useAuthStore.setState({
        user: { id: 'u1', email: 'a@b.com', name: 'A', role: 'admin', tenant_id: 't1' },
        isAuthenticated: true,
      });
      vi.mocked(api.post).mockResolvedValue(undefined);

      useAuthStore.getState().logout();

      expect(useAuthStore.getState().user).toBeNull();
      expect(useAuthStore.getState().isAuthenticated).toBe(false);
      expect(api.post).toHaveBeenCalledWith('/auth/logout', {});
    });
  });

  describe('updateUser', () => {
    it('merges updates into the current user', () => {
      useAuthStore.setState({
        user: { id: 'u1', email: 'a@b.com', name: 'A', role: 'admin', tenant_id: 't1' },
        isAuthenticated: true,
      });

      useAuthStore.getState().updateUser({ name: 'Alice Zhang' });

      expect(useAuthStore.getState().user).toMatchObject({
        id: 'u1',
        email: 'a@b.com',
        name: 'Alice Zhang',
        role: 'admin',
        tenant_id: 't1',
      });
    });
  });

  describe('initialize', () => {
    it('sets user when /auth/me succeeds', async () => {
      // /auth/me returns a wrapped MeResponse ({user, membership, role_id,
      // tenant_id}) — see stores/auth.ts:48. The test must match that shape
      // or the initialize() branch that reads resp.user.id short-circuits.
      vi.mocked(api.get).mockResolvedValue({
        user: { id: 'u1', email: 'a@b.com', name: 'A' },
        membership: { role_id: 'admin', tenant_id: 't1' },
        role_id: 'admin',
        tenant_id: 't1',
      });

      await useAuthStore.getState().initialize();

      const state = useAuthStore.getState();
      expect(state.isAuthenticated).toBe(true);
      expect(state.user?.id).toBe('u1');
      expect(state.isLoading).toBe(false);
    });

    it('stays unauthenticated when /auth/me fails', async () => {
      vi.mocked(api.get).mockRejectedValue(new Error('401'));

      await useAuthStore.getState().initialize();

      expect(useAuthStore.getState().isAuthenticated).toBe(false);
      expect(useAuthStore.getState().isLoading).toBe(false);
    });
  });
});
