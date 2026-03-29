import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { HardDrive, Folder, MoreHorizontal } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { VolumeNodeData } from '@/types/topology';
import { useTopologyStore } from '@/stores/topologyStore';

const statusColors: Record<string, string> = {
  pending: 'bg-gray-400',
  attached: 'bg-green-400',
  error: 'bg-red-400',
};

type VolumeNodeProps = NodeProps;

function VolumeNodeComponent({ data, selected, id }: VolumeNodeProps) {
  const nodeData = data as VolumeNodeData;
  const status = nodeData.status || 'pending';
  const { nodes } = useTopologyStore();

  // Check if this volume is mounted to any container
  const mountedToContainer = nodes.find((n) => {
    if (n.type !== 'app') return false;
    const appData = n.data as { volumeMounts?: { volumeId: string }[] };
    return (appData.volumeMounts || []).some((vm) => vm.volumeId === id);
  });

  return (
    <div
      className={cn(
        'min-w-[150px] rounded-lg border-2 bg-card shadow-md transition-all',
        selected ? 'border-orange-500 ring-2 ring-orange-500/20' : 'border-orange-500/50',
        'hover:border-orange-500 hover:shadow-lg',
        mountedToContainer && 'opacity-60 scale-95' // Dimmed when mounted
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!w-3 !h-3 !border-2 !border-orange-500 !bg-white"
      />

      {/* Header */}
      <div className="flex items-center gap-2 rounded-t-md bg-orange-500/10 px-3 py-2">
        <div className="flex h-8 w-8 items-center justify-center rounded-md bg-orange-500 text-white">
          <HardDrive className="h-4 w-4" />
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
        {nodeData.sizeGB && (
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">Size</span>
            <span className="font-medium text-orange-600">{nodeData.sizeGB} GB</span>
          </div>
        )}
        {nodeData.mountPath && (
          <div className="flex items-center gap-1 text-muted-foreground">
            <Folder className="h-3 w-3" />
            <span className="truncate font-mono">{nodeData.mountPath}</span>
          </div>
        )}
        {mountedToContainer && (
          <div className="mt-1 flex items-center gap-1 border-t border-dashed pt-1 text-muted-foreground">
            <span className="text-[10px] italic">
              Mounted in {(mountedToContainer.data as { name: string }).name}
            </span>
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
        className="!w-3 !h-3 !border-2 !border-orange-500 !bg-white"
      />
    </div>
  );
}

export const VolumeNode = memo(VolumeNodeComponent);
