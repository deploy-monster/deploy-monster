import { Cpu, MemoryStick, Network, Timer } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import type { App } from '@/api/apps';

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

export const STATUS_CONFIG: Record<string, {
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

function buildMetricCards(
  stats: AppStatsResponse | null,
  app: App,
  statsError: string | null,
) {
  const haveStats = !!stats && (stats.count ?? 0) > 0;
  const cpuPercent = haveStats ? Math.min(100, stats!.cpu_percent) : 0;
  const memPercent = haveStats ? Math.min(100, stats!.memory_percent) : 0;
  const memValue = haveStats
    ? stats!.memory_limit > 0
      ? `${formatBytes(stats!.memory_usage)} / ${formatBytes(stats!.memory_limit)}`
      : formatBytes(stats!.memory_usage)
    : statsError
      ? '—'
      : 'No container';
  const netValue = haveStats
    ? `${formatBytes(stats!.network_rx)} ↓ / ${formatBytes(stats!.network_tx)} ↑`
    : statsError
      ? '—'
      : 'No container';
  const uptime = app.status === 'running' ? timeAgo(app.created_at).replace(' ago', '') : '—';
  return [
    {
      icon: Cpu,
      label: 'CPU Usage',
      value: haveStats ? `${cpuPercent.toFixed(1)}%` : statsError ? '—' : 'No container',
      color: 'text-blue-500',
      barColor: 'bg-blue-500',
      percent: cpuPercent,
    },
    {
      icon: MemoryStick,
      label: 'Memory',
      value: memValue,
      color: 'text-emerald-500',
      barColor: 'bg-emerald-500',
      percent: memPercent,
    },
    {
      icon: Network,
      label: 'Network I/O',
      value: netValue,
      color: 'text-violet-500',
      barColor: 'bg-violet-500',
      percent: 0,
    },
    {
      icon: Timer,
      label: 'Uptime',
      value: uptime,
      color: 'text-amber-500',
      barColor: 'bg-amber-500',
      percent: app.status === 'running' ? 100 : 0,
    },
  ];
}

interface AppStatsCardsProps {
  stats: AppStatsResponse | null;
  app: App;
  statsError: string | null;
}

export function AppStatsCards({ stats, app, statsError }: AppStatsCardsProps) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      {buildMetricCards(stats, app, statsError).map(({ icon: Icon, label, value, color, barColor, percent }) => (
        <Card key={label}>
          <CardContent className="pt-5">
            <div className="flex items-center gap-3 mb-3">
              <div className={cn('p-2 rounded-lg bg-muted')}>
                <Icon className={cn('size-4', color)} />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-xs text-muted-foreground">{label}</p>
                <p className="text-sm font-semibold truncate">{value}</p>
              </div>
            </div>
            <div className="w-full h-1.5 bg-muted rounded-full overflow-hidden">
              <div
                className={cn('h-full rounded-full transition-all duration-500', barColor)}
                style={{ width: `${percent}%` }}
              />
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}