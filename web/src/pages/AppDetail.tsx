import { useEffect, useState, useCallback } from 'react';
import { useParams, Link } from 'react-router';
import {
  ArrowLeft,
  Play,
  Square,
  RotateCcw,
  Trash2,
  GitBranch,
  Clock,
  Container,
  Cpu,
  MemoryStick,
  Activity,
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
} from 'lucide-react';
import { appsAPI, type App } from '../api/apps';
import { useApi } from '../hooks';
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

interface Deployment {
  id: string;
  version: number;
  image: string;
  status: string;
  commit_sha: string;
  triggered_by: string;
  created_at: string;
}

interface EnvVar {
  key: string;
  value: string;
  isSecret: boolean;
}

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  running: 'default',
  stopped: 'secondary',
  deploying: 'outline',
  building: 'outline',
  failed: 'destructive',
  success: 'default',
  pending: 'secondary',
};

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

export function AppDetail() {
  const { id } = useParams();
  const { data: app, loading: appLoading, refetch: refetchApp } = useApi<App>(`/apps/${id}`);
  const { data: deploymentsData } = useApi<Deployment[]>(`/apps/${id}/deployments`);
  const deployments = deploymentsData || [];

  const [actionLoading, setActionLoading] = useState<string | null>(null);

  // Environment variables local state (demo — would be fetched from API)
  const [envVars, setEnvVars] = useState<EnvVar[]>([
    { key: 'NODE_ENV', value: 'production', isSecret: false },
    { key: 'DATABASE_URL', value: '${SECRET:db_url}', isSecret: true },
  ]);
  const [newEnvKey, setNewEnvKey] = useState('');
  const [newEnvValue, setNewEnvValue] = useState('');
  const [showSecrets, setShowSecrets] = useState(false);
  const [_editingEnv, setEditingEnv] = useState<string | null>(null);

  const handleAction = useCallback(async (action: 'start' | 'stop' | 'restart') => {
    if (!id) return;
    setActionLoading(action);
    try {
      await appsAPI[action](id);
      refetchApp();
    } catch {
      // Error handled by API layer
    } finally {
      setActionLoading(null);
    }
  }, [id, refetchApp]);

  const handleDelete = useCallback(async () => {
    if (!id) return;
    if (!confirm('Are you sure you want to delete this application? All deployments, domains, and data will be permanently removed.')) return;
    setActionLoading('delete');
    try {
      await appsAPI.delete(id);
      window.location.href = '/apps';
    } catch {
      setActionLoading(null);
    }
  }, [id]);

  const handleDeploy = useCallback(async () => {
    if (!id) return;
    setActionLoading('deploy');
    try {
      await appsAPI.restart(id); // Trigger redeploy
      refetchApp();
    } catch {
      // Error handled by API layer
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

  // SSE log streaming
  useEffect(() => {
    if (!id) return;
    // Only connect when on logs tab — managed by tab visibility
  }, [id]);

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

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-4">
          <Link to="/apps">
            <Button variant="ghost" size="icon" className="mt-1">
              <ArrowLeft className="size-4" />
            </Button>
          </Link>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-3xl font-bold tracking-tight">{app.name}</h1>
              <Badge variant={STATUS_VARIANT[app.status] || 'secondary'} className="text-xs">
                {app.status}
              </Badge>
            </div>
            <p className="text-muted-foreground mt-1">
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
        <div className="flex items-center gap-2 ml-12 sm:ml-0">
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleAction('restart')}
            disabled={actionLoading !== null}
          >
            <RotateCcw className={cn('size-4', actionLoading === 'restart' && 'animate-spin')} />
            Restart
          </Button>
          <Button
            variant="outline"
            size="sm"
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
            onClick={handleDeploy}
            disabled={actionLoading !== null}
          >
            <Upload className={cn('size-4', actionLoading === 'deploy' && 'animate-bounce')} />
            Deploy
          </Button>
        </div>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">
            <Layers className="size-4" />
            Overview
          </TabsTrigger>
          <TabsTrigger value="deployments">
            <Rocket className="size-4" />
            Deployments
          </TabsTrigger>
          <TabsTrigger value="env">
            <Settings className="size-4" />
            Environment
          </TabsTrigger>
          <TabsTrigger value="logs">
            <Terminal className="size-4" />
            Logs
          </TabsTrigger>
          <TabsTrigger value="settings">
            <Settings className="size-4" />
            Settings
          </TabsTrigger>
        </TabsList>

        {/* Overview Tab */}
        <TabsContent value="overview" className="space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Application Info */}
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Application Info</CardTitle>
                <CardDescription>Source and configuration details</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div>
                    <p className="text-muted-foreground">Type</p>
                    <p className="font-medium">{app.type}</p>
                  </div>
                  <div>
                    <p className="text-muted-foreground">Source</p>
                    <p className="font-medium">{app.source_type}</p>
                  </div>
                  <div>
                    <p className="text-muted-foreground">Branch</p>
                    <p className="font-medium inline-flex items-center gap-1">
                      <GitBranch className="size-3" />
                      {app.branch || 'main'}
                    </p>
                  </div>
                  <div>
                    <p className="text-muted-foreground">Replicas</p>
                    <p className="font-medium">{app.replicas}</p>
                  </div>
                </div>
                {app.source_url && (
                  <>
                    <Separator />
                    <div className="text-sm">
                      <p className="text-muted-foreground mb-1">Repository URL</p>
                      <p className="font-mono text-xs bg-muted rounded-md px-3 py-2 truncate">
                        {app.source_url}
                      </p>
                    </div>
                  </>
                )}
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
                    <div className="grid grid-cols-2 gap-4 text-sm">
                      <div>
                        <p className="text-muted-foreground">Version</p>
                        <p className="font-medium">v{deployments[0].version}</p>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Status</p>
                        <Badge variant={STATUS_VARIANT[deployments[0].status] || 'secondary'}>
                          {deployments[0].status}
                        </Badge>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Triggered by</p>
                        <p className="font-medium">{deployments[0].triggered_by}</p>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Date</p>
                        <p className="font-medium">{timeAgo(deployments[0].created_at)}</p>
                      </div>
                    </div>
                    {deployments[0].commit_sha && (
                      <>
                        <Separator />
                        <div className="text-sm">
                          <p className="text-muted-foreground mb-1">Commit</p>
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
                    <p className="text-sm text-muted-foreground">No deployments yet</p>
                    <Button size="sm" className="mt-3" onClick={handleDeploy}>
                      <Upload className="size-4" />
                      Deploy Now
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

          {/* Resource Usage */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Resource Usage</CardTitle>
              <CardDescription>Current resource consumption</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
                {[
                  { icon: Cpu, label: 'CPU', value: '0%', color: 'text-blue-500' },
                  { icon: MemoryStick, label: 'Memory', value: '0 MB', color: 'text-green-500' },
                  { icon: Activity, label: 'Requests/min', value: '0', color: 'text-violet-500' },
                  { icon: Container, label: 'Containers', value: `${app.replicas}`, color: 'text-amber-500' },
                ].map(({ icon: Icon, label, value, color }) => (
                  <div key={label} className="flex items-center gap-3 rounded-lg border p-4">
                    <Icon className={cn('size-5', color)} />
                    <div>
                      <p className="text-lg font-bold">{value}</p>
                      <p className="text-xs text-muted-foreground">{label}</p>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Deployments Tab */}
        <TabsContent value="deployments" className="space-y-4">
          <Card>
            <CardHeader className="flex-row items-center justify-between space-y-0">
              <div>
                <CardTitle className="text-base">Deployment History</CardTitle>
                <CardDescription>{deployments.length} deployment{deployments.length !== 1 ? 's' : ''}</CardDescription>
              </div>
              <Button size="sm" onClick={handleDeploy}>
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
                  <Button size="sm" onClick={handleDeploy}>
                    <Rocket className="size-4" />
                    Deploy Now
                  </Button>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Version</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead className="hidden md:table-cell">Image</TableHead>
                      <TableHead>Commit</TableHead>
                      <TableHead className="hidden sm:table-cell">Triggered By</TableHead>
                      <TableHead>Date</TableHead>
                      <TableHead className="text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {deployments.map((d, index) => (
                      <TableRow key={d.id}>
                        <TableCell className="font-medium">v{d.version}</TableCell>
                        <TableCell>
                          <Badge variant={STATUS_VARIANT[d.status] || 'secondary'}>
                            {d.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="hidden md:table-cell">
                          <span className="font-mono text-xs text-muted-foreground max-w-48 truncate block">
                            {d.image}
                          </span>
                        </TableCell>
                        <TableCell>
                          <code className="font-mono text-xs">
                            {d.commit_sha?.slice(0, 8) || '-'}
                          </code>
                        </TableCell>
                        <TableCell className="hidden sm:table-cell text-muted-foreground">
                          {d.triggered_by}
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          <span className="inline-flex items-center gap-1">
                            <Clock className="size-3" />
                            {timeAgo(d.created_at)}
                          </span>
                        </TableCell>
                        <TableCell className="text-right">
                          {index > 0 && (
                            <Button variant="ghost" size="sm" className="h-7 text-xs">
                              <History className="size-3" />
                              Rollback
                            </Button>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Environment Tab */}
        <TabsContent value="env" className="space-y-4">
          <Card>
            <CardHeader className="flex-row items-center justify-between space-y-0">
              <div>
                <CardTitle className="text-base">Environment Variables</CardTitle>
                <CardDescription>
                  Manage your application's environment configuration. Use <code className="text-xs bg-muted px-1 py-0.5 rounded">{'${SECRET:name}'}</code> for encrypted references.
                </CardDescription>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setShowSecrets(!showSecrets)}
                >
                  {showSecrets ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  {showSecrets ? 'Hide' : 'Reveal'}
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Add new variable */}
              <div className="flex items-end gap-3">
                <div className="flex-1">
                  <Label htmlFor="env-key" className="mb-1.5">Key</Label>
                  <Input
                    id="env-key"
                    placeholder="VARIABLE_NAME"
                    value={newEnvKey}
                    onChange={(e) => setNewEnvKey(e.target.value.toUpperCase())}
                    className="font-mono text-sm"
                  />
                </div>
                <div className="flex-1">
                  <Label htmlFor="env-value" className="mb-1.5">Value</Label>
                  <Input
                    id="env-value"
                    placeholder="value"
                    value={newEnvValue}
                    onChange={(e) => setNewEnvValue(e.target.value)}
                    className="font-mono text-sm"
                  />
                </div>
                <Button size="sm" onClick={addEnvVar} disabled={!newEnvKey.trim()}>
                  <Plus className="size-4" />
                  Add
                </Button>
              </div>

              <Separator />

              {/* Variable list */}
              {envVars.length === 0 ? (
                <p className="text-sm text-muted-foreground text-center py-6">
                  No environment variables configured.
                </p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Key</TableHead>
                      <TableHead>Value</TableHead>
                      <TableHead className="w-24 text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {envVars.map((v) => (
                      <TableRow key={v.key}>
                        <TableCell className="font-mono text-sm font-medium">
                          {v.key}
                        </TableCell>
                        <TableCell className="font-mono text-sm text-muted-foreground">
                          {v.isSecret && !showSecrets
                            ? '\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022'
                            : v.value}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex items-center justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-7"
                              onClick={() => navigator.clipboard.writeText(v.value)}
                            >
                              <Copy className="size-3" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-7"
                              onClick={() => setEditingEnv(v.key)}
                            >
                              <Pencil className="size-3" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-7 text-destructive hover:text-destructive"
                              onClick={() => removeEnvVar(v.key)}
                            >
                              <Trash2 className="size-3" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Logs Tab */}
        <TabsContent value="logs" className="space-y-4">
          <LogsPanel appId={id || ''} />
        </TabsContent>

        {/* Settings Tab */}
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
                    Permanently remove this application, all deployments, domains, and associated data.
                  </p>
                </div>
                <Button
                  variant="destructive"
                  size="sm"
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

/** Separated logs panel to manage SSE lifecycle independently */
function LogsPanel({ appId }: { appId: string }) {
  const [logs, setLogs] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    if (!appId) return;

    const eventSource = new EventSource(`/api/v1/apps/${appId}/logs/stream`);
    setConnected(true);

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
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div>
          <CardTitle className="text-base inline-flex items-center gap-2">
            <Terminal className="size-4" />
            Application Logs
          </CardTitle>
          <CardDescription>Real-time log stream via SSE</CardDescription>
        </div>
        <div className="flex items-center gap-2">
          <div className={cn(
            'size-2 rounded-full',
            connected ? 'bg-green-500' : 'bg-muted-foreground'
          )} />
          <span className="text-xs text-muted-foreground">
            {connected ? 'Connected' : 'Disconnected'}
          </span>
        </div>
      </CardHeader>
      <CardContent>
        <div className="relative rounded-lg bg-zinc-950 border border-zinc-800 overflow-hidden">
          <div className="flex items-center gap-2 px-4 py-2 border-b border-zinc-800 bg-zinc-900">
            <div className="flex gap-1.5">
              <div className="size-3 rounded-full bg-red-500/80" />
              <div className="size-3 rounded-full bg-yellow-500/80" />
              <div className="size-3 rounded-full bg-green-500/80" />
            </div>
            <span className="text-xs text-zinc-500 ml-2 font-mono">logs — {appId}</span>
          </div>
          <div className="h-96 overflow-auto p-4 font-mono text-sm leading-relaxed">
            {logs.length === 0 ? (
              <div className="flex items-center gap-2 text-zinc-500">
                <div className="size-1.5 rounded-full bg-zinc-500 animate-pulse" />
                Waiting for logs...
              </div>
            ) : (
              logs.map((line, i) => (
                <div key={i} className="text-zinc-300 hover:bg-zinc-900/50 px-1 -mx-1 rounded">
                  <span className="text-zinc-600 select-none mr-3">{String(i + 1).padStart(4, ' ')}</span>
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
