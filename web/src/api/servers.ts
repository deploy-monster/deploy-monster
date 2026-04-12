import { api } from './client';

export interface ServerNode {
  id: string;
  hostname: string;
  ip_address: string;
  provider: string;
  region: string;
  size: string;
  status: string;
  role: string;
  created_at: string;
}

export interface CreateServerRequest {
  hostname: string;
  ip_address: string;
  provider: string;
  region: string;
  size: string;
}

export const serversAPI = {
  list: () => api.get<ServerNode[]>('/servers'),
  create: (data: CreateServerRequest) => api.post<ServerNode>('/servers', data),
};
