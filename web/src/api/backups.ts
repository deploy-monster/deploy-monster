import { api } from './client';

export interface BackupEntry {
  key: string;
  size: number;
  type: string;
  status: string;
  created_at: number;
}

interface CreateBackupRequest {
  source_type: string;
  source_id: string;
}

export const backupsAPI = {
  list: () => api.get<BackupEntry[]>('/backups'),
  create: (data: CreateBackupRequest) => api.post('/backups', data),
  restore: (key: string) => api.post(`/backups/${encodeURIComponent(key)}/restore`, {}),
};
