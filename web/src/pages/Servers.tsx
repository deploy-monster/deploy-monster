import { useState } from 'react';
import { Plus, Server, Loader2, AlertCircle } from 'lucide-react';
import type { ServerNode } from '@/api/servers';
import { useApi, useMutation } from '@/hooks';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Select } from '@/components/ui/select';
import {
  Sheet, SheetContent, SheetHeader, SheetFooter, SheetTitle, SheetDescription, SheetBody,
} from '@/components/ui/sheet';
import { toast } from '@/stores/toastStore';
import {
  providers,
  regions,
  sizes,
  getProviderConfig,
} from '@/components/Servers';
import {
  ServerCardSkeleton,
  ServerCard,
  LocalhostCard,
} from '@/components/Servers';

// P1-12: hand-rolled loading/error state migrated to useMutation

export function Servers() {
  const { data: serversResp, loading, refetch } = useApi<
    { data: ServerNode[]; total: number } | ServerNode[]
  >('/servers');
  const servers: ServerNode[] = Array.isArray(serversResp)
    ? serversResp
    : serversResp?.data ?? [];
  const [dialogOpen, setDialogOpen] = useState(false);
  const [provider, setProvider] = useState('custom');
  const [hostname, setHostname] = useState('');
  const [region, setRegion] = useState('');
  const [size, setSize] = useState('small');
  const [ipAddress, setIpAddress] = useState('');

  const { mutate: addServer, loading: adding, error: addError } = useMutation('post', '/servers');

  const isCustom = provider === 'custom';
  const providerRegions = regions[provider] || [];

  const handleAdd = async () => {
    if (!hostname) return;
    try {
      await addServer({
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
    } catch {
      // addError is set by the hook
    }
  };

  const remoteServers = servers.filter((s) => s.id !== 'local' && s.provider !== 'local');
  const activeCount = remoteServers.filter((s) => s.status === 'active' || s.status === 'running').length;
  const connectedCount = remoteServers.filter((s) => s.connected).length;

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Server className="size-5 text-primary" />
              <Badge variant="secondary" className="text-xs font-normal">
                {remoteServers.length + 1} server{remoteServers.length !== 0 ? 's' : ''}
                {' \u00b7 '}{activeCount + 1} active
                {' \u00b7 '}{connectedCount + 1} connected
              </Badge>
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Servers
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base max-w-lg">
              Manage your infrastructure. Provision cloud servers or connect existing ones via SSH.
            </p>
          </div>
          <Button onClick={() => setDialogOpen(true)} className="shrink-0 cursor-pointer">
            <Plus className="size-4" />
            Add Server
          </Button>
        </div>
        {/* Decorative */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Add Server Sheet */}
      <Sheet open={dialogOpen} onOpenChange={(open) => !open && setDialogOpen(false)}>
        <SheetContent onClose={() => setDialogOpen(false)}>
          <SheetHeader>
            <SheetTitle className="flex items-center gap-3">
              <div className={cn(
                'flex items-center justify-center rounded-xl size-9',
                getProviderConfig(provider).bgColor
              )}>
                <Server className={cn('size-4', getProviderConfig(provider).textColor)} />
              </div>
              Add Server
            </SheetTitle>
            <SheetDescription>
              Provision a new cloud server or connect an existing one via SSH.
            </SheetDescription>
          </SheetHeader>

          <SheetBody>
            <div className="space-y-4">
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
          </SheetBody>

          <SheetFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={adding} className="cursor-pointer">
              Cancel
            </Button>
            <Button
              onClick={handleAdd}
              disabled={!hostname || (isCustom && !ipAddress) || adding}
              className="cursor-pointer"
            >
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
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {/* Loading */}
      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <ServerCardSkeleton key={i} />
          ))}
        </div>
      )}

      {/* Server Cards */}
      {!loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <LocalhostCard />
          {remoteServers.map((s) => (
            <ServerCard key={s.id} server={s} />
          ))}
        </div>
      )}
    </div>
  );
}