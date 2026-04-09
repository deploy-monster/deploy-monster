import { useState } from 'react';
import { Link } from 'react-router';
import {
  Rocket,
  Plus,
  Search,
  Play,
  Square,
  RotateCcw,
  Trash2,
  GitBranch,
  Clock,
  Box,
  Container,
  Store,
} from 'lucide-react';
import { useApi } from '../hooks';
import { appsAPI, type App } from '../api/apps';
import { toast } from '@/stores/toastStore';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip } from '@/components/ui/tooltip';

/* ------------------------------------------------------------------ */
/*  Constants                                                         */
/* ------------------------------------------------------------------ */

const STATUS_CONFIG: Record<string, {
  variant: 'default' | 'secondary' | 'destructive' | 'outline';
  dot: string;
  label: string;
}> = {
  running: { variant: 'default', dot: 'bg-emerald-500', label: 'Running' },
  stopped: { variant: 'secondary', dot: 'bg-red-500', label: 'Stopped' },
  deploying: { variant: 'outline', dot: 'bg-amber-500', label: 'Deploying' },
  building: { variant: 'outline', dot: 'bg-amber-500', label: 'Building' },
  failed: { variant: 'destructive', dot: 'bg-red-500', label: 'Failed' },
  pending: { variant: 'secondary', dot: 'bg-slate-400', label: 'Pending' },
  created: { variant: 'secondary', dot: 'bg-slate-400', label: 'Created' },
};

const SOURCE_CONFIG: Record<string, { icon: typeof GitBranch; label: string; color: string }> = {
  git: { icon: GitBranch, label: 'Git', color: 'bg-orange-500/10 text-orange-600 dark:text-orange-400' },
  docker: { icon: Container, label: 'Docker', color: 'bg-blue-500/10 text-blue-600 dark:text-blue-400' },
  marketplace: { icon: Store, label: 'Marketplace', color: 'bg-violet-500/10 text-violet-600 dark:text-violet-400' },
};

const FILTER_TABS = [
  { key: 'all', label: 'All' },
  { key: 'running', label: 'Running' },
  { key: 'stopped', label: 'Stopped' },
  { key: 'deploying', label: 'Deploying' },
] as const;

type FilterKey = (typeof FILTER_TABS)[number]['key'];

/* ------------------------------------------------------------------ */
/*  Helpers                                                           */
/* ------------------------------------------------------------------ */

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function getStatusConfig(status: string) {
  return STATUS_CONFIG[status] || { variant: 'secondary' as const, dot: 'bg-slate-400', label: status };
}

function getSourceConfig(sourceType: string) {
  const key = sourceType.toLowerCase();
  return SOURCE_CONFIG[key] || SOURCE_CONFIG['git'];
}

/* ------------------------------------------------------------------ */
/*  Empty states                                                      */
/* ------------------------------------------------------------------ */

const EMPTY_MESSAGES: Record<string, { title: string; description: string }> = {
  all: {
    title: 'No applications yet',
    description: 'Deploy your first application to get started with DeployMonster.',
  },
  running: {
    title: 'No running applications',
    description: 'Start an application or deploy a new one to see it here.',
  },
  stopped: {
    title: 'No stopped applications',
    description: 'All your applications are currently running. Great job!',
  },
  deploying: {
    title: 'No active deployments',
    description: 'Trigger a new deployment to see it appear here.',
  },
  search: {
    title: 'No matching applications',
    description: 'Try adjusting your search or filter criteria.',
  },
};

/* ------------------------------------------------------------------ */
/*  Skeleton                                                          */
/* ------------------------------------------------------------------ */

function AppCardSkeleton() {
  return (
    <Card className="py-5">
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 flex-1">
            <Skeleton className="h-2.5 w-2.5 rounded-full shrink-0" />
            <Skeleton className="h-5 w-32" />
          </div>
          <Skeleton className="h-5 w-14 rounded-md" />
        </div>
      </CardHeader>
      <CardContent className="pb-3 space-y-3">
        <Skeleton className="h-5 w-16 rounded-md" />
        <div className="space-y-1.5">
          <Skeleton className="h-3.5 w-28" />
          <Skeleton className="h-3.5 w-36" />
          <Skeleton className="h-3.5 w-24" />
        </div>
      </CardContent>
    </Card>
  );
}

