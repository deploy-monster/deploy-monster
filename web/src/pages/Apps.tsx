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
  MoreVertical,
  GitBranch,
  Box,
  Clock,
} from 'lucide-react';
import { useApi } from '../hooks';
import { appsAPI, type App } from '../api/apps';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  running: 'default',
  stopped: 'secondary',
  deploying: 'outline',
  building: 'outline',
  failed: 'destructive',
  pending: 'secondary',
};

const FILTER_TABS = [
  { key: 'all', label: 'All' },
  { key: 'running', label: 'Running' },
  { key: 'stopped', label: 'Stopped' },
  { key: 'deploying', label: 'Deploying' },
] as const;

type FilterKey = typeof FILTER_TABS[number]['key'];

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

export function Apps() {
  const { data: appsResponse, refetch } = useApi<{ data: App[]; total: number }>(
    '/apps?page=1&per_page=50',
    { refreshInterval: 10000 }
  );
  const allApps = appsResponse?.data || [];
  const total = appsResponse?.total || 0;

  const [filter, setFilter] = useState<FilterKey>('all');
  const [search, setSearch] = useState('');
  const [menuOpen, setMenuOpen] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const filteredApps = allApps.filter((app) => {
    const matchesFilter = filter === 'all' || app.status === filter;
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

  const handleAction = async (appId: string, action: 'start' | 'stop' | 'restart' | 'delete') => {
    setMenuOpen(null);
    setActionLoading(appId);
    try {
      if (action === 'delete') {
        if (!confirm('Are you sure you want to delete this application? This action cannot be undone.')) {
          setActionLoading(null);
          return;
        }
        await appsAPI.delete(appId);
      } else {
        await appsAPI[action](appId);
      }
      refetch();
    } catch {
      // Error handled by API layer
    } finally {
      setActionLoading(null);
    }
  };

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Applications</h1>
          <p className="text-muted-foreground mt-1">
            {total} application{total !== 1 ? 's' : ''} deployed
          </p>
        </div>
        <div className="flex items-center gap-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
            <Input
              type="text"
              placeholder="Search applications..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 w-64"
            />
          </div>
          <Link to="/apps/new">
            <Button>
              <Plus className="size-4" />
              New Application
            </Button>
          </Link>
        </div>
      </div>

      {/* Filter Tabs */}
      <div className="flex items-center gap-1 border-b">
        {FILTER_TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setFilter(tab.key)}
            className={cn(
              'inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px cursor-pointer',
              filter === tab.key
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground hover:border-border'
            )}
          >
            {tab.label}
            <span
              className={cn(
                'inline-flex items-center justify-center rounded-full px-2 py-0.5 text-xs font-medium',
                filter === tab.key
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-muted text-muted-foreground'
              )}
            >
              {filterCounts[tab.key]}
            </span>
          </button>
        ))}
      </div>

      {/* App Grid or Empty State */}
      {filteredApps.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <div className="rounded-full bg-muted p-5 mb-5">
            <Box className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold text-foreground mb-2">
            {search || filter !== 'all' ? 'No matching applications' : 'No applications yet'}
          </h2>
          <p className="text-muted-foreground mb-6 max-w-sm">
            {search || filter !== 'all'
              ? 'Try adjusting your search or filter criteria.'
              : 'Deploy your first application to get started with DeployMonster.'}
          </p>
          {!search && filter === 'all' && (
            <Link to="/apps/new">
              <Button>
                <Rocket className="size-4" />
                Deploy Your First App
              </Button>
            </Link>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredApps.map((app) => (
            <Card
              key={app.id}
              className={cn(
                'group relative transition-all hover:shadow-md',
                actionLoading === app.id && 'opacity-60 pointer-events-none'
              )}
            >
              <CardHeader className="flex-row items-start justify-between space-y-0 pb-0 gap-6">
                <div className="flex-1 min-w-0">
                  <Link to={`/apps/${app.id}`}>
                    <CardTitle className="text-base truncate hover:text-primary transition-colors cursor-pointer">
                      {app.name}
                    </CardTitle>
                  </Link>
                  <div className="flex items-center gap-2 mt-2">
                    <Badge variant={STATUS_VARIANT[app.status] || 'secondary'}>
                      {app.status}
                    </Badge>
                    <span className="text-xs text-muted-foreground">{app.type}</span>
                  </div>
                </div>

                {/* Actions Menu */}
                <div className="relative">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8 opacity-0 group-hover:opacity-100 transition-opacity"
                    onClick={() => setMenuOpen(menuOpen === app.id ? null : app.id)}
                  >
                    <MoreVertical className="size-4" />
                  </Button>
                  {menuOpen === app.id && (
                    <>
                      <div
                        className="fixed inset-0 z-40"
                        onClick={() => setMenuOpen(null)}
                      />
                      <div className="absolute right-0 top-8 z-50 w-44 rounded-md border bg-popover p-1 shadow-md">
                        <button
                          onClick={() => handleAction(app.id, 'start')}
                          className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-accent transition-colors cursor-pointer"
                        >
                          <Play className="size-4" /> Start
                        </button>
                        <button
                          onClick={() => handleAction(app.id, 'stop')}
                          className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-accent transition-colors cursor-pointer"
                        >
                          <Square className="size-4" /> Stop
                        </button>
                        <button
                          onClick={() => handleAction(app.id, 'restart')}
                          className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-accent transition-colors cursor-pointer"
                        >
                          <RotateCcw className="size-4" /> Restart
                        </button>
                        <div className="my-1 h-px bg-border" />
                        <button
                          onClick={() => handleAction(app.id, 'delete')}
                          className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-destructive hover:bg-destructive/10 transition-colors cursor-pointer"
                        >
                          <Trash2 className="size-4" /> Delete
                        </button>
                      </div>
                    </>
                  )}
                </div>
              </CardHeader>

              <CardContent className="pt-0">
                <div className="flex items-center gap-3 text-xs text-muted-foreground">
                  <span className="inline-flex items-center gap-1">
                    <GitBranch className="size-3" />
                    {app.source_type}
                  </span>
                  {app.branch && (
                    <span className="truncate">{app.branch}</span>
                  )}
                </div>
              </CardContent>

              <CardFooter className="border-t pt-4 pb-0 justify-between text-xs text-muted-foreground">
                <span className="inline-flex items-center gap-1">
                  <Clock className="size-3" />
                  {timeAgo(app.updated_at)}
                </span>
                <Link to={`/apps/${app.id}`}>
                  <Button variant="ghost" size="sm" className="h-7 text-xs">
                    View Details
                  </Button>
                </Link>
              </CardFooter>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
