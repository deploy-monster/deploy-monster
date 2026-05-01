import { api } from './client';

export interface GitProvider {
  id: string;
  name: string;
  type: string;
  connected: boolean;
  repo_count: number;
  url?: string;
}

interface ConnectGitProviderRequest {
  type: string;
  token: string;
  url?: string;
}

export const gitSourcesAPI = {
  list: () => api.get<GitProvider[]>('/git/providers'),
  connect: (data: ConnectGitProviderRequest) => api.post<GitProvider>('/git/providers', data),
  disconnect: (id: string) => api.delete(`/git/providers/${id}`),
};
