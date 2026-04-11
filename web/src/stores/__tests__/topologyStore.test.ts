import { describe, it, expect } from 'vitest';
import { createNode, createEdge } from '../topologyStore';

describe('createNode', () => {
  const pos = { x: 100, y: 200 };

  it('creates an app node with default values', () => {
    const node = createNode('app', pos);
    expect(node.type).toBe('app');
    expect(node.position).toEqual(pos);
    expect(node.id).toBeTruthy();
    expect(node.data.name).toBe('New app');
    expect(node.data.status).toBe('pending');
    if (node.type === 'app') {
      expect(node.data.branch).toBe('main');
      expect(node.data.port).toBe(3000);
      expect(node.data.replicas).toBe(1);
      expect(node.data.buildPack).toBe('auto');
    }
  });

  it('creates a database node', () => {
    const node = createNode('database', pos);
    expect(node.type).toBe('database');
    if (node.type === 'database') {
      expect(node.data.engine).toBe('postgres');
      expect(node.data.version).toBe('16');
      expect(node.data.sizeGB).toBe(10);
    }
  });

  it('creates a domain node', () => {
    const node = createNode('domain', pos);
    expect(node.type).toBe('domain');
    if (node.type === 'domain') {
      expect(node.data.fqdn).toBe('');
      expect(node.data.sslEnabled).toBe(true);
    }
  });

  it('creates a volume node', () => {
    const node = createNode('volume', pos);
    expect(node.type).toBe('volume');
    if (node.type === 'volume') {
      expect(node.data.sizeGB).toBe(10);
      expect(node.data.mountPath).toBe('/data');
    }
  });

  it('creates a worker node', () => {
    const node = createNode('worker', pos);
    expect(node.type).toBe('worker');
    if (node.type === 'worker') {
      expect(node.data.command).toBe('');
      expect(node.data.replicas).toBe(1);
      expect(node.data.branch).toBe('main');
    }
  });

  it('generates unique IDs', () => {
    const a = createNode('app', pos);
    const b = createNode('app', pos);
    expect(a.id).not.toBe(b.id);
  });

  it('throws for unknown node type', () => {
    // @ts-expect-error testing invalid type
    expect(() => createNode('invalid', pos)).toThrow('Unknown node type: invalid');
  });
});

describe('createEdge', () => {
  it('creates a dependency edge by default', () => {
    const edge = createEdge('src-1', 'tgt-2');
    expect(edge.id).toBe('edge-src-1-tgt-2');
    expect(edge.source).toBe('src-1');
    expect(edge.target).toBe('tgt-2');
    expect(edge.type).toBe('smoothstep');
    expect(edge.animated).toBe(true);
    expect(edge.data?.type).toBe('dependency');
  });

  it('creates an edge with a custom type', () => {
    const edge = createEdge('a', 'b', 'mount');
    expect(edge.data?.type).toBe('mount');
  });
});
