import { useCallback, useRef } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type OnNodesChange,
  type OnEdgesChange,
  type Connection,
  type OnConnect,
  Panel,
  BackgroundVariant,
  type Node,
  type Edge,
  type NodeChange,
  type EdgeChange,
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
    removeEdge: removeStoreEdge,
    updateNodePosition,
    selectNode,
    autoLayout,
  } = useTopologyStore();

  // Convert store nodes/edges to React Flow format on each render
  const reactFlowNodes: Node[] = storeNodes.map(n => ({ ...n, data: { ...n.data } }));
  const reactFlowEdges: Edge[] = storeEdges.map(e => ({ ...e, data: { ...e.data } }));

  // Handle node changes (position drag, selection, removal) and sync back to store
  const handleNodesChange: OnNodesChange = useCallback((changes: NodeChange[]) => {
    for (const change of changes) {
      if (change.type === 'position' && change.dragging && change.position) {
        updateNodePosition(change.id, change.position);
      } else if (change.type === 'select') {
        selectNode(change.selected ? change.id : null);
      } else if (change.type === 'remove') {
        const store = useTopologyStore.getState();
        store.removeNode(change.id);
      }
    }
  }, [updateNodePosition, selectNode]);

  // Handle edge changes and sync back to store
  const handleEdgesChange: OnEdgesChange = useCallback((changes: EdgeChange[]) => {
    for (const change of changes) {
      if (change.type === 'remove') {
        removeStoreEdge(change.id);
      }
    }
  }, [removeStoreEdge]);

  const onConnect: OnConnect = useCallback(
    (connection: Connection) => {
      if (connection.source && connection.target) {
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
      }
    },
    [addStoreEdge, storeNodes]
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
    },
    [addNode]
  );

  const handleAutoLayout = useCallback(() => {
    autoLayout();
  }, [autoLayout]);

  return (
    <div ref={reactFlowWrapper} className="h-full w-full">
      <ReactFlow
        nodes={reactFlowNodes}
        edges={reactFlowEdges}
        onNodesChange={handleNodesChange}
        onEdgesChange={handleEdgesChange}
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
            {storeNodes.length} components
          </span>
          <span className="text-muted-foreground">|</span>
          <span className="text-xs text-muted-foreground">
            {storeEdges.length} connections
          </span>
        </Panel>
      </ReactFlow>
    </div>
  );
}
