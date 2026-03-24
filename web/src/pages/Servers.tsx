import { useState } from 'react';
import {
  Plus, Wifi, WifiOff, Cloud, Key, Monitor, Cpu,
} from 'lucide-react';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
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
const providers = [
  { id: 'hetzner', name: 'Hetzner Cloud', icon: Cloud, desc: 'Provision new server' },
  { id: 'digitalocean', name: 'DigitalOcean', icon: Cloud, desc: 'Provision new server' },
  { id: 'vultr', name: 'Vultr', icon: Cloud, desc: 'Provision new server' },
  { id: 'custom', name: 'Custom SSH', icon: Key, desc: 'Connect existing server' },
];
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
export function Servers() {
  const { data: servers, loading, refetch } = useApi<ServerNode[]>('/servers');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [provider, setProvider] = useState('hetzner');
  const [hostname, setHostname] = useState('');
  const [region, setRegion] = useState('');
  const [size, setSize] = useState('small');
  const [ipAddress, setIpAddress] = useState('');
  const isCustom = provider === 'custom';
  const providerRegions = regions[provider] || [];
  const handleAdd = async () => {
    if (!hostname) return;
    await api.post('/servers', {
      hostname,
      provider,
      region: isCustom ? '' : region,
      size: isCustom ? '' : size,
      ip_address: isCustom ? ipAddress : undefined,
    });
    setHostname('');
    setRegion('');
    setSize('small');
    setIpAddress('');
    setDialogOpen(false);
    refetch();
  };
  const list = servers || [];
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Servers</h1>
          <p className="text-sm text-muted-foreground mt-1">Manage your infrastructure</p>
        </div>
        <Button onClick={() => setDialogOpen(true)}>
          <Plus /> Add Server
        </Button>
      </div>
      {/* Add Server Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent onClose={() => setDialogOpen(false)}>
          <DialogHeader>
            <DialogTitle>Add Server</DialogTitle>
            <DialogDescription>
              Provision a new cloud server or connect an existing one via SSH.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="srv-provider">Provider</Label>
              <Select
                id="srv-provider"
                value={provider}
                onChange={(e) => {
                  setProvider(e.target.value);
                  setRegion('');
                }}
              >
                {providers.map((p) => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="srv-hostname">Hostname</Label>
              <Input
                id="srv-hostname"
                value={hostname}
                onChange={(e) => setHostname(e.target.value)}
                placeholder="web-01"
              />
            </div>
            {isCustom ? (
              <div className="space-y-2">
                <Label htmlFor="srv-ip">IP Address</Label>
                <Input
                  id="srv-ip"
                  value={ipAddress}
                  onChange={(e) => setIpAddress(e.target.value)}
                  placeholder="203.0.113.10"
                />
              </div>
            ) : (
              <>
                <div className="space-y-2">
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
                <div className="space-y-2">
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
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleAdd} disabled={!hostname}>
              {isCustom ? 'Connect' : 'Provision'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {/* Loading */}
      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {[1, 2].map((i) => (
            <Card key={i}>
              <CardContent className="space-y-3">
                <Skeleton className="h-6 w-32" />
                <Skeleton className="h-4 w-48" />
                <Skeleton className="h-4 w-24" />
              </CardContent>
            </Card>
          ))}
        </div>
      )}
      {/* Server Cards */}
      {!loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* Localhost card -- always present */}
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="flex size-10 items-center justify-center rounded-lg bg-primary/10">
                    <Monitor size={20} className="text-primary" />
                  </div>
                  <div>
                    <CardTitle className="text-base">localhost</CardTitle>
                    <CardDescription>127.0.0.1 (this server)</CardDescription>
                  </div>
                </div>
                <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
                  <Wifi size={12} /> Active
                </Badge>
              </div>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-4 text-sm text-muted-foreground">
                <span className="flex items-center gap-1"><Cpu size={14} /> Local</span>
                <span>Master Node</span>
              </div>
            </CardContent>
          </Card>
          {/* Remote servers */}
          {list.map((s) => {
            const providerInfo = providers.find((p) => p.id === s.provider);
            const ProviderIcon = providerInfo?.icon || Cloud;
            const isActive = s.status === 'active' || s.status === 'running';
            return (
              <Card key={s.id}>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className="flex size-10 items-center justify-center rounded-lg bg-primary/10">
                        <ProviderIcon size={20} className="text-primary" />
                      </div>
                      <div>
                        <CardTitle className="text-base">{s.hostname}</CardTitle>
                        <CardDescription>{s.ip_address}</CardDescription>
                      </div>
                    </div>
                    {isActive ? (
                      <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
                        <Wifi size={12} /> Active
                      </Badge>
                    ) : (
                      <Badge variant="secondary">
                        <WifiOff size={12} /> {s.status}
                      </Badge>
                    )}
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center gap-4 text-sm text-muted-foreground">
                    <Badge variant="outline">{providerInfo?.name || s.provider}</Badge>
                    {s.region && <span>{s.region}</span>}
                    {s.size && <span>{s.size}</span>}
                  </div>
                </CardContent>
                <CardFooter className="text-xs text-muted-foreground">
                  Added {new Date(s.created_at).toLocaleDateString()}
                </CardFooter>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
