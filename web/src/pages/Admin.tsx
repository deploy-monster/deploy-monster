import { useEffect, useState } from 'react';
import { Shield, Server, Activity, RefreshCw } from 'lucide-react';
import { api } from '../api/client';

interface SystemInfo {
  version: string;
  commit: string;
  go: string;
  os: string;
  arch: string;
  goroutines: number;
  memory: { alloc_mb: number; sys_mb: number };
  modules: Array<{ id: string; status: string }>;
  events: { published: number; errors: number; subscriptions: number };
}

export function Admin() {
  const [system, setSystem] = useState<SystemInfo | null>(null);
  const [tab, setTab] = useState<'system' | 'modules' | 'events'>('system');

  useEffect(() => {
    api.get<SystemInfo>('/admin/system').then(setSystem).catch(() => {});
  }, []);

  const refresh = () => {
    api.get<SystemInfo>('/admin/system').then(setSystem);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Admin Panel</h1>
          <p className="text-sm text-text-secondary mt-1">System administration and monitoring</p>
        </div>
        <button onClick={refresh}
          className="flex items-center gap-2 px-3 py-2 border border-border text-text-secondary text-sm rounded-lg hover:bg-surface-secondary">
          <RefreshCw size={14} /> Refresh
        </button>
      </div>

      {/* Tabs */}
      <div className="border-b border-border">
        <nav className="flex gap-6">
          {[
            { key: 'system' as const, label: 'System', icon: Server },
            { key: 'modules' as const, label: 'Modules', icon: Shield },
            { key: 'events' as const, label: 'Events', icon: Activity },
          ].map(({ key, label, icon: Icon }) => (
            <button key={key} onClick={() => setTab(key)}
              className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 transition-colors ${
                tab === key ? 'border-monster-green text-monster-green' : 'border-transparent text-text-secondary'
              }`}>
              <Icon size={16} /> {label}
            </button>
          ))}
        </nav>
      </div>

      {tab === 'system' && system && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          <div className="bg-surface border border-border rounded-xl p-5">
            <p className="text-sm text-text-secondary mb-1">Version</p>
            <p className="text-lg font-semibold text-text-primary">{system.version}</p>
            <p className="text-xs text-text-muted mt-1">{system.commit}</p>
          </div>
          <div className="bg-surface border border-border rounded-xl p-5">
            <p className="text-sm text-text-secondary mb-1">Runtime</p>
            <p className="text-lg font-semibold text-text-primary">{system.go}</p>
            <p className="text-xs text-text-muted mt-1">{system.os}/{system.arch}</p>
          </div>
          <div className="bg-surface border border-border rounded-xl p-5">
            <p className="text-sm text-text-secondary mb-1">Goroutines</p>
            <p className="text-lg font-semibold text-text-primary">{system.goroutines}</p>
          </div>
          <div className="bg-surface border border-border rounded-xl p-5">
            <p className="text-sm text-text-secondary mb-1">Memory (alloc)</p>
            <p className="text-lg font-semibold text-text-primary">{system.memory?.alloc_mb} MB</p>
          </div>
          <div className="bg-surface border border-border rounded-xl p-5">
            <p className="text-sm text-text-secondary mb-1">Memory (sys)</p>
            <p className="text-lg font-semibold text-text-primary">{system.memory?.sys_mb} MB</p>
          </div>
          <div className="bg-surface border border-border rounded-xl p-5">
            <p className="text-sm text-text-secondary mb-1">Events Published</p>
            <p className="text-lg font-semibold text-text-primary">{system.events?.published || 0}</p>
          </div>
        </div>
      )}

      {tab === 'modules' && system && (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          <table className="w-full">
            <thead><tr className="border-b border-border bg-surface-secondary text-left">
              <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Module</th>
              <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Status</th>
            </tr></thead>
            <tbody className="divide-y divide-border">
              {(system.modules || []).map((m) => (
                <tr key={m.id} className="hover:bg-surface-secondary">
                  <td className="px-5 py-3 font-medium text-text-primary font-mono text-sm">{m.id}</td>
                  <td className="px-5 py-3">
                    <span className={`inline-flex items-center px-2 py-0.5 rounded-md text-xs font-medium ${
                      m.status === 'ok' ? 'bg-status-running/10 text-status-running' :
                      m.status === 'degraded' ? 'bg-yellow-100 text-yellow-600' :
                      'bg-red-100 text-red-600'
                    }`}>{m.status}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'events' && system && (
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div className="bg-surface border border-border rounded-xl p-5 text-center">
            <p className="text-3xl font-bold text-text-primary">{system.events?.published || 0}</p>
            <p className="text-sm text-text-secondary">Published</p>
          </div>
          <div className="bg-surface border border-border rounded-xl p-5 text-center">
            <p className="text-3xl font-bold text-text-primary">{system.events?.errors || 0}</p>
            <p className="text-sm text-text-secondary">Errors</p>
          </div>
          <div className="bg-surface border border-border rounded-xl p-5 text-center">
            <p className="text-3xl font-bold text-text-primary">{system.events?.subscriptions || 0}</p>
            <p className="text-sm text-text-secondary">Subscriptions</p>
          </div>
        </div>
      )}
    </div>
  );
}
