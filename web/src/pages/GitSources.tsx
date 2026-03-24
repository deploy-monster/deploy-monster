import { useEffect, useState } from 'react';
import { GitBranch, Plus, ExternalLink } from 'lucide-react';
import { api } from '../api/client';

interface GitProvider {
  id: string;
  name: string;
}

export function GitSources() {
  const [providers, setProviders] = useState<GitProvider[]>([]);
  const [showConnect, setShowConnect] = useState(false);

  useEffect(() => {
    api.get<{ data: GitProvider[] }>('/git/providers').then((r) => setProviders(r.data || [])).catch(() => {});
  }, []);

  const providerIcons: Record<string, string> = {
    github: 'GH',
    gitlab: 'GL',
    gitea: 'GT',
    bitbucket: 'BB',
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Git Sources</h1>
          <p className="text-sm text-text-secondary mt-1">Connect Git providers for auto-deploy</p>
        </div>
        <button onClick={() => setShowConnect(!showConnect)}
          className="flex items-center gap-2 px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors">
          <Plus size={16} /> Connect Provider
        </button>
      </div>

      {showConnect && (
        <div className="bg-surface border border-border rounded-xl p-6">
          <h2 className="font-medium text-text-primary mb-4">Connect a Git Provider</h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            {[
              { id: 'github', name: 'GitHub', desc: 'Connect via OAuth or personal token' },
              { id: 'gitlab', name: 'GitLab', desc: 'GitLab.com or self-hosted' },
              { id: 'gitea', name: 'Gitea', desc: 'Self-hosted Gitea instance' },
              { id: 'bitbucket', name: 'Bitbucket', desc: 'Bitbucket Cloud' },
            ].map((p) => (
              <button key={p.id}
                className="flex flex-col items-center gap-2 p-4 rounded-xl border border-border hover:border-monster-green/50 hover:bg-surface-secondary transition-colors">
                <div className="w-12 h-12 rounded-xl bg-surface-tertiary flex items-center justify-center text-text-primary font-bold">
                  {providerIcons[p.id] || p.id[0].toUpperCase()}
                </div>
                <span className="font-medium text-text-primary text-sm">{p.name}</span>
                <span className="text-xs text-text-muted text-center">{p.desc}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Connected providers */}
      <div className="bg-surface border border-border rounded-xl">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-medium text-text-primary">Available Providers</h2>
        </div>
        {providers.length === 0 ? (
          <div className="px-5 py-12 text-center">
            <GitBranch className="mx-auto mb-4 text-text-muted" size={48} />
            <p className="text-text-secondary">No Git providers connected yet</p>
            <p className="text-sm text-text-muted mt-1">Connect a provider to enable auto-deploy from push</p>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {providers.map((p) => (
              <div key={p.id} className="flex items-center justify-between px-5 py-4">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-lg bg-surface-tertiary flex items-center justify-center font-bold text-text-primary">
                    {providerIcons[p.id] || p.id[0].toUpperCase()}
                  </div>
                  <div>
                    <p className="font-medium text-text-primary">{p.name}</p>
                    <p className="text-sm text-text-secondary">{p.id}</p>
                  </div>
                </div>
                <button className="flex items-center gap-1 text-sm text-monster-green hover:underline">
                  Browse Repos <ExternalLink size={14} />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
