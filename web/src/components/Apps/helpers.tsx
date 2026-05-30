import { STATUS_CONFIG, SOURCE_CONFIG } from './constants';
import { cn } from '@/lib/utils';

export { cn };

export function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function getStatusConfig(status: string) {
  return STATUS_CONFIG[status] || { variant: 'secondary' as const, dot: 'bg-slate-400', label: status };
}

export function getSourceConfig(sourceType: string) {
  const key = sourceType.toLowerCase();
  return SOURCE_CONFIG[key] || SOURCE_CONFIG['git'];
}

export function getAppStatusDot(status: string, animate = false) {
  const cfg = getStatusConfig(status);
  return (
    <span className="relative flex h-2.5 w-2.5 shrink-0">
      {animate && status === 'running' && (
        <span
          className={cn(
            'absolute inline-flex h-full w-full rounded-full opacity-75 animate-ping',
            cfg.dot
          )}
        />
      )}
      <span className={cn('relative inline-flex rounded-full h-2.5 w-2.5', cfg.dot)} />
    </span>
  );
}