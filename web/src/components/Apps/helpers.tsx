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
