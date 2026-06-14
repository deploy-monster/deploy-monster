import { Cpu, MemoryStick, Network, Timer } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import type { App } from '@/api/apps';
import { cn, formatBytes, timeAgo, type AppStatsResponse } from './helpers';

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
