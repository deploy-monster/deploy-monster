import { useState, useMemo } from 'react';
import { useDebouncedValue } from '../hooks';
import {
  Globe,
  Plus,
  ShieldCheck,
  ShieldAlert,
  Trash2,
  CheckCircle,
  ExternalLink,
  Copy,
  Check,
  Loader2,
  AlertCircle,
  Search,
} from 'lucide-react';
import type { Domain } from '@/api/domains';
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
import { toast } from '@/stores/toastStore';
import { AlertDialog } from '@/components/ui/alert-dialog';

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

function TableSkeleton() {
  return (
    <Card>
      <CardContent className="space-y-3 py-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="flex items-center gap-4">
            <Skeleton className="size-5 rounded-full" />
            <Skeleton className="h-4 w-48" />
            <Skeleton className="h-5 w-16 rounded-md" />
            <Skeleton className="h-5 w-20 rounded-md" />
            <Skeleton className="h-4 w-24 ml-auto" />
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Domains
// ---------------------------------------------------------------------------

export function Domains() {
  const { data: domains, loading, refetch } = useApi<Domain[]>('/domains');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [newFQDN, setNewFQDN] = useState('');
  const [newAppID, setNewAppID] = useState('');
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState('');
  const [verifyingId, setVerifyingId] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const debouncedSearch = useDebouncedValue(searchQuery);
  const [deleteDomainId, setDeleteDomainId] = useState<string | null>(null);
  // Defensive: ensure domains is always an array (handles API response edge cases)
  const domainList: Domain[] = Array.isArray(domains) ? domains : [];
  const pendingDeleteDomain = domainList.find((d) => d.id === deleteDomainId);

  const handleAdd = async () => {
    if (!newFQDN) return;
    setAdding(true);
    setAddError('');
    try {
      await api.post('/domains', { fqdn: newFQDN, app_id: newAppID });
      toast.success('Domain added successfully');
      setNewFQDN('');
      setNewAppID('');
      setDialogOpen(false);
      refetch();
    } catch (err) {
      setAddError(err instanceof Error ? err.message : 'Failed to add domain');
    } finally {
      setAdding(false);
    }
  };

  const handleVerify = async (id: string) => {
    setVerifyingId(id);
    try {
      await api.post(`/domains/${id}/verify`, {});
      toast.success('Domain verified');
      refetch();
    } catch {
      toast.error('Verification failed');
    } finally {
      setVerifyingId(null);
    }
  };

  const handleDelete = async (id: string) => {
    setDeleteDomainId(id);
  };

  const handleCopyFQDN = (id: string, fqdn: string) => {
    navigator.clipboard.writeText(fqdn);
    setCopiedId(id);
    toast.success('Domain copied to clipboard');
    setTimeout(() => setCopiedId(null), 2000);
  };

  const list = domainList;
  const filtered = useMemo(() => debouncedSearch
    ? list.filter((d) => d.fqdn.toLowerCase().includes(debouncedSearch.toLowerCase()))
    : list, [list, debouncedSearch]);

  const sslActive = useMemo(() => list.filter((d) => d.verified).length, [list]);
  const sslPending = useMemo(() => list.filter((d) => !d.verified).length, [list]);

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Globe className="size-5 text-primary" />
              <Badge variant="secondary" className="text-xs font-normal">
                {list.length} domain{list.length !== 1 ? 's' : ''}
                {sslActive > 0 && ` \u00b7 ${sslActive} secured`}
              </Badge>
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Domains
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base max-w-lg">
              Manage custom domains and SSL certificates for your applications.
            </p>
          </div>
          <Button onClick={() => setDialogOpen(true)} className="shrink-0">
            <Plus className="size-4" />
            Add Domain
          </Button>
        </div>
        {/* Decorative */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Summary Cards */}
      {!loading && list.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <Card className="py-4 group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
            <CardContent className="flex items-center gap-4">
              <div className="flex items-center justify-center rounded-xl size-11 shrink-0 bg-emerald-500/10">
                <Globe className="size-5 text-emerald-500" />
              </div>
              <div>
                <p className="text-2xl font-bold tracking-tight tabular-nums">{list.length}</p>
                <p className="text-xs text-muted-foreground">Total Domains</p>
              </div>
            </CardContent>
          </Card>
          <Card className="py-4 group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
            <CardContent className="flex items-center gap-4">
              <div className="flex items-center justify-center rounded-xl size-11 shrink-0 bg-emerald-500/10">
                <ShieldCheck className="size-5 text-emerald-500" />
              </div>
              <div>
                <p className="text-2xl font-bold tracking-tight tabular-nums">{sslActive}</p>
                <p className="text-xs text-muted-foreground">SSL Active</p>
              </div>
            </CardContent>
          </Card>
          <Card className="py-4 group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
            <CardContent className="flex items-center gap-4">
              <div className="flex items-center justify-center rounded-xl size-11 shrink-0 bg-amber-500/10">
                <ShieldAlert className="size-5 text-amber-500" />
              </div>
              <div>
                <p className="text-2xl font-bold tracking-tight tabular-nums">{sslPending}</p>
                <p className="text-xs text-muted-foreground">Pending Verification</p>
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Search */}
      {!loading && list.length > 0 && (
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
          <Input
            type="text"
            placeholder="Search domains..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-9 max-w-sm"
          />
        </div>
      )}

      {/* Add Domain Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent onClose={() => setDialogOpen(false)} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-3">
              <div className="flex items-center justify-center rounded-xl size-9 bg-primary">
                <Globe className="size-4 text-primary-foreground" />
              </div>
              Add Custom Domain
            </DialogTitle>
            <DialogDescription>
              Point your domain to your server by adding an A record before verifying.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="fqdn">Domain (FQDN)</Label>
              <div className="relative">
                <Globe className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                <Input
                  id="fqdn"
                  value={newFQDN}
                  onChange={(e) => setNewFQDN(e.target.value)}
                  placeholder="app.example.com"
                  className="pl-9"
                />
              </div>
              <p className="text-[11px] text-muted-foreground">
                Fully qualified domain name, e.g. app.example.com
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="app-id">Application ID (optional)</Label>
              <Input
                id="app-id"
                value={newAppID}
                onChange={(e) => setNewAppID(e.target.value)}
                placeholder="app_xxxxxxxx"
                className="font-mono"
              />
            </div>
            <div className="rounded-lg border bg-muted/30 p-4 space-y-2">
              <p className="text-sm font-medium text-foreground">DNS Configuration</p>
              <p className="text-xs text-muted-foreground">
                Point your domain to your server by adding an A record:
              </p>
              <code className="block bg-background px-3 py-2 rounded-md font-mono text-xs border">
                {newFQDN || 'app.example.com'}  A  &rarr; your-server-ip
              </code>
            </div>

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
            <Button onClick={handleAdd} disabled={!newFQDN || adding}>
              {adding ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Adding...
                </>
              ) : (
                <>
                  <Plus className="size-4" />
                  Add Domain
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Loading */}
      {loading && <TableSkeleton />}

      {/* Empty State */}
      {!loading && list.length === 0 && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="rounded-full bg-muted p-6 mb-5">
            <Globe className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
            No domains configured
          </h2>
          <p className="text-muted-foreground max-w-sm text-sm mb-6">
            Add a custom domain to route traffic to your applications with automatic SSL certificates.
          </p>
          <Button onClick={() => setDialogOpen(true)}>
            <Plus className="size-4" />
            Add your first domain
          </Button>
        </div>
      )}

      {/* Domain Table */}
      {!loading && filtered.length > 0 && (
        <Card className="py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>SSL Status</TableHead>
                <TableHead className="hidden md:table-cell">DNS</TableHead>
                <TableHead className="hidden sm:table-cell">Added</TableHead>
                <TableHead className="w-[120px] text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((d) => (
                <TableRow key={d.id} className="group/row transition-colors hover:bg-muted/50">
                  <TableCell>
                    <div className="flex items-center gap-2.5">
                      <div className="flex items-center justify-center rounded-lg size-8 bg-primary/10 shrink-0">
                        <Globe className="size-4 text-primary" />
                      </div>
                      <div className="min-w-0">
                        <span className="font-medium text-foreground">{d.fqdn}</span>
                        {d.app_id && (
                          <p className="text-[11px] text-muted-foreground font-mono truncate mt-0.5">
                            {d.app_id}
                          </p>
                        )}
                      </div>
                      <button
                        onClick={() => handleCopyFQDN(d.id, d.fqdn)}
                        className="text-muted-foreground hover:text-foreground transition-colors opacity-0 group-hover/row:opacity-100 cursor-pointer"
                        title="Copy domain"
                      >
                        {copiedId === d.id ? (
                          <Check className="size-3.5 text-emerald-500" />
                        ) : (
                          <Copy className="size-3.5" />
                        )}
                      </button>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" className="text-xs font-normal">{d.type}</Badge>
                  </TableCell>
                  <TableCell>
                    {d.verified ? (
                      <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400 gap-1.5">
                        <ShieldCheck className="size-3" />
                        Active
                      </Badge>
                    ) : (
                      <Badge variant="secondary" className="text-amber-600 dark:text-amber-400 gap-1.5">
                        <ShieldAlert className="size-3" />
                        Pending
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    {d.dns_synced ? (
                      <span className="flex items-center gap-1.5 text-sm text-emerald-600 dark:text-emerald-400">
                        <CheckCircle className="size-3.5" />
                        Synced
                      </span>
                    ) : (
                      <span className="text-sm text-muted-foreground">
                        {d.dns_provider || 'Manual'}
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="hidden sm:table-cell">
                    <span className="text-sm text-muted-foreground tabular-nums">
                      {timeAgo(d.created_at)}
                    </span>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      {!d.verified && (
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleVerify(d.id)}
                          title="Verify domain"
                          disabled={verifyingId === d.id}
                          className="size-8"
                        >
                          {verifyingId === d.id ? (
                            <Loader2 className="size-4 animate-spin" />
                          ) : (
                            <CheckCircle className="size-4 text-emerald-600 dark:text-emerald-400" />
                          )}
                        </Button>
                      )}
                      {d.app_id && (
                        <a href={`/apps/${d.app_id}`}>
                          <Button variant="ghost" size="icon" title="Open application" className="size-8">
                            <ExternalLink className="size-4" />
                          </Button>
                        </a>
                      )}
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleDelete(d.id)}
                        className="size-8 text-muted-foreground hover:text-destructive transition-colors"
                        title="Delete domain"
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* No search results */}
      {!loading && list.length > 0 && filtered.length === 0 && debouncedSearch && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <div className="rounded-full bg-muted p-4 mb-3">
            <Search className="size-6 text-muted-foreground" />
          </div>
          <p className="text-sm font-medium text-foreground mb-1">No domains found</p>
          <p className="text-xs text-muted-foreground">
            No domains match &quot;{searchQuery}&quot;. Try a different search term.
          </p>
        </div>
      )}

      {/* Delete Confirmation Dialog */}
      <AlertDialog
        open={deleteDomainId !== null}
        onOpenChange={(open) => !open && setDeleteDomainId(null)}
        title="Remove Domain"
        description={`Remove "${pendingDeleteDomain?.fqdn}"? This action cannot be undone.`}
        confirmLabel="Remove"
        cancelLabel="Cancel"
        variant="destructive"
        onConfirm={async () => {
          if (!deleteDomainId) return;
          try {
            await api.delete(`/domains/${deleteDomainId}`);
            toast.success('Domain removed');
            refetch();
          } catch {
            toast.error('Failed to remove domain');
          } finally {
            setDeleteDomainId(null);
          }
        }}
      />
    </div>
  );
}
