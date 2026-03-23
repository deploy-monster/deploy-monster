import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { Rocket, MoreVertical, Play, Square, RotateCcw, Trash2 } from 'lucide-react';
import { appsAPI, type App } from '../api/apps';

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    running: 'bg-status-running/10 text-status-running',
    stopped: 'bg-status-stopped/10 text-status-stopped',
    deploying: 'bg-status-deploying/10 text-status-deploying',
    building: 'bg-status-building/10 text-status-building',
    pending: 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400',
    failed: 'bg-red-100 text-red-600 dark:bg-red-900/20 dark:text-red-400',
  };

  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-md text-xs font-medium ${colors[status] || colors.pending}`}>
      {status}
    </span>
  );
}

export function Apps() {
  const [apps, setApps] = useState<App[]>([]);
  const [total, setTotal] = useState(0);
  const [page] = useState(1);
  const [menuOpen, setMenuOpen] = useState<string | null>(null);

  const loadApps = () => {
    appsAPI.list(page, 20).then((res) => {
      setApps(res.data || []);
      setTotal(res.total);
    }).catch(() => {});
  };

  useEffect(() => { loadApps(); }, [page]);

  const handleAction = async (appId: string, action: 'start' | 'stop' | 'restart' | 'delete') => {
    setMenuOpen(null);
    try {
      if (action === 'delete') {
        if (!confirm('Are you sure you want to delete this application?')) return;
        await appsAPI.delete(appId);
      } else {
        await appsAPI[action](appId);
      }
      loadApps();
    } catch {}
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Applications</h1>
          <p className="text-sm text-text-secondary mt-1">{total} application{total !== 1 ? 's' : ''}</p>
        </div>
        <button className="px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors">
          Deploy New App
        </button>
      </div>

      {apps.length === 0 ? (
        <div className="bg-surface border border-border rounded-xl px-5 py-16 text-center">
          <Rocket className="mx-auto mb-4 text-text-muted" size={48} />
          <h2 className="text-lg font-medium text-text-primary mb-2">No applications yet</h2>
          <p className="text-text-secondary mb-4">Deploy your first application to get started.</p>
          <button className="px-4 py-2 bg-monster-green text-white text-sm font-medium rounded-lg">
            Deploy New App
          </button>
        </div>
      ) : (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border bg-surface-secondary text-left">
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">Name</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">Type</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">Status</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">Source</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">Created</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider w-10"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {apps.map((app) => (
                <tr key={app.id} className="hover:bg-surface-secondary transition-colors">
                  <td className="px-5 py-3">
                    <Link to={`/apps/${app.id}`} className="font-medium text-text-primary hover:text-monster-green">
                      {app.name}
                    </Link>
                  </td>
                  <td className="px-5 py-3 text-sm text-text-secondary">{app.type}</td>
                  <td className="px-5 py-3"><StatusBadge status={app.status} /></td>
                  <td className="px-5 py-3 text-sm text-text-secondary">{app.source_type}</td>
                  <td className="px-5 py-3 text-sm text-text-muted">
                    {new Date(app.created_at).toLocaleDateString()}
                  </td>
                  <td className="px-5 py-3 relative">
                    <button
                      onClick={() => setMenuOpen(menuOpen === app.id ? null : app.id)}
                      className="p-1 rounded hover:bg-surface-tertiary transition-colors"
                    >
                      <MoreVertical size={16} className="text-text-muted" />
                    </button>
                    {menuOpen === app.id && (
                      <div className="absolute right-5 top-10 z-10 w-40 bg-surface border border-border rounded-lg shadow-lg py-1">
                        <button onClick={() => handleAction(app.id, 'start')} className="flex items-center gap-2 px-3 py-2 text-sm w-full hover:bg-surface-secondary text-text-primary">
                          <Play size={14} /> Start
                        </button>
                        <button onClick={() => handleAction(app.id, 'stop')} className="flex items-center gap-2 px-3 py-2 text-sm w-full hover:bg-surface-secondary text-text-primary">
                          <Square size={14} /> Stop
                        </button>
                        <button onClick={() => handleAction(app.id, 'restart')} className="flex items-center gap-2 px-3 py-2 text-sm w-full hover:bg-surface-secondary text-text-primary">
                          <RotateCcw size={14} /> Restart
                        </button>
                        <hr className="my-1 border-border" />
                        <button onClick={() => handleAction(app.id, 'delete')} className="flex items-center gap-2 px-3 py-2 text-sm w-full hover:bg-red-50 dark:hover:bg-red-900/20 text-red-600">
                          <Trash2 size={14} /> Delete
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
