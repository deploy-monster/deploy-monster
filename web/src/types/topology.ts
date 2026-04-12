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

// API request/response types
export interface TopologyDeployRequest {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  projectId: string;
  environment: string;
}

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

// Component palette item
export interface PaletteItem {
  type: TopologyNodeType;
  label: string;
  icon: string;
  color: string;
  description: string;
}

// Default node positions
export const DEFAULT_NODE_POSITION = { x: 250, y: 150 };

// Palette items configuration
export const PALETTE_ITEMS: PaletteItem[] = [
  {
    type: 'app',
    label: 'App',
    icon: 'Container',
    color: 'blue',
    description: 'Container application from Git',
  },
  {
    type: 'database',
    label: 'Database',
    icon: 'Database',
    color: 'green',
    description: 'Managed database (PostgreSQL, MySQL, Redis, etc.)',
  },
  {
    type: 'domain',
    label: 'Domain',
    icon: 'Globe',
    color: 'purple',
    description: 'Custom domain with SSL',
  },
  {
    type: 'volume',
    label: 'Volume',
    icon: 'HardDrive',
    color: 'orange',
    description: 'Persistent storage volume',
  },
  {
    type: 'worker',
    label: 'Worker',
    icon: 'Cog',
    color: 'yellow',
    description: 'Background worker process',
  },
];
