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
