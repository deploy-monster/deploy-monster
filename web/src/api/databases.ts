import { api } from './client';

export interface DatabaseInstance {
  id: string;
  name: string;
  engine: string;
  version: string;
  status: string;
  connection_string: string;
  size_mb: number;
  created_at: string;
}

export interface CreateDatabaseRequest {
  name: string;
  engine: string;
  version: string;
}

export const databasesAPI = {
  list: () => api.get<DatabaseInstance[]>('/databases'),
  create: (data: CreateDatabaseRequest) => api.post<DatabaseInstance>('/databases', data),
};
