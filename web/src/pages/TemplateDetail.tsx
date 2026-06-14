import { useParams, useNavigate } from 'react-router';
import {
  ArrowLeft, Star, ShieldCheck, Rocket, Cpu, HardDrive, MemoryStick,
  Copy, Check, Box,
} from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useApi } from '@/hooks';
import { type Template } from '@/api/marketplace';
import { cn } from '@/lib/utils';
import { getCategoryColor } from '@/components/Marketplace';
import { TemplateIcon } from '@/components/Marketplace/TemplateCard';
import { DeployForm } from '@/components/TemplateDetail';
import { parseServices } from '@/components/TemplateDetail';
import { toast } from '@/stores/toastStore';

export function TemplateDetail() {
  const { slug } = useParams();
  const navigate = useNavigate();
  const { data: template, loading } = useApi<Template>(`/marketplace/${slug}`);
  const [copiedSection, setCopiedSection] = useState('');

  const copyToClipboard = async (text: string, section: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedSection(section);
      setTimeout(() => setCopiedSection(''), 2000);
      toast.success('Template copied to clipboard');
    } catch {
      toast.error('Failed to copy template');
    }
  };

  if (loading) {
    return (
      <div className="space-y-6 max-w-3xl">
        <div className="flex items-center gap-4">
          <Skeleton className="size-10 rounded-xl" />
          <div className="space-y-2">
            <Skeleton className="h-6 w-48" />
            <Skeleton className="h-4 w-24" />
          </div>
        </div>
        <Skeleton className="h-48 w-full rounded-xl" />
        <Skeleton className="h-96 w-full rounded-xl" />
      </div>
    );
  }

  if (!template) {
    return (
      <div className="flex flex-col items-center justify-center py-24 text-center">
        <div className="rounded-full bg-muted p-5 mb-5">
          <Box className="size-10 text-muted-foreground" />
        </div>
        <h2 className="text-xl font-semibold text-foreground mb-2">Template not found</h2>
        <p className="text-muted-foreground mb-6 max-w-sm">
          This template may have been removed or is temporarily unavailable.
        </p>
        <Button variant="outline" onClick={() => navigate('/marketplace')} className="cursor-pointer">
          Browse Marketplace
        </Button>
      </div>
    );
  }

  const catColor = getCategoryColor(template.category);
  const services = template.compose_yaml ? parseServices(template.compose_yaml) : [];
  const stats = template.stats || {};

  return (
    <div className="space-y-8">
      {/* Back + Header */}
      <div className="flex items-start gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate('/marketplace')} className="mt-1 cursor-pointer">
          <ArrowLeft className="size-4" />
        </Button>
        <div className="flex items-start gap-4 flex-1 min-w-0">
          <TemplateIcon template={template} size="size-12" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2 flex-wrap">
              <h1 className="text-2xl font-bold tracking-tight">{template.name}</h1>
              {template.verified && <ShieldCheck className="size-5 text-blue-500" />}
              {template.stars !== undefined && (
                <span className="inline-flex items-center gap-1 text-sm text-muted-foreground">
                  <Star className="size-4 fill-amber-400 text-amber-400" />
                  {template.stars}
                </span>
              )}
            </div>
            <div className="flex items-center gap-2 mt-1.5 flex-wrap">
              <span className={cn('text-xs font-medium px-2 py-0.5 rounded-full', catColor.bg, catColor.text)}>
                {template.category}
              </span>
              {template.version && (
                <Badge variant="secondary" className="text-xs">v{template.version}</Badge>
              )}
              {template.vendor && (
                <span className="text-xs text-muted-foreground">by {template.vendor}</span>
              )}
            </div>
          </div>
          <Button onClick={() => document.getElementById('deploy-form')?.scrollIntoView({ behavior: 'smooth' })} className="shrink-0 cursor-pointer">
            <Rocket className="size-4" />
            Deploy
          </Button>
        </div>
      </div>

      {/* Content grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Main column */}
        <div className="lg:col-span-2 space-y-6">
          {/* Description */}
          {template.description && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">About</CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-sm text-muted-foreground leading-relaxed">{template.description}</p>
              </CardContent>
            </Card>
          )}

          {/* Resource requirements */}
          {template.min_resources && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Resource Requirements</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-3 gap-4">
                  {template.min_resources.memory_mb && (
                    <div className="flex items-center gap-2">
                      <div className="rounded-lg bg-blue-500/10 p-2">
                        <MemoryStick className="size-4 text-blue-500" />
                      </div>
                      <div>
                        <p className="text-xs text-muted-foreground">Memory</p>
                        <p className="text-sm font-semibold">{template.min_resources.memory_mb} MB</p>
                      </div>
                    </div>
                  )}
                  {template.min_resources.disk_mb && (
                    <div className="flex items-center gap-2">
                      <div className="rounded-lg bg-purple-500/10 p-2">
                        <HardDrive className="size-4 text-purple-500" />
                      </div>
                      <div>
                        <p className="text-xs text-muted-foreground">Disk</p>
                        <p className="text-sm font-semibold">{template.min_resources.disk_mb} MB</p>
                      </div>
                    </div>
                  )}
                  {template.min_resources.cpu_cores && (
                    <div className="flex items-center gap-2">
                      <div className="rounded-lg bg-amber-500/10 p-2">
                        <Cpu className="size-4 text-amber-500" />
                      </div>
                      <div>
                        <p className="text-xs text-muted-foreground">CPU</p>
                        <p className="text-sm font-semibold">{template.min_resources.cpu_cores} vCPU</p>
                      </div>
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          )}

          {/* Services (from compose) */}
          {services.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Services ({services.length})</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-2 gap-2">
                  {services.map((svc) => (
                    <div key={svc} className="flex items-center gap-2 rounded-md border px-3 py-2">
                      <div className="size-2 rounded-full bg-primary shrink-0" />
                      <code className="text-xs font-mono text-muted-foreground">{svc}</code>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}

          {/* Compose YAML */}
          {template.compose_yaml && (
            <Card>
              <CardHeader className="flex-row items-center justify-between space-y-0">
                <CardTitle className="text-base">Compose YAML</CardTitle>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => { void copyToClipboard(template.compose_yaml!, 'yaml'); }}
                  className="text-xs cursor-pointer"
                >
                  {copiedSection === 'yaml' ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
                  {copiedSection === 'yaml' ? 'Copied!' : 'Copy'}
                </Button>
              </CardHeader>
              <CardContent>
                <pre className="text-xs font-mono bg-muted rounded-lg p-4 overflow-x-auto">
                  {template.compose_yaml}
                </pre>
              </CardContent>
            </Card>
          )}
        </div>

        {/* Deploy sidebar */}
        <div className="lg:col-span-1">
          <div id="deploy-form" className="sticky top-6 space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Deploy {template.name}</CardTitle>
              </CardHeader>
              <CardContent>
                <DeployForm
                  template={template}
                  onDeployed={(appId) => navigate(`/apps/${appId}`)}
                />
              </CardContent>
            </Card>

            {/* Stats */}
            {(stats as { deploys?: number }).deploys && (
              <Card>
                <CardContent className="pt-6">
                  <div className="text-center">
                    <p className="text-3xl font-bold tabular-nums">{(stats as { deploys?: number }).deploys?.toLocaleString()}</p>
                    <p className="text-xs text-muted-foreground mt-1">total deployments</p>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
