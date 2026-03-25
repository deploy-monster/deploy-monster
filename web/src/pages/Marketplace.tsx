import { useState } from 'react';
import { useNavigate } from 'react-router';
import {
  Search,
  Rocket,
  Star,
  ShieldCheck,
  Tag,
  Loader2,
  AlertCircle,
  Box,
  Eye,
  EyeOff,
  Cpu,
  Sparkles,
  Filter,
} from 'lucide-react';
import { api } from '../api/client';
import { useApi } from '../hooks';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

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

interface MarketplaceResponse {
  data: Template[];
  categories: string[];
}

// ---------------------------------------------------------------------------
// Category color mapping
// ---------------------------------------------------------------------------

const CATEGORY_COLORS: Record<string, { bg: string; text: string; iconBg: string }> = {
  ai:            { bg: 'bg-violet-500/10',  text: 'text-violet-600 dark:text-violet-400',  iconBg: 'bg-violet-500' },
  cms:           { bg: 'bg-blue-500/10',    text: 'text-blue-600 dark:text-blue-400',      iconBg: 'bg-blue-500' },
  monitoring:    { bg: 'bg-emerald-500/10',  text: 'text-emerald-600 dark:text-emerald-400', iconBg: 'bg-emerald-500' },
  devtools:      { bg: 'bg-orange-500/10',  text: 'text-orange-600 dark:text-orange-400',  iconBg: 'bg-orange-500' },
  storage:       { bg: 'bg-cyan-500/10',    text: 'text-cyan-600 dark:text-cyan-400',      iconBg: 'bg-cyan-500' },
  analytics:     { bg: 'bg-pink-500/10',    text: 'text-pink-600 dark:text-pink-400',      iconBg: 'bg-pink-500' },
  security:      { bg: 'bg-red-500/10',     text: 'text-red-600 dark:text-red-400',        iconBg: 'bg-red-500' },
  automation:    { bg: 'bg-amber-500/10',   text: 'text-amber-600 dark:text-amber-400',    iconBg: 'bg-amber-500' },
  database:      { bg: 'bg-indigo-500/10',  text: 'text-indigo-600 dark:text-indigo-400',  iconBg: 'bg-indigo-500' },
  finance:       { bg: 'bg-green-500/10',   text: 'text-green-600 dark:text-green-400',    iconBg: 'bg-green-500' },
  collaboration: { bg: 'bg-sky-500/10',     text: 'text-sky-600 dark:text-sky-400',        iconBg: 'bg-sky-500' },
  productivity:  { bg: 'bg-teal-500/10',    text: 'text-teal-600 dark:text-teal-400',      iconBg: 'bg-teal-500' },
  search:        { bg: 'bg-purple-500/10',  text: 'text-purple-600 dark:text-purple-400',  iconBg: 'bg-purple-500' },
};

const DEFAULT_CATEGORY_COLOR = { bg: 'bg-muted', text: 'text-muted-foreground', iconBg: 'bg-muted-foreground' };

