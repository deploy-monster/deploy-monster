import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { Container, GitBranch, MoreHorizontal } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { AppNodeData } from '@/types/topology';

const statusColors: Record<string, string> = {
  pending: 'bg-gray-400',
  configuring: 'bg-yellow-400',
  running: 'bg-green-400',
  error: 'bg-red-400',
  stopped: 'bg-gray-300',
};

type AppNodeProps = NodeProps;

function AppNodeComponent({ data, selected }: AppNodeProps) {
  const nodeData = data as AppNodeData;
  const status = nodeData.status || 'pending';

  return (
    <div
      className={cn(
        'min-w-[180px] rounded-lg border-2 bg-card shadow-md transition-all',
        selected ? 'border-blue-500 ring-2 ring-blue-500/20' : 'border-blue-500/50',
        'hover:border-blue-500 hover:shadow-lg'
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!w-3 !h-3 !border-2 !border-blue-500 !bg-white"
      />

      {/* Header */}
      <div className="flex items-center gap-2 rounded-t-md bg-blue-500/10 px-3 py-2">
        <div className="flex h-8 w-8 items-center justify-center rounded-md bg-blue-500 text-white">
          <Container className="h-4 w-4" />
        </div>
        <div className="flex-1 truncate">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-foreground">{nodeData.name}</span>
            <span className={cn('h-2 w-2 rounded-full', statusColors[status])} />
          </div>
          {nodeData.port && (
            <span className="text-xs text-muted-foreground">:{nodeData.port}</span>
          )}
        </div>
      </div>

      {/* Body */}
      <div className="space-y-1 px-3 py-2 text-xs">
        {nodeData.gitUrl ? (
          <div className="flex items-center gap-1 text-muted-foreground">
            <GitBranch className="h-3 w-3" />
            <span className="truncate">{nodeData.branch || 'main'}</span>
          </div>
        ) : (
          <span className="text-muted-foreground italic">No repo configured</span>
        )}
        {nodeData.replicas && nodeData.replicas > 1 && (
          <div className="text-muted-foreground">{nodeData.replicas} replicas</div>
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
        className="!w-3 !h-3 !border-2 !border-blue-500 !bg-white"
      />
    </div>
  );
}

export const AppNode = memo(AppNodeComponent);
