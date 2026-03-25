import { useState } from 'react';
import {
  Plus,
  Cloud,
  Key,
  Monitor,
  Cpu,
  Loader2,
  AlertCircle,
  Server,
  MapPin,
  Clock,
} from 'lucide-react';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Select } from '@/components/ui/select';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from '@/components/Toast';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ServerNode {
  id: string;
  hostname: string;
  ip_address: string;
  provider: string;
  region: string;
  size: string;
  status: string;
  role: string;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Provider configuration with colors
// ---------------------------------------------------------------------------

interface ProviderConfig {
  id: string;
  name: string;
  icon: typeof Cloud;
  desc: string;
  bgColor: string;
  textColor: string;
  badgeColor: string;
  letter: string;
}

const providers: ProviderConfig[] = [
  {
    id: 'hetzner',
    name: 'Hetzner Cloud',
    icon: Cloud,
    desc: 'Provision new server',
    bgColor: 'bg-red-500/10',
    textColor: 'text-red-500',
    badgeColor: 'bg-red-500/10 text-red-600 border-red-500/20 dark:text-red-400',
    letter: 'HZ',
  },
  {
    id: 'digitalocean',
    name: 'DigitalOcean',
    icon: Cloud,
    desc: 'Provision new server',
    bgColor: 'bg-blue-500/10',
    textColor: 'text-blue-500',
    badgeColor: 'bg-blue-500/10 text-blue-600 border-blue-500/20 dark:text-blue-400',
    letter: 'DO',
  },
  {
    id: 'vultr',
    name: 'Vultr',
    icon: Cloud,
    desc: 'Provision new server',
    bgColor: 'bg-purple-500/10',
    textColor: 'text-purple-500',
    badgeColor: 'bg-purple-500/10 text-purple-600 border-purple-500/20 dark:text-purple-400',
    letter: 'VL',
  },
  {
    id: 'custom',
    name: 'Custom SSH',
    icon: Key,
    desc: 'Connect existing server',
    bgColor: 'bg-muted',
    textColor: 'text-muted-foreground',
    badgeColor: 'bg-muted text-muted-foreground',
    letter: 'SSH',
  },
];

function getProviderConfig(providerId: string): ProviderConfig {
  return providers.find((p) => p.id === providerId) || providers[3];
}

const regions: Record<string, { id: string; name: string }[]> = {
  hetzner: [
    { id: 'fsn1', name: 'Falkenstein' },
    { id: 'nbg1', name: 'Nuremberg' },
    { id: 'hel1', name: 'Helsinki' },
    { id: 'ash', name: 'Ashburn' },
  ],
  digitalocean: [
    { id: 'nyc1', name: 'New York 1' },
    { id: 'sfo3', name: 'San Francisco 3' },
    { id: 'ams3', name: 'Amsterdam 3' },
    { id: 'fra1', name: 'Frankfurt 1' },
  ],
  vultr: [
    { id: 'ewr', name: 'New Jersey' },
    { id: 'lax', name: 'Los Angeles' },
    { id: 'fra', name: 'Frankfurt' },
    { id: 'nrt', name: 'Tokyo' },
  ],
};

const sizes = [
  { id: 'small', name: 'Small', desc: '2 vCPU / 2 GB RAM / 40 GB' },
  { id: 'medium', name: 'Medium', desc: '4 vCPU / 8 GB RAM / 80 GB' },
  { id: 'large', name: 'Large', desc: '8 vCPU / 16 GB RAM / 160 GB' },
  { id: 'xlarge', name: 'X-Large', desc: '16 vCPU / 32 GB RAM / 320 GB' },
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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
// Sub-components
// ---------------------------------------------------------------------------

function CardSkeleton() {
  return (
    <Card className="py-5">
      <CardHeader className="pb-0">
        <div className="flex items-start gap-3">
          <Skeleton className="size-11 rounded-xl shrink-0" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-5 w-24" />
            <Skeleton className="h-3.5 w-32" />
          </div>
          <Skeleton className="h-5 w-16 rounded-md" />
        </div>
      </CardHeader>
      <CardContent className="pt-0 mt-3">
        <Skeleton className="h-4 w-48" />
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Servers
// ---------------------------------------------------------------------------

export function Servers() {
  const { data: servers, loading, refetch } = useApi<ServerNode[]>('/servers');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [provider, setProvider] = useState('hetzner');
  const [hostname, setHostname] = useState('');
  const [region, setRegion] = useState('');
  const [size, setSize] = useState('small');
  const [ipAddress, setIpAddress] = useState('');
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState('');

  const isCustom = provider === 'custom';
  const providerRegions = regions[provider] || [];

  const handleAdd = async () => {
    if (!hostname) return;
    setAdding(true);
    setAddError('');
    try {
      await api.post('/servers', {
        hostname,
        provider,
        region: isCustom ? '' : region,
        size: isCustom ? '' : size,
        ip_address: isCustom ? ipAddress : undefined,
      });
      toast.success(isCustom ? 'Server connected' : 'Server provisioning started');
      setHostname('');
      setRegion('');
      setSize('small');
      setIpAddress('');
      setDialogOpen(false);
      refetch();
    } catch (err) {
      setAddError(err instanceof Error ? err.message : 'Failed to add server');
    } finally {
      setAdding(false);
    }
  };

  const list = servers || [];
  const activeCount = list.filter((s) => s.status === 'active' || s.status === 'running').length;

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Server className="size-5 text-primary" />
              <Badge variant="secondary" className="text-xs font-normal">
                {list.length + 1} server{list.length !== 0 ? 's' : ''}
                {activeCount > 0 && ` \u00b7 ${activeCount + 1} active`}
              </Badge>
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Servers
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base max-w-lg">
              Manage your infrastructure. Provision cloud servers or connect existing ones via SSH.
            </p>
          </div>
          <Button onClick={() => setDialogOpen(true)} className="shrink-0">
            <Plus className="size-4" />
            Add Server
          </Button>
        </div>
        {/* Decorative */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Add Server Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent onClose={() => setDialogOpen(false)} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-3">
              <div className={cn(
                'flex items-center justify-center rounded-xl size-9',
                getProviderConfig(provider).bgColor
              )}>
                <Server className={cn('size-4', getProviderConfig(provider).textColor)} />
              </div>
              Add Server
            </DialogTitle>
            <DialogDescription>
              Provision a new cloud server or connect an existing one via SSH.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            {/* Provider Selection Cards */}
            <div className="space-y-1.5">
              <Label>Provider</Label>
              <div className="grid grid-cols-4 gap-2">
                {providers.map((p) => {
                  const Icon = p.icon;
                  return (
                    <button
                      key={p.id}
                      type="button"
                      onClick={() => { setProvider(p.id); setRegion(''); }}
                      className={cn(
                        'flex flex-col items-center gap-1.5 rounded-lg border p-3 transition-all duration-200 cursor-pointer',
                        provider === p.id
                          ? 'border-primary bg-primary/5 ring-1 ring-primary/20'
                          : 'hover:bg-accent hover:text-accent-foreground'
                      )}
                    >
                      <div className={cn('flex items-center justify-center rounded-lg size-8', p.bgColor)}>
                        <Icon className={cn('size-4', p.textColor)} />
                      </div>
                      <span className="text-[10px] font-medium truncate w-full text-center">{p.name}</span>
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="srv-hostname">Hostname</Label>
              <div className="relative">
                <Server className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                <Input
                  id="srv-hostname"
                  value={hostname}
                  onChange={(e) => setHostname(e.target.value)}
                  placeholder="web-01"
                  className="pl-9"
                />
              </div>
            </div>

            {isCustom ? (
              <div className="space-y-1.5">
                <Label htmlFor="srv-ip">IP Address</Label>
                <Input
                  id="srv-ip"
                  value={ipAddress}
                  onChange={(e) => setIpAddress(e.target.value)}
                  placeholder="203.0.113.10"
                  className="font-mono"
                />
              </div>
            ) : (
              <>
                <div className="space-y-1.5">
                  <Label htmlFor="srv-region">Region</Label>
                  <Select
                    id="srv-region"
                    value={region}
                    onChange={(e) => setRegion(e.target.value)}
                  >
                    <option value="">Select region...</option>
                    {providerRegions.map((r) => (
                      <option key={r.id} value={r.id}>{r.name}</option>
                    ))}
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="srv-size">Size</Label>
                  <Select
                    id="srv-size"
                    value={size}
                    onChange={(e) => setSize(e.target.value)}
                  >
                    {sizes.map((s) => (
                      <option key={s.id} value={s.id}>{s.name} -- {s.desc}</option>
                    ))}
                  </Select>
                </div>
              </>
            )}

            {addError && (
              <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2.5 text-sm text-destructive">
                <AlertCircle className="size-4 shrink-0" />
                {addError}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={adding}>
              Cancel
            </Button>
            <Button onClick={handleAdd} disabled={!hostname || adding}>
              {adding ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  {isCustom ? 'Connecting...' : 'Provisioning...'}
                </>
              ) : (
                <>
                  <Plus className="size-4" />
                  {isCustom ? 'Connect' : 'Provision'}
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Loading */}
      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      )}

      {/* Server Cards */}
      {!loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* Localhost card -- always present */}
          <Card className="group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
            <CardHeader className="gap-4">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="flex items-center justify-center rounded-xl size-11 shrink-0 bg-emerald-500/10">
                    <Monitor className="size-5 text-emerald-500" />
                  </div>
                  <div>
                    <CardTitle className="text-base">localhost</CardTitle>
                    <CardDescription className="font-mono text-xs">127.0.0.1</CardDescription>
                  </div>
                </div>
                <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400 gap-1.5">
                  <span className="relative flex size-2">
                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                    <span className="relative inline-flex rounded-full size-2 bg-emerald-500" />
                  </span>
                  Active
                </Badge>
              </div>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-3 text-sm text-muted-foreground">
                <Badge variant="outline" className="gap-1 text-xs font-normal">
                  <Cpu className="size-3" /> Local
                </Badge>
                <Badge variant="secondary" className="text-xs font-normal">Master Node</Badge>
              </div>
            </CardContent>
          </Card>

          {/* Remote servers */}
          {list.map((s) => {
            const providerCfg = getProviderConfig(s.provider);
            const isActive = s.status === 'active' || s.status === 'running';

            return (
              <Card
                key={s.id}
                className="group transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg"
              >
                <CardHeader className="gap-4">
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                      <div className={cn(
                        'flex items-center justify-center rounded-xl size-11 shrink-0',
                        providerCfg.bgColor
                      )}>
                        <span className={cn('text-sm font-bold', providerCfg.textColor)}>
                          {providerCfg.letter}
                        </span>
                      </div>
                      <div>
                        <CardTitle className="text-base">{s.hostname}</CardTitle>
                        <CardDescription className="font-mono text-xs">{s.ip_address}</CardDescription>
                      </div>
                    </div>
                    {isActive ? (
                      <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400 gap-1.5">
                        <span className="relative flex size-2">
                          <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                          <span className="relative inline-flex rounded-full size-2 bg-emerald-500" />
                        </span>
                        Active
                      </Badge>
                    ) : (
                      <Badge variant="secondary" className="gap-1.5">
                        <span className="size-2 rounded-full bg-muted-foreground" />
                        {s.status.charAt(0).toUpperCase() + s.status.slice(1)}
                      </Badge>
                    )}
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center gap-3 flex-wrap text-sm text-muted-foreground">
                    <Badge variant="outline" className={cn('text-xs font-normal', providerCfg.badgeColor)}>
                      {providerCfg.name}
                    </Badge>
                    {s.region && (
                      <span className="flex items-center gap-1 text-xs">
                        <MapPin className="size-3" /> {s.region}
                      </span>
                    )}
                    {s.size && (
                      <span className="flex items-center gap-1 text-xs">
                        <Cpu className="size-3" /> {s.size}
                      </span>
                    )}
                    {s.role && (
                      <Badge variant="secondary" className="text-xs font-normal">
                        {s.role.charAt(0).toUpperCase() + s.role.slice(1)}
                      </Badge>
                    )}
                  </div>
                </CardContent>
                <CardFooter className="border-t pt-4 pb-0">
                  <span className="flex items-center gap-1.5 text-xs text-muted-foreground tabular-nums">
                    <Clock className="size-3" />
                    Added {timeAgo(s.created_at)}
                  </span>
                </CardFooter>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
