import { api } from './client';

export interface DashboardStats {
  apps: { total: number };
  containers: { running: number; stopped: number; total: number };
  domains: number;
  projects: number;
  events: { published: number; errors: number };
}

export interface ActivityEntry {
  id: number;
  action: string;
  resource_type: string;
  resource_id: string;
  created_at: string;
}

export const dashboardAPI = {
  stats: () => api.get<DashboardStats>('/dashboard/stats'),
  activity: (limit = 10) => api.get<{ data: ActivityEntry[] }>(`/activity?limit=${limit}`),
  announcements: () => api.get<{ data: Array<{ id: string; title: string; body: string; type: string }> }>('/announcements'),
  search: (q: string) => api.get<{ results: Array<{ type: string; id: string; name: string; info: string }> }>(`/search?q=${q}`),
};
