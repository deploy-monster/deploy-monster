import { useState, useMemo } from 'react';
import { useNavigate, Link } from 'react-router';
import { useAuthStore } from '../stores/auth';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import {
  Rocket,
  Shield,
  Zap,
  Globe,
  Loader2,
  AlertCircle,
  Mail,
  Lock,
  User,
  Eye,
  EyeOff,
} from 'lucide-react';

// ---------------------------------------------------------------------------
// Feature list (matches Login.tsx)
// ---------------------------------------------------------------------------

const features = [
  {
    icon: Rocket,
    title: 'One-Click Deploys',
    description: 'Push to deploy with zero configuration. Git-native workflow.',
  },
  {
    icon: Shield,
    title: 'Enterprise Security',
    description: 'AES-256-GCM secrets vault, RBAC, and audit logging built in.',
  },
  {
    icon: Zap,
    title: 'Auto Scaling',
    description: 'Scale horizontally across nodes with automatic load balancing.',
  },
  {
    icon: Globe,
    title: 'Custom Domains & SSL',
    description: 'Automatic HTTPS certificates and DNS management via Cloudflare.',
  },
];

const trustedLogos = [
  { name: 'Startups', width: 'w-16' },
  { name: 'Agencies', width: 'w-20' },
  { name: 'Enterprise', width: 'w-18' },
  { name: 'SaaS Teams', width: 'w-16' },
];

// ---------------------------------------------------------------------------
// Password strength
// ---------------------------------------------------------------------------

