import { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router';
import { ArrowLeft, Play, Square, RotateCcw, GitBranch, Clock } from 'lucide-react';
import { appsAPI, type App } from '../api/apps';
import { api } from '../api/client';

interface Deployment {
  id: string;
  version: number;
  image: string;
  status: string;
  commit_sha: string;
  triggered_by: string;
  created_at: string;
}

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
    <span className={`inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium ${colors[status] || colors.pending}`}>
      {status}
    </span>
  );
}

export function AppDetail() {
  const { id } = useParams();
  const [app, setApp] = useState<App | null>(null);
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [tab, setTab] = useState<'overview' | 'deployments' | 'env' | 'logs' | 'metrics' | 'settings'>('overview');
  const [logs, setLogs] = useState<string[]>([]);

  useEffect(() => {
    if (!id) return;
    appsAPI.get(id).then(setApp).catch(() => {});
    api.get<{ data: Deployment[] }>(`/apps/${id}/deployments`).then((r) => setDeployments(r.data || [])).catch(() => {});
  }, [id]);

  const handleAction = async (action: 'start' | 'stop' | 'restart') => {
    if (!id) return;
    await appsAPI[action](id);
    const updated = await appsAPI.get(id);
    setApp(updated);
  };

  useEffect(() => {
    if (tab !== 'logs' || !id) return;
    const eventSource = new EventSource(`/api/v1/apps/${id}/logs/stream`);
    eventSource.onmessage = (e) => {
      setLogs((prev) => [...prev.slice(-500), e.data]);
    };
    return () => eventSource.close();
  }, [tab, id]);

  if (!app) {
    return <div className="flex items-center justify-center h-64"><div className="w-8 h-8 border-2 border-monster-green border-t-transparent rounded-full animate-spin" /></div>;
  }

  const tabs = [
    { key: 'overview', label: 'Overview' },
    { key: 'deployments', label: 'Deployments' },
    { key: 'env', label: 'Environment' },
    { key: 'logs', label: 'Logs' },
    { key: 'metrics', label: 'Metrics' },
    { key: 'settings', label: 'Settings' },
  ] as const;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Link to="/apps" className="text-text-muted hover:text-text-primary"><ArrowLeft size={20} /></Link>
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold text-text-primary">{app.name}</h1>
            <StatusBadge status={app.status} />
          </div>
          <p className="text-sm text-text-secondary mt-1">{app.source_type} / {app.type}</p>
        </div>
        <div className="flex gap-2">
          <button onClick={() => handleAction('start')} className="p-2 rounded-lg border border-border hover:bg-surface-secondary" title="Start"><Play size={16} /></button>
          <button onClick={() => handleAction('stop')} className="p-2 rounded-lg border border-border hover:bg-surface-secondary" title="Stop"><Square size={16} /></button>
          <button onClick={() => handleAction('restart')} className="p-2 rounded-lg border border-border hover:bg-surface-secondary" title="Restart"><RotateCcw size={16} /></button>
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-border">
        <nav className="flex gap-6">
          {tabs.map((t) => (
            <button key={t.key} onClick={() => setTab(t.key)}
              className={`pb-3 text-sm font-medium border-b-2 transition-colors ${tab === t.key ? 'border-monster-green text-monster-green' : 'border-transparent text-text-secondary hover:text-text-primary'}`}>
              {t.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab Content */}
      {tab === 'overview' && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="bg-surface border border-border rounded-xl p-5 space-y-3">
            <h3 className="font-medium text-text-primary">Application Info</h3>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between"><span className="text-text-secondary">Type</span><span className="text-text-primary">{app.type}</span></div>
              <div className="flex justify-between"><span className="text-text-secondary">Source</span><span className="text-text-primary">{app.source_type}</span></div>
              {app.source_url && <div className="flex justify-between"><span className="text-text-secondary">URL</span><span className="text-text-primary truncate max-w-48">{app.source_url}</span></div>}
              <div className="flex justify-between"><span className="text-text-secondary">Branch</span><span className="text-text-primary flex items-center gap-1"><GitBranch size={14} />{app.branch}</span></div>
              <div className="flex justify-between"><span className="text-text-secondary">Replicas</span><span className="text-text-primary">{app.replicas}</span></div>
            </div>
          </div>
          <div className="bg-surface border border-border rounded-xl p-5 space-y-3">
            <h3 className="font-medium text-text-primary">Latest Deployment</h3>
            {deployments.length > 0 ? (
              <div className="space-y-2 text-sm">
                <div className="flex justify-between"><span className="text-text-secondary">Version</span><span className="text-text-primary">v{deployments[0].version}</span></div>
                <div className="flex justify-between"><span className="text-text-secondary">Image</span><span className="text-text-primary truncate max-w-48">{deployments[0].image}</span></div>
                <div className="flex justify-between"><span className="text-text-secondary">Triggered by</span><span className="text-text-primary">{deployments[0].triggered_by}</span></div>
                {deployments[0].commit_sha && <div className="flex justify-between"><span className="text-text-secondary">Commit</span><span className="text-text-primary font-mono">{deployments[0].commit_sha.slice(0, 8)}</span></div>}
              </div>
            ) : (
              <p className="text-sm text-text-muted">No deployments yet</p>
            )}
          </div>
        </div>
      )}

      {tab === 'deployments' && (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          {deployments.length === 0 ? (
            <div className="p-8 text-center text-text-muted">No deployments yet</div>
          ) : (
            <table className="w-full">
              <thead><tr className="border-b border-border bg-surface-secondary text-left">
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Version</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Status</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Image</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Commit</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Triggered</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Date</th>
              </tr></thead>
              <tbody className="divide-y divide-border">
                {deployments.map((d) => (
                  <tr key={d.id} className="hover:bg-surface-secondary">
                    <td className="px-5 py-3 font-medium text-text-primary">v{d.version}</td>
                    <td className="px-5 py-3"><StatusBadge status={d.status} /></td>
                    <td className="px-5 py-3 text-sm text-text-secondary truncate max-w-48">{d.image}</td>
                    <td className="px-5 py-3 text-sm font-mono text-text-secondary">{d.commit_sha?.slice(0, 8) || '-'}</td>
                    <td className="px-5 py-3 text-sm text-text-secondary">{d.triggered_by}</td>
                    <td className="px-5 py-3 text-sm text-text-muted flex items-center gap-1"><Clock size={14} />{new Date(d.created_at).toLocaleDateString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {tab === 'env' && (
        <div className="bg-surface border border-border rounded-xl p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-medium text-text-primary">Environment Variables</h3>
            <div className="flex gap-2">
              <button className="text-sm text-monster-green hover:underline">Import .env</button>
              <button className="text-sm text-text-secondary hover:underline">Export</button>
            </div>
          </div>
          <p className="text-sm text-text-muted">Use the API to manage environment variables:</p>
          <code className="block mt-2 bg-surface-tertiary px-3 py-2 rounded text-xs font-mono">
            GET /api/v1/apps/{app.id}/env
          </code>
          <p className="text-xs text-text-muted mt-2">Secret values are masked. Use ${'{'}SECRET:name{'}'} syntax for encrypted references.</p>
        </div>
      )}

      {tab === 'metrics' && (
        <div className="bg-surface border border-border rounded-xl p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-medium text-text-primary">Metrics</h3>
            <select className="text-sm border border-border rounded-lg px-2 py-1 bg-surface text-text-primary">
              <option>Last 1 hour</option>
              <option>Last 24 hours</option>
              <option>Last 7 days</option>
              <option>Last 30 days</option>
            </select>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div className="bg-surface-secondary rounded-lg p-4 text-center">
              <p className="text-2xl font-semibold text-text-primary">0%</p>
              <p className="text-sm text-text-secondary">CPU Usage</p>
            </div>
            <div className="bg-surface-secondary rounded-lg p-4 text-center">
              <p className="text-2xl font-semibold text-text-primary">0 MB</p>
              <p className="text-sm text-text-secondary">Memory</p>
            </div>
            <div className="bg-surface-secondary rounded-lg p-4 text-center">
              <p className="text-2xl font-semibold text-text-primary">0</p>
              <p className="text-sm text-text-secondary">Requests/min</p>
            </div>
          </div>
          <p className="text-xs text-text-muted mt-4">
            Detailed metrics available via API: GET /api/v1/apps/{app.id}/metrics?period=24h
          </p>
        </div>
      )}

      {tab === 'logs' && (
        <div className="bg-gray-900 rounded-xl p-4 h-96 overflow-auto font-mono text-sm text-green-400">
          {logs.length === 0 ? (
            <p className="text-gray-500">Waiting for logs...</p>
          ) : (
            logs.map((line, i) => <div key={i}>{line}</div>)
          )}
        </div>
      )}

      {tab === 'settings' && (
        <div className="bg-surface border border-border rounded-xl p-6 space-y-6">
          <div>
            <h3 className="font-medium text-text-primary mb-2">General</h3>
            <p className="text-sm text-text-secondary">Application ID: <code className="bg-surface-tertiary px-1.5 py-0.5 rounded">{app.id}</code></p>
          </div>
          <div className="border-t border-border pt-6">
            <h3 className="font-medium text-red-600 mb-2">Danger Zone</h3>
            <p className="text-sm text-text-secondary mb-3">Deleting this application will remove all deployments, domains, and data.</p>
            <button className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white text-sm font-medium rounded-lg transition-colors">
              Delete Application
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
