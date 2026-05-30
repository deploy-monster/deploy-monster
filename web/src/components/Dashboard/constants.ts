import { Rocket, Plus, CheckCircle2, XCircle, AlertTriangle, Zap, Info, TrendingUp, Database, Activity, Container, Globe, GitBranch, Store } from 'lucide-react';
import type { DashboardStats } from '@/api/dashboard';

// ---------------------------------------------------------------------------
// Status config
// ---------------------------------------------------------------------------

export const STATUS_CONFIG: Record<string, {
  variant: 'default' | 'secondary' | 'destructive' | 'outline';
  dotColor: string;
  label: string;
}> = {
  running:   { variant: 'default',     dotColor: 'bg-emerald-500', label: 'Running' },
  stopped:   { variant: 'secondary',   dotColor: 'bg-muted-foreground', label: 'Stopped' },
  deploying: { variant: 'outline',     dotColor: 'bg-amber-500',   label: 'Deploying' },
  building:  { variant: 'outline',     dotColor: 'bg-amber-500',   label: 'Building' },
  failed:    { variant: 'destructive', dotColor: 'bg-destructive', label: 'Failed' },
  pending:   { variant: 'secondary',   dotColor: 'bg-muted-foreground', label: 'Pending' },
};

// ---------------------------------------------------------------------------
// Activity config
// ---------------------------------------------------------------------------

export const ACTIVITY_COLORS: Record<string, string> = {
  deploy:  'bg-emerald-500',
  create:  'bg-blue-500',
  start:   'bg-emerald-500',
  stop:    'bg-amber-500',
  delete:  'bg-destructive',
  restart: 'bg-cyan-500',
  error:   'bg-destructive',
  update:  'bg-blue-500',
  scale:   'bg-purple-500',
};

export const ACTIVITY_ICONS: Record<string, typeof Rocket> = {
  deploy:  Rocket,
  create:  Plus,
  start:   CheckCircle2,
  stop:    XCircle,
  delete:  AlertTriangle,
  restart: Zap,
  error:   AlertTriangle,
  update:  Info,
  scale:   TrendingUp,
};

// ---------------------------------------------------------------------------
// Stat card definitions
// ---------------------------------------------------------------------------

export interface StatCardDef {
  key: string;
  icon: typeof Rocket;
  label: string;
  bgColor: string;
  iconColor: string;
  getValue: (s: DashboardStats) => number;
  getTrend: (s: DashboardStats) => { value: string; positive: boolean } | null;
}

export const STAT_CARDS: StatCardDef[] = [
  {
    key: 'apps',
    icon: Rocket,
    label: 'Applications',
    bgColor: 'bg-emerald-500/10',
    iconColor: 'text-emerald-500',
    getValue: (s) => s.apps.total,
    getTrend: () => null,
  },
  {
    key: 'running',
    icon: Activity,
    label: 'Running',
    bgColor: 'bg-blue-500/10',
    iconColor: 'text-blue-500',
    getValue: (s) => s.containers.running,
    getTrend: (s) => {
      if (s.containers.total === 0) return null;
      const pct = Math.round((s.containers.running / s.containers.total) * 100);
      return { value: `${pct}% uptime`, positive: pct >= 80 };
    },
  },
  {
    key: 'containers',
    icon: Container,
    label: 'Containers',
    bgColor: 'bg-purple-500/10',
    iconColor: 'text-purple-500',
    getValue: (s) => s.containers.total,
    getTrend: (s) => (s.containers.stopped > 0 ? { value: `${s.containers.stopped} stopped`, positive: false } : null),
  },
  {
    key: 'domains',
    icon: Globe,
    label: 'Domains',
    bgColor: 'bg-amber-500/10',
    iconColor: 'text-amber-500',
    getValue: (s) => s.domains,
    getTrend: () => null,
  },
  {
    key: 'projects',
    icon: Database,
    label: 'Projects',
    bgColor: 'bg-cyan-500/10',
    iconColor: 'text-cyan-500',
    getValue: (s) => s.projects,
    getTrend: () => null,
  },
];

// ---------------------------------------------------------------------------
// Quick actions
// ---------------------------------------------------------------------------

export const QUICK_ACTIONS = [
  {
    icon: GitBranch,
    title: 'Deploy from Git',
    description: 'Connect a GitHub, GitLab, or Gitea repository and deploy automatically.',
    href: '/apps/new?source=git',
    color: 'text-emerald-500',
    bgColor: 'bg-emerald-500/10',
  },
  {
    icon: Container,
    title: 'Deploy Docker Image',
    description: 'Run any Docker image from GHCR, or a private registry.',
    href: '/apps/new?source=docker',
    color: 'text-blue-500',
    bgColor: 'bg-blue-500/10',
  },
  {
    icon: Store,
    title: 'Browse Marketplace',
    description: 'One-click deploy popular databases, tools, and applications.',
    href: '/marketplace',
    color: 'text-purple-500',
    bgColor: 'bg-purple-500/10',
  },
];