import { useState } from 'react';
import { useNavigate } from 'react-router';
import {
  Rocket,
  GitBranch,
  Container,
  Store,
  ArrowRight,
  ArrowLeft,
  Check,
  AlertCircle,
  Loader2,
  Box,
  Globe,
  Layers,
  Hash,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { api } from '@/api/client';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Separator } from '@/components/ui/separator';

// ---------------------------------------------------------------------------
// Types & Config
// ---------------------------------------------------------------------------

type SourceType = 'git' | 'image' | 'marketplace';

const stepLabels = ['Source', 'Configure', 'Review'];

const sourceOptions: { type: SourceType; icon: typeof GitBranch; label: string; desc: string; color: string; bgColor: string }[] = [
  {
    type: 'git',
    icon: GitBranch,
    label: 'Git Repository',
    desc: 'Deploy from GitHub, GitLab, or Gitea. Auto-deploy on push.',
    color: 'text-emerald-500',
    bgColor: 'bg-emerald-500/10',
  },
  {
    type: 'image',
    icon: Container,
    label: 'Docker Image',
    desc: 'Deploy a pre-built image from GHCR or a private registry.',
    color: 'text-blue-500',
    bgColor: 'bg-blue-500/10',
  },
  {
    type: 'marketplace',
    icon: Store,
    label: 'Marketplace',
    desc: 'One-click deploy from curated application templates.',
    color: 'text-purple-500',
    bgColor: 'bg-purple-500/10',
  },
];

const REVIEW_ICONS: Record<string, typeof Box> = {
  Name:   Box,
  Source: Layers,
  URL:    Globe,
  Branch: GitBranch,
  Port:   Hash,
};

// ---------------------------------------------------------------------------
// DeployWizard
// ---------------------------------------------------------------------------

