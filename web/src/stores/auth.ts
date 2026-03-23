import { create } from 'zustand';
import { authAPI, type TokenPair } from '../api/auth';

interface User {
  id: string;
  email: string;
  name: string;
  role: string;
  tenant_id: string;
}

interface AuthState {
  token: string | null;
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;

  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => void;
  initialize: () => void;
}

function parseJWT(token: string): Record<string, unknown> | null {
  try {
    const payload = token.split('.')[1];
    return JSON.parse(atob(payload));
  } catch {
    return null;
  }
}

function userFromToken(token: string): User | null {
  const payload = parseJWT(token);
  if (!payload) return null;
  return {
    id: payload.uid as string,
    email: payload.email as string,
    name: (payload.name as string) || (payload.email as string),
    role: payload.rid as string,
    tenant_id: payload.tid as string,
  };
}

function saveTokens(pair: TokenPair) {
  localStorage.setItem('access_token', pair.access_token);
  localStorage.setItem('refresh_token', pair.refresh_token);
}

export const useAuthStore = create<AuthState>((set) => ({
  token: null,
  user: null,
  isAuthenticated: false,
  isLoading: true,

  login: async (email, password) => {
    const pair = await authAPI.login({ email, password });
    saveTokens(pair);
    const user = userFromToken(pair.access_token);
    set({ token: pair.access_token, user, isAuthenticated: true });
  },

  register: async (email, password, name) => {
    const pair = await authAPI.register({ email, password, name });
    saveTokens(pair);
    const user = userFromToken(pair.access_token);
    set({ token: pair.access_token, user, isAuthenticated: true });
  },

  logout: () => {
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
    set({ token: null, user: null, isAuthenticated: false });
  },

  initialize: () => {
    const token = localStorage.getItem('access_token');
    if (token) {
      const user = userFromToken(token);
      if (user) {
        set({ token, user, isAuthenticated: true, isLoading: false });
        return;
      }
    }
    set({ isLoading: false });
  },
}));
