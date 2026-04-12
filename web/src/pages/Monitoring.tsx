import { useState } from 'react';
import {
  Activity, Cpu, MemoryStick, HardDrive, Wifi, AlertTriangle,
  RefreshCw, Server, Container, Clock, TrendingUp, Zap,
} from 'lucide-react';
import type { ServerMetrics, AlertRule } from '@/api/monitoring';
import { cn } from '@/lib/utils';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

function getProgressColor(percent: number): string {
  if (percent >= 90) return 'bg-red-500';
  if (percent >= 70) return 'bg-amber-500';
  return 'bg-emerald-500';
}

function getStatusColor(status: string): string {
  if (status === 'ok') return 'bg-emerald-500';
  if (status === 'firing') return 'bg-red-500';
  return 'bg-amber-500';
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function MetricCard({
  icon: Icon,
  label,
  value,
  subtext,
  color,
  percent,
}: {
  icon: typeof Cpu;
  label: string;
  value: string;
  subtext?: string;
  color: string;
  percent?: number;
}) {
  return (
    <Card className="transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg">
      <CardContent className="pt-6">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className={cn('flex items-center justify-center rounded-xl size-11', color)}>
              <Icon className="size-5 text-white" />
            </div>
            <div>
              <p className="text-2xl font-bold tabular-nums tracking-tight text-foreground">{value}</p>
              <p className="text-sm text-muted-foreground">{label}</p>
            </div>
          </div>
          {subtext && (
            <Badge variant="outline" className="text-xs font-normal">
              {subtext}
            </Badge>
          )}
        </div>
        {percent !== undefined && (
          <div className="mt-4 space-y-1.5">
            <div className="flex justify-between text-xs text-muted-foreground">
              <span>Usage</span>
              <span className="tabular-nums">{percent.toFixed(1)}%</span>
            </div>
            <div className="h-2 rounded-full bg-muted overflow-hidden">
              <div
                className={cn('h-full rounded-full transition-all duration-500', getProgressColor(percent))}
                style={{ width: `${Math.min(percent, 100)}%` }}
              />
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function AlertRow({ alert }: { alert: AlertRule }) {
  const statusColor = getStatusColor(alert.status);
  return (
    <div className="flex items-center justify-between py-3 hover:bg-muted/50 transition-colors rounded-lg px-3 -mx-3">
      <div className="flex items-center gap-3">
        <div className={cn('size-2.5 rounded-full', statusColor)} />
        <div>
          <p className="text-sm font-medium text-foreground">{alert.name}</p>
          <p className="text-xs text-muted-foreground">
            {alert.metric} &gt; {alert.threshold}%
          </p>
        </div>
      </div>
      <div className="flex items-center gap-3">
        <Badge
          variant={alert.status === 'firing' ? 'destructive' : 'outline'}
          className="text-xs"
        >
          {alert.status}
        </Badge>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Monitoring Page
// ---------------------------------------------------------------------------

export function Monitoring() {
  const { data: metrics, loading, refetch } = useApi<ServerMetrics>('/metrics/server', { refreshInterval: 10000 });
  const { data: alerts } = useApi<AlertRule[]>('/alerts');
  const [, setRefreshing] = useState(false);

  const handleRefresh = async () => {
    setRefreshing(true);
    await refetch();
    setRefreshing(false);
  };

  const cpuPercent = metrics?.cpu_percent ?? 0;
  const memPercent = metrics ? (metrics.memory_used / Math.max(metrics.memory_total, 1)) * 100 : 0;
  const diskPercent = metrics ? (metrics.disk_used / Math.max(metrics.disk_total, 1)) * 100 : 0;
  const alertList = alerts || [];
  const firingAlerts = alertList.filter(a => a.status === 'firing');

  return (
    <div className="space-y-8">
      {/* Hero */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Activity className="size-5 text-primary" />
              {firingAlerts.length > 0 && (
                <Badge variant="destructive" className="text-xs">
                  {firingAlerts.length} alert{firingAlerts.length !== 1 ? 's' : ''} firing
                </Badge>
              )}
              {firingAlerts.length === 0 && (
                <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 text-xs">
                  All systems healthy
                </Badge>
              )}
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Monitoring
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm">
              Real-time server metrics, container stats, and alert management.
            </p>
          </div>
          <Button variant="outline" onClick={handleRefresh} className="shrink-0">
            <RefreshCw className="size-4" />
            Refresh
          </Button>
        </div>
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
      </div>

      {/* Metric Cards */}
      {loading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Card key={i}><CardContent className="pt-6"><Skeleton className="h-20 w-full" /></CardContent></Card>
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <MetricCard
            icon={Cpu}
            label="CPU Usage"
            value={`${cpuPercent.toFixed(1)}%`}
            color="bg-blue-500"
            percent={cpuPercent}
            subtext={metrics?.load_avg ? `Load: ${metrics.load_avg[0]?.toFixed(2)}` : undefined}
          />
          <MetricCard
            icon={MemoryStick}
            label="Memory"
            value={formatBytes(metrics?.memory_used ?? 0)}
            color="bg-purple-500"
            percent={memPercent}
            subtext={`of ${formatBytes(metrics?.memory_total ?? 0)}`}
          />
          <MetricCard
            icon={HardDrive}
            label="Disk"
            value={formatBytes(metrics?.disk_used ?? 0)}
            color="bg-amber-500"
            percent={diskPercent}
            subtext={`of ${formatBytes(metrics?.disk_total ?? 0)}`}
          />
          <MetricCard
            icon={Wifi}
            label="Network"
            value={`${formatBytes(metrics?.network_rx ?? 0)}/s`}
            color="bg-cyan-500"
            subtext={`TX: ${formatBytes(metrics?.network_tx ?? 0)}/s`}
          />
        </div>
      )}

      {/* Secondary Stats */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <Card className="transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
          <CardContent className="pt-5 pb-5 flex items-center gap-3">
            <div className="flex items-center justify-center rounded-lg size-9 bg-emerald-500/10">
              <Container className="size-4 text-emerald-500" />
            </div>
            <div>
              <p className="text-lg font-bold tabular-nums text-foreground">
                {metrics?.containers_running ?? 0}
                <span className="text-sm font-normal text-muted-foreground">/{metrics?.containers_total ?? 0}</span>
              </p>
              <p className="text-xs text-muted-foreground">Containers</p>
            </div>
          </CardContent>
        </Card>
        <Card className="transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
          <CardContent className="pt-5 pb-5 flex items-center gap-3">
            <div className="flex items-center justify-center rounded-lg size-9 bg-blue-500/10">
              <Clock className="size-4 text-blue-500" />
            </div>
            <div>
              <p className="text-lg font-bold tabular-nums text-foreground">
                {formatUptime(metrics?.uptime ?? 0)}
              </p>
              <p className="text-xs text-muted-foreground">Uptime</p>
            </div>
          </CardContent>
        </Card>
        <Card className="transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
          <CardContent className="pt-5 pb-5 flex items-center gap-3">
            <div className="flex items-center justify-center rounded-lg size-9 bg-amber-500/10">
              <Server className="size-4 text-amber-500" />
            </div>
            <div>
              <p className="text-lg font-bold tabular-nums text-foreground">1</p>
              <p className="text-xs text-muted-foreground">Servers</p>
            </div>
          </CardContent>
        </Card>
        <Card className="transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
          <CardContent className="pt-5 pb-5 flex items-center gap-3">
            <div className="flex items-center justify-center rounded-lg size-9 bg-red-500/10">
              <AlertTriangle className="size-4 text-red-500" />
            </div>
            <div>
              <p className="text-lg font-bold tabular-nums text-foreground">{firingAlerts.length}</p>
              <p className="text-xs text-muted-foreground">Active Alerts</p>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="alerts">
        <TabsList>
          <TabsTrigger value="alerts">
            <AlertTriangle className="size-3.5" />
            Alerts
            {firingAlerts.length > 0 && (
              <Badge variant="destructive" className="ml-1.5 text-[10px] px-1.5 py-0">
                {firingAlerts.length}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="metrics">
            <TrendingUp className="size-3.5" />
            Metrics
          </TabsTrigger>
          <TabsTrigger value="prometheus">
            <Zap className="size-3.5" />
            Prometheus
          </TabsTrigger>
        </TabsList>

        <TabsContent value="alerts">
          {alertList.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-24 text-center">
              <div className="rounded-full bg-muted p-6 mb-5">
                <AlertTriangle className="size-10 text-muted-foreground" />
              </div>
              <h2 className="text-xl font-semibold tracking-tight mb-2">No alert rules configured</h2>
              <p className="text-muted-foreground max-w-sm text-sm">
                Alert rules will trigger notifications when metrics exceed thresholds.
                Default rules for CPU, memory, and disk are created automatically.
              </p>
            </div>
          ) : (
            <Card>
              <CardHeader>
                <CardTitle className="text-base flex items-center gap-2">
                  <AlertTriangle className="size-4 text-primary" />
                  Alert Rules
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-1">
                  {alertList.map((alert, i) => (
                    <div key={alert.id}>
                      <AlertRow alert={alert} />
                      {i < alertList.length - 1 && <Separator className="my-1" />}
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="metrics">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Metric History</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex flex-col items-center justify-center py-16 text-center">
                <TrendingUp className="size-10 text-muted-foreground mb-4" />
                <p className="text-muted-foreground text-sm">
                  Metric charts will be available here. Data is collected every 30 seconds.
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="prometheus">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Prometheus Endpoint</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Prometheus metrics are available at the <code className="font-mono bg-muted px-1.5 py-0.5 rounded text-xs">/metrics</code> endpoint.
              </p>
              <div className="rounded-lg bg-[#0d1117] p-4 font-mono text-sm text-emerald-400">
                <p>$ curl http://localhost:8443/metrics</p>
                <p className="text-muted-foreground mt-2"># HELP deploymonster_apps_total Total applications</p>
                <p className="text-muted-foreground"># TYPE deploymonster_apps_total gauge</p>
                <p>deploymonster_apps_total 0</p>
                <p className="text-muted-foreground mt-1"># HELP deploymonster_containers_running Running containers</p>
                <p className="text-muted-foreground"># TYPE deploymonster_containers_running gauge</p>
                <p>deploymonster_containers_running 0</p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
