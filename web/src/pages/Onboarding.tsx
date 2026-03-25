import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Rocket, Server, Globe, GitBranch, Check, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle, CardDescription } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

const steps = [
  { icon: Rocket, title: 'Welcome', desc: "Let's set up your platform" },
  { icon: Server, title: 'Server', desc: 'Configure your server' },
  { icon: Globe, title: 'Domain', desc: 'Set your platform domain' },
  { icon: GitBranch, title: 'Git', desc: 'Connect a Git provider' },
  { icon: Check, title: 'Done', desc: "You're all set!" },
];

const serverInfo = [
  { label: 'Hostname', value: 'localhost', className: 'text-foreground font-mono' },
  { label: 'Docker', value: 'Connected', className: 'text-emerald-500' },
  { label: 'SSL', value: "Auto (Let's Encrypt)", className: 'text-foreground' },
  { label: 'Ports', value: '80, 443, 8443', className: 'text-foreground' },
];

const gitProviders = ['GitHub', 'GitLab', 'Gitea'] as const;

const quickActions = [
  'Deploy an app from the Marketplace',
  'Connect your Git repository',
  'Deploy a Docker image',
  'Invite your team members',
];

export function Onboarding() {
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [config, setConfig] = useState({
    domain: '',
    gitProvider: '',
  });

  const next = () => {
    if (step < steps.length - 1) {
      setStep(step + 1);
    } else {
      localStorage.setItem('onboarding_complete', 'true');
      navigate('/');
    }
  };

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      <div className="w-full max-w-lg space-y-8">
        {/* Progress dots */}
        <div className="flex justify-center gap-2">
          {steps.map((_, i) => (
            <div
              key={i}
              className={cn(
                'h-2.5 w-2.5 rounded-full transition-colors',
                i <= step ? 'bg-primary' : 'bg-muted'
              )}
            />
          ))}
        </div>

        {/* Step content */}
        <Card>
          <CardContent className="text-center">
            {step === 0 && (
              <div className="space-y-4">
                <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
                  <Rocket size={32} />
                </div>
                <CardTitle className="text-2xl">Welcome to DeployMonster</CardTitle>
                <CardDescription className="text-base">
                  Your self-hosted PaaS is ready. Let's configure a few things to get you started.
                </CardDescription>
              </div>
            )}

            {step === 1 && (
              <div className="space-y-4">
                <CardTitle className="text-xl">Server Configuration</CardTitle>
                <CardDescription>
                  Your server is already running. Here's what we detected:
                </CardDescription>
                <Card className="border-muted bg-muted/30 py-4 shadow-none">
                  <CardContent className="space-y-2 text-sm">
                    {serverInfo.map(({ label, value, className }) => (
                      <div key={label} className="flex justify-between">
                        <span className="text-muted-foreground">{label}</span>
                        <span className={className}>{value}</span>
                      </div>
                    ))}
                  </CardContent>
                </Card>
              </div>
            )}

            {step === 2 && (
              <div className="space-y-4">
                <CardTitle className="text-xl">Platform Domain</CardTitle>
                <CardDescription>
                  Set your platform's domain for accessing the dashboard and auto-subdomains.
                </CardDescription>
                <div className="space-y-2">
                  <Label htmlFor="domain" className="sr-only">
                    Platform Domain
                  </Label>
                  <Input
                    id="domain"
                    type="text"
                    value={config.domain}
                    onChange={(e) => setConfig({ ...config, domain: e.target.value })}
                    placeholder="deploy.example.com"
                    className="text-center"
                  />
                  <p className="text-xs text-muted-foreground">
                    Apps will get subdomains like app-name.deploy.example.com
                  </p>
                </div>
              </div>
            )}

            {step === 3 && (
              <div className="space-y-4">
                <CardTitle className="text-xl">Connect Git Provider</CardTitle>
                <CardDescription>
                  Connect a Git provider to enable auto-deploy from push.
                </CardDescription>
                <div className="grid grid-cols-3 gap-3">
                  {gitProviders.map((provider) => (
                    <Button
                      key={provider}
                      variant="outline"
                      className={cn(
                        'h-auto py-4 text-sm font-medium',
                        config.gitProvider === provider.toLowerCase() &&
                          'ring-2 ring-primary border-primary bg-primary/5 text-primary'
                      )}
                      onClick={() => setConfig({ ...config, gitProvider: provider.toLowerCase() })}
                    >
                      {provider}
                    </Button>
                  ))}
                </div>
                <button
                  className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                  onClick={next}
                >
                  Skip for now
                </button>
              </div>
            )}

            {step === 4 && (
              <div className="space-y-4">
                <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
                  <Check size={32} />
                </div>
                <CardTitle className="text-xl">You're All Set!</CardTitle>
                <CardDescription className="text-base">
                  Your DeployMonster platform is configured and ready to deploy.
                </CardDescription>
                <Card className="border-muted bg-muted/30 py-4 shadow-none text-left">
                  <CardContent className="space-y-2 text-sm">
                    <p className="font-medium text-foreground">Quick actions:</p>
                    <ul className="space-y-1 text-muted-foreground">
                      {quickActions.map((action) => (
                        <li key={action} className="flex items-center gap-2">
                          <ChevronRight size={14} className="text-primary shrink-0" />
                          {action}
                        </li>
                      ))}
                    </ul>
                  </CardContent>
                </Card>
              </div>
            )}

            {/* Next / Finish button */}
            <Button onClick={next} className="mt-6 w-full" size="lg">
              {step === steps.length - 1 ? 'Go to Dashboard' : 'Continue'}
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
