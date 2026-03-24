// Standardized API response types matching the Go backend

export interface APIResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: APIError;
  meta?: APIMeta;
}

export interface APIError {
  code: string;
  message: string;
  details?: unknown;
}

export interface APIMeta {
  request_id?: string;
  page?: number;
  per_page?: number;
  total?: number;
  total_pages?: number;
}

// Common entity types
export interface Tenant {
  id: string;
  name: string;
  slug: string;
  plan_id: string;
  status: string;
  created_at: string;
}

export interface User {
  id: string;
  email: string;
  name: string;
  avatar_url: string;
  status: string;
  totp_enabled: boolean;
  created_at: string;
}

export interface Project {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  environment: string;
  created_at: string;
}

export interface Domain {
  id: string;
  app_id: string;
  fqdn: string;
  type: string;
  dns_provider: string;
  dns_synced: boolean;
  verified: boolean;
  created_at: string;
}

export interface Deployment {
  id: string;
  app_id: string;
  version: number;
  image: string;
  container_id: string;
  status: string;
  commit_sha: string;
  commit_message: string;
  triggered_by: string;
  strategy: string;
  created_at: string;
}

export interface Secret {
  name: string;
  scope: string;
  description: string;
  reference: string;
}

export interface ServerProvider {
  id: string;
  name: string;
}

export interface VPSInstance {
  id: string;
  name: string;
  ip_address: string;
  status: string;
  region: string;
  size: string;
}

export interface GitRepo {
  full_name: string;
  clone_url: string;
  description: string;
  default_branch: string;
  private: boolean;
}

export interface MarketplaceTemplate {
  slug: string;
  name: string;
  description: string;
  category: string;
  tags: string[];
  version: string;
  featured: boolean;
  verified: boolean;
}

export interface Plan {
  id: string;
  name: string;
  description: string;
  price_cents: number;
  currency: string;
  max_apps: number;
  max_containers: number;
  features: string[];
}
