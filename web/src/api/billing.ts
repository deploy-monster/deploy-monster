export interface Plan {
  id: string;
  name: string;
  description: string;
  price_cents: number;
  currency: string;
  max_apps: number;
  max_containers: number;
  max_ram_mb: number;
  features: string[];
}

export interface UsageData {
  apps_used: number;
  apps_limit: number;
  containers_used: number;
  containers_limit: number;
  ram_used_mb: number;
  ram_limit_mb: number;
  plan: { id: string; name: string };
  quota: { apps_ok: boolean; containers_ok: boolean; ram_ok: boolean };
}
