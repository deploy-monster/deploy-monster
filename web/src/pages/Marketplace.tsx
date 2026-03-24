import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router';
import { Search, Package, Rocket, X } from 'lucide-react';
import { api } from '../api/client';

interface Template {
  slug: string;
  name: string;
  description: string;
  category: string;
  tags: string[];
  version: string;
  featured: boolean;
  verified: boolean;
  min_resources: { memory_mb: number };
}

export function Marketplace() {
  const navigate = useNavigate();
  const [templates, setTemplates] = useState<Template[]>([]);
  const [categories, setCategories] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const [deploying, setDeploying] = useState<Template | null>(null);
  const [deployName, setDeployName] = useState('');
  const [deployConfig, setDeployConfig] = useState<Record<string, string>>({});
  const [deployLoading, setDeployLoading] = useState(false);
  const [deployError, setDeployError] = useState('');

  useEffect(() => {
    const params = new URLSearchParams();
    if (search) params.set('q', search);
    if (category) params.set('category', category);

    api.get<{ data: Template[]; categories: string[] }>(`/marketplace?${params}`)
      .then((r) => {
        setTemplates(r.data || []);
        setCategories(r.categories || []);
      })
      .catch(() => {});
  }, [search, category]);

  const handleDeploy = async () => {
    if (!deploying || !deployName) return;
    setDeployLoading(true);
    setDeployError('');

    try {
      const result = await api.post<{ app_id: string }>('/marketplace/deploy', {
        slug: deploying.slug,
        name: deployName,
        config: deployConfig,
      });
      setDeploying(null);
      navigate(`/apps/${result.app_id}`);
    } catch (err) {
      setDeployError(err instanceof Error ? err.message : 'Deploy failed');
    } finally {
      setDeployLoading(false);
    }
  };

  const openDeploy = (t: Template) => {
    setDeploying(t);
    setDeployName(t.slug);
    setDeployConfig({ DB_PASSWORD: '', ADMIN_PASSWORD: '' });
    setDeployError('');
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-text-primary">Marketplace</h1>
        <p className="text-sm text-text-secondary mt-1">One-click deploy {templates.length}+ applications</p>
      </div>

      {/* Search & Filter */}
      <div className="flex gap-3">
        <div className="relative flex-1">
          <Search size={16} className="absolute left-3 top-2.5 text-text-muted" />
          <input type="text" placeholder="Search templates..." value={search} onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-9 pr-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:outline-none focus:ring-2 focus:ring-monster-green/50" />
        </div>
        <select value={category} onChange={(e) => setCategory(e.target.value)}
          className="px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:outline-none">
          <option value="">All Categories</option>
          {categories.map((c) => <option key={c} value={c}>{c}</option>)}
        </select>
      </div>

      {/* Deploy Dialog */}
      {deploying && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="bg-surface border border-border rounded-2xl p-6 w-full max-w-md shadow-xl space-y-4">
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-semibold text-text-primary flex items-center gap-2">
                <Rocket size={20} className="text-monster-green" />Deploy {deploying.name}
              </h2>
              <button onClick={() => setDeploying(null)} className="text-text-muted hover:text-text-primary"><X size={20} /></button>
            </div>

            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">App Name</label>
              <input type="text" value={deployName} onChange={(e) => setDeployName(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50" />
            </div>

            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">Database Password</label>
              <input type="password" value={deployConfig.DB_PASSWORD || ''}
                onChange={(e) => setDeployConfig({ ...deployConfig, DB_PASSWORD: e.target.value })}
                className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                placeholder="Strong password for database" />
            </div>

            {deployError && (
              <div className="bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 text-sm px-3 py-2 rounded-lg">{deployError}</div>
            )}

            <div className="flex gap-3">
              <button onClick={() => setDeploying(null)} className="flex-1 py-2 border border-border text-text-secondary rounded-lg hover:bg-surface-secondary">Cancel</button>
              <button onClick={handleDeploy} disabled={deployLoading || !deployName}
                className="flex-1 py-2 bg-monster-green hover:bg-monster-green-dark text-white font-medium rounded-lg disabled:opacity-50 flex items-center justify-center gap-2">
                <Rocket size={16} />{deployLoading ? 'Deploying...' : 'Deploy'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Grid */}
      {templates.length === 0 ? (
        <div className="text-center py-16 text-text-muted">
          <Package size={48} className="mx-auto mb-4" />
          <p>No templates found</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {templates.map((t) => (
            <div key={t.slug} className="bg-surface border border-border rounded-xl p-5 hover:border-monster-green/50 transition-colors">
              <div className="flex items-start justify-between mb-3">
                <div>
                  <h3 className="font-medium text-text-primary">{t.name}</h3>
                  <p className="text-xs text-text-muted">{t.category} {t.verified && '• Verified'}</p>
                </div>
                {t.featured && <span className="text-xs bg-monster-green/10 text-monster-green px-2 py-0.5 rounded-full">Featured</span>}
              </div>
              <p className="text-sm text-text-secondary mb-4 line-clamp-2">{t.description}</p>
              <div className="flex items-center justify-between">
                <div className="flex gap-1">
                  {t.tags.slice(0, 3).map((tag) => (
                    <span key={tag} className="text-xs bg-surface-tertiary text-text-muted px-2 py-0.5 rounded">{tag}</span>
                  ))}
                </div>
                <button onClick={() => openDeploy(t)}
                  className="text-xs bg-monster-green hover:bg-monster-green-dark text-white px-3 py-1.5 rounded-lg transition-colors">
                  Deploy
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
