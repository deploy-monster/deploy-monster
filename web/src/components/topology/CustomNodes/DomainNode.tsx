import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { Globe, Lock, MoreHorizontal } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { DomainNodeData } from '@/types/topology';

const statusColors: Record<string, string> = {
  pending: 'bg-gray-400',
  configuring: 'bg-yellow-400',
  active: 'bg-green-400',
  error: 'bg-red-400',
};

type DomainNodeProps = NodeProps;

function DomainNodeComponent({ data, selected }: DomainNodeProps) {
  const nodeData = data as DomainNodeData;
  const status = nodeData.status || 'pending';

  return (
    <div
      className={cn(
        'min-w-[160px] rounded-lg border-2 bg-card shadow-md transition-all',
        selected ? 'border-purple-500 ring-2 ring-purple-500/20' : 'border-purple-500/50',
        'hover:border-purple-500 hover:shadow-lg'
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!w-3 !h-3 !border-2 !border-purple-500 !bg-white"
      />

      {/* Header */}
      <div className="flex items-center gap-2 rounded-t-md bg-purple-500/10 px-3 py-2">
        <div className="flex h-8 w-8 items-center justify-center rounded-md bg-purple-500 text-white">
          <Globe className="h-4 w-4" />
        </div>
        <div className="flex-1 truncate">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-foreground">{nodeData.name}</span>
            <span className={cn('h-2 w-2 rounded-full', statusColors[status])} />
          </div>
        </div>
      </div>

      {/* Body */}
      <div className="space-y-1 px-3 py-2 text-xs">
        {nodeData.fqdn ? (
          <span className="block truncate font-mono text-purple-600">{nodeData.fqdn}</span>
        ) : (
          <span className="text-muted-foreground italic">No domain configured</span>
        )}
        {nodeData.sslEnabled && (
          <div className="flex items-center gap-1 text-green-600">
            <Lock className="h-3 w-3" />
            <span>SSL enabled</span>
          </div>
        )}
      </div>

      {/* Footer */}
      <div className="flex items-center justify-end rounded-b-md border-t bg-muted/30 px-2 py-1">
        <button className="rounded p-1 hover:bg-muted">
          <MoreHorizontal className="h-3 w-3 text-muted-foreground" />
        </button>
      </div>

      <Handle
        type="source"
        position={Position.Right}
        className="!w-3 !h-3 !border-2 !border-purple-500 !bg-white"
      />
    </div>
  );
}

export const DomainNode = memo(DomainNodeComponent);
