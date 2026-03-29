import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { Database, HardDrive, MoreHorizontal } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { DatabaseNodeData } from '@/types/topology';

const statusColors: Record<string, string> = {
  pending: 'bg-gray-400',
  configuring: 'bg-yellow-400',
  running: 'bg-green-400',
  error: 'bg-red-400',
  stopped: 'bg-gray-300',
};

const engineLabels: Record<string, string> = {
  postgres: 'PostgreSQL',
  mysql: 'MySQL',
  mariadb: 'MariaDB',
  mongodb: 'MongoDB',
  redis: 'Redis',
};

type DatabaseNodeProps = NodeProps;

function DatabaseNodeComponent({ data, selected }: DatabaseNodeProps) {
  const nodeData = data as DatabaseNodeData;
  const status = nodeData.status || 'pending';
  const engine = nodeData.engine || 'postgres';

  return (
    <div
      className={cn(
        'min-w-[160px] rounded-lg border-2 bg-card shadow-md transition-all',
        selected ? 'border-green-500 ring-2 ring-green-500/20' : 'border-green-500/50',
        'hover:border-green-500 hover:shadow-lg'
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!w-3 !h-3 !border-2 !border-green-500 !bg-white"
      />

      {/* Header */}
      <div className="flex items-center gap-2 rounded-t-md bg-green-500/10 px-3 py-2">
        <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-500 text-white">
          <Database className="h-4 w-4" />
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
        <div className="flex items-center justify-between">
          <span className="font-medium text-green-600">{engineLabels[engine] || engine}</span>
          <span className="text-muted-foreground">{nodeData.version || 'latest'}</span>
        </div>
        {nodeData.sizeGB && (
          <div className="flex items-center gap-1 text-muted-foreground">
            <HardDrive className="h-3 w-3" />
            <span>{nodeData.sizeGB} GB</span>
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
        className="!w-3 !h-3 !border-2 !border-green-500 !bg-white"
      />
    </div>
  );
}

export const DatabaseNode = memo(DatabaseNodeComponent);