function getCategoryColor(category: string) {
  return CATEGORY_COLORS[category.toLowerCase()] || DEFAULT_CATEGORY_COLOR;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function TemplateCardSkeleton() {
  return (
    <Card className="py-5">
      <CardHeader className="pb-0">
        <div className="flex items-start gap-3">
          <Skeleton className="size-10 rounded-xl shrink-0" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-3 w-16" />
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <div className="space-y-2 mt-3">
          <Skeleton className="h-3 w-full" />
          <Skeleton className="h-3 w-3/4" />
        </div>
        <div className="flex gap-1.5 mt-3">
          <Skeleton className="h-5 w-14 rounded-md" />
          <Skeleton className="h-5 w-14 rounded-md" />
          <Skeleton className="h-5 w-14 rounded-md" />
        </div>
      </CardContent>
      <CardFooter className="border-t pt-4 pb-0 justify-between">
        <Skeleton className="h-3 w-10" />
        <Skeleton className="h-8 w-20 rounded-md" />
      </CardFooter>
    </Card>
  );
}

function PasswordInput({
  id,
  value,
  onChange,
  placeholder,
}: {
  id: string;
  value: string;
  onChange: (val: string) => void;
  placeholder: string;
}) {
  const [visible, setVisible] = useState(false);

  return (
    <div className="relative">
      <Input
        id={id}
        type={visible ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="pr-10"
      />
      <button
        type="button"
        onClick={() => setVisible((v) => !v)}
        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
        tabIndex={-1}
      >
        {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Marketplace
// ---------------------------------------------------------------------------

export function Marketplace() {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const [deploying, setDeploying] = useState<Template | null>(null);
  const [deployName, setDeployName] = useState('');
  const [deployConfig, setDeployConfig] = useState<Record<string, string>>({});
  const [deployLoading, setDeployLoading] = useState(false);
  const [deployError, setDeployError] = useState('');

  const params = new URLSearchParams();
  if (search) params.set('q', search);
  if (category) params.set('category', category);

  const { data: marketplaceData, loading } = useApi<MarketplaceResponse>(`/marketplace?${params}`);
  const templates = marketplaceData?.data || [];
  const categories = marketplaceData?.categories || [];

  const featuredCount = templates.filter((t) => t.featured).length;

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
      setDeployError(err instanceof Error ? err.message : 'Deployment failed. Please try again.');
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

  const closeDeploy = () => {
    setDeploying(null);
    setDeployError('');
    setDeployLoading(false);
  };

  return (
    <div className="space-y-8">
      {/* Hero Section */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10">
          <div className="flex items-center gap-2 mb-2">
            <Sparkles className="size-5 text-primary" />
            <Badge variant="secondary" className="text-xs font-normal">
              {templates.length} templates
              {featuredCount > 0 && ` \u00b7 ${featuredCount} featured`}
            </Badge>
          </div>
          <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
            Marketplace
          </h1>
          <p className="text-muted-foreground mt-1.5 text-sm sm:text-base max-w-lg">
            Deploy popular applications in one click. Databases, CMS, monitoring tools, and more &mdash; all pre-configured and ready to run.
          </p>
        </div>
        {/* Decorative */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Search & Filter */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
          <Input
            type="text"
            placeholder="Search templates by name, tag, or description..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>
        <div className="flex items-center gap-2">
          <Filter className="size-4 text-muted-foreground shrink-0 hidden sm:block" />
          <Select
            value={category}
            onChange={(e) => setCategory(e.target.value)}
            className="w-full sm:w-48"
          >
            <option value="">All Categories</option>
            {categories.map((c) => (
              <option key={c} value={c}>
                {c.charAt(0).toUpperCase() + c.slice(1)}
              </option>
            ))}
          </Select>
        </div>
      </div>

      {/* Active Filters */}
      {(search || category) && (
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-xs text-muted-foreground">Filters:</span>
          {search && (
            <Badge variant="secondary" className="gap-1 text-xs font-normal">
              Search: &quot;{search}&quot;
              <button
                onClick={() => setSearch('')}
                className="ml-0.5 hover:text-foreground transition-colors cursor-pointer"
              >
                &times;
              </button>
            </Badge>
          )}
          {category && (
            <Badge variant="secondary" className="gap-1 text-xs font-normal">
              Category: {category}
              <button
                onClick={() => setCategory('')}
                className="ml-0.5 hover:text-foreground transition-colors cursor-pointer"
              >
                &times;
              </button>
            </Badge>
          )}
          <button
            onClick={() => { setSearch(''); setCategory(''); }}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            Clear all
          </button>
        </div>
      )}

      {/* Deploy Dialog */}
      <Dialog open={deploying !== null} onOpenChange={(open) => !open && closeDeploy()}>
        <DialogContent onClose={closeDeploy} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-3">
              {deploying && (
                <div className={cn(
                  'flex items-center justify-center rounded-xl size-9',
                  getCategoryColor(deploying.category).iconBg
                )}>
                  <span className="text-sm font-bold text-white">
                    {deploying.name.charAt(0).toUpperCase()}
                  </span>
                </div>
              )}
              Deploy {deploying?.name}
            </DialogTitle>
            <DialogDescription>
              Configure and deploy {deploying?.name} to your platform.
              {deploying?.version && ` Version ${deploying.version}.`}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-2">
            {/* Stack name */}
            <div className="space-y-1.5">
              <Label htmlFor="deploy-name">Stack Name</Label>
              <Input
                id="deploy-name"
                type="text"
                value={deployName}
                onChange={(e) => setDeployName(e.target.value)}
                placeholder="my-app"
              />
              <p className="text-[11px] text-muted-foreground">
                Lowercase letters, numbers, and hyphens only.
              </p>
            </div>

            {/* Config variables */}
            <div className="space-y-1.5">
              <Label htmlFor="deploy-db-pw">Database Password</Label>
              <PasswordInput
                id="deploy-db-pw"
                value={deployConfig.DB_PASSWORD || ''}
                onChange={(val) => setDeployConfig({ ...deployConfig, DB_PASSWORD: val })}
                placeholder="Strong password for database"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="deploy-admin-pw">Admin Password</Label>
              <PasswordInput
                id="deploy-admin-pw"
                value={deployConfig.ADMIN_PASSWORD || ''}
                onChange={(val) => setDeployConfig({ ...deployConfig, ADMIN_PASSWORD: val })}
                placeholder="Admin panel password"
              />
            </div>

            {/* Minimum resources info */}
            {deploying?.min_resources && (
              <div className="flex items-center gap-2 rounded-lg border bg-muted/30 px-3 py-2.5">
                <Cpu className="size-4 text-muted-foreground shrink-0" />
                <p className="text-xs text-muted-foreground">
                  Minimum resources: <span className="font-medium text-foreground">{deploying.min_resources.memory_mb} MB RAM</span>
                </p>
              </div>
            )}

            {/* Error */}
            {deployError && (
              <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2.5 text-sm text-destructive">
                <AlertCircle className="size-4 shrink-0" />
                {deployError}
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={closeDeploy} disabled={deployLoading}>
              Cancel
            </Button>
            <Button onClick={handleDeploy} disabled={deployLoading || !deployName}>
              {deployLoading ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Deploying...
                </>
              ) : (
                <>
                  <Rocket className="size-4" />
                  Deploy
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Template Grid */}
      {loading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <TemplateCardSkeleton key={i} />
          ))}
        </div>
      ) : templates.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="rounded-full bg-muted p-6 mb-5">
            <Box className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">No templates found</h2>
          <p className="text-muted-foreground max-w-sm text-sm">
            {search || category
              ? 'Try adjusting your search or clearing filters to see more results.'
              : 'The marketplace is empty. Check back later for new templates.'}
          </p>
          {(search || category) && (
            <Button
              variant="outline"
              className="mt-4"
              onClick={() => { setSearch(''); setCategory(''); }}
            >
              Clear filters
            </Button>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {templates.map((t) => {
            const catColor = getCategoryColor(t.category);
            return (
              <Card
                key={t.slug}
                className={cn(
                  'group relative transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg hover:ring-2 hover:ring-primary/30',
                  t.featured && 'ring-1 ring-amber-400/30'
                )}
              >
                {/* Featured star badge */}
                {t.featured && (
                  <div className="absolute -top-1.5 -right-1.5 z-10">
                    <div className="flex items-center justify-center size-7 rounded-full bg-amber-400 shadow-md ring-2 ring-background">
                      <Star className="size-3.5 text-white fill-white" />
                    </div>
                  </div>
                )}

                <CardHeader className="pb-0 gap-4">
                  <div className="flex items-start gap-3 min-w-0">
                    {/* App icon */}
                    <div className={cn(
                      'flex items-center justify-center rounded-xl size-11 shrink-0',
                      catColor.iconBg
                    )}>
                      <span className="text-base font-bold text-white">
                        {t.name.charAt(0).toUpperCase()}
                      </span>
                    </div>

                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <CardTitle className="text-base truncate">{t.name}</CardTitle>
                        {t.verified && (
                          <ShieldCheck className="size-4 text-blue-500 shrink-0" />
                        )}
                      </div>
                      <Badge variant="outline" className={cn('mt-1.5 text-[10px] font-normal px-1.5 py-0', catColor.text)}>
                        {t.category}
                      </Badge>
                    </div>
                  </div>
                </CardHeader>

                <CardContent className="pt-0">
                  {/* Description */}
                  <p className="text-sm text-muted-foreground line-clamp-2 leading-relaxed">
                    {t.description}
                  </p>

                  {/* Tags */}
                  <div className="flex flex-wrap gap-1.5 mt-3">
                    {t.tags.slice(0, 3).map((tag) => (
                      <Badge key={tag} variant="secondary" className="text-[10px] font-normal gap-1 px-1.5 py-0">
                        <Tag className="size-2.5" />
                        {tag}
                      </Badge>
                    ))}
                    {t.tags.length > 3 && (
                      <span className="text-[10px] text-muted-foreground self-center">
                        +{t.tags.length - 3} more
                      </span>
                    )}
                  </div>
                </CardContent>

                <CardFooter className="border-t pt-4 pb-0 justify-between items-center">
                  <span className="text-xs text-muted-foreground tabular-nums">
                    v{t.version}
                  </span>
                  <Button
                    size="sm"
                    onClick={() => openDeploy(t)}
                    className="h-8 gap-1.5"
                  >
                    <Rocket className="size-3.5" />
                    Deploy
                  </Button>
                </CardFooter>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