export function DeployWizard() {
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [sourceType, setSourceType] = useState<SourceType | null>(null);
  const [config, setConfig] = useState({
    name: '',
    sourceURL: '',
    branch: 'main',
    port: '3000',
  });
  const [deploying, setDeploying] = useState(false);
  const [error, setError] = useState('');

  const handleDeploy = async () => {
    setError('');
    setDeploying(true);
    try {
      const app = await api.post<{ id: string }>('/apps', {
        name: config.name,
        source_type: sourceType || 'image',
        source_url: config.sourceURL,
        branch: config.branch,
      });
      navigate(`/apps/${app.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deploy failed');
      setDeploying(false);
    }
  };

  const reviewRows = [
    { label: 'Name', value: config.name },
    { label: 'Source', value: sourceType },
    ...(config.sourceURL ? [{ label: 'URL', value: config.sourceURL }] : []),
    ...(sourceType === 'git' ? [{ label: 'Branch', value: config.branch }] : []),
    { label: 'Port', value: config.port },
  ];

  return (
    <div className="mx-auto max-w-2xl space-y-8">
      {/* Page header */}
      <div>
        <div className="flex items-center gap-2 mb-2">
          <Rocket className="size-5 text-primary" />
          <Badge variant="secondary" className="text-xs font-normal">
            New Deployment
          </Badge>
        </div>
        <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
          Deploy New Application
        </h1>
        <p className="text-muted-foreground mt-1.5 text-sm">
          Follow the steps to configure and deploy your application.
        </p>
      </div>

      {/* Step progress with connected line */}
      <div className="flex items-center">
        {stepLabels.map((label, i) => (
          <div key={label} className="flex items-center flex-1 last:flex-none">
            <div className="flex items-center gap-2.5">
              <div
                className={cn(
                  'flex items-center justify-center size-9 rounded-full text-sm font-medium transition-all duration-300',
                  i < step && 'bg-primary text-primary-foreground shadow-md shadow-primary/30',
                  i === step && 'border-2 border-primary bg-primary/10 text-primary ring-4 ring-primary/10',
                  i > step && 'bg-muted text-muted-foreground'
                )}
              >
                {i < step ? <Check className="size-4" /> : i + 1}
              </div>
              <span
                className={cn(
                  'text-sm font-medium',
                  i <= step ? 'text-foreground' : 'text-muted-foreground'
                )}
              >
                {label}
              </span>
            </div>
            {i < stepLabels.length - 1 && (
              <div className={cn(
                'flex-1 h-px mx-4 transition-colors duration-500',
                i < step ? 'bg-primary' : 'bg-border'
              )} />
            )}
          </div>
        ))}
      </div>

      {/* ================================================================
          Step 1: Source
      ================================================================ */}
      {step === 0 && (
        <div className="space-y-5">
          <div>
            <h2 className="font-semibold text-foreground">Choose deployment source</h2>
            <p className="text-sm text-muted-foreground mt-1">
              Select how you want to deploy your application.
            </p>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            {sourceOptions.map(({ type, icon: Icon, label, desc, color, bgColor }) => (
              <Card
                key={type}
                className={cn(
                  'cursor-pointer transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md',
                  sourceType === type
                    ? 'ring-2 ring-primary border-primary shadow-md'
                    : 'hover:ring-2 hover:ring-primary/20'
                )}
                onClick={() => setSourceType(type)}
              >
                <CardContent className="pt-6 pb-5 text-center">
                  <div className={cn(
                    'mx-auto flex items-center justify-center rounded-xl size-14 mb-4',
                    sourceType === type ? 'bg-primary/10' : bgColor
                  )}>
                    <Icon className={cn(
                      'size-7',
                      sourceType === type ? 'text-primary' : color
                    )} />
                  </div>
                  <p className="font-semibold text-foreground">{label}</p>
                  <p className="mt-1.5 text-xs text-muted-foreground leading-relaxed">{desc}</p>
                  {sourceType === type && (
                    <div className="mt-3 flex items-center justify-center">
                      <div className="flex items-center justify-center size-5 rounded-full bg-primary">
                        <Check className="size-3 text-primary-foreground" />
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            ))}
          </div>
          <div className="flex justify-end">
            <Button onClick={() => sourceType && setStep(1)} disabled={!sourceType} size="lg">
              Next
              <ArrowRight className="size-4" />
            </Button>
          </div>
        </div>
      )}

      {/* ================================================================
          Step 2: Configure
      ================================================================ */}
      {step === 1 && (
        <div className="space-y-5">
          <div>
            <h2 className="font-semibold text-foreground">Configure your application</h2>
            <p className="text-sm text-muted-foreground mt-1">
              Provide the details needed to set up your deployment.
            </p>
          </div>
          <Card>
            <CardHeader>
              <CardTitle className="text-base flex items-center gap-2">
                <Box className="size-4 text-primary" />
                Application Details
              </CardTitle>
              <CardDescription>
                Give your application a name and configure the source.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="app-name">Application Name</Label>
                <Input
                  id="app-name"
                  type="text"
                  value={config.name}
                  onChange={(e) => setConfig({ ...config, name: e.target.value })}
                  placeholder="my-awesome-app"
                />
                <p className="text-[11px] text-muted-foreground">
                  Lowercase letters, numbers, and hyphens only.
                </p>
              </div>

              {sourceType === 'image' && (
                <div className="space-y-1.5">
                  <Label htmlFor="docker-image">Docker Image</Label>
                  <Input
                    id="docker-image"
                    type="text"
                    value={config.sourceURL}
                    onChange={(e) => setConfig({ ...config, sourceURL: e.target.value })}
                    placeholder="ghcr.io/org/image:latest"
                  />
                </div>
              )}

              {sourceType === 'git' && (
                <>
                  <div className="space-y-1.5">
                    <Label htmlFor="repo-url">Repository URL</Label>
                    <Input
                      id="repo-url"
                      type="text"
                      value={config.sourceURL}
                      onChange={(e) => setConfig({ ...config, sourceURL: e.target.value })}
                      placeholder="https://github.com/user/repo.git"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="branch">Branch</Label>
                    <Input
                      id="branch"
                      type="text"
                      value={config.branch}
                      onChange={(e) => setConfig({ ...config, branch: e.target.value })}
                      placeholder="main"
                    />
                  </div>
                </>
              )}

              <Separator />

              <div className="space-y-1.5">
                <Label htmlFor="port">Exposed Port</Label>
                <Input
                  id="port"
                  type="text"
                  value={config.port}
                  onChange={(e) => setConfig({ ...config, port: e.target.value })}
                  placeholder="3000"
                  className="max-w-[120px]"
                />
                <p className="text-[11px] text-muted-foreground">
                  The port your application listens on inside the container.
                </p>
              </div>
            </CardContent>
          </Card>
          <div className="flex justify-between">
            <Button variant="outline" onClick={() => setStep(0)} size="lg">
              <ArrowLeft className="size-4" />
              Back
            </Button>
            <Button onClick={() => config.name && setStep(2)} disabled={!config.name} size="lg">
              Next
              <ArrowRight className="size-4" />
            </Button>
          </div>
        </div>
      )}

      {/* ================================================================
          Step 3: Review & Deploy
      ================================================================ */}
      {step === 2 && !deploying && (
        <div className="space-y-5">
          <div>
            <h2 className="font-semibold text-foreground">Review and deploy</h2>
            <p className="text-sm text-muted-foreground mt-1">
              Verify your configuration before deploying.
            </p>
          </div>
          <Card>
            <CardHeader>
              <CardTitle className="text-base flex items-center gap-2">
                <Layers className="size-4 text-primary" />
                Deployment Summary
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-0 text-sm">
              {reviewRows.map(({ label, value }, i) => {
                const RowIcon = REVIEW_ICONS[label] || Box;
                return (
                  <div key={label}>
                    <div className="flex items-center justify-between py-3">
                      <div className="flex items-center gap-2.5">
                        <div className="flex items-center justify-center size-7 rounded-lg bg-muted">
                          <RowIcon className="size-3.5 text-muted-foreground" />
                        </div>
                        <span className="text-muted-foreground">{label}</span>
                      </div>
                      <span className="font-medium text-foreground max-w-64 truncate">
                        {value}
                      </span>
                    </div>
                    {i < reviewRows.length - 1 && <Separator />}
                  </div>
                );
              })}
            </CardContent>
          </Card>

          {error && (
            <div className="flex items-center gap-2 rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              <AlertCircle className="size-4 shrink-0" />
              {error}
            </div>
          )}

          <div className="flex justify-between">
            <Button variant="outline" onClick={() => setStep(1)} size="lg">
              <ArrowLeft className="size-4" />
              Back
            </Button>
            <Button
              onClick={handleDeploy}
              size="lg"
              className="bg-gradient-to-r from-primary to-primary/90 shadow-lg shadow-primary/25 hover:shadow-xl hover:shadow-primary/30 transition-all"
            >
              <Rocket className="size-4" />
              Deploy Application
            </Button>
          </div>
        </div>
      )}

      {/* Deploying state */}
      {step === 2 && deploying && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="relative mb-6">
            <div className="flex items-center justify-center size-20 rounded-2xl bg-primary/10">
              <Loader2 className="size-9 text-primary animate-spin" />
            </div>
            <div className="absolute inset-0 rounded-2xl bg-primary/5 animate-ping" />
          </div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
            Deploying your application...
          </h2>
          <p className="text-sm text-muted-foreground max-w-sm">
            Setting up containers, configuring networking, and provisioning your application.
            This usually takes less than a minute.
          </p>
          <div className="mt-6 flex items-center gap-2">
            <div className="size-1.5 rounded-full bg-primary animate-bounce" style={{ animationDelay: '0ms' }} />
            <div className="size-1.5 rounded-full bg-primary animate-bounce" style={{ animationDelay: '150ms' }} />
            <div className="size-1.5 rounded-full bg-primary animate-bounce" style={{ animationDelay: '300ms' }} />
          </div>
        </div>
      )}
    </div>
  );
}
