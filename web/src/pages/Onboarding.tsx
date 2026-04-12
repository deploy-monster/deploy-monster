import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import {
  Rocket,
  Server,
  Globe,
  GitBranch,
  Check,
  ChevronRight,
  Loader2,
  Sparkles,
  ArrowRight,
  Shield,
  Zap,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

const steps = [
  { icon: Rocket,    title: 'Welcome',   desc: "Let's set up your platform" },
  { icon: Server,    title: 'Server',    desc: 'Detect your server' },
  { icon: Globe,     title: 'Domain',    desc: 'Set your platform domain' },
  { icon: GitBranch, title: 'Git',       desc: 'Connect a Git provider' },
  { icon: Check,     title: 'Done',      desc: "You're all set!" },
];

interface ServerCheckItem {
  label: string;
  value: string;
  status: 'pending' | 'checking' | 'ok';
}

const GIT_PROVIDERS = [
  {
    id: 'github',
    name: 'GitHub',
    color: 'bg-slate-800 dark:bg-white',
    textColor: 'text-white dark:text-slate-900',
    letter: 'GH',
    desc: 'Connect repositories from GitHub.com or GitHub Enterprise.',
  },
  {
    id: 'gitlab',
    name: 'GitLab',
    color: 'bg-orange-500',
    textColor: 'text-white',
    letter: 'GL',
    desc: 'Connect repositories from GitLab.com or self-hosted GitLab.',
  },
  {
    id: 'gitea',
    name: 'Gitea',
    color: 'bg-emerald-600',
    textColor: 'text-white',
    letter: 'GT',
    desc: 'Connect repositories from any Gitea instance.',
  },
] as const;

const quickActions = [
  { label: 'Deploy an app from the Marketplace', icon: Sparkles },
  { label: 'Connect your Git repository',        icon: GitBranch },
  { label: 'Deploy a Docker image',              icon: Rocket },
  { label: 'Invite your team members',           icon: Shield },
];

// ---------------------------------------------------------------------------
// Onboarding
// ---------------------------------------------------------------------------

export function Onboarding() {
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [config, setConfig] = useState({
    domain: '',
    gitProvider: '',
  });

  // Server detection animation state
  const [serverChecks, setServerChecks] = useState<ServerCheckItem[]>([
    { label: 'Hostname',      value: 'localhost',            status: 'pending' },
    { label: 'Docker Engine', value: 'Connected',            status: 'pending' },
    { label: 'SSL',           value: "Let's Encrypt (auto)", status: 'pending' },
    { label: 'Ports',         value: '80, 443, 8443',        status: 'pending' },
  ]);

  // Simulate detection when entering server step
  useEffect(() => {
    if (step !== 1) return;

    // Reset checks
    setServerChecks((prev) => prev.map((c) => ({ ...c, status: 'pending' as const })));

    const timers: ReturnType<typeof setTimeout>[] = [];
    serverChecks.forEach((_, i) => {
      // Start checking
      timers.push(setTimeout(() => {
        setServerChecks((prev) =>
          prev.map((c, j) => (j === i ? { ...c, status: 'checking' as const } : c))
        );
      }, i * 400));

      // Mark as ok
      timers.push(setTimeout(() => {
        setServerChecks((prev) =>
          prev.map((c, j) => (j === i ? { ...c, status: 'ok' as const } : c))
        );
      }, i * 400 + 600));
    });

    return () => timers.forEach(clearTimeout);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step]);

  const next = () => {
    if (step < steps.length - 1) {
      setStep(step + 1);
    } else {
      localStorage.setItem('onboarding_complete', 'true');
      navigate('/');
    }
  };

  const progress = ((step + 1) / steps.length) * 100;

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      {/* Background decorations */}
      <div className="pointer-events-none fixed inset-0 overflow-hidden">
        <div className="absolute -top-32 -right-32 size-96 rounded-full bg-primary/5 blur-3xl" />
        <div className="absolute -bottom-32 -left-32 size-80 rounded-full bg-primary/3 blur-3xl" />
      </div>

      <div className="relative z-10 w-full max-w-lg space-y-8">
        {/* Logo */}
        <div className="flex items-center justify-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-primary flex items-center justify-center text-primary-foreground font-bold text-lg shadow-lg shadow-primary/30">
            DM
          </div>
          <span className="font-bold text-xl text-foreground tracking-tight">DeployMonster</span>
        </div>

        {/* Progress bar */}
        <div className="space-y-3">
          <div className="flex justify-between items-center text-xs text-muted-foreground">
            <span>Step {step + 1} of {steps.length}</span>
            <span>{steps[step].title}</span>
          </div>
          <div className="h-2 rounded-full bg-muted overflow-hidden">
            <div
              className="h-full rounded-full bg-gradient-to-r from-primary to-primary/80 transition-all duration-500 ease-out"
              style={{ width: `${progress}%` }}
            />
          </div>
          {/* Step labels */}
          <div className="flex justify-between">
            {steps.map((s, i) => (
              <div key={i} className="flex flex-col items-center gap-1">
                <div className={cn(
                  'flex items-center justify-center size-7 rounded-full text-[10px] font-medium transition-all duration-300',
                  i < step && 'bg-primary text-primary-foreground',
                  i === step && 'bg-primary/10 text-primary ring-2 ring-primary/30',
                  i > step && 'bg-muted text-muted-foreground'
                )}>
                  {i < step ? <Check className="size-3.5" /> : i + 1}
                </div>
                <span className={cn(
                  'text-[10px] hidden sm:block',
                  i <= step ? 'text-foreground' : 'text-muted-foreground'
                )}>
                  {s.title}
                </span>
              </div>
            ))}
          </div>
        </div>

        {/* Step Content Card */}
        <Card className="group transition-all duration-200 hover:shadow-lg overflow-hidden">
          <CardContent className="text-center pt-8 pb-6">
            {/* ============================================================
                Step 0: Welcome
            ============================================================ */}
            {step === 0 && (
              <div className="space-y-5">
                <div className="mx-auto flex size-20 items-center justify-center rounded-2xl bg-gradient-to-br from-primary to-primary/80 text-primary-foreground shadow-xl shadow-primary/30">
                  <Rocket className="size-9" />
                </div>
                <div>
                  <h2 className="text-2xl font-bold tracking-tight bg-gradient-to-r from-foreground to-foreground/70 bg-clip-text text-transparent">
                    Welcome to DeployMonster
                  </h2>
                  <p className="text-muted-foreground mt-2 text-base max-w-sm mx-auto leading-relaxed">
                    Your self-hosted PaaS is ready. Let&apos;s configure a few things to get you started.
                  </p>
                </div>
                <div className="flex items-center justify-center gap-2 text-xs text-muted-foreground">
                  <Zap className="size-3.5 text-primary" />
                  Setup takes less than 2 minutes
                </div>
              </div>
            )}

            {/* ============================================================
                Step 1: Server Detection
            ============================================================ */}
            {step === 1 && (
              <div className="space-y-5">
                <div className="mx-auto flex size-16 items-center justify-center rounded-2xl bg-blue-500/10">
                  <Server className="size-7 text-blue-500" />
                </div>
                <div>
                  <h2 className="text-xl font-semibold tracking-tight">Server Detection</h2>
                  <p className="text-sm text-muted-foreground mt-1.5">
                    Detecting your server configuration automatically.
                  </p>
                </div>

                <Card className="border-muted bg-muted/20 shadow-none text-left">
                  <CardContent className="space-y-0 pt-4 pb-4">
                    {serverChecks.map(({ label, value, status }, i) => (
                      <div key={label}>
                        <div className="flex items-center justify-between py-2.5">
                          <span className="text-sm text-muted-foreground">{label}</span>
                          <div className="flex items-center gap-2">
                            <span className={cn(
                              'text-sm transition-opacity duration-300',
                              status === 'ok' ? 'text-foreground font-medium opacity-100' : 'opacity-0'
                            )}>
                              {value}
                            </span>
                            {status === 'pending' && (
                              <div className="size-4 rounded-full bg-muted" />
                            )}
                            {status === 'checking' && (
                              <Loader2 className="size-4 text-primary animate-spin" />
                            )}
                            {status === 'ok' && (
                              <div className="flex items-center justify-center size-5 rounded-full bg-emerald-500 animate-in fade-in zoom-in duration-300">
                                <Check className="size-3 text-white" />
                              </div>
                            )}
                          </div>
                        </div>
                        {i < serverChecks.length - 1 && (
                          <div className="border-b border-border/50" />
                        )}
                      </div>
                    ))}
                  </CardContent>
                </Card>

                {serverChecks.every((c) => c.status === 'ok') && (
                  <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
                    <Check className="size-3" />
                    All checks passed
                  </Badge>
                )}
              </div>
            )}

            {/* ============================================================
                Step 2: Domain
            ============================================================ */}
            {step === 2 && (
              <div className="space-y-5">
                <div className="mx-auto flex size-16 items-center justify-center rounded-2xl bg-purple-500/10">
                  <Globe className="size-7 text-purple-500" />
                </div>
                <div>
                  <h2 className="text-xl font-semibold tracking-tight">Platform Domain</h2>
                  <p className="text-sm text-muted-foreground mt-1.5">
                    Set the domain for accessing your dashboard and auto-generated subdomains.
                  </p>
                </div>
                <div className="space-y-2 text-left max-w-sm mx-auto">
                  <Label htmlFor="domain">Platform Domain</Label>
                  <Input
                    id="domain"
                    type="text"
                    value={config.domain}
                    onChange={(e) => setConfig({ ...config, domain: e.target.value })}
                    placeholder="deploy.example.com"
                    className="text-center"
                  />
                  <p className="text-xs text-muted-foreground text-center">
                    Apps will get subdomains like <code className="font-mono bg-muted/50 px-1 rounded text-[11px]">app-name.deploy.example.com</code>
                  </p>
                </div>
              </div>
            )}

            {/* ============================================================
                Step 3: Git Provider
            ============================================================ */}
            {step === 3 && (
              <div className="space-y-5">
                <div className="mx-auto flex size-16 items-center justify-center rounded-2xl bg-amber-500/10">
                  <GitBranch className="size-7 text-amber-500" />
                </div>
                <div>
                  <h2 className="text-xl font-semibold tracking-tight">Connect Git Provider</h2>
                  <p className="text-sm text-muted-foreground mt-1.5">
                    Connect a Git provider to enable auto-deploy from push.
                  </p>
                </div>
                <div className="grid grid-cols-1 gap-3 text-left">
                  {GIT_PROVIDERS.map((provider) => (
                    <button
                      key={provider.id}
                      onClick={() => setConfig({ ...config, gitProvider: provider.id })}
                      className={cn(
                        'flex items-center gap-4 rounded-xl border-2 p-4 transition-all duration-200 cursor-pointer text-left',
                        'hover:translate-y-[-1px] hover:shadow-md',
                        config.gitProvider === provider.id
                          ? 'border-primary bg-primary/5 ring-1 ring-primary/20'
                          : 'border-transparent bg-muted/30 hover:border-border'
                      )}
                    >
                      <div className={cn(
                        'flex items-center justify-center rounded-xl size-12 shrink-0 font-bold text-sm',
                        provider.color,
                        provider.textColor
                      )}>
                        {provider.letter}
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <p className="font-semibold text-foreground">{provider.name}</p>
                          {config.gitProvider === provider.id && (
                            <div className="flex items-center justify-center size-5 rounded-full bg-primary">
                              <Check className="size-3 text-primary-foreground" />
                            </div>
                          )}
                        </div>
                        <p className="text-xs text-muted-foreground mt-0.5">{provider.desc}</p>
                      </div>
                    </button>
                  ))}
                </div>
                <button
                  className="text-sm text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                  onClick={next}
                >
                  Skip for now
                </button>
              </div>
            )}

            {/* ============================================================
                Step 4: Done
            ============================================================ */}
            {step === 4 && (
              <div className="space-y-5">
                <div className="mx-auto flex size-20 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-emerald-600 text-white shadow-xl shadow-emerald-500/30">
                  <Check className="size-9" />
                </div>
                <div>
                  <h2 className="text-2xl font-bold tracking-tight">
                    <span className="bg-gradient-to-r from-emerald-500 to-primary bg-clip-text text-transparent">
                      You&apos;re All Set!
                    </span>
                  </h2>
                  <p className="text-muted-foreground mt-2 text-base max-w-sm mx-auto leading-relaxed">
                    Your DeployMonster platform is configured and ready to deploy applications.
                  </p>
                </div>

                <Card className="border-muted bg-muted/20 shadow-none text-left">
                  <CardContent className="pt-4 pb-4">
                    <p className="font-medium text-foreground text-sm mb-3">Quick actions:</p>
                    <div className="space-y-2">
                      {quickActions.map((action) => (
                        <div
                          key={action.label}
                          className="flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-background/50 transition-colors"
                        >
                          <div className="flex items-center justify-center size-7 rounded-lg bg-primary/10 shrink-0">
                            <action.icon className="size-3.5 text-primary" />
                          </div>
                          <span className="text-sm text-muted-foreground">{action.label}</span>
                          <ChevronRight className="size-3.5 text-muted-foreground/40 ml-auto" />
                        </div>
                      ))}
                    </div>
                  </CardContent>
                </Card>

                <div className="flex items-center justify-center gap-3">
                  <Sparkles className="size-4 text-amber-500" />
                  <span className="text-xs text-muted-foreground">
                    You can change all settings later from the Settings page
                  </span>
                </div>
              </div>
            )}

            {/* Next / Finish button */}
            <Button onClick={next} className="mt-6 w-full" size="lg">
              {step === steps.length - 1 ? (
                <>
                  Go to Dashboard
                  <ArrowRight className="size-4" />
                </>
              ) : (
                <>
                  Continue
                  <ArrowRight className="size-4" />
                </>
              )}
            </Button>
          </CardContent>
        </Card>

        {/* Footer */}
        <p className="text-center text-xs text-muted-foreground">
          Self-hosted PaaS by{' '}
          <span className="font-medium text-foreground/60">ECOSTACK</span>
        </p>
      </div>
    </div>
  );
}
