import { cn } from '@/lib/utils';
import { Badge } from '@/components/ui/badge';
import { STATUS_CONFIG } from './constants';

interface StatusBadgeProps {
  status: string;
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const config = STATUS_CONFIG[status] || STATUS_CONFIG.pending;
  return (
    <Badge variant={config.variant} className="gap-1.5">
      <span className={cn('size-1.5 rounded-full', config.dotColor)} />
      {config.label}
    </Badge>
  );
}