import type { Node, Edge } from '@xyflow/react';

// Volume mount configuration (stored on the container)
export interface VolumeMount {
  volumeId: string;
  mountPath: string;
}

// Node data types for each component type
export interface AppNodeData {
  name: string;
  status?: 'pending' | 'configuring' | 'running' | 'error' | 'stopped';
  gitUrl?: string;
  branch?: string;
  buildPack?: string;
  port?: number;
  replicas?: number;
  envVars?: Record<string, string>;
  volumeMounts?: VolumeMount[]; // Volumes mounted inside this container with their paths
  [key: string]: unknown;
}

export interface DatabaseNodeData {
  name: string;
  status?: 'pending' | 'configuring' | 'running' | 'error' | 'stopped';
  engine?: 'postgres' | 'mysql' | 'mariadb' | 'mongodb' | 'redis';
  version?: string;
  sizeGB?: number;
  [key: string]: unknown;
}

export interface DomainNodeData {
  name: string;
  status?: 'pending' | 'configuring' | 'active' | 'error';
  fqdn?: string;
  sslEnabled?: boolean;
  [key: string]: unknown;
}

export interface VolumeNodeData {
  name: string;
  status?: 'pending' | 'attached' | 'error';
  sizeGB?: number;
  mountPath?: string; // Default mount path (can be overridden per-container)
  [key: string]: unknown;
}

export interface WorkerNodeData {
  name: string;
  status?: 'pending' | 'running' | 'error' | 'stopped';
  command?: string;
  replicas?: number;
  gitUrl?: string;
  branch?: string;
  [key: string]: unknown;
}

// Union type for all node data
export type TopologyNodeData = AppNodeData | DatabaseNodeData | DomainNodeData | VolumeNodeData | WorkerNodeData;

// Node types
export type TopologyNodeType = 'app' | 'database' | 'domain' | 'volume' | 'worker';

// Typed nodes
export type AppNode = Node<AppNodeData, 'app'>;
export type DatabaseNode = Node<DatabaseNodeData, 'database'>;
export type DomainNode = Node<DomainNodeData, 'domain'>;
export type VolumeNode = Node<VolumeNodeData, 'volume'>;
export type WorkerNode = Node<WorkerNodeData, 'worker'>;

export type TopologyNode = AppNode | DatabaseNode | DomainNode | VolumeNode | WorkerNode;

// Edge types
export type TopologyEdgeType = 'default' | 'dependency' | 'mount' | 'dns';

export interface TopologyEdgeData {
  type?: TopologyEdgeType;
  label?: string;
  [key: string]: unknown;
}

export type TopologyEdge = Edge<TopologyEdgeData>;

// Store state
export interface TopologyState {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  selectedNodeId: string | null;
  projectId: string;
  environment: string;
  isDirty: boolean;
  isDeploying: boolean;
}

// Store actions
export interface TopologyActions {
  setNodes: (nodes: TopologyNode[]) => void;
  setEdges: (edges: TopologyEdge[]) => void;
  addNode: (node: TopologyNode) => void;
  updateNode: (id: string, data: Partial<TopologyNodeData>) => void;
  updateNodePosition: (id: string, position: { x: number; y: number }) => void;
  removeNode: (id: string) => void;
  addEdge: (edge: TopologyEdge) => void;
  removeEdge: (id: string) => void;
  selectNode: (id: string | null) => void;
  setProjectId: (id: string) => void;
  setEnvironment: (env: string) => void;
  markDirty: () => void;
  markClean: () => void;
  loadTopology: (topology: { nodes: TopologyNode[]; edges: TopologyEdge[] }) => void;
  setDeploying: (deploying: boolean) => void;
  clearTopology: () => void;
  autoLayout: () => void;
}

export type TopologyStore = TopologyState & TopologyActions;

// API response type
export interface TopologyDeployResponse {
  success: boolean;
  message: string;
  duration?: string;
  containers?: string[];
  networks?: string[];
  volumes?: string[];
  errors?: string[];
}

// Compile result from topology compiler
export interface CompileResult {
  success: boolean;
  message: string;
  composeYaml?: string;
  caddyfile?: string;
  envFile?: string;
  errors?: string[];
}
