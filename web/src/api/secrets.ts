import { api } from './client';

export interface SecretEntry {
  id: string;
  name: string;
  scope: string;
  created_at: string;
  updated_at: string;
}

interface CreateSecretRequest {
  name: string;
  value: string;
  scope: string;
}

export const secretsAPI = {
  list: () => api.get<SecretEntry[]>('/secrets'),
  create: (data: CreateSecretRequest) => api.post<SecretEntry>('/secrets', data),
  delete: (id: string) => api.delete(`/secrets/${id}`),
};
