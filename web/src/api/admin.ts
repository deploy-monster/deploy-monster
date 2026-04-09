import { api } from './client';

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

export const adminAPI = {
  system: () => api.get<SystemInfo>('/admin/system'),
  tenants: () => api.get<Tenant[]>('/admin/tenants'),
  saveSettings: (data: AdminSettings) => api.put('/admin/settings', data),
  generateApiKey: () => api.post<{ key: string }>('/admin/api-keys'),
};
