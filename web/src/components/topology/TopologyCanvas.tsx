import { useCallback, useRef, useMemo } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  addEdge,
  type Connection,
  type OnConnect,
  Panel,
  BackgroundVariant,
  type Node,
  type Edge,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { nodeTypes } from './CustomNodes';
import { useTopologyStore, createNode, createEdge } from '@/stores/topologyStore';
import type { TopologyNodeType, TopologyNode, TopologyEdge } from '@/types/topology';
import { Button } from '@/components/ui/button';
import { Sparkles } from 'lucide-react';

export function TopologyCanvas() {
  const reactFlowWrapper = useRef<HTMLDivElement>(null);
  const {
    nodes: storeNodes,
    edges: storeEdges,
    addNode,
    addEdge: addStoreEdge,
    selectNode,
    autoLayout,
  } = useTopologyStore();

  // Convert store nodes/edges to React Flow format
  const nodes = useMemo(() => storeNodes.map(n => ({ ...n, data: { ...n.data } } as Node)), [storeNodes]);
  const edges = useMemo(() => storeEdges.map(e => ({ ...e, data: { ...e.data } } as Edge)), [storeEdges]);

  const [localNodes, setLocalNodes, onNodesChange] = useNodesState(nodes);
  const [localEdges, setLocalEdges, onEdgesChange] = useEdgesState(edges);

  const onConnect: OnConnect = useCallback(
    (connection: Connection) => {
      if (connection.source && connection.target) {
        // Determine edge type based on source and target node types
        const sourceNode = storeNodes.find(n => n.id === connection.source);
        const targetNode = storeNodes.find(n => n.id === connection.target);

        let edgeType: 'dependency' | 'mount' | 'dns' = 'dependency';

        if (sourceNode?.type === 'domain' && targetNode?.type === 'app') {
          edgeType = 'dns';
        } else if (sourceNode?.type === 'app' && targetNode?.type === 'database') {
          edgeType = 'dependency';
        } else if ((sourceNode?.type === 'app' || sourceNode?.type === 'worker') && targetNode?.type === 'volume') {
          edgeType = 'mount';
        } else if (sourceNode?.type === 'app' && targetNode?.type === 'app') {
          edgeType = 'dependency';
        }

        const newEdge = createEdge(connection.source, connection.target, edgeType);
        addStoreEdge(newEdge as TopologyEdge);
        setLocalEdges((eds) => addEdge({ ...connection, data: { type: edgeType } }, eds));
      }
    },
    [addStoreEdge, setLocalEdges, storeNodes]
  );

  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      selectNode(node.id);
    },
    [selectNode]
  );

  const onPaneClick = useCallback(() => {
    selectNode(null);
  }, [selectNode]);

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  }, []);

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();

      const type = event.dataTransfer.getData('application/topology-node') as TopologyNodeType;
      if (!type || !reactFlowWrapper.current) return;

      const bounds = reactFlowWrapper.current.getBoundingClientRect();
      const position = {
        x: event.clientX - bounds.left - 100,
        y: event.clientY - bounds.top - 50,
      };

      const newNode = createNode(type, position);
      addNode(newNode as TopologyNode);
      setLocalNodes((nds) => [...nds, { ...newNode, data: { ...newNode.data } } as Node]);
    },
    [addNode, setLocalNodes]
  );

  const handleAutoLayout = useCallback(() => {
    autoLayout();
    const updatedNodes = useTopologyStore.getState().nodes;
    setLocalNodes(updatedNodes.map(n => ({ ...n, data: { ...n.data } } as Node)));
  }, [autoLayout, setLocalNodes]);

  return (
    <div ref={reactFlowWrapper} className="h-full w-full">
      <ReactFlow
        nodes={localNodes}
        edges={localEdges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        onDragOver={onDragOver}
        onDrop={onDrop}
        nodeTypes={nodeTypes}
        fitView
        snapToGrid
        snapGrid={[15, 15]}
        defaultEdgeOptions={{
          type: 'smoothstep',
          animated: true,
        }}
        connectionLineStyle={{ strokeWidth: 2, stroke: '#3b82f6' }}
        proOptions={{ hideAttribution: true }}
      >
        <Background variant={BackgroundVariant.Dots} gap={20} size={1} />
        <MiniMap
          nodeColor={(node) => {
            switch (node.type) {
              case 'app':
                return '#3b82f6';
              case 'database':
                return '#22c55e';
              case 'domain':
                return '#a855f7';
              case 'volume':
                return '#f97316';
              case 'worker':
                return '#eab308';
              default:
                return '#6b7280';
            }
          }}
          className="!bg-muted"
        />
        <Controls showInteractive={false} />
        <Panel position="top-right" className="flex gap-2">
          <Button variant="outline" size="sm" onClick={handleAutoLayout}>
            <Sparkles className="mr-2 h-4 w-4" />
            Auto Layout
          </Button>
        </Panel>
        <Panel position="bottom-center" className="flex items-center gap-2 rounded-lg bg-card/80 px-3 py-2 backdrop-blur">
          <span className="text-xs text-muted-foreground">
            {localNodes.length} components
          </span>
          <span className="text-muted-foreground">|</span>
          <span className="text-xs text-muted-foreground">
            {localEdges.length} connections
          </span>
        </Panel>
      </ReactFlow>
    </div>
  );
}
