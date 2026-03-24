import { useState } from 'react';
import { useNavigate } from 'react-router';
import {
  Search,
  Package,
  Rocket,
  Star,
  ShieldCheck,
  Tag,
  Loader2,
  AlertCircle,
  Box,
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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';

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
      {/* Page Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Marketplace</h1>
        <p className="text-muted-foreground mt-1">
          Deploy popular applications in one click. {templates.length > 0 && `${templates.length} templates available.`}
        </p>
      </div>

      {/* Search & Filter */}
      <div className="flex flex-col gap-3 sm:flex-row">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
          <Input
            type="text"
            placeholder="Search templates..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>
        <Select
          value={category}
          onChange={(e) => setCategory(e.target.value)}
          className="w-full sm:w-48"
        >
          <option value="">All Categories</option>
          {categories.map((c) => (
            <option key={c} value={c}>{c}</option>
          ))}
        </Select>
      </div>

      {/* Deploy Dialog */}
      <Dialog open={deploying !== null} onOpenChange={(open) => !open && closeDeploy()}>
        <DialogContent onClose={closeDeploy} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <div className="flex items-center justify-center rounded-lg size-8 bg-primary/10">
                <Rocket className="size-4 text-primary" />
              </div>
              Deploy {deploying?.name}
            </DialogTitle>
            <DialogDescription>
              Configure and deploy {deploying?.name} to your platform. Version {deploying?.version}.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-2">
            <div>
              <Label htmlFor="deploy-name" className="mb-1.5">Application Name</Label>
              <Input
                id="deploy-name"
                type="text"
                value={deployName}
                onChange={(e) => setDeployName(e.target.value)}
                placeholder="my-app"
              />
            </div>

            <div>
              <Label htmlFor="deploy-db-pw" className="mb-1.5">Database Password</Label>
              <Input
                id="deploy-db-pw"
                type="password"
                value={deployConfig.DB_PASSWORD || ''}
                onChange={(e) => setDeployConfig({ ...deployConfig, DB_PASSWORD: e.target.value })}
                placeholder="Strong password for database"
              />
            </div>

            <div>
              <Label htmlFor="deploy-admin-pw" className="mb-1.5">Admin Password</Label>
              <Input
                id="deploy-admin-pw"
                type="password"
                value={deployConfig.ADMIN_PASSWORD || ''}
                onChange={(e) => setDeployConfig({ ...deployConfig, ADMIN_PASSWORD: e.target.value })}
                placeholder="Admin panel password"
              />
            </div>

            {deploying?.min_resources && (
              <p className="text-xs text-muted-foreground">
                Minimum resources: {deploying.min_resources.memory_mb} MB RAM
              </p>
            )}

            {deployError && (
              <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
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
        <div className="flex flex-col items-center justify-center py-20">
          <Loader2 className="size-8 animate-spin text-muted-foreground mb-3" />
          <p className="text-sm text-muted-foreground">Loading templates...</p>
        </div>
      ) : templates.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <div className="rounded-full bg-muted p-5 mb-5">
            <Box className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold text-foreground mb-2">No templates found</h2>
          <p className="text-muted-foreground max-w-sm">
            {search || category
              ? 'Try adjusting your search or category filter.'
              : 'The marketplace is empty. Check back later for new templates.'}
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {templates.map((t) => (
            <Card
              key={t.slug}
              className={cn(
                'group transition-all hover:shadow-md',
                t.featured && 'ring-1 ring-primary/20'
              )}
            >
              <CardHeader className="pb-0 gap-6">
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="flex items-center justify-center rounded-lg size-10 bg-muted shrink-0">
                      <Package className="size-5 text-muted-foreground" />
                    </div>
                    <div className="min-w-0">
                      <CardTitle className="text-base truncate">{t.name}</CardTitle>
                      <div className="flex items-center gap-1.5 mt-1">
                        {t.verified && (
                          <ShieldCheck className="size-3.5 text-blue-500 shrink-0" />
                        )}
                        <span className="text-xs text-muted-foreground truncate">{t.category}</span>
                      </div>
                    </div>
                  </div>
                  {t.featured && (
                    <Badge variant="secondary" className="shrink-0 gap-1">
                      <Star className="size-3 fill-current" />
                      Featured
                    </Badge>
                  )}
                </div>
              </CardHeader>

              <CardContent className="pt-0">
                <p className="text-sm text-muted-foreground line-clamp-2 mb-3">
                  {t.description}
                </p>
                <div className="flex flex-wrap gap-1.5">
                  {t.tags.slice(0, 3).map((tag) => (
                    <Badge key={tag} variant="outline" className="text-xs font-normal">
                      <Tag className="size-3" />
                      {tag}
                    </Badge>
                  ))}
                  {t.tags.length > 3 && (
                    <span className="text-xs text-muted-foreground self-center">
                      +{t.tags.length - 3} more
                    </span>
                  )}
                </div>
              </CardContent>

              <CardFooter className="border-t pt-4 pb-0 justify-between">
                <span className="text-xs text-muted-foreground">v{t.version}</span>
                <Button
                  size="sm"
                  onClick={() => openDeploy(t)}
                  className="h-8"
                >
                  <Rocket className="size-4" />
                  Deploy
                </Button>
              </CardFooter>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
