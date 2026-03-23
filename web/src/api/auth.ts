import { api } from './client';

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  token_type: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface RegisterRequest {
  email: string;
  password: string;
  name: string;
}

export const authAPI = {
  login: (data: LoginRequest) => api.post<TokenPair>('/auth/login', data),
  register: (data: RegisterRequest) => api.post<TokenPair>('/auth/register', data),
  refresh: (refreshToken: string) =>
    api.post<TokenPair>('/auth/refresh', { refresh_token: refreshToken }),
};
