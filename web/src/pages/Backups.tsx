import { useState } from 'react';
import {
  Archive,
  Plus,
  Download,
  Clock,
  RotateCcw,
  HardDrive,
  Database,
  CheckCircle,
  Loader2,
  AlertTriangle,
  ShieldCheck,
} from 'lucide-react';
import { backupsAPI, type BackupEntry } from '@/api/backups';
import { useApi } from '@/hooks';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
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
// Helpers
// ---------------------------------------------------------------------------

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB';
  return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB';
}

function timeAgo(timestamp: number): string {
  const seconds = Math.floor((Date.now() - timestamp * 1000) / 1000);
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

const TYPE_CONFIG: Record<string, { icon: typeof HardDrive; label: string; color: string }> = {
  full:     { icon: HardDrive, label: 'Full',     color: 'text-blue-500' },
  database: { icon: Database,  label: 'Database', color: 'text-purple-500' },
  volume:   { icon: Archive,   label: 'Volume',   color: 'text-cyan-500' },
};

function TypeBadge({ type }: { type: string }) {
  const config = TYPE_CONFIG[type] || TYPE_CONFIG.full;
  const Icon = config.icon;
  return (
    <Badge variant="outline" className="gap-1.5 text-xs font-normal">
      <Icon className={cn('size-3', config.color)} />
      {config.label}
    </Badge>
  );
}

function StatusBadge({ status }: { status: string }) {
  switch (status) {
    case 'completed':
      return (
        <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400 gap-1.5">
          <CheckCircle className="size-3" />
          Completed
        </Badge>
      );
    case 'running':
    case 'in_progress':
      return (
        <Badge variant="outline" className="gap-1.5 text-amber-600 dark:text-amber-400 border-amber-500/20">
          <Loader2 className="size-3 animate-spin" />
          In Progress
        </Badge>
      );
    case 'failed':
      return (
        <Badge variant="destructive" className="gap-1.5">
          <AlertTriangle className="size-3" />
          Failed
        </Badge>
      );
    default:
      return <Badge variant="outline">{status}</Badge>;
  }
}

function TableSkeleton() {
  return (
    <Card>
      <CardContent className="space-y-3 py-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="flex items-center gap-4">
            <Skeleton className="h-4 w-48" />
            <Skeleton className="h-5 w-16 rounded-md" />
            <Skeleton className="h-4 w-16" />
            <Skeleton className="h-5 w-20 rounded-md" />
            <Skeleton className="h-4 w-20 ml-auto" />
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Backups
// ---------------------------------------------------------------------------

export function Backups() {
  const { data: backups, loading, refetch } = useApi<BackupEntry[]>('/backups');
  const [restoreDialog, setRestoreDialog] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [restoring, setRestoring] = useState(false);

  const handleCreate = async () => {
    setCreating(true);
    try {
      await backupsAPI.create({ source_type: 'full', source_id: 'all' });
      toast.success('Backup started');
      refetch();
    } catch {
      toast.error('Failed to create backup');
    } finally {
      setCreating(false);
    }
  };

  const handleRestore = async (key: string) => {
    setRestoring(true);
    try {
      await backupsAPI.restore(key);
      toast.success('Restore started');
      setRestoreDialog(null);
      refetch();
    } catch {
      toast.error('Failed to restore backup');
    } finally {
      setRestoring(false);
    }
  };

  const list = backups || [];
  const completedCount = list.filter((b) => b.status === 'completed').length;
  const totalSize = list.reduce((acc, b) => acc + (b.size || 0), 0);

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Archive className="size-5 text-primary" />
              <Badge variant="secondary" className="text-xs font-normal">
                {list.length} backup{list.length !== 1 ? 's' : ''}
                {completedCount > 0 && ` \u00b7 ${completedCount} completed`}
              </Badge>
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Backups
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base max-w-lg">
              Volume snapshots and database dumps. Automatic daily backups at 02:00 AM.
            </p>
          </div>
          <Button onClick={handleCreate} disabled={creating} className="shrink-0">
            {creating ? (
              <>
                <Loader2 className="size-4 animate-spin" />
                Creating...
              </>
            ) : (
              <>
                <Plus className="size-4" />
                Create Backup
              </>
            )}
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
                <CheckCircle className="size-5 text-emerald-500" />
              </div>
              <div>
                <p className="text-2xl font-bold tracking-tight tabular-nums">{completedCount}</p>
                <p className="text-xs text-muted-foreground">Completed</p>
              </div>
            </CardContent>
          </Card>
          <Card className="py-4 group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
            <CardContent className="flex items-center gap-4">
              <div className="flex items-center justify-center rounded-xl size-11 shrink-0 bg-blue-500/10">
                <HardDrive className="size-5 text-blue-500" />
              </div>
              <div>
                <p className="text-2xl font-bold tracking-tight tabular-nums">{formatSize(totalSize)}</p>
                <p className="text-xs text-muted-foreground">Total Size</p>
              </div>
            </CardContent>
          </Card>
          <Card className="py-4 group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
            <CardContent className="flex items-center gap-4">
              <div className="flex items-center justify-center rounded-xl size-11 shrink-0 bg-purple-500/10">
                <ShieldCheck className="size-5 text-purple-500" />
              </div>
              <div>
                <p className="text-2xl font-bold tracking-tight tabular-nums">02:00</p>
                <p className="text-xs text-muted-foreground">Daily Schedule</p>
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Restore Confirmation Dialog */}
      <Dialog open={restoreDialog !== null} onOpenChange={() => setRestoreDialog(null)}>
        <DialogContent onClose={() => setRestoreDialog(null)} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-3">
              <div className="flex items-center justify-center rounded-xl size-9 bg-amber-500/10">
                <RotateCcw className="size-4 text-amber-500" />
              </div>
              Restore Backup
            </DialogTitle>
            <DialogDescription>
              Are you sure you want to restore this backup? This will overwrite current data and cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-lg border bg-muted/30 px-4 py-3">
            <div className="flex items-center gap-2">
              <Archive className="size-4 text-muted-foreground shrink-0" />
              <code className="text-sm font-mono truncate">{restoreDialog}</code>
            </div>
          </div>
          <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2.5">
            <AlertTriangle className="size-4 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" />
            <p className="text-xs text-amber-700 dark:text-amber-300">
              All current data for the affected resources will be replaced with this backup snapshot.
            </p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestoreDialog(null)} disabled={restoring}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => restoreDialog && handleRestore(restoreDialog)}
              disabled={restoring}
            >
              {restoring ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Restoring...
                </>
              ) : (
                <>
                  <RotateCcw className="size-4" />
                  Restore Backup
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
            <Archive className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
            No backups yet
          </h2>
          <p className="text-muted-foreground max-w-sm text-sm mb-2">
            Backups run automatically on your configured schedule.
          </p>
          <p className="text-xs text-muted-foreground mb-6">
            Default: Daily at 02:00 AM
          </p>
          <Button onClick={handleCreate} disabled={creating}>
            <Plus className="size-4" />
            Create your first backup
          </Button>
        </div>
      )}

      {/* Backup Table */}
      {!loading && list.length > 0 && (
        <Card className="py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="hidden sm:table-cell">Created</TableHead>
                <TableHead className="w-[100px] text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {list.map((b) => (
                <TableRow key={b.key} className="group/row transition-colors hover:bg-muted/50">
                  <TableCell>
                    <div className="flex items-center gap-2.5">
                      <div className="flex items-center justify-center rounded-lg size-8 bg-muted shrink-0">
                        <Archive className="size-4 text-muted-foreground" />
                      </div>
                      <span className="font-mono text-sm font-medium truncate max-w-[200px]">
                        {b.key}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <TypeBadge type={b.type || 'full'} />
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-muted-foreground tabular-nums">
                      {formatSize(b.size)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={b.status || 'completed'} />
                  </TableCell>
                  <TableCell className="hidden sm:table-cell">
                    <span className="flex items-center gap-1.5 text-sm text-muted-foreground tabular-nums">
                      <Clock className="size-3" />
                      {timeAgo(b.created_at)}
                    </span>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Download backup"
                        className="size-8 opacity-0 group-hover/row:opacity-100 transition-opacity"
                      >
                        <Download className="size-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setRestoreDialog(b.key)}
                        title="Restore backup"
                        className="size-8 opacity-0 group-hover/row:opacity-100 transition-opacity"
                      >
                        <RotateCcw className="size-4" />
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