/* ------------------------------------------------------------------ */
/*  Component                                                         */
/* ------------------------------------------------------------------ */

export function Apps() {
  const { data: appsResponse, loading, refetch } = useApi<{ data: App[]; total: number }>(
    '/apps?page=1&per_page=50',
    { refreshInterval: 10000 }
  );
  const allApps = appsResponse?.data || [];
  const total = appsResponse?.total || 0;

  const [filter, setFilter] = useState<FilterKey>('all');
  const [search, setSearch] = useState('');
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const filteredApps = allApps.filter((app) => {
    const matchesFilter =
      filter === 'all' ||
      app.status === filter ||
      (filter === 'deploying' && app.status === 'building');
    const matchesSearch =
      !search ||
      app.name.toLowerCase().includes(search.toLowerCase()) ||
      app.type.toLowerCase().includes(search.toLowerCase());
    return matchesFilter && matchesSearch;
  });

  const filterCounts = {
    all: allApps.length,
    running: allApps.filter((a) => a.status === 'running').length,
    stopped: allApps.filter((a) => a.status === 'stopped').length,
    deploying: allApps.filter((a) => a.status === 'deploying' || a.status === 'building').length,
  };

  const handleAction = async (
    e: React.MouseEvent,
    appId: string,
    action: 'start' | 'stop' | 'restart' | 'delete'
  ) => {
    e.preventDefault();
    e.stopPropagation();
    setActionLoading(appId);
    try {
      if (action === 'delete') {
        if (
          !confirm(
            'Are you sure you want to delete this application? This action cannot be undone.'
          )
        ) {
          setActionLoading(null);
          return;
        }
        await appsAPI.delete(appId);
      } else {
        await appsAPI[action](appId);
      }
      refetch();
    } catch {
      toast.error('Action failed');
    } finally {
      setActionLoading(null);
    }
  };

  const emptyKey = search ? 'search' : filter;
  const emptyMsg = EMPTY_MESSAGES[emptyKey] || EMPTY_MESSAGES['all'];

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Applications</h1>
          <p className="text-muted-foreground mt-1">
            {total} application{total !== 1 ? 's' : ''} deployed
          </p>
        </div>
        <Link to="/apps/new">
          <Button className="cursor-pointer">
            <Plus className="size-4" />
            New Application
          </Button>
        </Link>
      </div>

      {/* Filter bar + Search */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        {/* Segmented filter group */}
        <div className="inline-flex items-center gap-1 rounded-lg bg-muted p-1">
          {FILTER_TABS.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setFilter(tab.key)}
              className={cn(
                'inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-all cursor-pointer',
                filter === tab.key
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-muted-foreground/10 hover:text-foreground'
              )}
            >
              {tab.label}
              <span
                className={cn(
                  'inline-flex items-center justify-center rounded-full min-w-5 h-5 px-1.5 text-xs font-semibold',
                  filter === tab.key
                    ? 'bg-primary-foreground/20 text-primary-foreground'
                    : 'bg-muted-foreground/15 text-muted-foreground'
                )}
              >
                {filterCounts[tab.key]}
              </span>
            </button>
          ))}
        </div>

        {/* Search */}
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
          <Input
            type="text"
            placeholder="Search applications..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 w-full sm:w-72"
          />
        </div>
      </div>

      {/* Loading skeleton */}
      {loading && !appsResponse && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <AppCardSkeleton key={i} />
          ))}
        </div>
      )}

      {/* App Grid or Empty State */}
      {appsResponse && filteredApps.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="rounded-full bg-muted p-5 mb-5">
            <Box className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold text-foreground mb-2">{emptyMsg.title}</h2>
          <p className="text-muted-foreground mb-6 max-w-sm">{emptyMsg.description}</p>
          {!search && filter === 'all' && (
            <Link to="/apps/new">
              <Button className="cursor-pointer">
                <Rocket className="size-4" />
                Deploy Your First App
              </Button>
            </Link>
          )}
        </div>
      ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            {filteredApps.map((app) => {
              const statusCfg = getStatusConfig(app.status);
              const sourceCfg = getSourceConfig(app.source_type);
              const SourceIcon = sourceCfg.icon;
              const isLoading = actionLoading === app.id;

              return (
                <Link key={app.id} to={`/apps/${app.id}`} className="block group">
                  <Card
                    className={cn(
                      'relative transition-all duration-200 h-full',
                      'hover:ring-1 hover:ring-primary/20 hover:shadow-md hover:-translate-y-px',
                      isLoading && 'opacity-60 pointer-events-none'
                    )}
                  >
                    <CardHeader className="pb-3">
                      <div className="flex items-start justify-between gap-3">
                        <div className="flex items-center gap-3 min-w-0 flex-1">
                          {/* Status dot */}
                          <span className="relative flex h-2.5 w-2.5 shrink-0">
                            {app.status === 'running' && (
                              <span
                                className={cn(
                                  'absolute inline-flex h-full w-full rounded-full opacity-75 animate-ping',
                                  statusCfg.dot
                                )}
                              />
                            )}
                            <span
                              className={cn(
                                'relative inline-flex rounded-full h-2.5 w-2.5',
                                statusCfg.dot
                              )}
                            />
                          </span>
                          <CardTitle className="text-base truncate">{app.name}</CardTitle>
                        </div>
                        {/* Source badge */}
                        <span
                          className={cn(
                            'inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium shrink-0',
                            sourceCfg.color
                          )}
                        >
                          <SourceIcon className="size-3" />
                          {sourceCfg.label}
                        </span>
                      </div>
                    </CardHeader>

                    <CardContent className="pb-3 space-y-3">
                      {/* Status badge */}
                      <Badge variant={statusCfg.variant} className="text-xs">
                        {statusCfg.label}
                      </Badge>

                      {/* Meta info */}
                      <div className="space-y-1.5 text-xs text-muted-foreground">
                        <div className="flex items-center gap-1.5">
                          <Clock className="size-3 shrink-0" />
                          <span>Last deploy: {timeAgo(app.updated_at)}</span>
                        </div>
                        {app.branch && (
                          <div className="flex items-center gap-1.5">
                            <GitBranch className="size-3 shrink-0" />
                            <span className="truncate">{app.branch}</span>
                          </div>
                        )}
                        <div className="flex items-center gap-1.5">
                          <Container className="size-3 shrink-0" />
                          <span className="font-mono text-[11px] truncate">
                            {app.id.slice(0, 12)}
                          </span>
                        </div>
                      </div>
                    </CardContent>

                    {/* Action row */}
                    <div className="border-t px-4 py-2.5 flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                      {app.status === 'running' ? (
                        <Tooltip content="Stop">
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-7 cursor-pointer"
                              onClick={(e) => handleAction(e, app.id, 'stop')}
                            >
                              <Square className="size-3.5" />
                            </Button>
                        </Tooltip>
                      ) : (
                        <Tooltip content="Start">
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-7 cursor-pointer"
                              onClick={(e) => handleAction(e, app.id, 'start')}
                            >
                              <Play className="size-3.5" />
                            </Button>
                        </Tooltip>
                      )}
                      <Tooltip content="Restart">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7 cursor-pointer"
                            onClick={(e) => handleAction(e, app.id, 'restart')}
                          >
                            <RotateCcw className="size-3.5" />
                          </Button>
                      </Tooltip>
                      <Tooltip content="Delete">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7 text-destructive hover:text-destructive cursor-pointer"
                            onClick={(e) => handleAction(e, app.id, 'delete')}
                          >
                            <Trash2 className="size-3.5" />
                          </Button>
                      </Tooltip>
                    </div>
                  </Card>
                </Link>
              );
            })}
          </div>
      )}
    </div>
  );
}
