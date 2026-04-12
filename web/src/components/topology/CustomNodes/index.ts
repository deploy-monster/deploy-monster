export { AppNode } from './AppNode';
export { DatabaseNode } from './DatabaseNode';
export { DomainNode } from './DomainNode';
export { VolumeNode } from './VolumeNode';
export { WorkerNode } from './WorkerNode';

import { AppNode } from './AppNode';
import { DatabaseNode } from './DatabaseNode';
import { DomainNode } from './DomainNode';
import { VolumeNode } from './VolumeNode';
import { WorkerNode } from './WorkerNode';

// Map of node types to their components
export const nodeTypes = {
  app: AppNode,
  database: DatabaseNode,
  domain: DomainNode,
  volume: VolumeNode,
  worker: WorkerNode,
};