function getPasswordStrength(password: string): { level: number; label: string; color: string } {
  if (!password) return { level: 0, label: '', color: '' };

  let score = 0;
  if (password.length >= 8) score++;
  if (password.length >= 12) score++;
  if (/[A-Z]/.test(password)) score++;
  if (/[a-z]/.test(password)) score++;
  if (/[0-9]/.test(password)) score++;
  if (/[^A-Za-z0-9]/.test(password)) score++;

  if (score <= 2) return { level: 1, label: 'Weak', color: 'bg-red-500' };
  if (score <= 4) return { level: 2, label: 'Fair', color: 'bg-amber-500' };
  return { level: 3, label: 'Strong', color: 'bg-emerald-500' };
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

export function Register() {
  const navigate = useNavigate();
  const register = useAuthStore((s) => s.register);
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const passwordStrength = useMemo(() => getPasswordStrength(password), [password]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (password !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    if (password.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }

    setLoading(true);
    try {
      await register(email, password, name);
      navigate('/');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Registration failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex">
      {/* Left panel - Branding (matches Login.tsx exactly) */}
      <div className="hidden lg:flex lg:w-1/2 relative overflow-hidden bg-gradient-to-b from-slate-900 via-emerald-950 to-slate-900">
        {/* Grid pattern overlay */}
        <div className="absolute inset-0">
          <svg className="absolute inset-0 w-full h-full" xmlns="http://www.w3.org/2000/svg">
            <defs>
              <pattern id="grid-reg" width="40" height="40" patternUnits="userSpaceOnUse">
                <path
                  d="M 40 0 L 0 0 0 40"
                  fill="none"
                  stroke="white"
                  strokeWidth="0.5"
                  opacity="0.07"
                />
              </pattern>
            </defs>
            <rect width="100%" height="100%" fill="url(#grid-reg)" />
          </svg>
        </div>

        {/* Glow orbs */}
        <div className="absolute top-1/4 left-1/3 w-96 h-96 bg-emerald-500/10 rounded-full blur-3xl" />
        <div className="absolute bottom-1/4 right-1/4 w-64 h-64 bg-emerald-400/5 rounded-full blur-3xl" />

        <div className="relative z-10 flex flex-col justify-between px-12 xl:px-20 py-12 w-full">
          {/* Top: Logo */}
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-xl bg-emerald-500/20 backdrop-blur-sm flex items-center justify-center text-emerald-400 font-bold text-xl border border-emerald-500/20 shadow-lg shadow-emerald-500/30">
              DM
            </div>
            <span className="text-white font-bold text-2xl tracking-tight">DeployMonster</span>
          </div>

          {/* Center: Hero content */}
          <div className="flex-1 flex flex-col justify-center -mt-8">
            <h1 className="text-4xl xl:text-5xl font-bold mb-4 leading-tight">
              <span className="bg-gradient-to-r from-white to-emerald-200 bg-clip-text text-transparent">
                Tame Your
              </span>
              <br />
              <span className="bg-gradient-to-r from-white to-emerald-200 bg-clip-text text-transparent">
                Deployments
              </span>
            </h1>
            <p className="text-slate-400 text-lg mb-10 max-w-md leading-relaxed">
              The self-hosted PaaS that gives you full control. Deploy, scale, and manage your
              applications with enterprise-grade tooling.
            </p>

            {/* Feature pills */}
            <div className="grid grid-cols-2 gap-3">
              {features.map((feature) => (
                <div
                  key={feature.title}
                  className="bg-white/5 backdrop-blur rounded-xl p-4 border border-white/5 hover:border-emerald-500/20 transition-colors"
                >
                  <div className="flex items-center gap-3 mb-2">
                    <div className="w-8 h-8 rounded-lg bg-emerald-500/10 flex items-center justify-center shrink-0">
                      <feature.icon className="w-4 h-4 text-emerald-400" />
                    </div>
                    <h3 className="text-white font-semibold text-sm">{feature.title}</h3>
                  </div>
                  <p className="text-slate-500 text-xs leading-relaxed pl-11">
                    {feature.description}
                  </p>
                </div>
              ))}
            </div>
          </div>

          {/* Bottom: Trust bar */}
          <div>
            <p className="text-slate-600 text-xs uppercase tracking-wider font-medium mb-4">
              Trusted by developers worldwide
            </p>
            <div className="flex items-center gap-6">
              {trustedLogos.map((logo) => (
                <div
                  key={logo.name}
                  className={cn(
                    'h-6 rounded bg-white/5 flex items-center justify-center px-3',
                    logo.width
                  )}
                >
                  <span className="text-slate-600 text-[10px] font-medium tracking-wide">
                    {logo.name.toUpperCase()}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Right panel - Register form */}
      <div className="flex-1 flex items-center justify-center bg-background px-4 sm:px-8">
        <div className="w-full max-w-sm">
          {/* Mobile logo */}
          <div className="flex items-center justify-center gap-3 mb-8 lg:hidden">
            <div className="w-10 h-10 rounded-xl bg-primary flex items-center justify-center text-primary-foreground font-bold text-lg shadow-lg shadow-primary/30">
              DM
            </div>
            <span className="font-bold text-xl text-foreground tracking-tight">DeployMonster</span>
          </div>

          <Card className="border-0 shadow-none lg:border lg:shadow-sm">
            <CardHeader className="text-center pb-2">
              <CardTitle className="text-2xl font-bold">Create your account</CardTitle>
              <CardDescription className="text-muted-foreground">
                Get started with DeployMonster
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={handleSubmit} className="space-y-4">
                {error && (
                  <div className="flex items-center gap-2 rounded-lg bg-destructive/10 border border-destructive/20 px-4 py-3 text-sm text-destructive">
                    <AlertCircle className="h-4 w-4 shrink-0" />
                    <span>{error}</span>
                  </div>
                )}

                <div className="space-y-2">
                  <Label htmlFor="name">Name</Label>
                  <div className="relative">
                    <User className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                      id="name"
                      type="text"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      placeholder="Your full name"
                      autoComplete="name"
                      autoFocus
                      className="pl-10"
                    />
                  </div>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="email">Email</Label>
                  <div className="relative">
                    <Mail className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                      id="email"
                      type="email"
                      required
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder="you@example.com"
                      autoComplete="email"
                      className="pl-10"
                    />
                  </div>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="password">Password</Label>
                  <div className="relative">
                    <Lock className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                      id="password"
                      type={showPassword ? 'text' : 'password'}
                      required
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      placeholder="Min. 8 characters"
                      autoComplete="new-password"
                      className="pl-10 pr-10"
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      tabIndex={-1}
                    >
                      {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                  </div>
                  {/* Password strength indicator */}
                  {password && (
                    <div className="space-y-1.5">
                      <div className="flex gap-1 h-1">
                        {[1, 2, 3].map((level) => (
                          <div
                            key={level}
                            className={cn(
                              'flex-1 rounded-full transition-colors duration-300',
                              passwordStrength.level >= level
                                ? passwordStrength.color
                                : 'bg-muted'
                            )}
                          />
                        ))}
                      </div>
                      <p className={cn(
                        'text-[11px] font-medium',
                        passwordStrength.level === 1 && 'text-red-500',
                        passwordStrength.level === 2 && 'text-amber-500',
                        passwordStrength.level === 3 && 'text-emerald-500',
                      )}>
                        {passwordStrength.label}
                      </p>
                    </div>
                  )}
                </div>

                <div className="space-y-2">
                  <Label htmlFor="confirm-password">Confirm password</Label>
                  <div className="relative">
                    <Lock className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                      id="confirm-password"
                      type={showConfirm ? 'text' : 'password'}
                      required
                      value={confirmPassword}
                      onChange={(e) => setConfirmPassword(e.target.value)}
                      placeholder="Repeat your password"
                      autoComplete="new-password"
                      className={cn(
                        'pl-10 pr-10',
                        confirmPassword && confirmPassword !== password && 'border-red-500 focus-visible:ring-red-500'
                      )}
                    />
                    <button
                      type="button"
                      onClick={() => setShowConfirm(!showConfirm)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      tabIndex={-1}
                    >
                      {showConfirm ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                  </div>
                  {confirmPassword && confirmPassword !== password && (
                    <p className="text-[11px] text-red-500">Passwords do not match</p>
                  )}
                </div>

                <Button type="submit" className="w-full cursor-pointer" size="lg" disabled={loading}>
                  {loading ? (
                    <>
                      <Loader2 className="h-4 w-4 animate-spin" />
                      Creating account...
                    </>
                  ) : (
                    'Create account'
                  )}
                </Button>

                <p className="text-center text-sm text-muted-foreground pt-2">
                  Already have an account?{' '}
                  <Link
                    to="/login"
                    className="font-medium text-primary hover:text-primary/80 transition-colors underline-offset-4 hover:underline"
                  >
                    Sign in
                  </Link>
                </p>
              </form>
            </CardContent>
          </Card>

          <p className="mt-8 text-center text-xs text-muted-foreground">
            Self-hosted PaaS by{' '}
            <span className="font-medium text-foreground/60">ECOSTACK</span>
          </p>
        </div>
      </div>
    </div>
  );
}
