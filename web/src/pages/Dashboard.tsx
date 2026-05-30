import { useState, useMemo } from 'react';
import { Link } from 'react-router';
import {
  Search,
  Plus,
  ArrowRight,
  Clock,
  Box,
  ExternalLink,
  CircleDot,
} from 'lucide-react';
import { useApi } from '../hooks';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
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
import {
  STAT_CARDS,
  ACTIVITY_COLORS,
  ACTIVITY_ICONS,
} from '@/components/Dashboard';
import {
  StatCardSkeleton,
  StatusBadge,
  QuickActions,
  AnnouncementsBanner,
  getGreeting,
  timeAgo,
} from '@/components/Dashboard';

interface Announcement {
  id: string;
  title: string;
  body: string;
  type: string;
}

function WelcomeBanner({ greeting }: { greeting: string }) {
  const [searchQuery, setSearchQuery] = useState('');

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (searchQuery.length >= 2) {
      // Navigate to search results
    }
  };

  return (
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
  );
}

function StatCardsGrid({
  stats,
  statsLoading,
}: {
  stats: DashboardStats | null;
  statsLoading: boolean;
}) {
  return (
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
  );
}

function RecentAppsTable({ apps }: { apps: App[] }) {
  return (
    <Card className="lg:col-span-2">
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div className="flex items-center gap-2">
          <CardTitle className="text-base">Recent Applications</CardTitle>
          {apps.length > 0 && (
            <span className="inline-flex items-center justify-center rounded-full bg-muted px-2 py-0.5 text-[10px] font-medium">
              {apps.length}
            </span>
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
  );
}

function ActivityFeed({ activity }: { activity: ActivityEntry[] }) {
  return (
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
  );
}

export function Dashboard() {
  const { data: stats, loading: statsLoading } = useApi<DashboardStats>('/dashboard/stats', { refreshInterval: 30000 });
  const { data: appsData } = useApi<App[] | { data: App[] }>('/apps?page=1&per_page=5');
  const { data: activityData } = useApi<ActivityEntry[] | { data: ActivityEntry[] }>('/activity?limit=10');
  const { data: announcementsData } = useApi<Announcement[] | { data: Announcement[] }>('/announcements');

  const apps = Array.isArray(appsData) ? appsData : appsData?.data || [];
  const activity = Array.isArray(activityData) ? activityData : activityData?.data || [];
  const announcements = Array.isArray(announcementsData) ? announcementsData : announcementsData?.data || [];

  const greeting = useMemo(() => getGreeting(), []);

  return (
    <div className="space-y-8">
      <AnnouncementsBanner announcements={announcements} />

      <WelcomeBanner greeting={greeting} />

      <StatCardsGrid stats={stats ?? null} statsLoading={statsLoading} />

      <QuickActions />

      {/* Main Content: Applications + Activity */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <RecentAppsTable apps={apps} />
        <ActivityFeed activity={activity} />
      </div>
    </div>
  );
}