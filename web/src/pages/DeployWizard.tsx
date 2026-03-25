import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Rocket, GitBranch, Container, Store, ArrowRight, ArrowLeft, Check } from 'lucide-react';
import { cn } from '@/lib/utils';
import { api } from '@/api/client';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Separator } from '@/components/ui/separator';

type SourceType = 'git' | 'image' | 'marketplace';

const stepLabels = ['Source', 'Configure', 'Review'];

const sourceOptions: { type: SourceType; icon: typeof GitBranch; label: string; desc: string }[] = [
  { type: 'git', icon: GitBranch, label: 'Git Repository', desc: 'Deploy from GitHub, GitLab, etc.' },
  { type: 'image', icon: Container, label: 'Docker Image', desc: 'Deploy a pre-built image' },
  { type: 'marketplace', icon: Store, label: 'Marketplace', desc: 'One-click app template' },
];

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
  ];

  return (
    <div className="mx-auto max-w-2xl space-y-8">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-semibold text-foreground">Deploy New Application</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Follow the steps to deploy your application
        </p>
      </div>

      {/* Step progress */}
      <div className="flex items-center justify-between">
        {stepLabels.map((label, i) => (
          <div key={label} className="flex items-center gap-2">
            <div
              className={cn(
                'flex h-8 w-8 items-center justify-center rounded-full text-sm font-medium',
                i < step && 'bg-primary text-primary-foreground',
                i === step && 'border-2 border-primary bg-primary/10 text-primary',
                i > step && 'bg-muted text-muted-foreground'
              )}
            >
              {i < step ? <Check size={16} /> : i + 1}
            </div>
            <span
              className={cn(
                'text-sm',
                i <= step ? 'text-foreground' : 'text-muted-foreground'
              )}
            >
              {label}
            </span>
            {i < stepLabels.length - 1 && <div className="mx-2 h-px w-16 bg-border" />}
          </div>
        ))}
      </div>

      {/* Step 1: Source */}
      {step === 0 && (
        <div className="space-y-4">
          <h2 className="font-medium text-foreground">Choose deployment source</h2>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            {sourceOptions.map(({ type, icon: Icon, label, desc }) => (
              <Card
                key={type}
                className={cn(
                  'cursor-pointer transition-all hover:border-primary/30',
                  sourceType === type
                    ? 'ring-2 ring-primary border-primary'
                    : ''
                )}
                onClick={() => setSourceType(type)}
              >
                <CardContent className="p-4">
                  <Icon
                    size={24}
                    className={cn(
                      sourceType === type ? 'text-primary' : 'text-muted-foreground'
                    )}
                  />
                  <p className="mt-2 font-medium text-foreground">{label}</p>
                  <p className="mt-1 text-xs text-muted-foreground">{desc}</p>
                </CardContent>
              </Card>
            ))}
          </div>
          <div className="flex justify-end">
            <Button onClick={() => sourceType && setStep(1)} disabled={!sourceType}>
              Next <ArrowRight size={16} />
            </Button>
          </div>
        </div>
      )}

      {/* Step 2: Configure */}
      {step === 1 && (
        <div className="space-y-4">
          <h2 className="font-medium text-foreground">Configure your application</h2>
          <Card>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="app-name">Application Name</Label>
                <Input
                  id="app-name"
                  type="text"
                  value={config.name}
                  onChange={(e) => setConfig({ ...config, name: e.target.value })}
                  placeholder="my-awesome-app"
                />
              </div>

              {sourceType === 'image' && (
                <div className="space-y-2">
                  <Label htmlFor="docker-image">Docker Image</Label>
                  <Input
                    id="docker-image"
                    type="text"
                    value={config.sourceURL}
                    onChange={(e) => setConfig({ ...config, sourceURL: e.target.value })}
                    placeholder="nginx:latest"
                  />
                </div>
              )}

              {sourceType === 'git' && (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="repo-url">Repository URL</Label>
                    <Input
                      id="repo-url"
                      type="text"
                      value={config.sourceURL}
                      onChange={(e) => setConfig({ ...config, sourceURL: e.target.value })}
                      placeholder="https://github.com/user/repo.git"
                    />
                  </div>
                  <div className="space-y-2">
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
            </CardContent>
          </Card>
          <div className="flex justify-between">
            <Button variant="outline" onClick={() => setStep(0)}>
              <ArrowLeft size={16} /> Back
            </Button>
            <Button onClick={() => config.name && setStep(2)} disabled={!config.name}>
              Next <ArrowRight size={16} />
            </Button>
          </div>
        </div>
      )}

      {/* Step 3: Review */}
      {step === 2 && (
        <div className="space-y-4">
          <h2 className="font-medium text-foreground">Review and deploy</h2>
          <Card>
            <CardContent className="space-y-0 text-sm">
              {reviewRows.map(({ label, value }, i) => (
                <div key={label}>
                  <div className="flex justify-between py-3">
                    <span className="text-muted-foreground">{label}</span>
                    <span className="font-medium text-foreground max-w-64 truncate">
                      {value}
                    </span>
                  </div>
                  {i < reviewRows.length - 1 && <Separator />}
                </div>
              ))}
            </CardContent>
          </Card>

          {error && (
            <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          )}

          <div className="flex justify-between">
            <Button variant="outline" onClick={() => setStep(1)}>
              <ArrowLeft size={16} /> Back
            </Button>
            <Button onClick={handleDeploy} disabled={deploying} size="lg">
              <Rocket size={16} />
              {deploying ? 'Deploying...' : 'Deploy Application'}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
