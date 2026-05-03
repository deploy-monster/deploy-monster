import { useParams, useNavigate } from 'react-router';
import {
  ArrowLeft,
  Star,
  ShieldCheck,
  Rocket,
  Cpu,
  HardDrive,
  MemoryStick,
  Copy,
  Check,
  Box,
  Loader2,
  AlertCircle,
  Eye,
  EyeOff,
} from 'lucide-react';
import { useState, useMemo, useCallback } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { useApi } from '@/hooks';
import { type Template } from '@/api/marketplace';
import { marketplaceAPI } from '@/api/marketplace';
import { generatedSecretEntries, formatGeneratedSecrets } from '@/lib/generatedSecrets';
import { toast } from '@/stores/toastStore';

// ---------------------------------------------------------------------------
// Helpers
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
  communication: { bg: 'bg-sky-500/10',     text: 'text-sky-600 dark:text-sky-400',        iconBg: 'bg-sky-500' },
  media:         { bg: 'bg-rose-500/10',    text: 'text-rose-600 dark:text-rose-400',      iconBg: 'bg-rose-500' },
  ecommerce:     { bg: 'bg-emerald-500/10', text: 'text-emerald-600 dark:text-emerald-400', iconBg: 'bg-emerald-500' },
  iot:           { bg: 'bg-lime-500/10',    text: 'text-lime-600 dark:text-lime-400',      iconBg: 'bg-lime-500' },
  design:        { bg: 'bg-fuchsia-500/10', text: 'text-fuchsia-600 dark:text-fuchsia-400', iconBg: 'bg-fuchsia-500' },
  networking:    { bg: 'bg-slate-500/10',   text: 'text-slate-600 dark:text-slate-400',    iconBg: 'bg-slate-500' },
};

const DEFAULT_CATEGORY_COLOR = { bg: 'bg-muted', text: 'text-muted-foreground', iconBg: 'bg-muted-foreground' };

function getCategoryColor(category: string) {
  return CATEGORY_COLORS[category.toLowerCase()] || DEFAULT_CATEGORY_COLOR;
}

function cn(...classes: (string | undefined | false)[]) {
  return classes.filter(Boolean).join(' ');
}

function TemplateIcon({ template, size = 'size-12' }: { template: Template; size?: string }) {
  const catColor = getCategoryColor(template.category);
  if (template.icon && template.icon.length <= 4) {
    return (
      <div className={cn('flex items-center justify-center rounded-xl shrink-0 bg-card border', size)}>
        <span className="text-2xl">{template.icon}</span>
      </div>
    );
  }
  return (
    <div className={cn('flex items-center justify-center rounded-xl shrink-0', catColor.iconBg, size)}>
      <span className="text-lg font-bold text-white">
        {template.name.charAt(0).toUpperCase()}
      </span>
    </div>
  );
}

/** Parse service names from compose YAML */
function parseServices(yaml: string): string[] {
  const services: string[] = [];
  const lines = yaml.split('\n');
  let inServices = false;
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed === 'services:') {
      inServices = true;
      continue;
    }
    if (inServices) {
      const match = /^ {2}([a-zA-Z0-9_-]+):/.exec(line);
      if (match) {
        services.push(match[1]);
      } else if (!line.startsWith(' ') && trimmed !== '') {
        inServices = false;
      }
    }
  }
  return services;
}

// ---------------------------------------------------------------------------
// PasswordInput
// ---------------------------------------------------------------------------

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
// DeployForm
// ---------------------------------------------------------------------------

