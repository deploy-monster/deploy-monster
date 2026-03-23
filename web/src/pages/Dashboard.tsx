import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { Rocket, Database, Server, Activity } from 'lucide-react';
import { appsAPI, type App } from '../api/apps';

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
    pending: 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400',
    failed: 'bg-red-100 text-red-600 dark:bg-red-900/20 dark:text-red-400',
    crashed: 'bg-red-100 text-red-600 dark:bg-red-900/20 dark:text-red-400',
  };

  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-md text-xs font-medium ${colors[status] || colors.pending}`}>
      {status}
    </span>
  );
}

export function Dashboard() {
  const [apps, setApps] = useState<App[]>([]);
  const [total, setTotal] = useState(0);

  useEffect(() => {
    appsAPI.list(1, 5).then((res) => {
      setApps(res.data || []);
      setTotal(res.total);
    }).catch(() => {});
  }, []);

  const running = apps.filter((a) => a.status === 'running').length;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-text-primary">Dashboard</h1>
        <Link
          to="/apps"
          className="px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors"
        >
          Deploy New App
        </Link>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard icon={Rocket} label="Applications" value={total} color="bg-monster-green" />
        <StatCard icon={Activity} label="Running" value={running} color="bg-status-running" />
        <StatCard icon={Database} label="Databases" value={0} color="bg-monster-purple" />
        <StatCard icon={Server} label="Servers" value={1} color="bg-blue-500" />
      </div>

      {/* Recent Apps */}
      <div className="bg-surface border border-border rounded-xl">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-medium text-text-primary">Recent Applications</h2>
        </div>
        {apps.length === 0 ? (
          <div className="px-5 py-12 text-center text-text-muted">
            <Rocket className="mx-auto mb-3 text-text-muted" size={32} />
            <p>No applications yet</p>
            <Link to="/apps" className="text-monster-green hover:underline text-sm mt-1 inline-block">
              Deploy your first app
            </Link>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {apps.map((app) => (
              <Link
                key={app.id}
                to={`/apps/${app.id}`}
                className="flex items-center justify-between px-5 py-3 hover:bg-surface-secondary transition-colors"
              >
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
    </div>
  );
}
