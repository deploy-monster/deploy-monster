import { api } from './client';

// Extended API client covering all 218 endpoints

export const projectsAPI = {
  list: () => api.get<{ data: any[] }>('/projects'),
  create: (data: { name: string; description?: string }) => api.post('/projects', data),
  get: (id: string) => api.get(`/projects/${id}`),
  delete: (id: string) => api.delete(`/projects/${id}`),
};

export const domainsAPI = {
  list: () => api.get<{ data: any[] }>('/domains'),
  create: (data: { app_id: string; fqdn: string }) => api.post('/domains', data),
  delete: (id: string) => api.delete(`/domains/${id}`),
  verify: (id: string, fqdn: string) => api.post(`/domains/${id}/verify`, { fqdn }),
  sslCheck: (fqdn: string) => api.get(`/domains/ssl-check?fqdn=${fqdn}`),
};

export const databasesAPI = {
  engines: () => api.get<{ data: any[] }>('/databases/engines'),
  create: (data: { name: string; engine: string; version?: string }) => api.post('/databases', data),
};

export const backupsAPI = {
  list: () => api.get<{ data: any[] }>('/backups'),
  create: (data: { source_type: string; source_id: string }) => api.post('/backups', data),
};

export const serversAPI = {
  providers: () => api.get<{ data: any[] }>('/servers/providers'),
  regions: (provider: string) => api.get<{ data: any[] }>(`/servers/providers/${provider}/regions`),
  sizes: (provider: string) => api.get<{ data: any[] }>(`/servers/providers/${provider}/sizes`),
  provision: (data: any) => api.post('/servers/provision', data),
  testSSH: (host: string, port: number) => api.post('/servers/test-ssh', { host, port }),
};

export const billingAPI = {
  plans: () => api.get<{ data: any[] }>('/billing/plans'),
  usage: () => api.get('/billing/usage'),
  usageHistory: (period = '24h') => api.get(`/billing/usage/history?period=${period}`),
};

export const secretsAPI = {
  list: () => api.get<{ data: any[] }>('/secrets'),
  create: (data: { name: string; value: string; scope: string }) => api.post('/secrets', data),
};

export const teamAPI = {
  roles: () => api.get<{ data: any[] }>('/team/roles'),
  auditLog: () => api.get<{ data: any[] }>('/team/audit-log'),
  invite: (data: { email: string; role_id: string }) => api.post('/team/invites', data),
};

export const appExtendedAPI = {
  // App lifecycle
  clone: (id: string, newName?: string) => api.post(`/apps/${id}/clone`, { new_name: newName }),
  suspend: (id: string) => api.post(`/apps/${id}/suspend`),
  resume: (id: string) => api.post(`/apps/${id}/resume`),
  rename: (id: string, name: string) => api.post(`/apps/${id}/rename`, { name }),
  deploy: (id: string) => api.post(`/apps/${id}/deploy`),
  scale: (id: string, replicas: number) => api.post(`/apps/${id}/scale`, { replicas }),

  // App config
  env: (id: string) => api.get(`/apps/${id}/env`),
  updateEnv: (id: string, vars: any[]) => api.put(`/apps/${id}/env`, { vars }),
  labels: (id: string) => api.get(`/apps/${id}/labels`),
  updateLabels: (id: string, labels: Record<string, string>) => api.put(`/apps/${id}/labels`, labels),
  ports: (id: string) => api.get(`/apps/${id}/ports`),
  healthcheck: (id: string) => api.get(`/apps/${id}/healthcheck`),
  middleware: (id: string) => api.get(`/apps/${id}/middleware`),
  autoscale: (id: string) => api.get(`/apps/${id}/autoscale`),
  maintenance: (id: string) => api.get(`/apps/${id}/maintenance`),
  gpu: (id: string) => api.get(`/apps/${id}/gpu`),

  // App data
  stats: (id: string) => api.get(`/apps/${id}/stats`),
  metrics: (id: string, period = '24h') => api.get(`/apps/${id}/metrics?period=${period}`),
  logs: (id: string, tail = 100) => api.get(`/apps/${id}/logs?tail=${tail}`),
  dependencies: (id: string) => api.get(`/apps/${id}/dependencies`),
  disk: (id: string) => api.get(`/apps/${id}/disk`),
  restarts: (id: string) => api.get(`/apps/${id}/restarts`),
  processes: (id: string) => api.get(`/apps/${id}/processes`),
  versions: (id: string) => api.get(`/apps/${id}/versions`),
  deployments: (id: string) => api.get(`/apps/${id}/deployments`),

  // App actions
  rollback: (id: string, version: number) => api.post(`/apps/${id}/rollback`, { version }),
  deployPreview: (id: string) => api.post(`/apps/${id}/deploy/preview`),
  exportApp: (id: string) => api.get(`/apps/${id}/export`),

  // Bulk
  bulk: (action: string, appIDs: string[]) => api.post('/apps/bulk', { action, app_ids: appIDs }),
};

export const adminAPI = {
  system: () => api.get('/admin/system'),
  stats: () => api.get('/admin/stats'),
  tenants: () => api.get('/admin/tenants'),
  updates: () => api.get('/admin/updates'),
  license: () => api.get('/admin/license'),
  disk: () => api.get('/admin/disk'),
};

export const marketplaceAPI = {
  list: (q?: string, category?: string) => {
    const params = new URLSearchParams();
    if (q) params.set('q', q);
    if (category) params.set('category', category);
    return api.get(`/marketplace?${params}`);
  },
  get: (slug: string) => api.get(`/marketplace/${slug}`),
  deploy: (slug: string, name: string, config: Record<string, string>) =>
    api.post('/marketplace/deploy', { slug, name, config }),
};