function DeployForm({ template, onDeployed }: { template: Template; onDeployed: (appId: string) => void }) {
  const [name, setName] = useState(template.slug);
  const [config, setConfig] = useState<Record<string, string>>(() => {
    const defaults: Record<string, string> = {};
    if (template.config_schema?.properties) {
      for (const [key, prop] of Object.entries(template.config_schema!.properties!)) {
        defaults[key] = (prop as { default?: string }).default || '';
      }
    }
    return defaults;
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [generatedSecrets, setGeneratedSecrets] = useState<Record<string, string>>({});
  const [deployedAppId, setDeployedAppId] = useState('');

  const configFields = useMemo(() => {
    if (!template.config_schema?.properties) return [];
    const schema = template.config_schema;
    const required = new Set(schema.required || []);
    const fields: Array<{
      key: string;
      title: string;
      description?: string;
      format?: string;
      required: boolean;
    }> = [];
    for (const [key, prop] of Object.entries(schema.properties!)) {
      fields.push({
        key,
        title: prop.title || key,
        description: prop.description,
        format: prop.format,
        required: required.has(key),
      });
    }
    return fields;
  }, [template]);

  const handleDeploy = async () => {
    if (!name) return;
    setLoading(true);
    setError('');
    try {
      const result = await marketplaceAPI.deploy({ slug: template.slug, name, config });
      const secrets = result.generated_secrets || {};
      if (Object.keys(secrets).length > 0) {
        setGeneratedSecrets(secrets);
        setDeployedAppId(result.app_id);
      } else {
        onDeployed(result.app_id);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deployment failed');
    } finally {
      setLoading(false);
    }
  };

  const copyGeneratedSecrets = async () => {
    try {
      await navigator.clipboard.writeText(formatGeneratedSecrets(generatedSecrets));
      toast.success('Credentials copied');
    } catch {
      toast.error('Failed to copy credentials');
    }
  };

  if (Object.keys(generatedSecrets).length > 0) {
    return (
      <div className="space-y-4">
        <div className="space-y-3 rounded-lg border bg-muted/30 p-4">
          <Label className="text-sm font-medium">Generated Credentials</Label>
          <p className="text-xs text-muted-foreground">
            These generated values are shown once. Save them before opening the app.
          </p>
          <div className="space-y-2">
            {generatedSecretEntries(generatedSecrets).map(([key, value]) => (
              <div key={key} className="grid gap-1 rounded-md border bg-background px-3 py-2">
                <span className="text-xs font-medium text-muted-foreground">{key}</span>
                <code className="break-all text-sm">{value}</code>
              </div>
            ))}
          </div>
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          <Button variant="outline" onClick={copyGeneratedSecrets}>
            <Copy className="size-4" />
            Copy Credentials
          </Button>
          <Button onClick={() => onDeployed(deployedAppId)}>
            Open App
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="deploy-name">Stack Name</Label>
        <Input
          id="deploy-name"
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="my-app"
        />
      </div>

      {configFields.length > 0 && (
        <div className="space-y-3">
          <Label className="text-sm font-medium">Configuration</Label>
          {configFields.map((field) => (
            <div key={field.key} className="space-y-1.5">
              <Label htmlFor={`config-${field.key}`}>
                {field.title}
                {field.required && <span className="text-destructive ml-1">*</span>}
              </Label>
              {field.format === 'password' ? (
                <PasswordInput
                  id={`config-${field.key}`}
                  value={config[field.key] || ''}
                  onChange={(val) => setConfig({ ...config, [field.key]: val })}
                  placeholder={field.description || field.title}
                />
              ) : (
                <Input
                  id={`config-${field.key}`}
                  type="text"
                  value={config[field.key] || ''}
                  onChange={(e) => setConfig({ ...config, [field.key]: e.target.value })}
                  placeholder={field.description || field.title}
                />
              )}
              {field.description && (
                <p className="text-[11px] text-muted-foreground">{field.description}</p>
              )}
            </div>
          ))}
        </div>
      )}

      {error && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2.5 text-sm text-destructive">
          <AlertCircle className="size-4 shrink-0" />
          {error}
        </div>
      )}

      <Button onClick={handleDeploy} disabled={loading || !name} className="w-full">
        {loading ? (
          <>
            <Loader2 className="size-4 animate-spin" />
            Deploying...
          </>
        ) : (
          <>
            <Rocket className="size-4" />
            Deploy {template.name}
          </>
        )}
      </Button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// ComposePreview
// ---------------------------------------------------------------------------

function ComposePreview({ yaml }: { yaml: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(yaml);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [yaml]);

  return (
    <div className="relative rounded-lg border bg-muted/30 overflow-hidden">
      <div className="flex items-center justify-between px-4 py-2 border-b bg-muted/50">
        <span className="text-xs font-medium text-muted-foreground">compose.yaml</span>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 text-xs px-2"
          onClick={handleCopy}
        >
          {copied ? (
            <>
              <Check className="size-3" />
              Copied
            </>
          ) : (
            <>
              <Copy className="size-3" />
              Copy
            </>
          )}
        </Button>
      </div>
      <pre className="text-xs font-mono p-4 overflow-x-auto text-foreground leading-relaxed max-h-80">
        {yaml}
      </pre>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Skeleton
// ---------------------------------------------------------------------------

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-8 w-24" />
      <div className="flex gap-4">
        <Skeleton className="size-14 rounded-xl" />
        <div className="space-y-2 flex-1">
          <Skeleton className="h-7 w-48" />
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-4 w-full" />
        </div>
      </div>
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 space-y-4">
          <Skeleton className="h-5 w-32" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-48 w-full rounded-lg" />
        </div>
        <Skeleton className="h-64 w-full rounded-lg" />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// TemplateDetail Page
// ---------------------------------------------------------------------------

export function TemplateDetail() {
  const { slug } = useParams<{ slug: string }>();
  const navigate = useNavigate();
  const { data, loading } = useApi<Template>(`/marketplace/${slug}`);

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-24" />
        <DetailSkeleton />
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center py-24 text-center">
        <div className="rounded-full bg-muted p-6 mb-5">
          <Box className="size-10 text-muted-foreground" />
        </div>
        <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">Template not found</h2>
        <p className="text-muted-foreground max-w-sm text-sm mb-4">
          The template you&apos;re looking for doesn&apos;t exist or has been removed.
        </p>
        <Button variant="outline" onClick={() => navigate('/marketplace')}>
          <ArrowLeft className="size-4" />
          Back to Marketplace
        </Button>
      </div>
    );
  }

  const catColor = getCategoryColor(data.category);
  const services = parseServices(data.compose_yaml || '');

  return (
    <div className="space-y-6">
      {/* Back + Header */}
      <div>
        <Button
          variant="ghost"
          size="sm"
          className="mb-4 gap-1.5 text-muted-foreground"
          onClick={() => navigate('/marketplace')}
        >
          <ArrowLeft className="size-3.5" />
          Back to Marketplace
        </Button>

        <div className="flex flex-col sm:flex-row sm:items-start gap-4">
          <TemplateIcon template={data} size="size-14" />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h1 className="text-2xl font-semibold tracking-tight">{data.name}</h1>
              {data.featured && (
                <Badge variant="secondary" className="gap-1 text-xs font-normal">
                  <Star className="size-3 text-amber-500 fill-amber-500" />
                  Featured
                </Badge>
              )}
              {data.verified && (
                <Badge variant="outline" className="gap-1 text-xs font-normal text-blue-600 dark:text-blue-400">
                  <ShieldCheck className="size-3" />
                  Verified
                </Badge>
              )}
            </div>
            <div className="flex items-center gap-3 mt-1.5">
              <Badge variant="outline" className={cn('text-xs font-normal', catColor.text)}>
                {data.category}
              </Badge>
              {data.author && (
                <span className="text-sm text-muted-foreground">by {data.author}</span>
              )}
              <span className="text-sm text-muted-foreground">v{data.version}</span>
            </div>
            <p className="text-sm text-muted-foreground mt-2 max-w-2xl">{data.description}</p>
          </div>
        </div>
      </div>

      {/* Tags */}
      {data.tags.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {data.tags.map((tag) => (
            <Badge key={tag} variant="secondary" className="text-xs font-normal">
              {tag}
            </Badge>
          ))}
        </div>
      )}

      {/* Resource Requirements */}
      {data.min_resources && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Resource Requirements</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-3 gap-4">
              <div className="flex items-center gap-3">
                <div className={cn('p-2 rounded-lg', catColor.bg)}>
                  <MemoryStick className={cn('size-4', catColor.text)} />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Memory</p>
                  <p className="text-sm font-medium">{data.min_resources.memory_mb} MB</p>
                </div>
              </div>
              <div className="flex items-center gap-3">
                <div className={cn('p-2 rounded-lg', catColor.bg)}>
                  <HardDrive className={cn('size-4', catColor.text)} />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Disk</p>
                  <p className="text-sm font-medium">{data.min_resources.disk_mb} MB</p>
                </div>
              </div>
              <div className="flex items-center gap-3">
                <div className={cn('p-2 rounded-lg', catColor.bg)}>
                  <Cpu className={cn('size-4', catColor.text)} />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">CPU</p>
                  <p className="text-sm font-medium">{data.min_resources.cpu_mb} mCPU</p>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Two-column: Services + Compose | Deploy */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left */}
        <div className="lg:col-span-2 space-y-6">
          {services.length > 0 && (
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium">
                  Services ({services.length})
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-2">
                  {services.map((svc) => (
                    <div
                      key={svc}
                      className="flex items-center gap-3 rounded-lg border px-4 py-2.5"
                    >
                      <div className={cn('p-1.5 rounded-md', catColor.bg)}>
                        <Box className={cn('size-3.5', catColor.text)} />
                      </div>
                      <span className="text-sm font-medium">{svc}</span>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}

          {data.compose_yaml && (
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium">Compose Configuration</CardTitle>
              </CardHeader>
              <CardContent>
                <ComposePreview yaml={data.compose_yaml} />
              </CardContent>
            </Card>
          )}
        </div>

        {/* Right: Deploy */}
        <div>
          <Card className="sticky top-4">
            <CardHeader className="pb-3">
              <CardTitle className="text-base flex items-center gap-2">
                <Rocket className="size-4" />
                Deploy
              </CardTitle>
            </CardHeader>
            <CardContent>
              <DeployForm
                template={data}
                onDeployed={(appId) => navigate(`/apps/${appId}`)}
              />
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
