import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { Container, GitBranch, HardDrive, MoreHorizontal } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { AppNodeData, VolumeNodeData, VolumeMount } from '@/types/topology';
import { useTopologyStore } from '@/stores/topologyStore';

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
  const { nodes } = useTopologyStore();

  // Get volume mounts for this container
  const volumeMounts = (nodeData.volumeMounts as VolumeMount[]) || [];

  // Get mounted volume nodes with their mount paths
  const mountedVolumes = volumeMounts.map((vm) => {
    const volNode = nodes.find((n) => n.id === vm.volumeId);
    const volData = volNode?.data as VolumeNodeData | undefined;
    return {
      ...vm,
      node: volNode,
      data: volData,
    };
  }).filter((vm) => vm.node);

  return (
    <div
      className={cn(
        'rounded-xl border-2 bg-card shadow-md transition-all',
        selected ? 'border-blue-500 ring-2 ring-blue-500/20' : 'border-blue-500/50',
        'hover:border-blue-500 hover:shadow-lg',
        mountedVolumes.length > 0 ? 'min-w-[200px]' : 'min-w-[180px]'
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!w-3 !h-3 !border-2 !border-blue-500 !bg-white"
      />

      {/* Container Header */}
      <div className="flex items-center gap-2 rounded-t-lg bg-blue-500/10 px-3 py-2">
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
        <span className="rounded bg-blue-500/20 px-1.5 py-0.5 text-[10px] font-medium text-blue-600">
          container
        </span>
      </div>

      {/* Git Source */}
      <div className="border-t border-dashed border-blue-500/20 px-3 py-2 text-xs">
        {nodeData.gitUrl ? (
          <div className="flex items-center gap-1 text-muted-foreground">
            <GitBranch className="h-3 w-3" />
            <span className="truncate">{nodeData.branch || 'main'}</span>
          </div>
        ) : (
          <span className="text-muted-foreground italic">No repo configured</span>
        )}
        {nodeData.replicas && nodeData.replicas > 1 && (
          <div className="mt-1 text-muted-foreground">
            {nodeData.replicas} replicas
          </div>
        )}
      </div>

      {/* Mounted Volumes - Visual Nested Display */}
      {mountedVolumes.length > 0 && (
        <div className="border-t border-dashed border-blue-500/30 bg-muted/30 p-2">
          <div className="mb-1 flex items-center gap-1 px-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
            <HardDrive className="h-3 w-3" />
            Mounted Volumes
          </div>
          <div className="space-y-1">
            {mountedVolumes.map(({ volumeId, mountPath, data: volData }) => (
              <div
                key={volumeId}
                className="flex items-center gap-2 rounded-md border border-orange-500/40 bg-orange-500/5 px-2 py-1.5"
              >
                <div className="flex h-5 w-5 items-center justify-center rounded bg-orange-500 text-white">
                  <HardDrive className="h-3 w-3" />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="truncate text-xs font-medium text-foreground">
                    {volData?.name || 'Volume'}
                  </div>
                  <div className="flex items-center gap-1 text-[10px] text-muted-foreground">
                    <span>{volData?.sizeGB}GB</span>
                    {mountPath && (
                      <>
                        <span>→</span>
                        <code className="rounded bg-muted px-0.5 font-mono">
                          {mountPath}
                        </code>
                      </>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Footer */}
      <div className="flex items-center justify-end rounded-b-lg border-t bg-muted/30 px-2 py-1">
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
