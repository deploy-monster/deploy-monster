import { useEffect, useState, useCallback, useRef } from 'react';
import { useParams, Link } from 'react-router';
import {
  ArrowLeft,
  Play,
  Square,
  RotateCcw,
  Trash2,
  GitBranch,
  Clock,
  Cpu,
  MemoryStick,
  Upload,
  Plus,
  Pencil,
  Eye,
  EyeOff,
  Copy,
  History,
  Settings,
  Terminal,
  Layers,
  Rocket,
  Network,
  Timer,
  Server,
  Calendar,
  User,
  CheckCircle2,
} from 'lucide-react';
import { appsAPI, type App } from '../api/apps';
import type { Deployment, EnvVar } from '@/api/deployments';
import { useApi } from '../hooks';
import { toast } from '@/stores/toastStore';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Separator } from '@/components/ui/separator';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Tooltip } from '@/components/ui/tooltip';

/* ------------------------------------------------------------------ */
/*  Constants                                                         */
/* ------------------------------------------------------------------ */

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

/* ------------------------------------------------------------------ */
/*  Helpers                                                           */
/* ------------------------------------------------------------------ */

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function getStatusConfig(status: string) {
  return STATUS_CONFIG[status] || { variant: 'secondary' as const, dot: 'bg-slate-400', label: status };
}

/* ------------------------------------------------------------------ */
/*  Main Component                                                    */
/* ------------------------------------------------------------------ */

