import { useState } from 'react';
import {
  Shield, Server, RefreshCw, Users, Settings as SettingsIcon,
  Cpu, MemoryStick, Gauge, Radio,
} from 'lucide-react';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
} from '@/components/ui/table';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from '@/components/Toast';

interface SystemInfo {
  version: string;
  commit: string;
  go: string;
  os: string;
  arch: string;
  goroutines: number;
  memory: { alloc_mb: number; sys_mb: number };
  modules: Array<{ id: string; status: string }>;
  events: { published: number; errors: number; subscriptions: number };
}

interface Tenant {
  id: string;
  name: string;
  slug: string;
  plan: string;
  status: string;
  members_count: number;
  created_at: string;
}

interface AdminSettings {
  registration_mode: string;
  auto_ssl: boolean;
  telemetry: boolean;
  backup_retention_days: number;
}

function ModuleStatusBadge({ status }: { status: string }) {
  switch (status) {
    case 'ok':
      return (
        <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
          OK
        </Badge>
      );
    case 'degraded':
      return (
        <Badge className="bg-amber-500/10 text-amber-600 border-amber-500/20 dark:text-amber-400">
          Degraded
        </Badge>
      );
    default:
      return <Badge variant="destructive">{status}</Badge>;
  }
}

export function Admin() {
  const { data: system, loading: systemLoading, refetch: refresh } = useApi<SystemInfo>('/admin/system');
  const { data: tenants, loading: tenantsLoading } = useApi<Tenant[]>('/admin/tenants');
  const [settings, setSettings] = useState<AdminSettings>({
    registration_mode: 'open',
    auto_ssl: true,
    telemetry: false,
    backup_retention_days: 30,
  });
  const [savingSettings, setSavingSettings] = useState(false);

  const handleSaveSettings = async () => {
    setSavingSettings(true);
    try {
      await api.put('/admin/settings', settings);
      toast.success('Settings saved');
    } catch {
      toast.error('Failed to save settings');
    } finally {
      setSavingSettings(false);
    }
  };

  const tenantList = tenants || [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Admin Panel</h1>
          <p className="text-sm text-muted-foreground mt-1">System administration and monitoring</p>
        </div>
        <Button variant="outline" onClick={refresh}>
          <RefreshCw size={14} /> Refresh
        </Button>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="system">
        <TabsList>
          <TabsTrigger value="system">
            <Server size={14} /> System
          </TabsTrigger>
          <TabsTrigger value="tenants">
            <Users size={14} /> Tenants
          </TabsTrigger>
          <TabsTrigger value="settings">
            <SettingsIcon size={14} /> Settings
          </TabsTrigger>
        </TabsList>

        {/* System Tab */}
        <TabsContent value="system">
          {systemLoading && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {[1, 2, 3, 4, 5, 6].map((i) => (
                <Card key={i}>
                  <CardContent className="space-y-2">
                    <Skeleton className="h-4 w-24" />
                    <Skeleton className="h-6 w-32" />
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          {system && (
            <div className="space-y-6">
              {/* Info Cards */}
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                <Card>
                  <CardHeader className="pb-2">
                    <CardDescription>Version</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <p className="text-lg font-semibold">{system.version}</p>
                    <p className="text-xs text-muted-foreground mt-1 font-mono">{system.commit}</p>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader className="pb-2">
                    <CardDescription className="flex items-center gap-1.5">
                      <Cpu size={14} /> Runtime
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <p className="text-lg font-semibold">{system.go}</p>
                    <p className="text-xs text-muted-foreground mt-1">{system.os}/{system.arch}</p>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader className="pb-2">
                    <CardDescription className="flex items-center gap-1.5">
                      <Gauge size={14} /> Goroutines
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <p className="text-lg font-semibold">{system.goroutines}</p>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader className="pb-2">
                    <CardDescription className="flex items-center gap-1.5">
                      <MemoryStick size={14} /> Memory (alloc)
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <p className="text-lg font-semibold">{system.memory?.alloc_mb} MB</p>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader className="pb-2">
                    <CardDescription className="flex items-center gap-1.5">
                      <MemoryStick size={14} /> Memory (sys)
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <p className="text-lg font-semibold">{system.memory?.sys_mb} MB</p>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader className="pb-2">
                    <CardDescription className="flex items-center gap-1.5">
                      <Radio size={14} /> Events
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <p className="text-lg font-semibold">{system.events?.published || 0}</p>
                    <p className="text-xs text-muted-foreground mt-1">
                      {system.events?.errors || 0} errors / {system.events?.subscriptions || 0} subs
                    </p>
                  </CardContent>
                </Card>
              </div>

              {/* Modules Table */}
              <div>
                <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
                  <Shield size={18} /> Modules
                </h2>
                <Card className="py-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Module</TableHead>
                        <TableHead className="w-[120px]">Status</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {(system.modules || []).map((m) => (
                        <TableRow key={m.id}>
                          <TableCell className="font-mono text-sm font-medium">{m.id}</TableCell>
                          <TableCell>
                            <ModuleStatusBadge status={m.status} />
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </Card>
              </div>
            </div>
          )}
        </TabsContent>

        {/* Tenants Tab */}
        <TabsContent value="tenants">
          {tenantsLoading && (
            <Card>
              <CardContent className="space-y-3 py-2">
                {[1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </CardContent>
            </Card>
          )}

          {!tenantsLoading && tenantList.length === 0 && (
            <Card className="py-16">
              <CardContent className="flex flex-col items-center text-center">
                <Users className="mb-4 text-muted-foreground" size={48} />
                <h2 className="text-lg font-medium mb-2">No tenants</h2>
                <p className="text-muted-foreground">Tenants will appear here when users register.</p>
              </CardContent>
            </Card>
          )}

          {!tenantsLoading && tenantList.length > 0 && (
            <Card className="py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Slug</TableHead>
                    <TableHead>Plan</TableHead>
                    <TableHead>Members</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Created</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tenantList.map((t) => (
                    <TableRow key={t.id}>
                      <TableCell className="font-medium">{t.name}</TableCell>
                      <TableCell className="font-mono text-sm text-muted-foreground">{t.slug}</TableCell>
                      <TableCell>
                        <Badge variant="outline">{t.plan}</Badge>
                      </TableCell>
                      <TableCell className="text-muted-foreground">{t.members_count}</TableCell>
                      <TableCell>
                        {t.status === 'active' ? (
                          <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
                            Active
                          </Badge>
                        ) : (
                          <Badge variant="secondary">{t.status}</Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {new Date(t.created_at).toLocaleDateString()}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </TabsContent>

        {/* Settings Tab */}
        <TabsContent value="settings">
          <Card>
            <CardHeader>
              <CardTitle>Platform Settings</CardTitle>
              <CardDescription>Global configuration for the DeployMonster instance.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-2 max-w-md">
                <Label htmlFor="admin-reg-mode">Registration Mode</Label>
                <Input
                  id="admin-reg-mode"
                  value={settings.registration_mode}
                  onChange={(e) => setSettings((s) => ({ ...s, registration_mode: e.target.value }))}
                  placeholder="open / invite / closed"
                />
              </div>

              <div className="space-y-2 max-w-md">
                <Label htmlFor="admin-retention">Backup Retention (days)</Label>
                <Input
                  id="admin-retention"
                  type="number"
                  value={settings.backup_retention_days}
                  onChange={(e) =>
                    setSettings((s) => ({ ...s, backup_retention_days: Number(e.target.value) }))
                  }
                />
              </div>

              <Separator />

              <div className="space-y-4">
                <div className="flex items-center justify-between max-w-md">
                  <div>
                    <Label>Automatic SSL</Label>
                    <p className="text-sm text-muted-foreground">
                      Auto-provision SSL via Let's Encrypt
                    </p>
                  </div>
                  <Switch
                    checked={settings.auto_ssl}
                    onCheckedChange={(v) => setSettings((s) => ({ ...s, auto_ssl: v }))}
                  />
                </div>

                <div className="flex items-center justify-between max-w-md">
                  <div>
                    <Label>Anonymous Telemetry</Label>
                    <p className="text-sm text-muted-foreground">
                      Send anonymous usage statistics
                    </p>
                  </div>
                  <Switch
                    checked={settings.telemetry}
                    onCheckedChange={(v) => setSettings((s) => ({ ...s, telemetry: v }))}
                  />
                </div>
              </div>

              <Separator />

              <Button onClick={handleSaveSettings} disabled={savingSettings}>
                {savingSettings ? 'Saving...' : 'Save Settings'}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
