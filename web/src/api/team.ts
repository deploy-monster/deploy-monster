import { api, type PaginatedResponse } from './client';

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

interface InviteRequest {
  email: string;
  role_id: string;
}

export interface InviteResponse {
  id: string;
  email: string;
  role_id: string;
  /** One-time plaintext invite code. Shown to the inviter only at creation
   *  time — never retrievable again. */
  token: string;
  token_hash: string;
  expires_at: string;
}

export const teamAPI = {
  members: () => api.get<PaginatedResponse<TeamMember>>('/team/members'),
  auditLog: () => api.get<PaginatedResponse<AuditEntry>>('/team/audit-log'),
  invite: (data: InviteRequest) => api.post<InviteResponse>('/team/invites', data),
  removeMember: (id: string) => api.delete(`/team/members/${id}`),
};
