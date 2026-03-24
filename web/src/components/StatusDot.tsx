interface StatusDotProps {
  status: 'running' | 'stopped' | 'building' | 'failed' | 'unknown';
  label?: string;
  pulse?: boolean;
}

const dotColors = {
  running: 'bg-green-500',
  stopped: 'bg-neutral-400',
  building: 'bg-yellow-500',
  failed: 'bg-red-500',
  unknown: 'bg-neutral-300',
};

export function StatusDot({ status, label, pulse }: StatusDotProps) {
  const color = dotColors[status] || dotColors.unknown;
  const shouldPulse = pulse ?? (status === 'running' || status === 'building');

  return (
    <span className="inline-flex items-center gap-2">
      <span className="relative flex h-2.5 w-2.5">
        {shouldPulse && (
          <span className={`absolute inline-flex h-full w-full animate-ping rounded-full opacity-75 ${color}`} />
        )}
        <span className={`relative inline-flex h-2.5 w-2.5 rounded-full ${color}`} />
      </span>
      {label && (
        <span className="text-sm text-neutral-600 dark:text-neutral-400">{label}</span>
      )}
    </span>
  );
}
