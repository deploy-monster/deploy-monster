import { useState, useMemo } from 'react';
import { Link } from 'react-router';
import { Rocket, Plus, Search, Box } from 'lucide-react';
import { useApi, useDebouncedValue } from '../hooks';
import { appsAPI, type App } from '../api/apps';
import { toast } from '@/stores/toastStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { AlertDialog } from '@/components/ui/alert-dialog';
import {
  FILTER_TABS,
  EMPTY_MESSAGES,
  type FilterKey,
  cn,
  AppCard,
  AppCardSkeleton,
} from '@/components/Apps';

type AppsResponse = App[] | { data: App[]; total?: number };

export function Apps() {
  const { data: appsResponse, loading, refetch } = useApi<AppsResponse>(
    '/apps?page=1&per_page=50',
    { refreshInterval: 10000 }
  );
  const allApps = useMemo(
    () => (Array.isArray(appsResponse) ? appsResponse : appsResponse?.data || []),
    [appsResponse]
  );
  const total = Array.isArray(appsResponse) ? allApps.length : appsResponse?.total || allApps.length;

  const [filter, setFilter] = useState<FilterKey>('all');
  const [search, setSearch] = useState('');
  const debouncedSearch = useDebouncedValue(search);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [deleteAppId, setDeleteAppId] = useState<string | null>(null);
  const pendingDeleteApp = allApps.find((a) => a.id === deleteAppId);

  const filteredApps = useMemo(() => allApps.filter((app) => {
    const matchesFilter =
      filter === 'all' ||
      app.status === filter ||
      (filter === 'deploying' && app.status === 'building');
    const matchesSearch =
      !debouncedSearch ||
      app.name.toLowerCase().includes(debouncedSearch.toLowerCase()) ||
      app.type.toLowerCase().includes(debouncedSearch.toLowerCase());
    return matchesFilter && matchesSearch;
  }), [allApps, filter, debouncedSearch]);

  const filterCounts = useMemo(() => ({
    all: allApps.length,
    running: allApps.filter((a) => a.status === 'running').length,
    stopped: allApps.filter((a) => a.status === 'stopped').length,
    deploying: allApps.filter((a) => a.status === 'deploying' || a.status === 'building').length,
  }), [allApps]);

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
        setDeleteAppId(appId);
        setActionLoading(null);
        return;
      }
      await appsAPI[action](appId);
      refetch();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Action failed');
    } finally {
      setActionLoading(null);
    }
  };

  const emptyKey = debouncedSearch ? 'search' : filter;
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

      {/* Empty state */}
      {appsResponse && filteredApps.length === 0 && (
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
      )}

      {/* App grid */}
      {!loading || appsResponse ? filteredApps.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredApps.map((app) => (
            <AppCard
              key={app.id}
              app={app}
              isLoading={actionLoading === app.id}
              onAction={handleAction}
            />
          ))}
        </div>
      ) : null}

      {/* Delete Confirmation Dialog */}
      <AlertDialog
        open={deleteAppId !== null}
        onOpenChange={(open) => !open && setDeleteAppId(null)}
        title="Delete Application"
        description={`Are you sure you want to delete "${pendingDeleteApp?.name}"? This action cannot be undone.`}
        confirmLabel="Delete"
        cancelLabel="Cancel"
        variant="destructive"
        onConfirm={async () => {
          if (!deleteAppId) return;
          setActionLoading(deleteAppId);
          try {
            await appsAPI.delete(deleteAppId);
            refetch();
          } catch {
            toast.error('Action failed');
          } finally {
            setActionLoading(null);
            setDeleteAppId(null);
          }
        }}
      />
    </div>
  );
}