import { useState } from 'react';
import { Globe, Plus, Shield, ShieldCheck, ShieldAlert, Trash2 } from 'lucide-react';
import { api } from '../api/client';
import { useApi } from '../hooks';

interface Domain {
  id: string;
  app_id: string;
  fqdn: string;
  type: string;
  dns_provider: string;
  dns_synced: boolean;
  verified: boolean;
  created_at: string;
}

function SSLBadge({ verified }: { verified: boolean }) {
  if (verified) {
    return <span className="flex items-center gap-1 text-xs text-status-running"><ShieldCheck size={14} /> Active</span>;
  }
  return <span className="flex items-center gap-1 text-xs text-status-deploying"><ShieldAlert size={14} /> Pending</span>;
}

export function Domains() {
  const { data: domains, refetch } = useApi<Domain[]>('/domains');
  const [showAdd, setShowAdd] = useState(false);
  const [newFQDN, setNewFQDN] = useState('');
  const [newAppID, setNewAppID] = useState('');

  const handleAdd = async () => {
    if (!newFQDN) return;
    await api.post('/domains', { fqdn: newFQDN, app_id: newAppID });
    setNewFQDN('');
    setNewAppID('');
    setShowAdd(false);
    refetch();
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Remove this domain?')) return;
    await api.delete(`/domains/${id}`);
    refetch();
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Domains</h1>
          <p className="text-sm text-text-secondary mt-1">{(domains || []).length} domain{(domains || []).length !== 1 ? 's' : ''} configured</p>
        </div>
        <button onClick={() => setShowAdd(!showAdd)}
          className="flex items-center gap-2 px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors">
          <Plus size={16} /> Add Domain
        </button>
      </div>

      {showAdd && (
        <div className="bg-surface border border-border rounded-xl p-6 space-y-4">
          <h2 className="font-medium text-text-primary">Add Custom Domain</h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">Domain (FQDN)</label>
              <input type="text" value={newFQDN} onChange={(e) => setNewFQDN(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                placeholder="app.example.com" />
            </div>
            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">Application ID</label>
              <input type="text" value={newAppID} onChange={(e) => setNewAppID(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                placeholder="app_xxxxxxxx" />
            </div>
          </div>
          <div className="bg-surface-secondary rounded-lg p-4 text-sm text-text-secondary">
            <p className="font-medium text-text-primary mb-2">DNS Configuration</p>
            <p>Point your domain to your server by adding an A record:</p>
            <code className="block mt-2 bg-surface-tertiary px-3 py-2 rounded font-mono text-xs">
              {newFQDN || 'app.example.com'}  A  → your-server-ip
            </code>
          </div>
          <div className="flex gap-2">
            <button onClick={handleAdd} className="px-4 py-2 bg-monster-green text-white text-sm rounded-lg">Add Domain</button>
            <button onClick={() => setShowAdd(false)} className="px-4 py-2 border border-border text-text-secondary text-sm rounded-lg">Cancel</button>
          </div>
        </div>
      )}

      {(!domains || domains.length === 0) ? (
        <div className="bg-surface border border-border rounded-xl px-5 py-16 text-center">
          <Globe className="mx-auto mb-4 text-text-muted" size={48} />
          <h2 className="text-lg font-medium text-text-primary mb-2">No domains configured</h2>
          <p className="text-text-secondary">Add a custom domain to route traffic to your applications.</p>
        </div>
      ) : (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border bg-surface-secondary text-left">
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Domain</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Type</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">SSL</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">DNS</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Added</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase w-10"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {(domains || []).map((d) => (
                <tr key={d.id} className="hover:bg-surface-secondary transition-colors">
                  <td className="px-5 py-3">
                    <div className="flex items-center gap-2">
                      <Shield size={16} className="text-monster-green" />
                      <span className="font-medium text-text-primary">{d.fqdn}</span>
                    </div>
                  </td>
                  <td className="px-5 py-3 text-sm text-text-secondary">{d.type}</td>
                  <td className="px-5 py-3"><SSLBadge verified={d.verified} /></td>
                  <td className="px-5 py-3 text-sm text-text-secondary">
                    {d.dns_synced ?
                      <span className="text-status-running">Synced</span> :
                      <span className="text-text-muted">{d.dns_provider}</span>
                    }
                  </td>
                  <td className="px-5 py-3 text-sm text-text-muted">{new Date(d.created_at).toLocaleDateString()}</td>
                  <td className="px-5 py-3">
                    <button onClick={() => handleDelete(d.id)} className="p-1 rounded hover:bg-red-50 dark:hover:bg-red-900/20 text-text-muted hover:text-red-600">
                      <Trash2 size={16} />
                    </button>
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
