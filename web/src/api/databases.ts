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
