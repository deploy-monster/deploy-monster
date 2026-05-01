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
