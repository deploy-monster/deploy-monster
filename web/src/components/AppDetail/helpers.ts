import { cn } from '@/lib/utils';

export { cn };

export interface AppStatsResponse {
  app_id: string;
  count: number;
  running: number;
  cpu_percent: number;
  memory_usage: number;
  memory_limit: number;
  memory_percent: number;
  network_rx: number;
  network_tx: number;
  containers: { id: string; name: string; state: string; running: boolean }[];
}

const STATUS_CONFIG: Record<string, {
  variant: 'default' | 'secondary' | 'destructive' | 'outline';
  dot: string;
  label: string;
}> = {
  running: { variant: 'default', dot: 'bg-emerald-500', label: 'Running' },
  stopped: { variant: 'secondary', dot: 'bg-red-500', label: 'Stopped' },
  deploying: { variant: 'outline', dot: 'bg-amber-500', label: 'Deploying' },
  building: { variant: 'outline', dot: 'bg-amber-500', label: 'Building' },
  failed: { variant: 'destructive', dot: 'bg-red-500', label: 'Failed' },
  success: { variant: 'default', dot: 'bg-emerald-500', label: 'Success' },
  pending: { variant: 'secondary', dot: 'bg-slate-400', label: 'Pending' },
};

export function getStatusConfig(status: string) {
  return STATUS_CONFIG[status] || { variant: 'secondary' as const, dot: 'bg-slate-400', label: status };
}

export function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`;
}
