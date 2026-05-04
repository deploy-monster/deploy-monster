import { useState, useMemo } from 'react';
import { useDebouncedValue } from '../hooks';
import {
  Database,
  Plus,
  Copy,
  Check,
  Loader2,
  AlertCircle,
  Search,
  HardDrive,
  Clock,
} from 'lucide-react';
import type { DatabaseInstance } from '@/api/databases';
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
  Sheet, SheetContent, SheetHeader, SheetFooter, SheetTitle, SheetDescription, SheetBody,
} from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from '@/stores/toastStore';

// ---------------------------------------------------------------------------
// Engine configuration with colors
// ---------------------------------------------------------------------------

interface EngineConfig {
  id: string;
  name: string;
  versions: string[];
  bgColor: string;
  iconColor: string;
  letter: string;
}

const engines: EngineConfig[] = [
  { id: 'postgres', name: 'PostgreSQL', versions: ['17', '16', '15'], bgColor: 'bg-blue-500/10', iconColor: 'text-blue-500', letter: 'PG' },
  { id: 'mysql', name: 'MySQL', versions: ['8.4', '8.0'], bgColor: 'bg-orange-500/10', iconColor: 'text-orange-500', letter: 'MY' },
  { id: 'mariadb', name: 'MariaDB', versions: ['11', '10.11'], bgColor: 'bg-sky-500/10', iconColor: 'text-sky-500', letter: 'MA' },
  { id: 'redis', name: 'Redis', versions: ['7'], bgColor: 'bg-red-500/10', iconColor: 'text-red-500', letter: 'RD' },
  { id: 'mongodb', name: 'MongoDB', versions: ['7'], bgColor: 'bg-emerald-500/10', iconColor: 'text-emerald-500', letter: 'MG' },
];

