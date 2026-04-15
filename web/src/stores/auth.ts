import { create } from 'zustand';
import { authAPI, type TokenPair } from '../api/auth';
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

  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => void;
  initialize: () => Promise<void>;
}

function userFromTokenResponse(pair: TokenPair): User | null {
  // Parse the access token payload for display info
  try {
    const payload = pair.access_token.split('.')[1];
    const decoded = JSON.parse(atob(payload));
    return {
      id: decoded.uid as string,
      email: decoded.email as string,
      name: (decoded.name as string) || (decoded.email as string),
      role: decoded.rid as string,
      tenant_id: decoded.tid as string,
    };
  } catch {
    return null;
  }
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

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,
  isLoading: true,

  login: async (email, password) => {
    const pair = await authAPI.login({ email, password });
    // Cookies are set by the server — we only use the response body for user info
    const user = userFromTokenResponse(pair);
    set({ user, isAuthenticated: true });
  },

  register: async (email, password, name) => {
    const pair = await authAPI.register({ email, password, name });
    const user = userFromTokenResponse(pair);
    set({ user, isAuthenticated: true });
  },

  logout: () => {
    // POST to logout endpoint (clears server-side cookies)
    api.post('/auth/logout', {}).catch(() => {});
    set({ user: null, isAuthenticated: false });
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
          const user: User = {
            id: resp.user.id,
            email: resp.user.email,
            name: resp.user.name,
            role: resp.role_id || resp.membership?.role_id || '',
            tenant_id: resp.tenant_id || resp.membership?.tenant_id || '',
          };
          set({ user, isAuthenticated: true, isLoading: false });
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
