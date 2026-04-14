import { useState, useMemo } from 'react';
import { Link } from 'react-router';
import {
  Rocket,
  Database,
  Activity,
  Globe,
  Search,
  Bell,
  Plus,
  ArrowRight,
  Clock,
  Box,
  GitBranch,
  Container,
  TrendingUp,
  Store,
  ExternalLink,
  CircleDot,
  Zap,
  AlertTriangle,
  Info,
  CheckCircle2,
  XCircle,
} from 'lucide-react';
import { useApi } from '../hooks';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STATUS_CONFIG: Record<string, { variant: 'default' | 'secondary' | 'destructive' | 'outline'; dotColor: string; label: string }> = {
  running:   { variant: 'default',     dotColor: 'bg-emerald-500', label: 'Running' },
  stopped:   { variant: 'secondary',   dotColor: 'bg-muted-foreground', label: 'Stopped' },
  deploying: { variant: 'outline',     dotColor: 'bg-amber-500',   label: 'Deploying' },
  building:  { variant: 'outline',     dotColor: 'bg-amber-500',   label: 'Building' },
  failed:    { variant: 'destructive', dotColor: 'bg-destructive', label: 'Failed' },
  pending:   { variant: 'secondary',   dotColor: 'bg-muted-foreground', label: 'Pending' },
};

const ACTIVITY_COLORS: Record<string, string> = {
  deploy:  'bg-emerald-500',
  create:  'bg-blue-500',
  start:   'bg-emerald-500',
  stop:    'bg-amber-500',
  delete:  'bg-destructive',
  restart: 'bg-cyan-500',
  error:   'bg-destructive',
  update:  'bg-blue-500',
  scale:   'bg-purple-500',
};

const ACTIVITY_ICONS: Record<string, typeof Rocket> = {
  deploy:  Rocket,
  create:  Plus,
  start:   CheckCircle2,
  stop:    XCircle,
  delete:  AlertTriangle,
  restart: Zap,
  error:   AlertTriangle,
  update:  Info,
  scale:   TrendingUp,
};

function getGreeting(): string {
  const hour = new Date().getHours();
  if (hour < 12) return 'Good morning';
  if (hour < 18) return 'Good afternoon';
  return 'Good evening';
}

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
// Stat card config
// ---------------------------------------------------------------------------

interface StatCardDef {
  key: string;
  icon: typeof Rocket;
  label: string;
  bgColor: string;
  iconColor: string;
  getValue: (s: DashboardStats) => number;
  getTrend: (s: DashboardStats) => { value: string; positive: boolean } | null;
}

const STAT_CARDS: StatCardDef[] = [
  {
    key: 'apps',
    icon: Rocket,
    label: 'Applications',
    bgColor: 'bg-emerald-500/10',
    iconColor: 'text-emerald-500',
    getValue: (s) => s.apps.total,
    getTrend: () => null,
  },
  {
    key: 'running',
    icon: Activity,
    label: 'Running',
    bgColor: 'bg-blue-500/10',
    iconColor: 'text-blue-500',
    getValue: (s) => s.containers.running,
    getTrend: (s) => {
      if (s.containers.total === 0) return null;
      const pct = Math.round((s.containers.running / s.containers.total) * 100);
      return { value: `${pct}% uptime`, positive: pct >= 80 };
    },
  },
  {
    key: 'containers',
    icon: Container,
    label: 'Containers',
    bgColor: 'bg-purple-500/10',
    iconColor: 'text-purple-500',
    getValue: (s) => s.containers.total,
    getTrend: (s) => (s.containers.stopped > 0 ? { value: `${s.containers.stopped} stopped`, positive: false } : null),
  },
  {
    key: 'domains',
    icon: Globe,
    label: 'Domains',
    bgColor: 'bg-amber-500/10',
    iconColor: 'text-amber-500',
    getValue: (s) => s.domains,
    getTrend: () => null,
  },
  {
    key: 'projects',
    icon: Database,
    label: 'Projects',
    bgColor: 'bg-cyan-500/10',
    iconColor: 'text-cyan-500',
    getValue: (s) => s.projects,
    getTrend: () => null,
  },
];

// ---------------------------------------------------------------------------
// Quick actions
// ---------------------------------------------------------------------------

