import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------
//
// Topology has heavy sub-components (TopologyCanvas, ComponentPalette,
// ConfigPanel — all React Flow based). Stub them out so we can test the
// page shell without bringing in the full flow renderer.

vi.mock('@/components/topology/ComponentPalette', () => ({
  ComponentPalette: () => <div data-testid="component-palette" />,
}));
vi.mock('@/components/topology/TopologyCanvas', () => ({
  TopologyCanvas: () => <div data-testid="topology-canvas" />,
}));
vi.mock('@/components/topology/ConfigPanel', () => ({
  ConfigPanel: () => <div data-testid="config-panel" />,
}));
vi.mock('@/components/topology/CompileModal', () => ({
  CompileModal: ({ open }: { open: boolean }) =>
    open ? <div data-testid="compile-modal-open" /> : null,
}));
vi.mock('@/components/topology/DeployModal', () => ({
  DeployModal: ({ open }: { open: boolean }) =>
    open ? <div data-testid="deploy-modal-open" /> : null,
}));
vi.mock('@/hooks/useDeployProgress', () => ({
  useDeployProgress: () => ({ status: 'idle' }),
}));

// Fresh hooks mock — the page uses useApi + useMutation for save/deploy/compile.
const refetchMock = vi.fn();
const saveMutationMock = vi.fn();
const deployMutationMock = vi.fn();
const compileMutationMock = vi.fn();

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks');
  return {
    ...actual,
    useApi: () => ({ data: null, loading: false, error: null, refetch: refetchMock }),
    useMutation: (_method: string, path: string) => ({
      mutate: (body: unknown) => {
        if (path === '/topology') return saveMutationMock(body);
        if (path === '/topology/deploy') return deployMutationMock(body);
        if (path === '/topology/compile') return compileMutationMock(body);
        return Promise.resolve(undefined);
      },
      loading: false,
      error: null,
    }),
  };
});

// Topology store mock — default export of the page uses it to read state and
// dispatch actions. We provide a minimal controllable mock.
type StoreShape = {
  nodes: Array<{ id: string; type: string; data: Record<string, unknown> }>;
  edges: unknown[];
  selectedNodeId: string | null;
  selectNode: (id: string | null) => void;
  environment: string;
  setEnvironment: (env: string) => void;
  isDirty: boolean;
  isDeploying: boolean;
  setDeploying: (v: boolean) => void;
  markClean: () => void;
  projectId: string;
  loadTopology: (t: unknown) => void;
};

const store: StoreShape = {
  nodes: [],
  edges: [],
  selectedNodeId: null,
  selectNode: vi.fn(),
  environment: 'production',
  setEnvironment: vi.fn((env: string) => { store.environment = env; }),
  isDirty: false,
  isDeploying: false,
  setDeploying: vi.fn((v: boolean) => { store.isDeploying = v; }),
  markClean: vi.fn(),
  projectId: 'proj-1',
  loadTopology: vi.fn(),
};

vi.mock('@/stores/topologyStore', () => ({
  useTopologyStore: () => store,
}));

import TopologyPage from '../Topology';

function renderTopology() {
  return render(
    <MemoryRouter>
      <TopologyPage />
    </MemoryRouter>
  );
}

describe('Topology page', () => {
  beforeEach(() => {
    refetchMock.mockReset();
    saveMutationMock.mockReset().mockResolvedValue(undefined);
    deployMutationMock.mockReset().mockResolvedValue({ success: true });
    compileMutationMock.mockReset().mockResolvedValue({ success: true });
    // Reset store
    store.nodes = [];
    store.edges = [];
    store.selectedNodeId = null;
    store.environment = 'production';
    store.isDirty = false;
    store.isDeploying = false;
  });

  it('renders the Topology Editor header and environment select', () => {
    renderTopology();

    expect(
      screen.getByRole('heading', { name: /topology editor/i })
    ).toBeInTheDocument();
    // The environment <select> is rendered without a label, so we query the
    // combobox role directly. Production is the default selected value.
    const select = screen.getByRole('combobox') as HTMLSelectElement;
    expect(select.value).toBe('production');
  });

  it('renders the three header action buttons (Save, Compile, Deploy)', () => {
    renderTopology();

    expect(screen.getByRole('button', { name: /save/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /compile/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /deploy/i })).toBeInTheDocument();
  });

  it('renders the mocked ComponentPalette, TopologyCanvas, and ConfigPanel', () => {
    renderTopology();

    expect(screen.getByTestId('component-palette')).toBeInTheDocument();
    expect(screen.getByTestId('topology-canvas')).toBeInTheDocument();
    expect(screen.getByTestId('config-panel')).toBeInTheDocument();
  });

  it('keeps Save disabled when the store has no dirty changes', () => {
    store.isDirty = false;
    renderTopology();
    expect(screen.getByRole('button', { name: /save/i })).toBeDisabled();
  });

  it('enables Save when the store marks the topology as dirty', () => {
    store.isDirty = true;
    renderTopology();
    expect(screen.getByRole('button', { name: /save/i })).not.toBeDisabled();
    // The "Unsaved changes" marker should also appear in the header.
    expect(screen.getByText(/unsaved changes/i)).toBeInTheDocument();
  });

  it('disables Deploy when there are no nodes to deploy', () => {
    store.nodes = [];
    renderTopology();
    expect(screen.getByRole('button', { name: /deploy/i })).toBeDisabled();
  });

  it('enables Deploy when there is at least one node in the topology', () => {
    store.nodes = [{ id: 'n1', type: 'app', data: { label: 'API' } }];
    renderTopology();
    expect(screen.getByRole('button', { name: /deploy/i })).not.toBeDisabled();
  });

  it('calls the save mutation when Save is clicked with a dirty topology', async () => {
    store.isDirty = true;
    store.nodes = [{ id: 'n1', type: 'app', data: { name: 'demo' } }];
    renderTopology();

    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() =>
      expect(saveMutationMock).toHaveBeenCalledWith(
        expect.objectContaining({ projectId: 'proj-1', environment: 'production' })
      )
    );
  });

  it('calls the compile mutation when Compile is clicked with nodes present', async () => {
    store.nodes = [{ id: 'n1', type: 'app', data: {} }];
    renderTopology();

    fireEvent.click(screen.getByRole('button', { name: /compile/i }));

    await waitFor(() =>
      expect(compileMutationMock).toHaveBeenCalledWith(
        expect.objectContaining({ projectId: 'proj-1' })
      )
    );
    // The CompileModal is opened (mocked to render a test id only when open).
    expect(screen.getByTestId('compile-modal-open')).toBeInTheDocument();
  });

  it('opens the DeployModal and calls the deploy mutation when Deploy is clicked', async () => {
    store.nodes = [{ id: 'n1', type: 'app', data: {} }];
    renderTopology();

    fireEvent.click(screen.getByRole('button', { name: /deploy/i }));

    await waitFor(() =>
      expect(deployMutationMock).toHaveBeenCalledWith(
        expect.objectContaining({ projectId: 'proj-1' })
      )
    );
    expect(screen.getByTestId('deploy-modal-open')).toBeInTheDocument();
  });
});
