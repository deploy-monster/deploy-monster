import { useState } from 'react';
import {
  Lock,
  Plus,
  Eye,
  EyeOff,
  Trash2,
  Copy,
  Check,
  ShieldCheck,
  Search,
  KeyRound,
  AlertCircle,
  Loader2,
} from 'lucide-react';
import { secretsAPI, type SecretEntry } from '@/api/secrets';
import { useApi } from '@/hooks';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
} from '@/components/ui/table';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from '@/stores/toastStore';

// ---------------------------------------------------------------------------
// Scope configuration with colors
// ---------------------------------------------------------------------------

const SCOPE_CONFIG: Record<string, { bgColor: string; textColor: string; label: string }> = {
  global:  { bgColor: 'bg-purple-500/10', textColor: 'text-purple-600 dark:text-purple-400 border-purple-500/20', label: 'Global' },
  tenant:  { bgColor: 'bg-blue-500/10',   textColor: 'text-blue-600 dark:text-blue-400 border-blue-500/20',     label: 'Tenant' },
  project: { bgColor: 'bg-emerald-500/10', textColor: 'text-emerald-600 dark:text-emerald-400 border-emerald-500/20', label: 'Project' },
  app:     { bgColor: 'bg-amber-500/10',   textColor: 'text-amber-600 dark:text-amber-400 border-amber-500/20',   label: 'App' },
};

function getScopeConfig(scope: string) {
  return SCOPE_CONFIG[scope] || SCOPE_CONFIG.tenant;
}

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

function ScopeBadge({ scope }: { scope: string }) {
  const config = getScopeConfig(scope);
  return (
    <Badge className={cn('gap-1.5 text-xs font-normal', config.bgColor, config.textColor)}>
      {config.label}
    </Badge>
  );
}

