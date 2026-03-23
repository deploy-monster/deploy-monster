import { api } from './client';

export interface App {
  id: string;
  project_id: string;
  tenant_id: string;
  name: string;
  type: string;
  source_type: string;
  source_url: string;
  branch: string;
  status: string;
  replicas: number;
  created_at: string;
  updated_at: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface CreateAppRequest {
  name: string;
  type?: string;
  source_type?: string;
  source_url?: string;
  branch?: string;
  project_id?: string;
}

export const appsAPI = {
  list: (page = 1, perPage = 20) =>
    api.get<PaginatedResponse<App>>(`/apps?page=${page}&per_page=${perPage}`),
  get: (id: string) => api.get<App>(`/apps/${id}`),
  create: (data: CreateAppRequest) => api.post<App>('/apps', data),
  delete: (id: string) => api.delete(`/apps/${id}`),
  restart: (id: string) => api.post(`/apps/${id}/restart`),
  stop: (id: string) => api.post(`/apps/${id}/stop`),
  start: (id: string) => api.post(`/apps/${id}/start`),
};
