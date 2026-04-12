import { api } from './client';

export interface Template {
  slug: string;
  name: string;
  description: string;
  category: string;
  tags: string[];
  version: string;
  featured: boolean;
  verified: boolean;
  min_resources: { memory_mb: number };
}

export interface MarketplaceResponse {
  data: Template[];
  categories: string[];
}

export interface DeployTemplateRequest {
  slug: string;
  name: string;
  config?: Record<string, string>;
}

export const marketplaceAPI = {
  list: (params?: string) => api.get<MarketplaceResponse>(`/marketplace${params ? `?${params}` : ''}`),
  deploy: (data: DeployTemplateRequest) => api.post<{ app_id: string }>('/marketplace/deploy', data),
};
