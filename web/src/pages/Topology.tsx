import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Save, Rocket, Loader2, FileCode, Layers } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Select } from '@/components/ui/select';
import { ComponentPalette } from '@/components/topology/ComponentPalette';
import { TopologyCanvas } from '@/components/topology/TopologyCanvas';
import { ConfigPanel } from '@/components/topology/ConfigPanel';
import { CompileModal } from '@/components/topology/CompileModal';
import { DeployModal, type DeployStatus } from '@/components/topology/DeployModal';
import { useTopologyStore } from '@/stores/topologyStore';
import type { CompileResult, TopologyDeployResponse } from '@/types/topology';
import { useApi, useMutation } from '@/hooks';
import { useDeployProgress } from '@/hooks/useDeployProgress';
import type { TopologyState } from '@/types/topology';

const ENVIRONMENTS = ['production', 'staging', 'development'];

export default function TopologyPage() {
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
    loadTopology,
  } = useTopologyStore();

  const [compileModalOpen, setCompileModalOpen] = useState(false);
  const [compileResult, setCompileResult] = useState<CompileResult | null>(null);
  const [isCompiling, setIsCompiling] = useState(false);
  const [deployModalOpen, setDeployModalOpen] = useState(false);
  const [deployResult, setDeployResult] = useState<TopologyDeployResponse | null>(null);
  const [wsDeployStatus, setWsDeployStatus] = useState<DeployStatus>('idle');
  const [wsEnabled, setWsEnabled] = useState(false);
  const wsCompletedRef = useRef(false);

  const saveMutation = useMutation('post', '/topology');
  const deployMutation = useMutation('post', '/topology/deploy');
  const compileMutation = useMutation('post', '/topology/compile');

  // WebSocket for real-time deploy progress
  const { status: wsStatus } = useDeployProgress({
    projectId,
    enabled: wsEnabled,
    onComplete: (result) => {
      wsCompletedRef.current = true;
      setDeployResult(result);
      setDeploying(false);
      setWsEnabled(false);
      if (result.success) {
        markClean();
      }
    },
  });

  // Update status from WebSocket
  useEffect(() => {
    if (wsEnabled && wsStatus !== 'idle') {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- sync ws status to local state for deploy modal
      setWsDeployStatus(wsStatus);
    }
  }, [wsStatus, wsEnabled]);

  // Load topology on mount
  const { data: topologyData, refetch: fetchTopology } = useApi<TopologyState>(
    `/topology/${projectId}/${environment}`,
    { immediate: false }
  );

  useEffect(() => {
    if (topologyData && topologyData.nodes && topologyData.edges) {
      loadTopology(topologyData);
    }
  }, [topologyData, loadTopology]);

  // Reload topology when environment changes
  useEffect(() => {
    if (projectId && environment) {
      fetchTopology();
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, environment]);

  const deployStatus = wsEnabled ? wsDeployStatus : 'idle';

  const selectedNode = useMemo(() => {
    if (!selectedNodeId) return null;
    return nodes.find(n => n.id === selectedNodeId) ?? null;
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

    wsCompletedRef.current = false;
    setDeployModalOpen(true);
    setDeployResult(null);
    setWsDeployStatus('idle');
    setDeploying(true);
    setWsEnabled(true);

    try {
      // Trigger deployment - WebSocket will provide real-time progress
      const result = await deployMutation.mutate({
        nodes,
        edges,
        projectId,
        environment,
      });

      // If WebSocket didn't complete, use the API response as fallback
      if (!wsCompletedRef.current) {
        setDeployResult(result as TopologyDeployResponse);

        if (result && typeof result === 'object' && 'success' in result && result.success) {
          setWsDeployStatus('success');
          markClean();
        } else {
          setWsDeployStatus('error');
        }
        setDeploying(false);
        setWsEnabled(false);
      }
    } catch (error) {
      console.error('Failed to deploy topology:', error);
      if (!wsCompletedRef.current) {
        setWsDeployStatus('error');
        setDeployResult({
          success: false,
          message: error instanceof Error ? error.message : 'Deployment failed',
        });
        setDeploying(false);
        setWsEnabled(false);
      }
    }
  }, [nodes, edges, projectId, environment, deployMutation, setDeploying, markClean]);

  const handleCompile = useCallback(async () => {
    if (nodes.length === 0) return;

    setIsCompiling(true);
    setCompileResult(null);
    setCompileModalOpen(true);

    try {
      const result = await compileMutation.mutate({
        nodes,
        edges,
        projectId,
        environment,
      });
      setCompileResult(result as CompileResult);
    } catch (error) {
      console.error('Failed to compile topology:', error);
      setCompileResult({
        success: false,
        message: error instanceof Error ? error.message : 'Compilation failed',
      });
    } finally {
      setIsCompiling(false);
    }
  }, [nodes, edges, projectId, environment, compileMutation]);

  const handleCloseConfig = useCallback(() => {
    selectNode(null);
  }, [selectNode]);

  const handleDragStart = () => {
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
            variant="outline"
            size="sm"
            onClick={handleCompile}
            disabled={nodes.length === 0}
          >
            <FileCode className="mr-2 h-4 w-4" />
            Compile
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
        {nodes.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center text-center">
            <div className="rounded-full bg-muted p-6 mb-5">
              <Layers className="size-10 text-muted-foreground" />
            </div>
            <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
              No components in topology
            </h2>
            <p className="text-muted-foreground max-w-sm text-sm mb-6">
              Drag components from the palette on the left to build your topology, or start with a template to quickly set up your infrastructure.
            </p>
            <Button variant="outline" onClick={() => selectNode(null)}>
              <Layers className="size-4 mr-2" />
              Start with a template
            </Button>
          </div>
        ) : (
          <>
            <ComponentPalette onDragStart={handleDragStart} />
            <div className="flex-1 bg-muted/30">
              <TopologyCanvas />
            </div>
            <ConfigPanel selectedNode={selectedNode} onClose={handleCloseConfig} />
          </>
        )}
      </div>

      <CompileModal
        open={compileModalOpen}
        onClose={() => setCompileModalOpen(false)}
        result={compileResult}
        isCompiling={isCompiling}
      />

      <DeployModal
        open={deployModalOpen}
        onClose={() => setDeployModalOpen(false)}
        result={deployResult}
        isDeploying={isDeploying}
        status={deployStatus}
      />
    </div>
  );
}
