import { useState } from 'react';
import {
  Globe, Plus, ShieldCheck, ShieldAlert, Trash2, CheckCircle, ExternalLink, Copy, Check,
} from 'lucide-react';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
} from '@/components/ui/table';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';

interface Domain {
  id: string;
  app_id: string;
  fqdn: string;
  type: string;
  dns_provider: string;
  dns_synced: boolean;
  verified: boolean;
  created_at: string;
}

export function Domains() {
  const { data: domains, loading, refetch } = useApi<Domain[]>('/domains');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [newFQDN, setNewFQDN] = useState('');
  const [newAppID, setNewAppID] = useState('');
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const handleAdd = async () => {
    if (!newFQDN) return;
    await api.post('/domains', { fqdn: newFQDN, app_id: newAppID });
    setNewFQDN('');
    setNewAppID('');
    setDialogOpen(false);
    refetch();
  };

  const handleVerify = async (id: string) => {
    await api.post(`/domains/${id}/verify`, {});
    refetch();
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Remove this domain?')) return;
    await api.delete(`/domains/${id}`);
    refetch();
  };

  const handleCopyFQDN = (id: string, fqdn: string) => {
    navigator.clipboard.writeText(fqdn);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const list = domains || [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Domains</h1>
          <p className="text-sm text-muted-foreground mt-1">
            {list.length} domain{list.length !== 1 ? 's' : ''} configured
          </p>
        </div>
        <Button onClick={() => setDialogOpen(true)}>
          <Plus /> Add Domain
        </Button>
      </div>

      {/* Add Domain Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent onClose={() => setDialogOpen(false)}>
          <DialogHeader>
            <DialogTitle>Add Custom Domain</DialogTitle>
            <DialogDescription>
              Point your domain to your server by adding an A record before verifying.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="fqdn">Domain (FQDN)</Label>
              <Input
                id="fqdn"
                value={newFQDN}
                onChange={(e) => setNewFQDN(e.target.value)}
                placeholder="app.example.com"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="app-id">Application ID</Label>
              <Input
                id="app-id"
                value={newAppID}
                onChange={(e) => setNewAppID(e.target.value)}
                placeholder="app_xxxxxxxx"
              />
            </div>
            <Card className="bg-muted/50 py-4">
              <CardContent className="text-sm space-y-2">
                <p className="font-medium">DNS Configuration</p>
                <p className="text-muted-foreground">Point your domain to your server by adding an A record:</p>
                <code className="block bg-background px-3 py-2 rounded-md font-mono text-xs border">
                  {newFQDN || 'app.example.com'}  A  &rarr; your-server-ip
                </code>
              </CardContent>
            </Card>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleAdd} disabled={!newFQDN}>Add Domain</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Loading */}
      {loading && (
        <Card>
          <CardContent className="space-y-3 py-2">
            {[1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </CardContent>
        </Card>
      )}

      {/* Empty State */}
      {!loading && list.length === 0 && (
        <Card className="py-16">
          <CardContent className="flex flex-col items-center text-center">
            <Globe className="mb-4 text-muted-foreground" size={48} />
            <h2 className="text-lg font-medium mb-2">No domains configured</h2>
            <p className="text-muted-foreground max-w-sm">
              Add a custom domain to route traffic to your applications.
            </p>
          </CardContent>
        </Card>
      )}

      {/* Domain Table */}
      {!loading && list.length > 0 && (
        <Card className="py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>SSL Status</TableHead>
                <TableHead>DNS</TableHead>
                <TableHead>Added</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {list.map((d) => (
                <TableRow key={d.id}>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Globe size={16} className="text-primary shrink-0" />
                      <span className="font-medium">{d.fqdn}</span>
                      <button
                        onClick={() => handleCopyFQDN(d.id, d.fqdn)}
                        className="text-muted-foreground hover:text-foreground"
                      >
                        {copiedId === d.id ? <Check size={14} /> : <Copy size={14} />}
                      </button>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline">{d.type}</Badge>
                  </TableCell>
                  <TableCell>
                    {d.verified ? (
                      <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
                        <ShieldCheck size={12} /> Active
                      </Badge>
                    ) : (
                      <Badge variant="secondary" className="text-amber-600 dark:text-amber-400">
                        <ShieldAlert size={12} /> Pending
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    {d.dns_synced ? (
                      <span className="flex items-center gap-1 text-sm text-emerald-600 dark:text-emerald-400">
                        <CheckCircle size={14} /> Synced
                      </span>
                    ) : (
                      <span className="text-sm text-muted-foreground">{d.dns_provider || 'Manual'}</span>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {new Date(d.created_at).toLocaleDateString()}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      {!d.verified && (
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleVerify(d.id)}
                          title="Verify domain"
                        >
                          <CheckCircle size={16} />
                        </Button>
                      )}
                      {d.app_id && (
                        <a href={`/apps/${d.app_id}`}>
                          <Button variant="ghost" size="icon" title="Open application">
                            <ExternalLink size={16} />
                          </Button>
                        </a>
                      )}
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleDelete(d.id)}
                        className="text-muted-foreground hover:text-destructive"
                        title="Delete domain"
                      >
                        <Trash2 size={16} />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </div>
  );
}
