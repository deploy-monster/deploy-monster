export interface ServerNode {
  id: string;
  hostname: string;
  ip_address: string;
  provider: string;
  region: string;
  size: string;
  status: string;
  agent_status?: string;
  connected?: boolean;
  role: string;
  created_at: string;
}
