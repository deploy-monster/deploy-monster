import { api } from './client';

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

export interface CreateDomainRequest {
  fqdn: string;
  app_id: string;
}

export const domainsAPI = {
  list: () => api.get<Domain[]>('/domains'),
  create: (data: CreateDomainRequest) => api.post<Domain>('/domains', data),
  verify: (id: string) => api.post(`/domains/${id}/verify`),
  delete: (id: string) => api.delete(`/domains/${id}`),
};
