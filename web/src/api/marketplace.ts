import { api } from './client';

export interface Template {
  slug: string;
  name: string;
  description: string;
  category: string;
  icon: string;
  author: string;
  version: string;
  featured: boolean;
  verified: boolean;
  tags: string[];
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
  config?: Record<string, string>;
}

export interface DeployTemplateResponse {
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
