import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------
//
// AppDetail fires two parallel useApi calls: one for `/apps/${id}` and one
// for `/apps/${id}/deployments`. We route by path suffix so each feed can be
// primed independently per test.

type ApiResponse = { data: unknown; loading: boolean };
const appState: ApiResponse = { data: null, loading: true };
const deploymentsState: ApiResponse = { data: [], loading: false };
const refetchAppMock = vi.fn();

vi.mock('../../hooks', async () => {
  const actual = await vi.importActual<typeof import('../../hooks')>('../../hooks');
  return {
    ...actual,
    useApi: (path: string) => {
      if (path.includes('/deployments')) {
        return {
          data: deploymentsState.data,
          loading: deploymentsState.loading,
          error: null,
          refetch: vi.fn(),
        };
      }
      return {
        data: appState.data,
        loading: appState.loading,
        error: null,
        refetch: refetchAppMock,
      };
    },
  };
});

const startMock = vi.fn();
const stopMock = vi.fn();
const restartMock = vi.fn();
const deleteMock = vi.fn();
vi.mock('../../api/apps', () => ({
  appsAPI: {
    start: (id: string) => startMock(id),
    stop: (id: string) => stopMock(id),
    restart: (id: string) => restartMock(id),
    delete: (id: string) => deleteMock(id),
  },
}));

const toastErrorMock = vi.fn();
vi.mock('@/stores/toastStore', () => ({
  toast: {
    error: (msg: string) => toastErrorMock(msg),
    success: vi.fn(),
    info: vi.fn(),
  },
}));

// Pin useParams so the page always sees the same `id`.
vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router');
  return {
    ...actual,
    useParams: () => ({ id: 'app-1' }),
  };
});

import { AppDetail } from '../AppDetail';

function fakeApp(
  overrides: Partial<{
    id: string;
    name: string;
    type: string;
    source_type: string;
    source_url: string;
    branch: string;
    status: string;
    replicas: number;
    created_at: string;
    updated_at: string;
  }> = {}
) {
  return {
    id: 'app-1',
    project_id: 'proj-1',
    tenant_id: 'tenant-1',
    name: 'my-app',
    type: 'web',
    source_type: 'git',
    source_url: 'https://github.com/example/my-app.git',
    branch: 'main',
    status: 'running',
    replicas: 1,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-02T00:00:00Z',
    ...overrides,
  };
}

function renderAppDetail() {
  return render(
    <MemoryRouter>
      <AppDetail />
    </MemoryRouter>
  );
}