const QUICK_ACTIONS = [
  {
    icon: GitBranch,
    title: 'Deploy from Git',
    description: 'Connect a GitHub, GitLab, or Gitea repository and deploy automatically.',
    href: '/apps/new?source=git',
    color: 'text-emerald-500',
    bgColor: 'bg-emerald-500/10',
  },
  {
    icon: Container,
    title: 'Deploy Docker Image',
    description: 'Run any Docker image from GHCR, or a private registry.',
    href: '/apps/new?source=docker',
    color: 'text-blue-500',
    bgColor: 'bg-blue-500/10',
  },
  {
    icon: Store,
    title: 'Browse Marketplace',
    description: 'One-click deploy popular databases, tools, and applications.',
    href: '/marketplace',
    color: 'text-purple-500',
    bgColor: 'bg-purple-500/10',
  },
];

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function StatCardSkeleton() {
  return (
    <Card className="py-4">
      <CardContent className="flex items-center gap-4">
        <Skeleton className="size-11 rounded-xl" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-7 w-12" />
          <Skeleton className="h-3 w-20" />
        </div>
      </CardContent>
    </Card>
  );
}

function StatusBadge({ status }: { status: string }) {
  const config = STATUS_CONFIG[status] || STATUS_CONFIG.pending;
  return (
    <Badge variant={config.variant} className="gap-1.5">
      <span className={cn('size-1.5 rounded-full', config.dotColor)} />
      {config.label}
    </Badge>
  );
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

export function Dashboard() {
  const { data: stats, loading: statsLoading } = useApi<DashboardStats>('/dashboard/stats', { refreshInterval: 30000 });
  const { data: appsData } = useApi<{ data: App[] }>('/apps?page=1&per_page=5');
  const { data: activityData } = useApi<{ data: ActivityEntry[] }>('/activity?limit=10');
  const { data: announcementsData } = useApi<{ data: Array<{ id: string; title: string; body: string; type: string }> }>('/announcements');
  const [searchQuery, setSearchQuery] = useState('');

  const apps = appsData?.data || [];
  const activity = activityData?.data || [];
  const announcements = announcementsData?.data || [];

  const greeting = useMemo(() => getGreeting(), []);

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (searchQuery.length >= 2) {
      // Navigate to search results
    }
  };

  return (
    <div data-testid="dashboard-shell" className="space-y-8">
      {/* Announcement Banner */}
      {announcements.length > 0 && (
        <div className="rounded-lg border border-primary/20 bg-primary/5 p-4 flex items-start gap-3">
          <div className="rounded-full bg-primary/10 p-1.5 mt-0.5 shrink-0">
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

      {/* Welcome Banner */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              {greeting}, admin
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base">
              Here is what is happening across your platform today.
            </p>
          </div>
          <div className="flex items-center gap-3">
            <form onSubmit={handleSearch} className="relative hidden sm:block">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
              <Input
                type="text"
                placeholder="Search apps, domains..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9 w-64 bg-background/80 backdrop-blur-sm"
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
        {/* Decorative gradient circles */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Mobile Search */}
      <form onSubmit={handleSearch} className="relative sm:hidden">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
        <Input
          type="text"
          placeholder="Search apps, domains..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="pl-9"
        />
      </form>

      {/* Stat Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4">
        {statsLoading
          ? Array.from({ length: 5 }).map((_, i) => <StatCardSkeleton key={i} />)
          : STAT_CARDS.map(({ key, icon: Icon, label, bgColor, iconColor, getValue, getTrend }) => {
              const value = stats ? getValue(stats) : 0;
              const trend = stats ? getTrend(stats) : null;
              return (
                <Card
                  key={key}
                  className="py-4 group transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg"
                >
                  <CardContent className="flex items-center gap-4">
                    <div className={cn('flex items-center justify-center rounded-xl size-11 shrink-0', bgColor)}>
                      <Icon className={cn('size-5', iconColor)} />
                    </div>
                    <div className="min-w-0">
                      <p className="text-2xl font-bold tracking-tight tabular-nums text-foreground">
                        {value}
                      </p>
                      <p className="text-xs text-muted-foreground truncate">{label}</p>
                      {trend ? (
                        <p className={cn(
                          'text-[10px] font-medium mt-0.5',
                          trend.positive ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'
                        )}>
                          {trend.positive ? '\u2191' : '\u2193'} {trend.value}
                        </p>
                      ) : (
                        <p className="text-[10px] text-muted-foreground/60 mt-0.5">&mdash; No change</p>
                      )}
                    </div>
                  </CardContent>
                </Card>
              );
            })}
      </div>

      {/* Quick Actions */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        {QUICK_ACTIONS.map((action) => (
          <Link key={action.href} to={action.href} className="group">
            <Card className="py-5 h-full transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg hover:border-primary/20">
              <CardContent className="flex items-start gap-4">
                <div className={cn('flex items-center justify-center rounded-xl size-11 shrink-0', action.bgColor)}>
                  <action.icon className={cn('size-5', action.color)} />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <h3 className="font-semibold text-sm text-foreground group-hover:text-primary transition-colors">
                      {action.title}
                    </h3>
                    <ArrowRight className="size-3.5 text-muted-foreground opacity-0 -translate-x-1 group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-200" />
                  </div>
                  <p className="text-xs text-muted-foreground mt-1 leading-relaxed">
                    {action.description}
                  </p>
                </div>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>

      {/* Main Content: Applications + Activity */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Recent Applications */}
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div className="flex items-center gap-2">
              <CardTitle className="text-base">Recent Applications</CardTitle>
              {apps.length > 0 && (
                <Badge variant="secondary" className="text-[10px] font-normal">
                  {apps.length}
                </Badge>
              )}
            </div>
            <Link to="/apps">
              <Button variant="ghost" size="sm" className="text-muted-foreground hover:text-foreground">
                View all
                <ArrowRight className="size-3.5" />
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            {apps.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-16 text-center">
                <div className="rounded-full bg-muted p-5 mb-4">
                  <Box className="size-8 text-muted-foreground" />
                </div>
                <h3 className="font-semibold text-foreground mb-1">No applications yet</h3>
                <p className="text-sm text-muted-foreground mb-6 max-w-xs">
                  Deploy your first application to see it here. Choose from Git, Docker, or the marketplace.
                </p>
                <Link to="/apps/new">
                  <Button>
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
                    <TableHead className="hidden md:table-cell">Source</TableHead>
                    <TableHead className="hidden sm:table-cell">Last Deploy</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {apps.map((app) => (
                    <TableRow key={app.id} className="group/row">
                      <TableCell>
                        <Link
                          to={`/apps/${app.id}`}
                          className="font-medium text-foreground hover:text-primary transition-colors"
                        >
                          {app.name}
                        </Link>
                        <p className="text-xs text-muted-foreground mt-0.5">{app.type}</p>
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={app.status} />
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        <span className="text-sm text-muted-foreground">{app.source_type}</span>
                      </TableCell>
                      <TableCell className="hidden sm:table-cell">
                        <span className="text-sm text-muted-foreground tabular-nums">
                          {timeAgo(app.updated_at)}
                        </span>
                      </TableCell>
                      <TableCell className="text-right">
                        <Link to={`/apps/${app.id}`}>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="opacity-0 group-hover/row:opacity-100 transition-opacity"
                          >
                            <ExternalLink className="size-3.5" />
                            View
                          </Button>
                        </Link>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Activity Feed */}
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <CardTitle className="text-base">Activity</CardTitle>
            <Link to="/activity">
              <Button variant="ghost" size="sm" className="text-muted-foreground hover:text-foreground">
                View all
                <ArrowRight className="size-3.5" />
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            {activity.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <div className="rounded-full bg-muted p-4 mb-3">
                  <Clock className="size-6 text-muted-foreground" />
                </div>
                <p className="text-sm font-medium text-foreground mb-1">No recent activity</p>
                <p className="text-xs text-muted-foreground">Events will appear here as you deploy and manage applications.</p>
              </div>
            ) : (
              <div className="relative">
                {/* Timeline line */}
                <div className="absolute left-[9px] top-2 bottom-2 w-px bg-border" />

                <div className="space-y-0">
                  {activity.map((entry, index) => {
                    const dotColor = ACTIVITY_COLORS[entry.action] || 'bg-muted-foreground';
                    const ActivityIcon = ACTIVITY_ICONS[entry.action] || CircleDot;
                    return (
                      <div
                        key={entry.id}
                        className={cn(
                          'relative flex gap-3 py-3 pl-0',
                          index !== activity.length - 1 && 'border-b border-transparent'
                        )}
                      >
                        {/* Timeline dot */}
                        <div className="relative z-10 flex items-center justify-center shrink-0">
                          <div className={cn(
                            'flex items-center justify-center size-[18px] rounded-full ring-2 ring-background',
                            dotColor
                          )}>
                            <ActivityIcon className="size-2.5 text-white" />
                          </div>
                        </div>

                        {/* Content */}
                        <div className="flex-1 min-w-0 -mt-0.5">
                          <p className="text-sm text-foreground leading-snug">
                            <span className="font-medium capitalize">{entry.action}</span>{' '}
                            <span className="text-muted-foreground">{entry.resource_type}</span>
                          </p>
                          {entry.resource_id && (
                            <p className="text-xs text-muted-foreground/80 mt-0.5 truncate font-mono">
                              {entry.resource_id}
                            </p>
                          )}
                          <p className="text-[11px] text-muted-foreground/60 mt-1 tabular-nums">
                            {timeAgo(entry.created_at)}
                          </p>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
