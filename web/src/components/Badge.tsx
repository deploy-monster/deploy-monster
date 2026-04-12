interface BadgeProps {
  children: React.ReactNode;
  variant?: 'default' | 'success' | 'warning' | 'danger' | 'info' | 'neutral';
  size?: 'sm' | 'md';
  dot?: boolean;
}

const variantClasses = {
  default: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400',
  success: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
  warning: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400',
  danger: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400',
  info: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400',
  neutral: 'bg-neutral-100 text-neutral-800 dark:bg-neutral-700 dark:text-neutral-300',
};

const dotColors = {
  default: 'bg-blue-500',
  success: 'bg-green-500',
  warning: 'bg-yellow-500',
  danger: 'bg-red-500',
  info: 'bg-purple-500',
  neutral: 'bg-neutral-500',
};

const sizeClasses = {
  sm: 'px-2 py-0.5 text-xs',
  md: 'px-2.5 py-1 text-sm',
};

export function Badge({ children, variant = 'default', size = 'sm', dot }: BadgeProps) {
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full font-medium ${variantClasses[variant]} ${sizeClasses[size]}`}>
      {dot && <span className={`h-1.5 w-1.5 rounded-full ${dotColors[variant]}`} />}
      {children}
    </span>
  );
}

/** Status badge that maps common status strings to variants. */
export function StatusBadge({ status }: { status: string }) {
  const variant = statusToVariant(status);
  return <Badge variant={variant} dot>{status}</Badge>;
}

function statusToVariant(status: string): BadgeProps['variant'] {
  switch (status.toLowerCase()) {
    case 'running': case 'healthy': case 'active': case 'deployed': case 'success':
      return 'success';
    case 'stopped': case 'paused': case 'suspended': case 'inactive':
      return 'neutral';
    case 'building': case 'deploying': case 'pending': case 'starting':
      return 'warning';
    case 'failed': case 'error': case 'crashed': case 'unhealthy':
      return 'danger';
    default:
      return 'default';
  }
}
