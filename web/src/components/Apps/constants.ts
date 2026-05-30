import { GitBranch, Container, Store } from 'lucide-react';

export const STATUS_CONFIG: Record<string, {
  variant: 'default' | 'secondary' | 'destructive' | 'outline';
  dot: string;
  label: string;
}> = {
  running: { variant: 'default', dot: 'bg-emerald-500', label: 'Running' },
  stopped: { variant: 'secondary', dot: 'bg-red-500', label: 'Stopped' },
  deploying: { variant: 'outline', dot: 'bg-amber-500', label: 'Deploying' },
  building: { variant: 'outline', dot: 'bg-amber-500', label: 'Building' },
  failed: { variant: 'destructive', dot: 'bg-red-500', label: 'Failed' },
  pending: { variant: 'secondary', dot: 'bg-slate-400', label: 'Pending' },
  created: { variant: 'secondary', dot: 'bg-slate-400', label: 'Created' },
};

export const SOURCE_CONFIG: Record<string, { icon: typeof GitBranch; label: string; color: string }> = {
  git: { icon: GitBranch, label: 'Git', color: 'bg-orange-500/10 text-orange-600 dark:text-orange-400' },
  docker: { icon: Container, label: 'Docker', color: 'bg-blue-500/10 text-blue-600 dark:text-blue-400' },
  marketplace: { icon: Store, label: 'Marketplace', color: 'bg-violet-500/10 text-violet-600 dark:text-violet-400' },
};

export const FILTER_TABS = [
  { key: 'all', label: 'All' },
  { key: 'running', label: 'Running' },
  { key: 'stopped', label: 'Stopped' },
  { key: 'deploying', label: 'Deploying' },
] as const;

export type FilterKey = (typeof FILTER_TABS)[number]['key'];

export const EMPTY_MESSAGES: Record<string, { title: string; description: string }> = {
  all: {
    title: 'No applications yet',
    description: 'Deploy your first application to get started with DeployMonster.',
  },
  running: {
    title: 'No running applications',
    description: 'Start an application or deploy a new one to see it here.',
  },
  stopped: {
    title: 'No stopped applications',
    description: 'All your applications are currently running. Great job!',
  },
  deploying: {
    title: 'No active deployments',
    description: 'Trigger a new deployment to see it appear here.',
  },
  search: {
    title: 'No matching applications',
    description: 'Try adjusting your search or filter criteria.',
  },
};