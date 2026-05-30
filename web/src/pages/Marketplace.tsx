import { useState, useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useDebouncedValue } from '../hooks';
import { Search, Star, ShieldCheck, Box, Sparkles } from 'lucide-react';
import { marketplaceAPI, type Template, type MarketplaceResponse } from '@/api/marketplace';
import { useApi } from '../hooks';
import { cn } from '@/lib/utils';
import { toast } from '@/stores/toastStore';
import { Input } from '@/components/ui/input';
import {
  TemplateCardSkeleton,
  TemplateCard,
  FeaturedTemplateCard,
  TemplateDeploySheet,
  getCategoryColor,
} from '@/components/Marketplace';

const CATEGORIES = ['all', 'database', 'ai', 'cms', 'monitoring', 'devtools', 'storage', 'analytics', 'security'];

const RATING_CATEGORIES = [
  { key: 'popular', label: 'Most Popular', icon: Star },
  { key: 'newest', label: 'Newest', icon: Sparkles },
  { key: 'verified', label: 'Verified', icon: ShieldCheck },
];

export function Marketplace() {
  const navigate = useNavigate();
  const { data: templatesResp, loading } = useApi<MarketplaceResponse>('/marketplace');
  const templates = templatesResp?.data ?? [];

  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('all');
  const [ratingTab, setRatingTab] = useState<'popular' | 'newest' | 'verified'>('popular');
  const [deployingTemplate, setDeployingTemplate] = useState<Template | null>(null);

  const debouncedSearch = useDebouncedValue(search, 300);

  const filtered = useMemo(() => {
    return templates.filter((t) => {
      const matchesCategory = category === 'all' || t.category === category;
      const matchesSearch =
        !debouncedSearch ||
        t.name.toLowerCase().includes(debouncedSearch.toLowerCase()) ||
        t.description.toLowerCase().includes(debouncedSearch.toLowerCase());
      return matchesCategory && matchesSearch;
    });
  }, [templates, category, debouncedSearch]);

  const featuredTemplates = useMemo(() => {
    const sorted = [...templates].sort(() => Math.random() - 0.5).slice(0, 8);
    return ratingTab === 'popular' ? sorted.sort((a, b) => (b.stars || 0) - (a.stars || 0)).slice(0, 6) :
           ratingTab === 'newest' ? sorted.sort((a, b) => new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime()).slice(0, 6) :
           sorted.filter((t) => t.verified).slice(0, 6);
  }, [templates, ratingTab]);

  const handleDeployTemplate = async (template: Template, variables: Record<string, string>, appName: string) => {
    try {
      const app = await marketplaceAPI.deploy({
        slug: template.slug,
        name: appName,
        config: variables,
      });
      toast.success(`${template.name} deployment started`);
      setDeployingTemplate(null);
      navigate(`/apps/${app.app_id}`);
    } catch (err) {
      throw err;
    }
  };

  const categoryCounts = useMemo(() => {
    const counts: Record<string, number> = { all: templates.length };
    templates.forEach((t) => {
      counts[t.category] = (counts[t.category] || 0) + 1;
    });
    return counts;
  }, [templates]);

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Marketplace</h1>
          <p className="text-muted-foreground mt-1">
            One-click deploy popular databases, tools, and applications.
          </p>
        </div>
      </div>

      {/* Search + Filter bar */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        {/* Category pills */}
        <div className="flex items-center gap-2 overflow-x-auto pb-1 scrollbar-hide">
          {CATEGORIES.filter((c) => c === 'all' || categoryCounts[c]).map((cat) => {
            const catColor = getCategoryColor(cat);
            return (
              <button
                key={cat}
                onClick={() => setCategory(cat)}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium transition-all whitespace-nowrap cursor-pointer',
                  category === cat
                    ? 'bg-primary text-primary-foreground shadow-sm'
                    : cat === 'all'
                    ? 'bg-muted text-muted-foreground hover:bg-muted/80'
                    : cn(catColor.bg, catColor.text, 'hover:opacity-80')
                )}
              >
                {cat === 'all' ? 'All' : cat.charAt(0).toUpperCase() + cat.slice(1)}
                <span className={cn(
                  'ml-0.5 inline-flex items-center justify-center rounded-full min-w-4 h-4 px-1 text-[10px]',
                  category === cat ? 'bg-primary-foreground/20' : 'bg-black/10 dark:bg-white/20'
                )}>
                  {categoryCounts[cat] || 0}
                </span>
              </button>
            );
          })}
        </div>

        {/* Search */}
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
          <Input
            type="text"
            placeholder="Search templates..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 w-full sm:w-64"
          />
        </div>
      </div>

      {/* Featured row */}
      {featuredTemplates.length > 0 && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold">Featured Templates</h2>
            {/* Rating tabs */}
            <div className="flex items-center gap-1 rounded-lg bg-muted p-1">
              {RATING_CATEGORIES.map(({ key, label, icon: Icon }) => (
                <button
                  key={key}
                  onClick={() => setRatingTab(key as typeof ratingTab)}
                  className={cn(
                    'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-all cursor-pointer',
                    ratingTab === key
                      ? 'bg-primary text-primary-foreground shadow-sm'
                      : 'text-muted-foreground hover:bg-muted/80'
                  )}
                >
                  <Icon className="size-3.5" />
                  {label}
                </button>
              ))}
            </div>
          </div>

          <div className="flex gap-4 overflow-x-auto pb-4 -mx-2 px-2 scrollbar-hide">
            {featuredTemplates.map((t) => (
              <FeaturedTemplateCard
                key={t.id}
                template={t}
                onDeploy={(template) => setDeployingTemplate(template)}
                onClick={(template) => navigate(`/marketplace/${template.slug}`)}
              />
            ))}
          </div>
        </div>
      )}

      {/* Loading skeleton */}
      {loading && templates.length === 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <TemplateCardSkeleton key={i} />
          ))}
        </div>
      )}

      {/* Template grid */}
      {!loading || filtered.length > 0 ? filtered.length > 0 ? (
        <>
          {category !== 'all' && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <span>
                <strong className="text-foreground">{filtered.length}</strong> template{filtered.length !== 1 ? 's' : ''} in{' '}
                <span className="capitalize text-foreground font-medium">{category}</span>
              </span>
            </div>
          )}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {filtered.map((t) => (
              <TemplateCard
                key={t.id}
                template={t}
                onDeploy={(template) => setDeployingTemplate(template)}
                onClick={(template) => navigate(`/marketplace/${template.slug}`)}
              />
            ))}
          </div>
        </>
      ) : (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="rounded-full bg-muted p-5 mb-5">
            <Box className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold text-foreground mb-2">No templates found</h2>
          <p className="text-muted-foreground max-w-sm">
            {debouncedSearch
              ? `No templates match "${debouncedSearch}". Try adjusting your search.`
              : `No templates in this category yet. Check back later.`}
          </p>
        </div>
      ) : null}

      {/* Deploy Sheet */}
      <TemplateDeploySheet
        template={deployingTemplate}
        open={!!deployingTemplate}
        onClose={() => setDeployingTemplate(null)}
        onDeploy={handleDeployTemplate}
      />
    </div>
  );
}