import { api } from './client';

export interface TeamMember {
  id: string;
  name: string;
  email: string;
  role: string;
  avatar_url?: string;
  joined_at: string;
}

export interface AuditEntry {
  id: number;
  action: string;
  user_name: string;
  resource_type: string;
  resource_id: string;
  ip_address: string;
  created_at: string;
}

export interface InviteRequest {
  email: string;
  role_id: string;
}

export const teamAPI = {
  members: () => api.get<TeamMember[]>('/team/members'),
  auditLog: () => api.get<AuditEntry[]>('/team/audit-log'),
  invite: (data: InviteRequest) => api.post('/team/invites', data),
  removeMember: (id: string) => api.delete(`/team/members/${id}`),
};
