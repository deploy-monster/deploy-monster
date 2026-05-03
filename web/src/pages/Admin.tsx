import { useState } from 'react';
import {
  Shield,
  Server,
  RefreshCw,
  Users,
  Settings as SettingsIcon,
  Cpu,
  MemoryStick,
  Gauge,
  Radio,
  Box,
  CheckCircle2,
  Save,
  Loader2,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { adminAPI, type SystemInfo, type Tenant, type AdminSettings } from '@/api/admin';
import type { PaginatedResponse } from '@/api/client';
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
import { toast } from '@/stores/toastStore';

// ---------------------------------------------------------------------------
// Stat card config
// ---------------------------------------------------------------------------

interface StatCardDef {
  key: string;
  icon: typeof Cpu;
  label: string;
  bgColor: string;
  iconColor: string;
  getValue: (s: SystemInfo) => string;
  getSub?: (s: SystemInfo) => string;
}

const STAT_CARDS: StatCardDef[] = [
  {
    key: 'version',
    icon: Box,
    label: 'Version',
    bgColor: 'bg-emerald-500/10',
    iconColor: 'text-emerald-500',
    getValue: (s) => s.version || '--',
    getSub: (s) => s.commit ? s.commit.slice(0, 8) : '',
  },
  {
    key: 'runtime',
    icon: Cpu,
    label: 'Runtime',
    bgColor: 'bg-blue-500/10',
    iconColor: 'text-blue-500',
    getValue: (s) => s.go || '--',
    getSub: (s) => `${s.os}/${s.arch}`,
  },
  {
    key: 'memory',
    icon: MemoryStick,
    label: 'Memory (alloc)',
    bgColor: 'bg-purple-500/10',
    iconColor: 'text-purple-500',
    getValue: (s) => `${s.memory?.alloc_mb || 0} MB`,
    getSub: (s) => `${s.memory?.sys_mb || 0} MB sys`,
  },
  {
    key: 'goroutines',
    icon: Gauge,
    label: 'Goroutines',
    bgColor: 'bg-amber-500/10',
    iconColor: 'text-amber-500',
    getValue: (s) => String(s.goroutines || 0),
  },
  {
    key: 'events',
    icon: Radio,
    label: 'Events Published',
    bgColor: 'bg-cyan-500/10',
    iconColor: 'text-cyan-500',
    getValue: (s) => String(s.events?.published || 0),
    getSub: (s) => `${s.events?.errors || 0} errors / ${s.events?.subscriptions || 0} subs`,
  },
];

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function StatCardSkeleton() {
  return (
    <Card className="py-4">
      <CardContent className="flex items-center gap-4">
        <Skeleton className="size-11 rounded-xl" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-6 w-16" />
          <Skeleton className="h-3 w-20" />
        </div>
      </CardContent>
    </Card>
  );
}

function ModuleStatusBadge({ status }: { status: string }) {
  switch (status) {
    case 'ok':
      return (
        <Badge className="gap-1.5 bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
          <span className="size-1.5 rounded-full bg-emerald-500" />
          OK
        </Badge>
      );
    case 'degraded':
      return (
        <Badge className="gap-1.5 bg-amber-500/10 text-amber-600 border-amber-500/20 dark:text-amber-400">
          <span className="size-1.5 rounded-full bg-amber-500" />
          Degraded
        </Badge>
      );
    default:
      return (
        <Badge variant="destructive" className="gap-1.5">
          <span className="size-1.5 rounded-full bg-white" />
          {status || 'Error'}
        </Badge>
      );
  }
}

function TenantStatusBadge({ status }: { status: string }) {
  if (status === 'active') {
    return (
      <Badge className="gap-1.5 bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
        <span className="size-1.5 rounded-full bg-emerald-500" />
        Active
      </Badge>
    );
  }
  if (status === 'suspended') {
    return (
      <Badge className="gap-1.5 bg-red-500/10 text-red-600 border-red-500/20 dark:text-red-400">
        <span className="size-1.5 rounded-full bg-red-500" />
        Suspended
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" className="gap-1.5">
      <span className="size-1.5 rounded-full bg-muted-foreground" />
      {status}
    </Badge>
  );
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  return `${months}mo ago`;
}

// ---------------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------------

export function Admin() {
  const { data: system, loading: systemLoading, refetch: refresh } = useApi<SystemInfo>('/admin/system');
  const { data: tenants, loading: tenantsLoading } = useApi<PaginatedResponse<Tenant> | Tenant[]>('/admin/tenants');
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
      await adminAPI.saveSettings(settings);
      toast.success('Settings saved');
    } catch {
      toast.error('Failed to save settings');
    } finally {
      setSavingSettings(false);
    }
  };

  const tenantList = Array.isArray(tenants) ? tenants : tenants?.data || [];

  return (
    <div className="space-y-8">
      {/* Hero Section */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Shield className="size-5 text-primary" />
              {system && (
                <Badge variant="secondary" className="text-xs font-normal">
                  v{system.version}
                </Badge>
              )}
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Admin Panel
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base">
              System administration, tenant management, and platform configuration.
            </p>
          </div>
          <Button variant="outline" onClick={refresh} className="shrink-0">
            <RefreshCw className="size-4" />
            Refresh
          </Button>
        </div>
        {/* Decorative */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Tabs */}
      <Tabs defaultValue="system">
        <TabsList>
          <TabsTrigger value="system">
            <Server className="size-3.5" />
            System
          </TabsTrigger>
          <TabsTrigger value="tenants">
            <Users className="size-3.5" />
            Tenants
          </TabsTrigger>
          <TabsTrigger value="settings">
            <SettingsIcon className="size-3.5" />
            Settings
          </TabsTrigger>
        </TabsList>

        {/* System Tab */}
        <TabsContent value="system" className="space-y-6">
          {/* Stat Cards */}
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4">
            {systemLoading
              ? Array.from({ length: 5 }).map((_, i) => <StatCardSkeleton key={i} />)
              : STAT_CARDS.map(({ key, icon: Icon, label, bgColor, iconColor, getValue, getSub }) => (
                  <Card
                    key={key}
                    className="py-4 group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md"
                  >
                    <CardContent className="flex items-center gap-4">
                      <div className={cn('flex items-center justify-center rounded-xl size-11 shrink-0', bgColor)}>
                        <Icon className={cn('size-5', iconColor)} />
                      </div>
                      <div className="min-w-0">
                        <p className="text-lg font-bold tracking-tight text-foreground truncate">
                          {system ? getValue(system) : '--'}
                        </p>
                        <p className="text-xs text-muted-foreground truncate">{label}</p>
                        {system && getSub && (
                          <p className="text-[10px] text-muted-foreground/60 mt-0.5 truncate font-mono">
                            {getSub(system)}
                          </p>
                        )}
                      </div>
                    </CardContent>
                  </Card>
                ))
            }
          </div>

          {/* Modules Table */}
          {system && (
            <Card>
              <CardHeader className="flex-row items-center justify-between space-y-0">
                <div className="flex items-center gap-2">
                  <CardTitle className="text-base">Registered Modules</CardTitle>
                  <Badge variant="secondary" className="text-[10px] font-normal">
                    {(system.modules || []).length}
                  </Badge>
                </div>
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <CheckCircle2 className="size-3 text-emerald-500" />
                  {(system.modules || []).filter((m) => m.status === 'ok').length} healthy
                </div>
              </CardHeader>
              <CardContent className="pt-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Module</TableHead>
                      <TableHead className="w-[120px] text-right">Status</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(system.modules || []).map((m) => (
                      <TableRow key={m.id} className="hover:bg-muted/50 transition-colors">
                        <TableCell>
                          <span className="font-mono text-sm font-medium">{m.id}</span>
                        </TableCell>
                        <TableCell className="text-right">
                          <ModuleStatusBadge status={m.status} />
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        {/* Tenants Tab */}
        <TabsContent value="tenants">
          {tenantsLoading && (
            <Card className="py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Plan</TableHead>
                    <TableHead>Members</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Created</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {Array.from({ length: 3 }).map((_, i) => (
                    <TableRow key={i}>
                      <TableCell><Skeleton className="h-4 w-28" /></TableCell>
                      <TableCell><Skeleton className="h-5 w-16 rounded-md" /></TableCell>
                      <TableCell><Skeleton className="h-4 w-8" /></TableCell>
                      <TableCell><Skeleton className="h-5 w-16 rounded-md" /></TableCell>
                      <TableCell><Skeleton className="h-4 w-20" /></TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}

          {!tenantsLoading && tenantList.length === 0 && (
            <div className="flex flex-col items-center justify-center py-24 text-center">
              <div className="rounded-full bg-muted p-6 mb-5">
                <Users className="size-10 text-muted-foreground" />
              </div>
              <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
                No tenants
              </h2>
              <p className="text-muted-foreground max-w-sm text-sm">
                Tenants will appear here as users register and create organizations.
              </p>
            </div>
          )}

          {!tenantsLoading && tenantList.length > 0 && (
            <Card className="py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead className="hidden md:table-cell">Slug</TableHead>
                    <TableHead>Plan</TableHead>
                    <TableHead className="hidden sm:table-cell">Members</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead className="hidden sm:table-cell">Created</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tenantList.map((t) => (
                    <TableRow key={t.id} className="hover:bg-muted/50 transition-colors">
                      <TableCell>
                        <span className="font-medium text-foreground">{t.name}</span>
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        <span className="font-mono text-sm text-muted-foreground">{t.slug}</span>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-xs font-normal">
                          {t.plan}
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden sm:table-cell">
                        <span className="text-sm text-muted-foreground tabular-nums">
                          {t.members_count}
                        </span>
                      </TableCell>
                      <TableCell>
                        <TenantStatusBadge status={t.status} />
                      </TableCell>
                      <TableCell className="hidden sm:table-cell">
                        <span className="text-sm text-muted-foreground tabular-nums">
                          {timeAgo(t.created_at)}
                        </span>
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
              <CardTitle className="flex items-center gap-2 text-base">
                <SettingsIcon className="size-4 text-primary" />
                Platform Settings
              </CardTitle>
              <CardDescription>
                Global configuration for the DeployMonster instance.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Text inputs */}
              <div className="space-y-2 max-w-md">
                <Label htmlFor="admin-reg-mode">Registration Mode</Label>
                <Input
                  id="admin-reg-mode"
                  value={settings.registration_mode}
                  onChange={(e) => setSettings((s) => ({ ...s, registration_mode: e.target.value }))}
                  placeholder="open / invite / closed"
                />
                <p className="text-[11px] text-muted-foreground">
                  Controls how new users can sign up. Use &quot;open&quot; for public, &quot;invite&quot; for invite-only, or &quot;closed&quot; to disable registration.
                </p>
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
                <p className="text-[11px] text-muted-foreground">
                  Backups older than this will be automatically removed.
                </p>
              </div>

              <Separator />

              {/* Toggle switches */}
              <div className="space-y-5">
                <div className="flex items-center justify-between max-w-md rounded-lg border p-4">
                  <div className="space-y-0.5">
                    <Label className="text-sm font-medium">Automatic SSL</Label>
                    <p className="text-xs text-muted-foreground">
                      Auto-provision SSL certificates via Let&apos;s Encrypt for all domains
                    </p>
                  </div>
                  <Switch
                    checked={settings.auto_ssl}
                    onCheckedChange={(v) => setSettings((s) => ({ ...s, auto_ssl: v }))}
                  />
                </div>

                <div className="flex items-center justify-between max-w-md rounded-lg border p-4">
                  <div className="space-y-0.5">
                    <Label className="text-sm font-medium">Anonymous Telemetry</Label>
                    <p className="text-xs text-muted-foreground">
                      Send anonymous usage statistics to help improve DeployMonster
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
                {savingSettings ? (
                  <>
                    <Loader2 className="size-4 animate-spin" />
                    Saving...
                  </>
                ) : (
                  <>
                    <Save className="size-4" />
                    Save Settings
                  </>
                )}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
