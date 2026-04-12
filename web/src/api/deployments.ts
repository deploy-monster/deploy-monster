export interface Deployment {
  id: string;
  version: number;
  image: string;
  status: string;
  commit_sha: string;
  triggered_by: string;
  created_at: string;
}

export interface EnvVar {
  key: string;
  value: string;
  isSecret: boolean;
}
