import { create } from 'zustand';
import { authAPI } from '../api/auth';
import { api } from '../api/client';

interface User {
  id: string;
  email: string;
  name: string;
  role: string;
  tenant_id: string;
}

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;

  login: (email: string, password: string, totpCode?: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => void;
  initialize: () => Promise<void>;
  updateUser: (updates: Partial<User>) => void;
}

let initPromise: Promise<void> | null = null;

/** Test-only reset for the initialization singleton state. */
export function __resetInitStateForTests(): void {
  initPromise = null;
}

interface MeResponse {
  user: { id: string; email: string; name: string; role?: string };
  membership: { role_id: string; tenant_id: string };
  role_id: string;
  tenant_id: string;
}

/** Map /auth/me response shape to internal User type. */
function mapUser(resp: MeResponse): User {
  return {
    id: resp.user.id,
    email: resp.user.email,
    name: resp.user.name,
    role: resp.role_id || resp.membership?.role_id || '',
    tenant_id: resp.tenant_id || resp.membership?.tenant_id || '',
  };
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,
  isLoading: true,

  login: async (email, password, totpCode) => {
    try {
      await authAPI.login({ email, password, totp_code: totpCode || undefined });
      // Use /auth/me to get verified user info instead of decoding JWT client-side
      const resp = await api.get<MeResponse>('/auth/me');
      if (resp?.user?.id) {
        set({ user: mapUser(resp), isAuthenticated: true });
      }
    } catch (e) {
      set({ isLoading: false });
      throw e; // re-throw so caller can handle the error
    }
  },

  register: async (email, password, name) => {
    try {
      await authAPI.register({ email, password, name });
      // Use /auth/me to get verified user info instead of decoding JWT client-side
      const resp = await api.get<MeResponse>('/auth/me');
      if (resp?.user?.id) {
        set({ user: mapUser(resp), isAuthenticated: true });
      }
    } catch (e) {
      set({ isLoading: false });
      throw e;
    }
  },

  logout: () => {
    // POST to logout endpoint (clears server-side cookies)
    api.post('/auth/logout', {}).catch(() => {});
    set({ user: null, isAuthenticated: false });
  },

  updateUser: (updates) => {
    set((state) => ({
      user: state.user ? { ...state.user, ...updates } : state.user,
    }));
  },

  initialize: async () => {
    // Prevent double initialization in React 19 StrictMode - reuse same promise
    if (initPromise) {
      return initPromise;
    }

    initPromise = (async () => {
      try {
        // Backend /me returns { user, membership, role_id, tenant_id }
        const resp = await api.get<MeResponse>('/auth/me');
        if (resp?.user?.id) {
          set({ user: mapUser(resp), isAuthenticated: true, isLoading: false });
          return;
        }
      } catch {
        // Not authenticated or cookies expired
      }
      set({ user: null, isAuthenticated: false, isLoading: false });
    })();

    return initPromise;
  },
}));