export function AppDetail() {
  const { id } = useParams();
  const { data: app, loading: appLoading, refetch: refetchApp } = useApi<App>(`/apps/${id}`);
  const { data: deploymentsData } = useApi<Deployment[]>(`/apps/${id}/deployments`);
  const deployments = deploymentsData || [];

  const [actionLoading, setActionLoading] = useState<string | null>(null);

  // Environment variables local state (demo -- would be fetched from API)
  const [envVars, setEnvVars] = useState<EnvVar[]>([
    { key: 'NODE_ENV', value: 'production', isSecret: false },
    { key: 'DATABASE_URL', value: '${SECRET:db_url}', isSecret: true },
    { key: 'API_KEY', value: '${SECRET:api_key}', isSecret: true },
    { key: 'PORT', value: '3000', isSecret: false },
  ]);
  const [newEnvKey, setNewEnvKey] = useState('');
  const [newEnvValue, setNewEnvValue] = useState('');
  const [showSecrets, setShowSecrets] = useState(false);
  const [, setEditingEnv] = useState<string | null>(null);

  /* -- Actions ---------------------------------------------------- */

  const handleAction = useCallback(
    async (action: 'start' | 'stop' | 'restart') => {
      if (!id) return;
      setActionLoading(action);
      try {
        await appsAPI[action](id);
        refetchApp();
      } catch {
        toast.error(`Failed to ${action} application`);
      } finally {
        setActionLoading(null);
      }
    },
    [id, refetchApp]
  );

  const handleDelete = useCallback(async () => {
    if (!id) return;
    if (
      !confirm(
        'Are you sure you want to delete this application? All deployments, domains, and data will be permanently removed.'
      )
    )
      return;
    setActionLoading('delete');
    try {
      await appsAPI.delete(id);
      window.location.href = '/apps';
    } catch {
      toast.error('Failed to delete application');
      setActionLoading(null);
    }
  }, [id]);

  const handleDeploy = useCallback(async () => {
    if (!id) return;
    setActionLoading('deploy');
    try {
      await appsAPI.restart(id);
      refetchApp();
    } catch {
      toast.error('Failed to deploy application');
    } finally {
      setActionLoading(null);
    }
  }, [id, refetchApp]);

  const addEnvVar = () => {
    if (!newEnvKey.trim()) return;
    setEnvVars((prev) => [...prev, { key: newEnvKey, value: newEnvValue, isSecret: false }]);
    setNewEnvKey('');
    setNewEnvValue('');
  };

  const removeEnvVar = (key: string) => {
    setEnvVars((prev) => prev.filter((v) => v.key !== key));
  };

  /* -- Loading state ---------------------------------------------- */

  if (appLoading || !app) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="flex flex-col items-center gap-3">
          <div className="size-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
          <p className="text-sm text-muted-foreground">Loading application...</p>
        </div>
      </div>
    );
  }

  const statusCfg = getStatusConfig(app.status);

  return (
    <div className="space-y-6">
      {/* ============================================================ */}
      {/*  Header                                                      */}
      {/* ============================================================ */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-4">
          <Link to="/apps">
            <Button variant="ghost" size="icon" className="mt-1 cursor-pointer">
              <ArrowLeft className="size-4" />
            </Button>
          </Link>
          <div>
            <div className="flex items-center gap-3 flex-wrap">
              <h1 className="text-3xl font-bold tracking-tight">{app.name}</h1>
              <Badge variant={statusCfg.variant} className="text-xs gap-1.5">
                <span className="relative flex h-2 w-2">
                  {app.status === 'running' && (
                    <span
                      className={cn(
                        'absolute inline-flex h-full w-full rounded-full opacity-75 animate-ping',
                        statusCfg.dot
                      )}
                    />
                  )}
                  <span className={cn('relative inline-flex rounded-full h-2 w-2', statusCfg.dot)} />
                </span>
                {statusCfg.label}
              </Badge>
            </div>
            <p className="text-muted-foreground mt-1 text-sm">
              {app.source_type} &middot; {app.type}
              {app.branch && (
                <span className="inline-flex items-center gap-1 ml-2">
                  <GitBranch className="size-3" />
                  {app.branch}
                </span>
              )}
            </p>
          </div>
        </div>

        {/* Action buttons */}
        <div className="flex items-center gap-2 ml-12 sm:ml-0">
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={() => handleAction('restart')}
            disabled={actionLoading !== null}
          >
            <RotateCcw className={cn('size-4', actionLoading === 'restart' && 'animate-spin')} />
            Restart
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={() => handleAction(app.status === 'running' ? 'stop' : 'start')}
            disabled={actionLoading !== null}
          >
            {app.status === 'running' ? (
              <>
                <Square className="size-4" />
                Stop
              </>
            ) : (
              <>
                <Play className="size-4" />
                Start
              </>
            )}
          </Button>
          <Button
            size="sm"
            className="cursor-pointer"
            onClick={handleDeploy}
            disabled={actionLoading !== null}
          >
            <Upload className={cn('size-4', actionLoading === 'deploy' && 'animate-bounce')} />
            Deploy
          </Button>
          <Button
            variant="destructive"
            size="sm"
            className="cursor-pointer"
            onClick={handleDelete}
            disabled={actionLoading !== null}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      </div>

      {/* ============================================================ */}
      {/*  Tabs                                                        */}
      {/* ============================================================ */}
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview" className="cursor-pointer">
            <Layers className="size-4" />
            Overview
          </TabsTrigger>
          <TabsTrigger value="deployments" className="cursor-pointer">
            <Rocket className="size-4" />
            Deployments
          </TabsTrigger>
          <TabsTrigger value="env" className="cursor-pointer">
            <Settings className="size-4" />
            Environment
          </TabsTrigger>
          <TabsTrigger value="logs" className="cursor-pointer">
            <Terminal className="size-4" />
            Logs
          </TabsTrigger>
          <TabsTrigger value="settings" className="cursor-pointer">
            <Settings className="size-4" />
            Settings
          </TabsTrigger>
        </TabsList>

        {/* ========================================================== */}
        {/*  Overview Tab                                               */}
        {/* ========================================================== */}
        <TabsContent value="overview" className="space-y-6">
          {/* Metric cards */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {[
              {
                icon: Cpu,
                label: 'CPU Usage',
                value: '12%',
                color: 'text-blue-500',
                barColor: 'bg-blue-500',
                percent: 12,
              },
              {
                icon: MemoryStick,
                label: 'Memory',
                value: '256 MB / 512 MB',
                color: 'text-emerald-500',
                barColor: 'bg-emerald-500',
                percent: 50,
              },
              {
                icon: Network,
                label: 'Network I/O',
                value: '1.2 KB/s',
                color: 'text-violet-500',
                barColor: 'bg-violet-500',
                percent: 8,
              },
              {
                icon: Timer,
                label: 'Uptime',
                value: app.status === 'running' ? timeAgo(app.created_at).replace(' ago', '') : '--',
                color: 'text-amber-500',
                barColor: 'bg-amber-500',
                percent: app.status === 'running' ? 100 : 0,
              },
            ].map(({ icon: Icon, label, value, color, barColor, percent }) => (
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

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Application Info */}
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Application Info</CardTitle>
                <CardDescription>Source and configuration details</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {[
                    { icon: GitBranch, label: 'Source', value: app.source_type },
                    { icon: GitBranch, label: 'Branch', value: app.branch || 'main' },
                    { icon: Server, label: 'Replicas', value: String(app.replicas) },
                    {
                      icon: Calendar,
                      label: 'Created',
                      value: new Date(app.created_at).toLocaleDateString('en-US', {
                        year: 'numeric',
                        month: 'short',
                        day: 'numeric',
                      }),
                    },
                  ].map(({ icon: Icon, label, value }) => (
                    <div key={label} className="flex items-center justify-between text-sm">
                      <span className="flex items-center gap-2 text-muted-foreground">
                        <Icon className="size-4" />
                        {label}
                      </span>
                      <span className="font-medium">{value}</span>
                    </div>
                  ))}
                  {app.source_url && (
                    <>
                      <Separator />
                      <div className="text-sm">
                        <p className="text-muted-foreground mb-1.5">Repository URL</p>
                        <p className="font-mono text-xs bg-muted rounded-md px-3 py-2 truncate">
                          {app.source_url}
                        </p>
                      </div>
                    </>
                  )}
                </div>
              </CardContent>
            </Card>

            {/* Latest Deployment */}
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Latest Deployment</CardTitle>
                <CardDescription>Most recent deployment information</CardDescription>
              </CardHeader>
              <CardContent>
                {deployments.length > 0 ? (
                  <div className="space-y-4">
                    {[
                      { icon: Rocket, label: 'Version', value: `v${deployments[0].version}` },
                      {
                        icon: CheckCircle2,
                        label: 'Status',
                        value: deployments[0].status,
                        isBadge: true,
                      },
                      { icon: User, label: 'Triggered by', value: deployments[0].triggered_by },
                      { icon: Clock, label: 'Date', value: timeAgo(deployments[0].created_at) },
                    ].map(({ icon: Icon, label, value, isBadge }) => (
                      <div key={label} className="flex items-center justify-between text-sm">
                        <span className="flex items-center gap-2 text-muted-foreground">
                          <Icon className="size-4" />
                          {label}
                        </span>
                        {isBadge ? (
                          <Badge variant={getStatusConfig(value).variant}>
                            {getStatusConfig(value).label}
                          </Badge>
                        ) : (
                          <span className="font-medium">{value}</span>
                        )}
                      </div>
                    ))}
                    {deployments[0].commit_sha && (
                      <>
                        <Separator />
                        <div className="text-sm">
                          <p className="text-muted-foreground mb-1.5">Commit SHA</p>
                          <code className="font-mono text-xs bg-muted rounded-md px-3 py-2 block">
                            {deployments[0].commit_sha.slice(0, 8)}
                          </code>
                        </div>
                      </>
                    )}
                  </div>
                ) : (
                  <div className="flex flex-col items-center justify-center py-8 text-center">
                    <div className="rounded-full bg-muted p-3 mb-3">
                      <Rocket className="size-5 text-muted-foreground" />
                    </div>
                    <p className="text-sm text-muted-foreground mb-3">No deployments yet</p>
                    <Button size="sm" onClick={handleDeploy} className="cursor-pointer">
                      <Upload className="size-4" />
                      Deploy Now
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* ========================================================== */}
        {/*  Deployments Tab                                            */}
        {/* ========================================================== */}
        <TabsContent value="deployments" className="space-y-4">
          <Card>
            <CardHeader className="flex-row items-center justify-between space-y-0">
              <div>
                <CardTitle className="text-base">Deployment History</CardTitle>
                <CardDescription>
                  {deployments.length} deployment{deployments.length !== 1 ? 's' : ''}
                </CardDescription>
              </div>
              <Button size="sm" onClick={handleDeploy} className="cursor-pointer">
                <Upload className="size-4" />
                New Deployment
              </Button>
            </CardHeader>
            <CardContent>
              {deployments.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-12 text-center">
                  <div className="rounded-full bg-muted p-4 mb-4">
                    <History className="size-8 text-muted-foreground" />
                  </div>
                  <h3 className="font-medium mb-1">No deployments yet</h3>
                  <p className="text-sm text-muted-foreground mb-4">
                    Trigger your first deployment to see the history here.
                  </p>
                  <Button size="sm" onClick={handleDeploy} className="cursor-pointer">
                    <Rocket className="size-4" />
                    Deploy Now
                  </Button>
                </div>
              ) : (
                <div className="rounded-lg border overflow-hidden">
                  <Table>
                    <TableHeader>
                      <TableRow className="bg-muted/50">
                        <TableHead className="font-semibold">Version</TableHead>
                        <TableHead className="font-semibold">Status</TableHead>
                        <TableHead className="font-semibold hidden md:table-cell">
                          Image
                        </TableHead>
                        <TableHead className="font-semibold">Commit</TableHead>
                        <TableHead className="font-semibold hidden sm:table-cell">
                          Triggered By
                        </TableHead>
                        <TableHead className="font-semibold">Date</TableHead>
                        <TableHead className="font-semibold text-right">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {deployments.map((d, index) => {
                        const dCfg = getStatusConfig(d.status);
                        return (
                          <TableRow
                            key={d.id}
                            className="hover:bg-muted/30 transition-colors"
                          >
                            <TableCell className="font-semibold">v{d.version}</TableCell>
                            <TableCell>
                              <Badge variant={dCfg.variant} className="text-xs gap-1.5">
                                <span className={cn('inline-flex rounded-full h-1.5 w-1.5', dCfg.dot)} />
                                {dCfg.label}
                              </Badge>
                            </TableCell>
                            <TableCell className="hidden md:table-cell">
                              <span className="font-mono text-xs text-muted-foreground max-w-48 truncate block">
                                {d.image}
                              </span>
                            </TableCell>
                            <TableCell>
                              <code className="font-mono text-xs bg-muted px-2 py-0.5 rounded">
                                {d.commit_sha?.slice(0, 8) || '--------'}
                              </code>
                            </TableCell>
                            <TableCell className="hidden sm:table-cell text-muted-foreground text-sm">
                              {d.triggered_by}
                            </TableCell>
                            <TableCell className="text-muted-foreground text-sm">
                              <span className="inline-flex items-center gap-1">
                                <Clock className="size-3" />
                                {timeAgo(d.created_at)}
                              </span>
                            </TableCell>
                            <TableCell className="text-right">
                              {index > 0 && (
                                <Button
                                  variant="outline"
                                  size="sm"
                                  className="h-7 text-xs cursor-pointer"
                                >
                                  <History className="size-3" />
                                  Rollback
                                </Button>
                              )}
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* ========================================================== */}
        {/*  Environment Tab                                            */}
        {/* ========================================================== */}
        <TabsContent value="env" className="space-y-4">
          <Card>
            <CardHeader className="flex-row items-center justify-between space-y-0">
              <div>
                <CardTitle className="text-base">Environment Variables</CardTitle>
                <CardDescription>
                  Manage your application's environment configuration. Use{' '}
                  <code className="text-xs bg-muted px-1.5 py-0.5 rounded font-mono">
                    {'${SECRET:name}'}
                  </code>{' '}
                  for encrypted references.
                </CardDescription>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="cursor-pointer"
                  onClick={() => setShowSecrets(!showSecrets)}
                >
                  {showSecrets ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  {showSecrets ? 'Hide Values' : 'Reveal Values'}
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Add new variable */}
              <div className="flex items-end gap-3 p-4 rounded-lg border border-dashed bg-muted/30">
                <div className="flex-1">
                  <Label htmlFor="env-key" className="mb-1.5 text-xs">
                    Key
                  </Label>
                  <Input
                    id="env-key"
                    placeholder="VARIABLE_NAME"
                    value={newEnvKey}
                    onChange={(e) => setNewEnvKey(e.target.value.toUpperCase())}
                    className="font-mono text-sm"
                  />
                </div>
                <div className="flex-1">
                  <Label htmlFor="env-value" className="mb-1.5 text-xs">
                    Value
                  </Label>
                  <Input
                    id="env-value"
                    placeholder="value or ${SECRET:name}"
                    value={newEnvValue}
                    onChange={(e) => setNewEnvValue(e.target.value)}
                    className="font-mono text-sm"
                  />
                </div>
                <Button
                  size="sm"
                  onClick={addEnvVar}
                  disabled={!newEnvKey.trim()}
                  className="cursor-pointer"
                >
                  <Plus className="size-4" />
                  Add Variable
                </Button>
              </div>

              <Separator />

              {/* Variable list */}
              {envVars.length === 0 ? (
                <div className="flex flex-col items-center py-8 text-center">
                  <div className="rounded-full bg-muted p-3 mb-3">
                    <Settings className="size-5 text-muted-foreground" />
                  </div>
                  <p className="text-sm text-muted-foreground">
                    No environment variables configured.
                  </p>
                </div>
              ) : (
                <div className="rounded-lg border overflow-hidden">
                    <Table>
                      <TableHeader>
                        <TableRow className="bg-muted/50">
                          <TableHead className="font-semibold">Key</TableHead>
                          <TableHead className="font-semibold">Value</TableHead>
                          <TableHead className="w-28 text-right font-semibold">Actions</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {envVars.map((v) => (
                          <TableRow key={v.key} className="hover:bg-muted/30 transition-colors">
                            <TableCell className="font-mono text-sm font-bold">
                              {v.key}
                            </TableCell>
                            <TableCell className="font-mono text-sm text-muted-foreground">
                              {v.isSecret && !showSecrets ? (
                                <span className="select-none tracking-wider">
                                  {'\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022'}
                                </span>
                              ) : (
                                v.value
                              )}
                            </TableCell>
                            <TableCell className="text-right">
                              <div className="flex items-center justify-end gap-1">
                                <Tooltip content="Copy value">
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      className="size-7 cursor-pointer"
                                      onClick={() => navigator.clipboard.writeText(v.value)}
                                    >
                                      <Copy className="size-3" />
                                    </Button>
                                </Tooltip>
                                <Tooltip content="Edit">
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      className="size-7 cursor-pointer"
                                      onClick={() => setEditingEnv(v.key)}
                                    >
                                      <Pencil className="size-3" />
                                    </Button>
                                </Tooltip>
                                <Tooltip content="Delete">
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      className="size-7 text-destructive hover:text-destructive cursor-pointer"
                                      onClick={() => removeEnvVar(v.key)}
                                    >
                                      <Trash2 className="size-3" />
                                    </Button>
                                </Tooltip>
                              </div>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* ========================================================== */}
        {/*  Logs Tab                                                   */}
        {/* ========================================================== */}
        <TabsContent value="logs" className="space-y-4">
          <LogsPanel appId={id || ''} />
        </TabsContent>

        {/* ========================================================== */}
        {/*  Settings Tab                                               */}
        {/* ========================================================== */}
        <TabsContent value="settings" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">General</CardTitle>
              <CardDescription>Basic application information</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <Label className="mb-1.5">Application Name</Label>
                  <Input value={app.name} readOnly className="bg-muted" />
                </div>
                <div>
                  <Label className="mb-1.5">Application ID</Label>
                  <div className="flex gap-2">
                    <Input value={app.id} readOnly className="bg-muted font-mono text-xs" />
                    <Button
                      variant="outline"
                      size="icon"
                      className="cursor-pointer"
                      onClick={() => navigator.clipboard.writeText(app.id)}
                    >
                      <Copy className="size-4" />
                    </Button>
                  </div>
                </div>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <Label className="mb-1.5">Created</Label>
                  <Input
                    value={new Date(app.created_at).toLocaleString()}
                    readOnly
                    className="bg-muted"
                  />
                </div>
                <div>
                  <Label className="mb-1.5">Last Updated</Label>
                  <Input
                    value={new Date(app.updated_at).toLocaleString()}
                    readOnly
                    className="bg-muted"
                  />
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className="border-destructive/50">
            <CardHeader>
              <CardTitle className="text-base text-destructive">Danger Zone</CardTitle>
              <CardDescription>
                These actions are irreversible. Please proceed with caution.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between rounded-lg border border-destructive/30 p-4">
                <div>
                  <p className="font-medium text-sm">Delete this application</p>
                  <p className="text-sm text-muted-foreground">
                    Permanently remove this application, all deployments, domains, and associated
                    data.
                  </p>
                </div>
                <Button
                  variant="destructive"
                  size="sm"
                  className="cursor-pointer shrink-0 ml-4"
                  onClick={handleDelete}
                  disabled={actionLoading === 'delete'}
                >
                  <Trash2 className="size-4" />
                  Delete Application
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Logs Panel (SSE lifecycle managed independently)                   */
/* ------------------------------------------------------------------ */

function LogsPanel({ appId }: { appId: string }) {
  const [logs, setLogs] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs]);

  useEffect(() => {
    if (!appId) return;

    const eventSource = new EventSource(`/api/v1/apps/${appId}/logs/stream`);

    eventSource.onopen = () => {
      setConnected(true);
    };

    eventSource.onmessage = (e) => {
      setLogs((prev) => [...prev.slice(-500), e.data]);
    };

    eventSource.onerror = () => {
      setConnected(false);
    };

    return () => {
      eventSource.close();
      setConnected(false);
    };
  }, [appId]);

  return (
    <Card className="overflow-hidden">
      <CardContent className="p-0">
        <div className="rounded-lg bg-[#0d1117] overflow-hidden">
          {/* Terminal header */}
          <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/5 bg-[#161b22]">
            <div className="flex items-center gap-3">
              <div className="flex gap-1.5">
                <div className="size-3 rounded-full bg-[#ff5f57] hover:bg-[#ff5f57]/80 transition-colors" />
                <div className="size-3 rounded-full bg-[#febc2e] hover:bg-[#febc2e]/80 transition-colors" />
                <div className="size-3 rounded-full bg-[#28c840] hover:bg-[#28c840]/80 transition-colors" />
              </div>
              <span className="text-xs text-[#8b949e] font-mono">Container Logs</span>
            </div>
            <div className="flex items-center gap-2">
              <span
                className={cn(
                  'size-2 rounded-full transition-colors',
                  connected ? 'bg-[#28c840] shadow-sm shadow-[#28c840]/50' : 'bg-[#8b949e]'
                )}
              />
              <span className="text-xs text-[#8b949e] font-mono">
                {connected ? 'Live' : 'Disconnected'}
              </span>
            </div>
          </div>

          {/* Log content */}
          <div
            ref={scrollRef}
            className="h-[28rem] overflow-auto p-4 font-mono text-sm leading-relaxed scroll-smooth"
          >
            {logs.length === 0 ? (
              <div className="flex items-center gap-2 text-[#8b949e]">
                <div className="size-1.5 rounded-full bg-[#8b949e] animate-pulse" />
                <span>Waiting for logs...</span>
              </div>
            ) : (
              logs.map((line, i) => (
                <div
                  key={i}
                  className="text-[#c9d1d9] hover:bg-[#161b22] px-2 -mx-2 py-px rounded group"
                >
                  <span className="text-[#484f58] select-none mr-4 inline-block w-10 text-right group-hover:text-[#6e7681]">
                    {String(i + 1).padStart(4, ' ')}
                  </span>
                  {line}
                </div>
              ))
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
