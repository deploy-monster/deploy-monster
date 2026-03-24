import { useState } from 'react';
import {
  Archive, Plus, Download, Clock, RotateCcw, HardDrive, Database, CheckCircle, Loader2,
} from 'lucide-react';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
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
import { toast } from '@/components/Toast';

interface BackupEntry {
  key: string;
  size: number;
  type: string;
  status: string;
  created_at: number;
}

function formatSize(bytes: number) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB';
  return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB';
}

function TypeBadge({ type }: { type: string }) {
  switch (type) {
    case 'full':
      return <Badge variant="outline"><HardDrive size={12} /> Full</Badge>;
    case 'database':
      return <Badge variant="outline"><Database size={12} /> Database</Badge>;
    case 'volume':
      return <Badge variant="outline"><Archive size={12} /> Volume</Badge>;
    default:
      return <Badge variant="outline">{type}</Badge>;
  }
}

function StatusBadge({ status }: { status: string }) {
  switch (status) {
    case 'completed':
      return (
        <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
          <CheckCircle size={12} /> Completed
        </Badge>
      );
    case 'running':
      return (
        <Badge variant="secondary">
          <Loader2 size={12} className="animate-spin" /> Running
        </Badge>
      );
    case 'failed':
      return <Badge variant="destructive">Failed</Badge>;
    default:
      return <Badge variant="outline">{status}</Badge>;
  }
}

export function Backups() {
  const { data: backups, loading, refetch } = useApi<BackupEntry[]>('/backups');
  const [restoreDialog, setRestoreDialog] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const handleCreate = async () => {
    setCreating(true);
    try {
      await api.post('/backups', { source_type: 'full', source_id: 'all' });
      toast.success('Backup started');
      refetch();
    } catch {
      toast.error('Failed to create backup');
    } finally {
      setCreating(false);
    }
  };

  const handleRestore = async (key: string) => {
    try {
      await api.post(`/backups/${encodeURIComponent(key)}/restore`, {});
      toast.success('Restore started');
      setRestoreDialog(null);
      refetch();
    } catch {
      toast.error('Failed to restore backup');
    }
  };

  const list = backups || [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Backups</h1>
          <p className="text-sm text-muted-foreground mt-1">Volume snapshots and database dumps</p>
        </div>
        <Button onClick={handleCreate} disabled={creating}>
          {creating ? <Loader2 className="animate-spin" /> : <Plus />}
          Create Backup
        </Button>
      </div>

      {/* Restore Confirmation Dialog */}
      <Dialog open={restoreDialog !== null} onOpenChange={() => setRestoreDialog(null)}>
        <DialogContent onClose={() => setRestoreDialog(null)}>
          <DialogHeader>
            <DialogTitle>Restore Backup</DialogTitle>
            <DialogDescription>
              Are you sure you want to restore this backup? This will overwrite current data.
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-md border bg-muted/50 px-4 py-3">
            <code className="text-sm font-mono">{restoreDialog}</code>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestoreDialog(null)}>Cancel</Button>
            <Button variant="destructive" onClick={() => restoreDialog && handleRestore(restoreDialog)}>
              <RotateCcw size={14} /> Restore
            </Button>
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
            <Archive className="mb-4 text-muted-foreground" size={48} />
            <h2 className="text-lg font-medium mb-2">No backups yet</h2>
            <p className="text-muted-foreground max-w-sm mb-2">
              Backups run automatically at your configured schedule.
            </p>
            <p className="text-sm text-muted-foreground">Default: Daily at 02:00 AM</p>
          </CardContent>
        </Card>
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
                <TableHead>Created</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {list.map((b) => (
                <TableRow key={b.key}>
                  <TableCell className="font-mono text-sm font-medium">{b.key}</TableCell>
                  <TableCell>
                    <TypeBadge type={b.type || 'full'} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">{formatSize(b.size)}</TableCell>
                  <TableCell>
                    <StatusBadge status={b.status || 'completed'} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <span className="flex items-center gap-1.5">
                      <Clock size={14} />
                      {new Date(b.created_at * 1000).toLocaleString()}
                    </span>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Download backup"
                      >
                        <Download size={16} />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setRestoreDialog(b.key)}
                        title="Restore backup"
                      >
                        <RotateCcw size={16} />
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
