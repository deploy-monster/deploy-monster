import { useEffect, useState } from 'react';
import { Search, Package } from 'lucide-react';
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
  const [templates, setTemplates] = useState<Template[]>([]);
  const [categories, setCategories] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');

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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-text-primary">Marketplace</h1>
        <p className="text-sm text-text-secondary mt-1">One-click deploy popular applications</p>
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

      {/* Grid */}
      {templates.length === 0 ? (
        <div className="text-center py-16 text-text-muted">
          <Package size={48} className="mx-auto mb-4" />
          <p>No templates found</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {templates.map((t) => (
            <div key={t.slug} className="bg-surface border border-border rounded-xl p-5 hover:border-monster-green/50 transition-colors cursor-pointer">
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
                <button className="text-xs bg-monster-green hover:bg-monster-green-dark text-white px-3 py-1.5 rounded-lg transition-colors">
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