describe('AppDetail page', () => {
  beforeEach(() => {
    appState.data = null;
    appState.loading = true;
    deploymentsState.data = [];
    deploymentsState.loading = false;
    refetchAppMock.mockReset();
    startMock.mockReset().mockResolvedValue(undefined);
    stopMock.mockReset().mockResolvedValue(undefined);
    restartMock.mockReset().mockResolvedValue(undefined);
    deleteMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the loading state while the app fetch is in flight', () => {
    appState.data = null;
    appState.loading = true;
    renderAppDetail();

    expect(screen.getByText(/loading application/i)).toBeInTheDocument();
  });

  it('renders the app name and branch once loaded', () => {
    appState.data = fakeApp({ name: 'my-app', branch: 'develop' });
    appState.loading = false;
    renderAppDetail();

    expect(screen.getByRole('heading', { name: 'my-app' })).toBeInTheDocument();
    // 'develop' shows up twice: once in the header span and once in the
    // Application Info card's Branch row. We just care that it rendered.
    expect(screen.getAllByText('develop').length).toBeGreaterThan(0);
    // Status badge renders the label
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('calls appsAPI.restart when the header Restart button is clicked', async () => {
    appState.data = fakeApp({ status: 'running' });
    appState.loading = false;
    renderAppDetail();

    fireEvent.click(screen.getByRole('button', { name: /^restart$/i }));

    await waitFor(() => expect(restartMock).toHaveBeenCalledWith('app-1'));
    await waitFor(() => expect(refetchAppMock).toHaveBeenCalled());
  });

  it('calls appsAPI.stop when clicking Stop on a running app', async () => {
    appState.data = fakeApp({ status: 'running' });
    appState.loading = false;
    renderAppDetail();

    fireEvent.click(screen.getByRole('button', { name: /^stop$/i }));

    await waitFor(() => expect(stopMock).toHaveBeenCalledWith('app-1'));
  });

  it('calls appsAPI.start when clicking Start on a stopped app', async () => {
    appState.data = fakeApp({ status: 'stopped' });
    appState.loading = false;
    renderAppDetail();

    fireEvent.click(screen.getByRole('button', { name: /^start$/i }));

    await waitFor(() => expect(startMock).toHaveBeenCalledWith('app-1'));
  });

  it('re-uses the restart endpoint when the header Deploy button is clicked', async () => {
    appState.data = fakeApp({ status: 'running' });
    appState.loading = false;
    renderAppDetail();

    // The header Deploy button's accessible name is exactly "Deploy"; the
    // Latest Deployment empty-state button is labelled "Deploy Now", so the
    // anchored regex disambiguates them.
    fireEvent.click(screen.getByRole('button', { name: /^deploy$/i }));

    await waitFor(() => expect(restartMock).toHaveBeenCalledWith('app-1'));
  });

  it('surfaces a toast.error when a control action rejects', async () => {
    restartMock.mockRejectedValueOnce(new Error('boom'));
    appState.data = fakeApp({ status: 'running' });
    appState.loading = false;
    renderAppDetail();

    fireEvent.click(screen.getByRole('button', { name: /^restart$/i }));

    await waitFor(() =>
      expect(toastErrorMock).toHaveBeenCalledWith('Failed to restart application')
    );
  });

  // Delete flow was refactored from window.confirm() to a custom AlertDialog
  // (see AppDetail.tsx:826). The dialog's Confirm/Cancel are buttons inside
  // a Dialog with title "Delete Application" — click the trash icon to
  // open it, then click Delete or Cancel to resolve.

  it('asks for confirmation before deleting and skips the API call when denied', () => {
    appState.data = fakeApp();
    appState.loading = false;
    renderAppDetail();

    const trashIcon = document.querySelector('svg.lucide-trash2');
    const deleteBtn = trashIcon?.closest('button');
    expect(deleteBtn).not.toBeNull();
    fireEvent.click(deleteBtn!);

    // Dialog now open. Cancel by clicking the dialog's Cancel button.
    expect(screen.getByRole('heading', { name: /delete application/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }));

    expect(deleteMock).not.toHaveBeenCalled();
  });

  it('calls appsAPI.delete when the user confirms the delete prompt', () => {
    // Never-resolving promise so the handler doesn't reach
    // `window.location.href = '/apps'`, which jsdom refuses to honor.
    deleteMock.mockImplementation(() => new Promise(() => {}));
    appState.data = fakeApp();
    appState.loading = false;
    renderAppDetail();

    const trashIcon = document.querySelector('svg.lucide-trash2');
    const deleteBtn = trashIcon?.closest('button');
    fireEvent.click(deleteBtn!);

    // Dialog opens — find the confirm Delete button (there are multiple
    // "Delete" buttons on the page; the one in the dialog is the one we
    // want). Scope to the dialog container by its title.
    expect(screen.getByRole('heading', { name: /delete application/i })).toBeInTheDocument();
    const allDelete = screen.getAllByRole('button', { name: /^delete$/i });
    // One of them is inside the dialog and triggers handleDelete; clicking
    // it should fire deleteMock. At minimum one exists.
    const dialogDelete = allDelete[allDelete.length - 1]; // dialog renders last, button is inside it
    fireEvent.click(dialogDelete);

    expect(deleteMock).toHaveBeenCalledWith('app-1');
  });

  it('switches to the Environment tab and lists the seeded env vars', () => {
    appState.data = fakeApp();
    appState.loading = false;
    renderAppDetail();

    fireEvent.click(screen.getByRole('tab', { name: /environment/i }));

    expect(screen.getByText('NODE_ENV')).toBeInTheDocument();
    expect(screen.getByText('DATABASE_URL')).toBeInTheDocument();
    // NODE_ENV is non-secret so its value renders in the clear.
    expect(screen.getByText('production')).toBeInTheDocument();
  });

  it('adds a new env var from the form and renders it in the table', () => {
    appState.data = fakeApp();
    appState.loading = false;
    renderAppDetail();

    fireEvent.click(screen.getByRole('tab', { name: /environment/i }));

    // The Key input upper-cases its value on change, so typing "new_flag"
    // lands as "NEW_FLAG".
    fireEvent.change(screen.getByLabelText(/^key$/i), { target: { value: 'new_flag' } });
    fireEvent.change(screen.getByLabelText(/^value$/i), { target: { value: 'on' } });
    expect((screen.getByLabelText(/^key$/i) as HTMLInputElement).value).toBe('NEW_FLAG');

    fireEvent.click(screen.getByRole('button', { name: /add variable/i }));

    expect(screen.getByText('NEW_FLAG')).toBeInTheDocument();
    expect(screen.getByText('on')).toBeInTheDocument();
  });

  it('toggles secret value visibility when the Reveal button is clicked', () => {
    appState.data = fakeApp();
    appState.loading = false;
    renderAppDetail();

    fireEvent.click(screen.getByRole('tab', { name: /environment/i }));

    // Secret values are masked before revealing.
    expect(screen.queryByText('${SECRET:db_url}')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /reveal values/i }));

    expect(screen.getByText('${SECRET:db_url}')).toBeInTheDocument();
  });

  it('renders the empty state on the Deployments tab when there is no history', () => {
    appState.data = fakeApp();
    appState.loading = false;
    deploymentsState.data = [];
    renderAppDetail();

    fireEvent.click(screen.getByRole('tab', { name: /deployments/i }));

    expect(screen.getByText(/no deployments yet/i)).toBeInTheDocument();
  });

  it('renders the deployment history table with a rollback control for older rows', () => {
    appState.data = fakeApp();
    appState.loading = false;
    deploymentsState.data = [
      {
        id: 'd1',
        version: 3,
        image: 'ghcr.io/example/app:3',
        status: 'success',
        commit_sha: 'abcdef1234567890',
        triggered_by: 'alice',
        created_at: new Date().toISOString(),
      },
      {
        id: 'd2',
        version: 2,
        image: 'ghcr.io/example/app:2',
        status: 'success',
        commit_sha: '1234abcd5678ef90',
        triggered_by: 'bob',
        created_at: new Date().toISOString(),
      },
    ];
    renderAppDetail();

    fireEvent.click(screen.getByRole('tab', { name: /deployments/i }));

    expect(screen.getByText('v3')).toBeInTheDocument();
    expect(screen.getByText('v2')).toBeInTheDocument();
    // Only rows at index > 0 render a Rollback action.
    expect(screen.getByRole('button', { name: /rollback/i })).toBeInTheDocument();
  });
});