function TableSkeleton() {
  return (
    <Card>
      <CardContent className="space-y-3 py-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="flex items-center gap-4">
            <Skeleton className="size-5 rounded-full" />
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-5 w-16 rounded-md" />
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-4 w-20 ml-auto" />
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Secrets
// ---------------------------------------------------------------------------

export function Secrets() {
  const { data: secrets, loading, refetch } = useApi<SecretEntry[]>('/secrets');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [name, setName] = useState('');
  const [value, setValue] = useState('');
  const [scope, setScope] = useState('tenant');
  const [showValue, setShowValue] = useState(false);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState('');
  const [searchQuery, setSearchQuery] = useState('');

  const handleCreate = async () => {
    if (!name || !value) return;
    setCreating(true);
    setCreateError('');
    try {
      await secretsAPI.create({ name, value, scope });
      toast.success('Secret created successfully');
      setName('');
      setValue('');
      setScope('tenant');
      setShowValue(false);
      setDialogOpen(false);
      refetch();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create secret');
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this secret? This action cannot be undone.')) return;
    try {
      await secretsAPI.delete(id);
      toast.success('Secret deleted');
      refetch();
    } catch {
      toast.error('Failed to delete secret');
    }
  };

  const handleCopyRef = (secretName: string, id: string) => {
    navigator.clipboard.writeText(`\${SECRET:${secretName}}`);
    setCopiedId(id);
    toast.success('Reference copied to clipboard');
    setTimeout(() => setCopiedId(null), 2000);
  };

  const list = secrets || [];
  const filtered = searchQuery
    ? list.filter((s) =>
        s.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        s.scope.toLowerCase().includes(searchQuery.toLowerCase())
      )
    : list;

  const scopeCounts = list.reduce<Record<string, number>>((acc, s) => {
    acc[s.scope] = (acc[s.scope] || 0) + 1;
    return acc;
  }, {});

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <ShieldCheck className="size-5 text-primary" />
              <Badge variant="secondary" className="text-xs font-normal">
                {list.length} secret{list.length !== 1 ? 's' : ''}
                {' \u00b7 '}AES-256-GCM
              </Badge>
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Secrets
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base max-w-lg">
              Encrypted secret storage. Values are never returned by the API &mdash; only names and metadata.
            </p>
          </div>
          <Button onClick={() => setDialogOpen(true)} className="shrink-0">
            <Plus className="size-4" />
            Add Secret
          </Button>
        </div>
        {/* Decorative */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Scope Summary */}
      {!loading && list.length > 0 && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
          {Object.entries(SCOPE_CONFIG).map(([key, config]) => (
            <Card key={key} className="py-4 group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
              <CardContent className="flex items-center gap-4">
                <div className={cn('flex items-center justify-center rounded-xl size-11 shrink-0', config.bgColor)}>
                  <KeyRound className={cn('size-5', config.textColor.split(' ')[0])} />
                </div>
                <div>
                  <p className="text-2xl font-bold tracking-tight tabular-nums">{scopeCounts[key] || 0}</p>
                  <p className="text-xs text-muted-foreground">{config.label}</p>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Search */}
      {!loading && list.length > 0 && (
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
          <Input
            type="text"
            placeholder="Search secrets..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-9 max-w-sm"
          />
        </div>
      )}

      {/* Add Secret Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent onClose={() => setDialogOpen(false)} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-3">
              <div className="flex items-center justify-center rounded-xl size-9 bg-primary">
                <Lock className="size-4 text-primary-foreground" />
              </div>
              Create Secret
            </DialogTitle>
            <DialogDescription>
              Secrets are encrypted with AES-256-GCM. Values are never returned by the API.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="secret-name">Name</Label>
              <div className="relative">
                <KeyRound className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                <Input
                  id="secret-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="DB_PASSWORD"
                  className="pl-9 font-mono"
                />
              </div>
              <p className="text-[11px] text-muted-foreground">
                Uppercase letters, numbers, and underscores recommended.
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="secret-value">Value</Label>
              <div className="relative">
                <Lock className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                <Input
                  id="secret-value"
                  type={showValue ? 'text' : 'password'}
                  value={value}
                  onChange={(e) => setValue(e.target.value)}
                  placeholder="Enter secret value"
                  className="pl-9 pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowValue(!showValue)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                  tabIndex={-1}
                >
                  {showValue ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </button>
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="secret-scope">Scope</Label>
              <div className="grid grid-cols-4 gap-2">
                {Object.entries(SCOPE_CONFIG).map(([key, config]) => (
                  <button
                    key={key}
                    type="button"
                    onClick={() => setScope(key)}
                    className={cn(
                      'flex flex-col items-center gap-1 rounded-lg border p-2.5 transition-all duration-200 cursor-pointer',
                      scope === key
                        ? 'border-primary bg-primary/5 ring-1 ring-primary/20'
                        : 'hover:bg-accent hover:text-accent-foreground'
                    )}
                  >
                    <div className={cn('flex items-center justify-center rounded-lg size-7', config.bgColor)}>
                      <KeyRound className={cn('size-3.5', config.textColor.split(' ')[0])} />
                    </div>
                    <span className="text-[10px] font-medium">{config.label}</span>
                  </button>
                ))}
              </div>
            </div>
            <div className="rounded-lg border bg-muted/30 p-3">
              <p className="text-xs text-muted-foreground">
                Reference in env vars:{' '}
                <code className="rounded bg-background border px-1.5 py-0.5 font-mono text-[11px] text-foreground">
                  {'${'}SECRET:{name || 'NAME'}{'}'}
                </code>
              </p>
            </div>

            {createError && (
              <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2.5 text-sm text-destructive">
                <AlertCircle className="size-4 shrink-0" />
                {createError}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={creating}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={!name || !value || creating}>
              {creating ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Creating...
                </>
              ) : (
                <>
                  <Lock className="size-4" />
                  Create Secret
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
            <Lock className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
            Secret Vault
          </h2>
          <p className="text-muted-foreground max-w-sm text-sm mb-2">
            Secrets are encrypted with AES-256-GCM and stored securely.
          </p>
          <p className="text-xs text-muted-foreground mb-6">
            Values are never returned by the API &mdash; only names and metadata.
          </p>
          <Button onClick={() => setDialogOpen(true)}>
            <Plus className="size-4" />
            Add your first secret
          </Button>
        </div>
      )}

      {/* Secrets Table */}
      {!loading && filtered.length > 0 && (
        <Card className="py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Scope</TableHead>
                <TableHead>Value</TableHead>
                <TableHead className="hidden sm:table-cell">Updated</TableHead>
                <TableHead className="w-[100px] text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((s) => (
                <TableRow key={s.id} className="group/row transition-colors hover:bg-muted/50">
                  <TableCell>
                    <div className="flex items-center gap-2.5">
                      <div className={cn(
                        'flex items-center justify-center rounded-lg size-8 shrink-0',
                        getScopeConfig(s.scope).bgColor
                      )}>
                        <ShieldCheck className={cn('size-4', getScopeConfig(s.scope).textColor.split(' ')[0])} />
                      </div>
                      <span className="font-mono font-medium text-sm">{s.name}</span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <ScopeBadge scope={s.scope} />
                  </TableCell>
                  <TableCell>
                    <code className="text-muted-foreground text-xs font-mono tracking-wider">
                      ••••••••••••
                    </code>
                  </TableCell>
                  <TableCell className="hidden sm:table-cell">
                    <span className="text-sm text-muted-foreground tabular-nums">
                      {timeAgo(s.updated_at || s.created_at)}
                    </span>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleCopyRef(s.name, s.id)}
                        title="Copy reference"
                        className="size-8"
                      >
                        {copiedId === s.id ? (
                          <Check className="size-3.5 text-emerald-500" />
                        ) : (
                          <Copy className="size-3.5" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleDelete(s.id)}
                        className="size-8 text-muted-foreground hover:text-destructive transition-colors opacity-0 group-hover/row:opacity-100"
                        title="Delete secret"
                      >
                        <Trash2 className="size-3.5" />
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
      {!loading && list.length > 0 && filtered.length === 0 && searchQuery && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <div className="rounded-full bg-muted p-4 mb-3">
            <Search className="size-6 text-muted-foreground" />
          </div>
          <p className="text-sm font-medium text-foreground mb-1">No secrets found</p>
          <p className="text-xs text-muted-foreground">
            No secrets match &quot;{searchQuery}&quot;.
          </p>
        </div>
      )}
    </div>
  );
}
