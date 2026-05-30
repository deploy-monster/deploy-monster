import { api } from './client';

export interface Template {
  id?: string;
  slug: string;
  name: string;
  description: string;
  category: string;
  icon: string;
  author: string;
  vendor?: string;
  version: string;
  featured: boolean;
  verified: boolean;
  stars?: number;
  stats?: {
    deploys: number;
    rating: number;
  };
  tags: string[];
  created_at?: string;
  compose_yaml: string;
  config_schema: {
    type?: string;
    properties?: Record<string, {
      type: string;
      title: string;
      description?: string;
      format?: string;
      minLength?: number;
      default?: string;
    }>;
    required?: string[];
  };
  min_resources: {
    cpu_cores?: number;
    cpu_mb: number;
    memory_mb: number;
    disk_mb: number;
  };
}

export interface MarketplaceResponse {
  data: Template[];
  categories: string[];
}

interface DeployTemplateRequest {
  slug: string;
  name: string;
  domain?: string;
  server_id?: string;
  config?: Record<string, string>;
}

interface DeployTemplateResponse {
  app_id: string;
  name?: string;
  template?: string;
  status?: string;
  services?: number;
  generated_secrets?: Record<string, string>;
}

export const marketplaceAPI = {
  list: (params?: string) => api.get<MarketplaceResponse>(`/marketplace${params ? `?${params}` : ''}`),
  deploy: (data: DeployTemplateRequest) => api.post<DeployTemplateResponse>('/marketplace/deploy', data),
};
