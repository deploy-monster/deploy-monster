import { useState } from 'react';
import { Link } from 'react-router';
import {
  Rocket,
  Database,
  Server,
  Activity,
  Globe,
  Search,
  Bell,
  Plus,
  ArrowRight,
  Clock,
  Box,
} from 'lucide-react';
import { useApi } from '../hooks';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { App } from '../api/apps';
import type { DashboardStats, ActivityEntry } from '../api/dashboard';

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  running: 'default',
  stopped: 'secondary',
  deploying: 'outline',
  building: 'outline',
  failed: 'destructive',
  pending: 'secondary',
};

const STAT_CARDS = [
  { key: 'apps', icon: Rocket, label: 'Applications', color: 'bg-emerald-500/10 text-emerald-500' },
  { key: 'running', icon: Activity, label: 'Running', color: 'bg-green-500/10 text-green-500' },
  { key: 'containers', icon: Server, label: 'Containers', color: 'bg-blue-500/10 text-blue-500' },
  { key: 'domains', icon: Globe, label: 'Domains', color: 'bg-violet-500/10 text-violet-500' },
  { key: 'projects', icon: Database, label: 'Projects', color: 'bg-amber-500/10 text-amber-500' },
] as const;

function getStatValue(key: typeof STAT_CARDS[number]['key'], stats: DashboardStats | null): number {
  if (!stats) return 0;
  switch (key) {
    case 'apps': return stats.apps.total;
    case 'running': return stats.containers.running;
    case 'containers': return stats.containers.total;
    case 'domains': return stats.domains;
    case 'projects': return stats.projects;
  }
}

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

export function Dashboard() {
  const { data: stats, loading: statsLoading } = useApi<DashboardStats>('/dashboard/stats', { refreshInterval: 30000 });
  const { data: appsData } = useApi<{ data: App[] }>('/apps?page=1&per_page=5');
  const { data: activityData } = useApi<{ data: ActivityEntry[] }>('/activity?limit=8');
  const { data: announcementsData } = useApi<{ data: Array<{ id: string; title: string; body: string; type: string }> }>('/announcements');
  const [searchQuery, setSearchQuery] = useState('');

  const apps = appsData?.data || [];
  const activity = activityData?.data || [];
  const announcements = announcementsData?.data || [];

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (searchQuery.length >= 2) {
      // Navigate to search results
    }
  };

  return (
    <div className="space-y-8">
      {/* Announcements banner */}
      {announcements.length > 0 && (
        <div className="bg-primary/5 border border-primary/20 rounded-lg p-4 flex items-start gap-3">
          <div className="rounded-full bg-primary/10 p-1.5 mt-0.5">
            <Bell className="size-4 text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-foreground">{announcements[0].title}</p>
            {announcements[0].body && (
              <p className="text-sm text-muted-foreground mt-0.5">{announcements[0].body}</p>
            )}
          </div>
        </div>
      )}

      {/* Page Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
          <p className="text-muted-foreground mt-1">Overview of your platform and applications.</p>
        </div>
        <div className="flex items-center gap-3">
          <form onSubmit={handleSearch} className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
            <Input
              type="text"
              placeholder="Search apps, domains..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 w-64"
            />
          </form>
          <Link to="/apps/new">
            <Button>
              <Plus className="size-4" />
              Deploy New App
            </Button>
          </Link>
        </div>
      </div>

      {/* Stat Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        {STAT_CARDS.map(({ key, icon: Icon, label, color }) => (
          <Card key={key} className="py-4">
            <CardContent className="flex items-center gap-4">
              <div className={cn('flex items-center justify-center rounded-lg size-10', color)}>
                <Icon className="size-5" />
              </div>
              <div>
                <p className={cn(
                  'text-2xl font-bold tracking-tight',
                  statsLoading && 'animate-pulse text-muted-foreground'
                )}>
                  {getStatValue(key, stats)}
                </p>
                <p className="text-xs text-muted-foreground">{label}</p>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Main Content Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Recent Applications */}
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <CardTitle className="text-base">Recent Applications</CardTitle>
            <Link to="/apps">
              <Button variant="ghost" size="sm">
                View all
                <ArrowRight className="size-4" />
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            {apps.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <div className="rounded-full bg-muted p-4 mb-4">
                  <Box className="size-8 text-muted-foreground" />
                </div>
                <h3 className="font-medium text-foreground mb-1">No applications yet</h3>
                <p className="text-sm text-muted-foreground mb-4">
                  Deploy your first application to get started.
                </p>
                <Link to="/apps/new">
                  <Button size="sm">
                    <Plus className="size-4" />
                    Deploy your first app
                  </Button>
                </Link>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead className="hidden sm:table-cell">Source</TableHead>
                    <TableHead className="text-right">Last Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {apps.map((app) => (
                    <TableRow key={app.id}>
                      <TableCell>
                        <Link
                          to={`/apps/${app.id}`}
                          className="font-medium text-foreground hover:text-primary transition-colors"
                        >
                          {app.name}
                        </Link>
                        <p className="text-xs text-muted-foreground">{app.type}</p>
                      </TableCell>
                      <TableCell>
                        <Badge variant={STATUS_VARIANT[app.status] || 'secondary'}>
                          {app.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden sm:table-cell text-muted-foreground">
                        {app.source_type}
                      </TableCell>
                      <TableCell className="text-right text-muted-foreground">
                        {timeAgo(app.updated_at)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Recent Activity */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Recent Activity</CardTitle>
          </CardHeader>
          <CardContent>
            {activity.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-8 text-center">
                <div className="rounded-full bg-muted p-3 mb-3">
                  <Clock className="size-5 text-muted-foreground" />
                </div>
                <p className="text-sm text-muted-foreground">No recent activity</p>
              </div>
            ) : (
              <div className="space-y-0">
                {activity.map((entry, index) => (
                  <div
                    key={entry.id}
                    className={cn(
                      'flex gap-3 py-3',
                      index !== activity.length - 1 && 'border-b'
                    )}
                  >
                    <div className="relative flex flex-col items-center">
                      <div className="flex size-2 rounded-full bg-primary mt-1.5" />
                      {index !== activity.length - 1 && (
                        <div className="flex-1 w-px bg-border mt-1" />
                      )}
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm text-foreground">
                        <span className="font-medium capitalize">{entry.action}</span>{' '}
                        <span className="text-muted-foreground">{entry.resource_type}</span>
                      </p>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        {timeAgo(entry.created_at)}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
