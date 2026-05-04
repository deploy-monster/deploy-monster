import { api, type PaginatedResponse } from './client';

export interface SystemInfo {
  version: string;
  commit: string;
  go: string;
  os: string;
  arch: string;
  goroutines: number;
  memory: { alloc_mb: number; sys_mb: number };
  modules: Array<{ id: string; status: string }>;
  events: { published: number; errors: number; subscriptions: number };
}

export interface Tenant {
  id: string;
  name: string;
  slug: string;
  plan: string;
  status: string;
  members_count: number;
  created_at: string;
}

export interface AdminSettings {
  registration_mode: string;
  auto_ssl: boolean;
  telemetry: boolean;
  backup_retention_days: number;
}

export interface APIKey {
  prefix: string;
  type: string;
  created_by: string;
  created_at: string;
  expires_at?: string;
}

interface GenerateAPIKeyResponse {
  key: string;
  prefix: string;
  type: string;
  message: string;
}

export const adminAPI = {
  system: () => api.get<SystemInfo>('/admin/system'),
  tenants: () => api.get<PaginatedResponse<Tenant>>('/admin/tenants'),
  saveSettings: (data: AdminSettings) => api.patch('/admin/settings', data),
  apiKeys: () => api.get<APIKey[]>('/admin/api-keys'),
  generateApiKey: () => api.post<GenerateAPIKeyResponse>('/admin/api-keys'),
  revokeApiKey: (prefix: string) => api.delete(`/admin/api-keys/${encodeURIComponent(prefix)}`),
};
