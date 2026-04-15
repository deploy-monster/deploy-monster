import { create } from 'zustand';
import dagre from 'dagre';
import type { TopologyStore, TopologyNode, TopologyEdge, TopologyEdgeType } from '@/types/topology';

// SECURITY FIX: Use crypto.getRandomValues instead of Math.random for ID generation
const generateId = () => {
  const array = new Uint8Array(16);
  crypto.getRandomValues(array);
  return `${Date.now()}-${Array.from(array, b => b.toString(16).padStart(2, '0')).join('')}`;
};

// Dagre layout configuration
const layoutWithDagre = (nodes: TopologyNode[], edges: TopologyEdge[]): TopologyNode[] => {
  if (nodes.length === 0) return nodes;

  const dagreGraph = new dagre.graphlib.Graph();
  dagreGraph.setDefaultEdgeLabel(() => ({}));

  dagreGraph.setGraph({
    rankdir: 'LR',
    nodesep: 100,
    ranksep: 150,
    marginx: 50,
    marginy: 50,
  });

  // Add nodes to dagre graph
  nodes.forEach((node) => {
    dagreGraph.setNode(node.id, {
      width: node.type === 'app' ? 200 : 180,
      height: node.type === 'app' ? 120 : 100,
    });
  });

  // Add edges to dagre graph
  edges.forEach((edge) => {
    dagreGraph.setEdge(edge.source, edge.target);
  });

  // Run layout
  dagre.layout(dagreGraph);

  // Return nodes with updated positions
  return nodes.map((node) => {
    const nodeWithPosition = dagreGraph.node(node.id);
    if (nodeWithPosition) {
      return {
        ...node,
        position: {
          x: nodeWithPosition.x - nodeWithPosition.width / 2,
          y: nodeWithPosition.y - nodeWithPosition.height / 2,
        },
      };
    }
    return node;
  });
};

export const useTopologyStore = create<TopologyStore>((set, get) => ({
  // Initial state
  nodes: [],
  edges: [],
  selectedNodeId: null,
  projectId: '',
  environment: 'production',
  isDirty: false,
  isDeploying: false,

  // Actions
  setNodes: (nodes) => set({ nodes, isDirty: true }),

  setEdges: (edges) => set({ edges, isDirty: true }),

  addNode: (node) =>
    set((state) => ({
      nodes: [...state.nodes, node],
      isDirty: true,
    })),

  updateNode: (id, data) =>
    set((state) => ({
      nodes: state.nodes.map((node) =>
        node.id === id ? { ...node, data: { ...node.data, ...data } } : node
      ) as TopologyNode[],
      isDirty: true,
    })),

  updateNodePosition: (id, position) =>
    set((state) => ({
      nodes: state.nodes.map((node) =>
        node.id === id ? { ...node, position } : node
      ) as TopologyNode[],
      isDirty: true,
    })),

  removeNode: (id) =>
    set((state) => {
      // Simply remove the node - volumes remain independent
      // When a container is deleted, its volumeMounts are deleted with it,
      // but the volume nodes themselves remain in the topology
      return {
        nodes: state.nodes.filter((node) => node.id !== id),
        edges: state.edges.filter((edge) => edge.source !== id && edge.target !== id),
        selectedNodeId: state.selectedNodeId === id ? null : state.selectedNodeId,
        isDirty: true,
      };
    }),

  addEdge: (edge) =>
    set((state) => ({
      edges: [...state.edges, edge],
      isDirty: true,
    })),

  removeEdge: (id) =>
    set((state) => ({
      edges: state.edges.filter((edge) => edge.id !== id),
      isDirty: true,
    })),

  selectNode: (id) => set({ selectedNodeId: id }),

  setProjectId: (id) => set({ projectId: id }),

  setEnvironment: (env) => set({ environment: env, isDirty: true }),

  markDirty: () => set({ isDirty: true }),

  markClean: () => set({ isDirty: false }),

  setDeploying: (deploying) => set({ isDeploying: deploying }),

  clearTopology: () =>
    set({
      nodes: [],
      edges: [],
      selectedNodeId: null,
      isDirty: false,
    }),

  loadTopology: (topology) =>
    set({
      nodes: topology.nodes,
      edges: topology.edges,
      isDirty: false,
    }),

  autoLayout: () => {
    const { nodes, edges } = get();
    const layoutedNodes = layoutWithDagre(nodes, edges);
    set({ nodes: layoutedNodes, isDirty: true });
  },
}));

// Helper function to create a new node
export const createNode = (
  type: TopologyNode['type'],
  position: { x: number; y: number }
): TopologyNode => {
  const id = generateId();
  const baseData = { name: `New ${type}`, status: 'pending' as const };

  switch (type) {
    case 'app':
      return {
        id,
        type: 'app',
        position,
        data: {
          ...baseData,
          gitUrl: '',
          branch: 'main',
          buildPack: 'auto',
          port: 3000,
          replicas: 1,
          envVars: {},
          volumeMounts: [],
        },
      };
    case 'database':
      return {
        id,
        type: 'database',
        position,
        data: {
          ...baseData,
          engine: 'postgres',
          version: '16',
          sizeGB: 10,
        },
      };
    case 'domain':
      return {
        id,
        type: 'domain',
        position,
        data: {
          ...baseData,
          fqdn: '',
          sslEnabled: true,
        },
      };
    case 'volume':
      return {
        id,
        type: 'volume',
        position,
        data: {
          ...baseData,
          sizeGB: 10,
          mountPath: '/data',
        },
      };
    case 'worker':
      return {
        id,
        type: 'worker',
        position,
        data: {
          ...baseData,
          command: '',
          replicas: 1,
          gitUrl: '',
          branch: 'main',
        },
      };
    default:
      throw new Error(`Unknown node type: ${type}`);
  }
};

// Helper function to create an edge
export const createEdge = (
  source: string,
  target: string,
  edgeType: TopologyEdgeType = 'dependency'
): TopologyEdge => ({
  id: `edge-${source}-${target}`,
  source,
  target,
  type: 'smoothstep',
  animated: true,
  data: { type: edgeType },
});
