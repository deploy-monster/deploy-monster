import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { Rocket, Database, Server, Activity, Globe, Search, Bell } from 'lucide-react';
import { appsAPI, type App } from '../api/apps';
import { dashboardAPI, type DashboardStats, type ActivityEntry } from '../api/dashboard';

function StatCard({ icon: Icon, label, value, color }: {
  icon: React.ElementType;
  label: string;
  value: string | number;
  color: string;
}) {
  return (
    <div className="bg-surface border border-border rounded-xl p-5">
      <div className="flex items-center gap-3">
        <div className={`w-10 h-10 rounded-lg ${color} flex items-center justify-center text-white`}>
          <Icon size={20} />
        </div>
        <div>
          <p className="text-2xl font-semibold text-text-primary">{value}</p>
          <p className="text-sm text-text-secondary">{label}</p>
        </div>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    running: 'bg-status-running/10 text-status-running',
    stopped: 'bg-status-stopped/10 text-status-stopped',
    deploying: 'bg-status-deploying/10 text-status-deploying',
    building: 'bg-status-building/10 text-status-building',
    suspended: 'bg-yellow-100 text-yellow-600 dark:bg-yellow-900/20 dark:text-yellow-400',
    pending: 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400',
    failed: 'bg-red-100 text-red-600 dark:bg-red-900/20 dark:text-red-400',
  };
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-md text-xs font-medium ${colors[status] || colors.pending}`}>
      {status}
    </span>
  );
}

export function Dashboard() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [apps, setApps] = useState<App[]>([]);
  const [activity, setActivity] = useState<ActivityEntry[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [announcements, setAnnouncements] = useState<any[]>([]);

  useEffect(() => {
    dashboardAPI.stats().then(setStats).catch(() => {});
    appsAPI.list(1, 5).then((r) => setApps(r.data || [])).catch(() => {});
    dashboardAPI.activity(5).then((r) => setActivity(r.data || [])).catch(() => {});
    dashboardAPI.announcements().then((r) => setAnnouncements(r.data || [])).catch(() => {});
  }, []);

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (searchQuery.length >= 2) {
      // Navigate to search or show results inline
    }
  };

  return (
    <div className="space-y-6">
      {/* Announcements banner */}
      {announcements.length > 0 && (
        <div className="bg-monster-purple/10 border border-monster-purple/30 rounded-xl p-4">
          <div className="flex items-center gap-2">
            <Bell size={16} className="text-monster-purple" />
            <span className="text-sm font-medium text-monster-purple">{announcements[0].title}</span>
          </div>
          {announcements[0].body && (
            <p className="text-sm text-text-secondary mt-1">{announcements[0].body}</p>
          )}
        </div>
      )}

      {/* Header with search */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-text-primary">Dashboard</h1>
        <div className="flex items-center gap-3">
          <form onSubmit={handleSearch} className="relative">
            <Search size={16} className="absolute left-3 top-2.5 text-text-muted" />
            <input type="text" placeholder="Search apps, domains..." value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 pr-3 py-2 w-64 rounded-lg border border-border bg-surface text-text-primary text-sm focus:ring-2 focus:ring-monster-green/50" />
          </form>
          <Link to="/apps/new"
            className="px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors">
            Deploy New App
          </Link>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <StatCard icon={Rocket} label="Applications" value={stats?.apps.total || 0} color="bg-monster-green" />
        <StatCard icon={Activity} label="Running" value={stats?.containers.running || 0} color="bg-status-running" />
        <StatCard icon={Server} label="Containers" value={stats?.containers.total || 0} color="bg-blue-500" />
        <StatCard icon={Globe} label="Domains" value={stats?.domains || 0} color="bg-monster-purple" />
        <StatCard icon={Database} label="Projects" value={stats?.projects || 0} color="bg-amber-500" />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Recent Apps */}
        <div className="lg:col-span-2 bg-surface border border-border rounded-xl">
          <div className="px-5 py-4 border-b border-border flex items-center justify-between">
            <h2 className="font-medium text-text-primary">Recent Applications</h2>
            <Link to="/apps" className="text-sm text-monster-green hover:underline">View all</Link>
          </div>
          {apps.length === 0 ? (
            <div className="px-5 py-12 text-center text-text-muted">
              <Rocket className="mx-auto mb-3 text-text-muted" size={32} />
              <p>No applications yet</p>
              <Link to="/apps/new" className="text-monster-green hover:underline text-sm mt-1 inline-block">
                Deploy your first app
              </Link>
            </div>
          ) : (
            <div className="divide-y divide-border">
              {apps.map((app) => (
                <Link key={app.id} to={`/apps/${app.id}`}
                  className="flex items-center justify-between px-5 py-3 hover:bg-surface-secondary transition-colors">
                  <div>
                    <p className="font-medium text-text-primary">{app.name}</p>
                    <p className="text-sm text-text-secondary">{app.source_type} / {app.type}</p>
                  </div>
                  <StatusBadge status={app.status} />
                </Link>
              ))}
            </div>
          )}
        </div>

        {/* Activity Feed */}
        <div className="bg-surface border border-border rounded-xl">
          <div className="px-5 py-4 border-b border-border">
            <h2 className="font-medium text-text-primary">Activity</h2>
          </div>
          {activity.length === 0 ? (
            <div className="px-5 py-8 text-center text-text-muted text-sm">No recent activity</div>
          ) : (
            <div className="divide-y divide-border">
              {activity.map((entry) => (
                <div key={entry.id} className="px-5 py-3">
                  <p className="text-sm text-text-primary">
                    <span className="font-medium">{entry.action}</span> {entry.resource_type}
                  </p>
                  <p className="text-xs text-text-muted mt-0.5">
                    {new Date(entry.created_at).toLocaleString()}
                  </p>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