function getEngineConfig(engineId: string): EngineConfig {
  return engines.find((e) => e.id === engineId) || engines[0];
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

function formatSize(mb: number): string {
  if (mb < 1) return '< 1 MB';
  if (mb < 1024) return `${mb} MB`;
  return `${(mb / 1024).toFixed(1)} GB`;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function StatusBadge({ status }: { status: string }) {
  switch (status) {
    case 'running':
      return (
        <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400 gap-1.5">
          <span className="relative flex size-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
            <span className="relative inline-flex rounded-full size-2 bg-emerald-500" />
          </span>
          Running
        </Badge>
      );
    case 'stopped':
      return (
        <Badge variant="secondary" className="gap-1.5">
          <span className="size-2 rounded-full bg-muted-foreground" />
          Stopped
        </Badge>
      );
    case 'creating':
      return (
        <Badge variant="outline" className="gap-1.5 text-amber-600 dark:text-amber-400">
          <Loader2 className="size-3 animate-spin" />
          Creating
        </Badge>
      );
    default:
      return <Badge variant="outline">{status}</Badge>;
  }
}

function CardSkeleton() {
  return (
    <Card className="py-5">
      <CardHeader className="pb-0">
        <div className="flex items-start gap-3">
          <Skeleton className="size-11 rounded-xl shrink-0" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-5 w-28" />
            <Skeleton className="h-3.5 w-20" />
          </div>
          <Skeleton className="h-5 w-16 rounded-md" />
        </div>
      </CardHeader>
      <CardContent className="pt-0 mt-4">
        <Skeleton className="h-8 w-full rounded-md" />
      </CardContent>
      <CardFooter className="border-t pt-4 pb-0">
        <Skeleton className="h-3 w-16" />
        <Skeleton className="h-3 w-24 ml-auto" />
      </CardFooter>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Databases
// ---------------------------------------------------------------------------

export function Databases() {
  const { data: databases, loading, refetch } = useApi<DatabaseInstance[]>('/databases');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const [engine, setEngine] = useState('postgres');
  const [version, setVersion] = useState('');
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const debouncedSearch = useDebouncedValue(searchQuery);

  const selectedEngine = getEngineConfig(engine);

  const handleCreate = async () => {
    if (!newName || !engine) return;
    setCreating(true);
    setCreateError('');
    try {
      await api.post('/databases', {
        name: newName,
        engine,
        version: version || selectedEngine.versions[0],
      });
      toast.success('Database created successfully');
      setNewName('');
      setEngine('postgres');
      setVersion('');
      setDialogOpen(false);
      refetch();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create database');
    } finally {
      setCreating(false);
    }
  };

  const handleCopy = (id: string, connStr: string) => {
    navigator.clipboard.writeText(connStr);
    setCopiedId(id);
    toast.success('Connection string copied');
    setTimeout(() => setCopiedId(null), 2000);
  };

  const list = useMemo(() => databases || [], [databases]);
  const filtered = useMemo(() => debouncedSearch
    ? list.filter((db) =>
        db.name.toLowerCase().includes(debouncedSearch.toLowerCase()) ||
        db.engine.toLowerCase().includes(debouncedSearch.toLowerCase())
      )
    : list, [list, debouncedSearch]);

  const runningCount = useMemo(() => list.filter((db) => db.status === 'running').length, [list]);

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Database className="size-5 text-primary" />
              <Badge variant="secondary" className="text-xs font-normal">
                {list.length} database{list.length !== 1 ? 's' : ''}
                {runningCount > 0 && ` \u00b7 ${runningCount} running`}
              </Badge>
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Databases
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base max-w-lg">
              Managed database instances with automatic backups and monitoring.
            </p>
          </div>
          <Button onClick={() => setDialogOpen(true)} className="shrink-0">
            <Plus className="size-4" />
            Create Database
          </Button>
        </div>
        {/* Decorative */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Search */}
      {!loading && list.length > 0 && (
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
          <Input
            type="text"
            placeholder="Search databases..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-9 max-w-sm"
          />
        </div>
      )}

      {/* Create Database Sheet */}
      <Sheet open={dialogOpen} onOpenChange={(open) => !open && setDialogOpen(false)}>
        <SheetContent onClose={() => setDialogOpen(false)}>
          <SheetHeader>
            <SheetTitle className="flex items-center gap-3">
              <div className={cn(
                'flex items-center justify-center rounded-xl size-9',
                selectedEngine.bgColor
              )}>
                <Database className={cn('size-4', selectedEngine.iconColor)} />
              </div>
              Create Database
            </SheetTitle>
            <SheetDescription>
              Provision a new managed database instance with automatic backups.
            </SheetDescription>
          </SheetHeader>

          <SheetBody>
            <div className="space-y-4">
              {/* Engine Selection Cards */}
              <div className="space-y-1.5">
                <Label>Engine</Label>
                <div className="grid grid-cols-5 gap-2">
                  {engines.map((e) => (
                    <button
                      key={e.id}
                      type="button"
                      onClick={() => { setEngine(e.id); setVersion(''); }}
                      className={cn(
                        'flex flex-col items-center gap-1.5 rounded-lg border p-3 transition-all duration-200 cursor-pointer',
                        engine === e.id
                          ? 'border-primary bg-primary/5 ring-1 ring-primary/20'
                          : 'hover:bg-accent hover:text-accent-foreground'
                      )}
                    >
                      <div className={cn('flex items-center justify-center rounded-lg size-8', e.bgColor)}>
                        <span className={cn('text-xs font-bold', e.iconColor)}>{e.letter}</span>
                      </div>
                      <span className="text-[10px] font-medium truncate w-full text-center">{e.name}</span>
                    </button>
                  ))}
                </div>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="db-name">Database Name</Label>
                <div className="relative">
                  <Database className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                  <Input
                    id="db-name"
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                    placeholder="my-database"
                    className="pl-9"
                  />
                </div>
                <p className="text-[11px] text-muted-foreground">
                  Lowercase letters, numbers, and hyphens only.
                </p>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="db-version">Version</Label>
                <Select
                  id="db-version"
                  value={version || selectedEngine.versions[0]}
                  onChange={(e) => setVersion(e.target.value)}
                >
                  {selectedEngine.versions.map((v) => (
                    <option key={v} value={v}>
                      {selectedEngine.name} v{v}
                    </option>
                  ))}
                </Select>
              </div>

              {createError && (
                <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2.5 text-sm text-destructive">
                  <AlertCircle className="size-4 shrink-0" />
                  {createError}
                </div>
              )}
            </div>
          </SheetBody>

          <SheetFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={creating}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={!newName || creating}>
              {creating ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Creating...
                </>
              ) : (
                <>
                  <Plus className="size-4" />
                  Create
                </>
              )}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {/* Loading */}
      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      )}

      {/* Empty State */}
      {!loading && list.length === 0 && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="rounded-full bg-muted p-6 mb-5">
            <Database className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
            No databases yet
          </h2>
          <p className="text-muted-foreground max-w-sm text-sm mb-6">
            Create a managed database instance. Supports PostgreSQL, MySQL, MariaDB, Redis, and MongoDB.
          </p>
          <Button onClick={() => setDialogOpen(true)}>
            <Plus className="size-4" />
            Create your first database
          </Button>
        </div>
      )}

      {/* Database Grid */}
      {!loading && filtered.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filtered.map((db) => {
            const engineConfig = getEngineConfig(db.engine);

            return (
              <Card
                key={db.id}
                className="group relative transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg"
              >
                <CardHeader className="pb-0 gap-4">
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3 min-w-0">
                      <div className={cn(
                        'flex items-center justify-center rounded-xl size-11 shrink-0',
                        engineConfig.bgColor
                      )}>
                        <span className={cn('text-sm font-bold', engineConfig.iconColor)}>
                          {engineConfig.letter}
                        </span>
                      </div>
                      <div className="min-w-0">
                        <CardTitle className="text-base truncate">{db.name}</CardTitle>
                        <CardDescription className="mt-0.5">
                          {engineConfig.name} v{db.version}
                        </CardDescription>
                      </div>
                    </div>
                    <StatusBadge status={db.status} />
                  </div>
                </CardHeader>
                <CardContent className="pt-0">
                  {db.connection_string && (
                    <div className="space-y-1.5">
                      <Label className="text-xs text-muted-foreground">Connection String</Label>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 font-mono text-xs">
                          {db.connection_string}
                        </code>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="shrink-0 size-8"
                          onClick={() => handleCopy(db.id, db.connection_string)}
                          title="Copy connection string"
                          aria-label="Copy connection string"
                        >
                          {copiedId === db.id ? (
                            <Check className="size-3.5 text-emerald-500" />
                          ) : (
                            <Copy className="size-3.5" />
                          )}
                        </Button>
                      </div>
                    </div>
                  )}
                </CardContent>
                <CardFooter className="border-t pt-4 pb-0 justify-between items-center">
                  <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <HardDrive className="size-3" />
                    {db.size_mb ? formatSize(db.size_mb) : '--'}
                  </span>
                  <span className="flex items-center gap-1.5 text-xs text-muted-foreground tabular-nums">
                    <Clock className="size-3" />
                    {timeAgo(db.created_at)}
                  </span>
                </CardFooter>
              </Card>
            );
          })}
        </div>
      )}

      {/* No search results */}
      {!loading && list.length > 0 && filtered.length === 0 && debouncedSearch && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <div className="rounded-full bg-muted p-4 mb-3">
            <Search className="size-6 text-muted-foreground" />
          </div>
          <p className="text-sm font-medium text-foreground mb-1">No databases found</p>
          <p className="text-xs text-muted-foreground">
            No databases match &quot;{searchQuery}&quot;.
          </p>
        </div>
      )}
    </div>
  );
}
