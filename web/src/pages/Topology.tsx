import { useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router';
import { Save, Rocket, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Select } from '@/components/ui/select';
import { ComponentPalette } from '@/components/topology/ComponentPalette';
import { TopologyCanvas } from '@/components/topology/TopologyCanvas';
import { ConfigPanel } from '@/components/topology/ConfigPanel';
import { useTopologyStore } from '@/stores/topologyStore';
import type { TopologyNodeType } from '@/types/topology';
import { useMutation } from '@/hooks';

const ENVIRONMENTS = ['production', 'staging', 'development'];

export default function TopologyPage() {
  const navigate = useNavigate();

  const {
    nodes,
    edges,
    selectedNodeId,
    selectNode,
    environment,
    setEnvironment,
    isDirty,
    isDeploying,
    setDeploying,
    markClean,
    projectId,
  } = useTopologyStore();

  const saveMutation = useMutation('post', '/topology');
  const deployMutation = useMutation('post', '/topology/deploy');

  const selectedNode = useMemo(() => {
    if (!selectedNodeId) return null;
    const node = nodes.find(n => n.id === selectedNodeId);
    if (!node) return null;
    return { id: node.id, type: node.type, data: node.data as Record<string, unknown> };
  }, [nodes, selectedNodeId]);

  const handleSave = useCallback(async () => {
    try {
      await saveMutation.mutate({
        nodes,
        edges,
        projectId,
        environment,
      });
      markClean();
    } catch (error) {
      console.error('Failed to save topology:', error);
    }
  }, [nodes, edges, projectId, environment, saveMutation, markClean]);

  const handleDeploy = useCallback(async () => {
    if (nodes.length === 0) return;

    setDeploying(true);
    try {
      const result = await deployMutation.mutate({
        nodes,
        edges,
        projectId,
        environment,
      });

      if (result && typeof result === 'object' && 'success' in result && result.success) {
        markClean();
        navigate('/apps');
      }
    } catch (error) {
      console.error('Failed to deploy topology:', error);
    } finally {
      setDeploying(false);
    }
  }, [nodes, edges, projectId, environment, deployMutation, setDeploying, markClean, navigate]);

  const handleCloseConfig = useCallback(() => {
    selectNode(null);
  }, [selectNode]);

  const handleDragStart = (_type: TopologyNodeType) => {
    // Could be used to show visual feedback
  };

  const canDeploy = nodes.length > 0 && !isDeploying;

  return (
    <div className="flex h-[calc(100vh-4rem)] flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b bg-card px-4 py-3">
        <div className="flex items-center gap-4">
          <h1 className="text-lg font-semibold">Topology Editor</h1>
          <Select
            value={environment}
            onChange={(e) => setEnvironment(e.target.value)}
          >
            {ENVIRONMENTS.map((env) => (
              <option key={env} value={env}>
                {env}
              </option>
            ))}
          </Select>
        </div>
        <div className="flex items-center gap-2">
          {isDirty && (
            <span className="text-xs text-muted-foreground">Unsaved changes</span>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={handleSave}
            disabled={!isDirty}
          >
            <Save className="mr-2 h-4 w-4" />
            Save
          </Button>
          <Button
            size="sm"
            onClick={handleDeploy}
            disabled={!canDeploy}
          >
            {isDeploying ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Rocket className="mr-2 h-4 w-4" />
            )}
            Deploy
          </Button>
        </div>
      </div>

      {/* Main content */}
      <div className="flex flex-1 overflow-hidden">
        <ComponentPalette onDragStart={handleDragStart} />
        <div className="flex-1 bg-muted/30">
          <TopologyCanvas />
        </div>
        <ConfigPanel selectedNode={selectedNode} onClose={handleCloseConfig} />
      </div>
    </div>
  );
}
