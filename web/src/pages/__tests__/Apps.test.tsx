import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------
//
// The Apps page hangs off three external modules we want to control:
//   1. useApi — feeds the list from a fake fixture rather than the network
//   2. appsAPI — asserts that Start/Stop/Restart/Delete fire the right calls
//   3. toast   — verifies failure paths surface a user-visible error
//
// We deliberately leave useDebouncedValue as its real implementation so the
// search-filter interaction is exercised end to end.

const refetchMock = vi.fn();
const useApiState: { data: unknown; loading: boolean } = {
  data: null,
  loading: true,
};
vi.mock('../../hooks', async () => {
  const actual = await vi.importActual<typeof import('../../hooks')>('../../hooks');
  return {
    ...actual,
    useApi: () => ({
      data: useApiState.data,
      loading: useApiState.loading,
      error: null,
      refetch: refetchMock,
    }),
    // Short-circuit debounce so the search input updates synchronously.
    useDebouncedValue: (value: string) => value,
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

import { Apps } from '../Apps';

function fakeApp(overrides: Partial<{
  id: string;
  name: string;
  type: string;
  source_type: string;
  status: string;
  branch: string;
  updated_at: string;
}> = {}) {
  return {
    id: 'app-1234567890abcdef',
    project_id: 'proj-1',
    tenant_id: 'tenant-1',
    name: 'alpha',
    type: 'service',
    source_type: 'git',
    source_url: 'https://github.com/example/alpha',
    branch: 'main',
    status: 'running',
    replicas: 1,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

function renderApps() {
  return render(
    <MemoryRouter>
      <Apps />
    </MemoryRouter>
  );
}

// The action buttons carry no accessible name — they are icon-only — so we
// key on the lucide-* class that each icon component emits. Returns the
// closest ancestor <button> so fireEvent.click lands on a real element.
function findActionButton(card: HTMLElement, lucideClass: string): HTMLElement {
  const icon = card.querySelector(`svg.${lucideClass}`);
  if (!icon) {
    throw new Error(`icon .${lucideClass} not found inside card`);
  }
  const btn = icon.closest('button');
  if (!btn) {
    throw new Error(`no <button> ancestor for .${lucideClass}`);
  }
  return btn as HTMLElement;
}

describe('Apps page', () => {
  beforeEach(() => {
    refetchMock.mockReset();
    startMock.mockReset().mockResolvedValue(undefined);
    stopMock.mockReset().mockResolvedValue(undefined);
    restartMock.mockReset().mockResolvedValue(undefined);
    deleteMock.mockReset().mockResolvedValue(undefined);
    toastErrorMock.mockReset();
    useApiState.data = null;
    useApiState.loading = true;
  });

  it('renders a loading skeleton before data arrives', () => {
    useApiState.data = null;
    useApiState.loading = true;
    renderApps();

    // The "New Application" CTA is always rendered; the real tell is the
    // absence of any app card or empty-state heading.
    expect(screen.getByRole('heading', { name: /applications/i })).toBeInTheDocument();
    expect(screen.queryByText(/no applications yet/i)).not.toBeInTheDocument();
    expect(screen.queryByText('alpha')).not.toBeInTheDocument();
  });

  it('shows the "no applications yet" empty state when the list is empty', () => {
    useApiState.data = { data: [], total: 0 };
    useApiState.loading = false;
    renderApps();

    expect(screen.getByText(/no applications yet/i)).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: /deploy your first app/i })
    ).toBeInTheDocument();
  });

  it('renders each app with its name, status and source badge', () => {
    useApiState.data = {
      data: [
        fakeApp({ id: 'a1', name: 'alpha', status: 'running', source_type: 'git' }),
        fakeApp({ id: 'a2', name: 'beta', status: 'stopped', source_type: 'docker' }),
        fakeApp({ id: 'a3', name: 'gamma', status: 'deploying', source_type: 'marketplace' }),
      ],
      total: 3,
    };
    useApiState.loading = false;
    renderApps();

    expect(screen.getByText('alpha')).toBeInTheDocument();
    expect(screen.getByText('beta')).toBeInTheDocument();
    expect(screen.getByText('gamma')).toBeInTheDocument();
    expect(screen.getByText('3 applications deployed')).toBeInTheDocument();

    // The status labels collide with segment-tab labels ("Running", etc.),
    // so assert the badges live inside each card, not globally.
    const alphaCard = screen.getByText('alpha').closest('a') as HTMLElement;
    expect(within(alphaCard).getByText('Running')).toBeInTheDocument();
    const betaCard = screen.getByText('beta').closest('a') as HTMLElement;
    expect(within(betaCard).getByText('Stopped')).toBeInTheDocument();
    const gammaCard = screen.getByText('gamma').closest('a') as HTMLElement;
    expect(within(gammaCard).getByText('Deploying')).toBeInTheDocument();
  });

  it('renders apps when the API client unwraps a paginated response to an array', () => {
    useApiState.data = [
      fakeApp({ id: 'a1', name: 'alpha', status: 'running', source_type: 'git' }),
      fakeApp({ id: 'a2', name: 'beta', status: 'stopped', source_type: 'docker' }),
    ];
    useApiState.loading = false;
    renderApps();

    expect(screen.getByText('alpha')).toBeInTheDocument();
    expect(screen.getByText('beta')).toBeInTheDocument();
    expect(screen.getByText('2 applications deployed')).toBeInTheDocument();
  });

  it('filters by status when a segment tab is clicked', () => {
    useApiState.data = {
      data: [
        fakeApp({ id: 'a1', name: 'alpha', status: 'running' }),
        fakeApp({ id: 'a2', name: 'beta', status: 'stopped' }),
      ],
      total: 2,
    };
    useApiState.loading = false;
    renderApps();

    expect(screen.getByText('alpha')).toBeInTheDocument();
    expect(screen.getByText('beta')).toBeInTheDocument();

    // The segment tabs and the status badges share the same label ("Running"),
    // so scope the click to the segment group by role=button.
    const stoppedTab = screen.getByRole('button', { name: /^stopped/i });
    fireEvent.click(stoppedTab);

    expect(screen.queryByText('alpha')).not.toBeInTheDocument();
    expect(screen.getByText('beta')).toBeInTheDocument();
  });

  it('filters by search text across app name', () => {
    useApiState.data = {
      data: [
        fakeApp({ id: 'a1', name: 'alpha' }),
        fakeApp({ id: 'a2', name: 'beta' }),
      ],
      total: 2,
    };
    useApiState.loading = false;
    renderApps();

    fireEvent.change(screen.getByPlaceholderText(/search applications/i), {
      target: { value: 'alph' },
    });

    expect(screen.getByText('alpha')).toBeInTheDocument();
    expect(screen.queryByText('beta')).not.toBeInTheDocument();
  });

  it('shows the "no matching" empty state when the search filter eliminates everything', () => {
    useApiState.data = {
      data: [fakeApp({ id: 'a1', name: 'alpha' })],
      total: 1,
    };
    useApiState.loading = false;
    renderApps();

    fireEvent.change(screen.getByPlaceholderText(/search applications/i), {
      target: { value: 'zzz' },
    });

    expect(screen.getByText(/no matching applications/i)).toBeInTheDocument();
  });

  it('calls appsAPI.stop and refetches when the Stop button is clicked', async () => {
    useApiState.data = {
      data: [fakeApp({ id: 'a1', name: 'alpha', status: 'running' })],
      total: 1,
    };
    useApiState.loading = false;
    renderApps();

    const card = screen.getByText('alpha').closest('a') as HTMLElement;
    fireEvent.click(findActionButton(card, 'lucide-square'));

    await waitFor(() => {
      expect(stopMock).toHaveBeenCalledWith('a1');
    });
    expect(refetchMock).toHaveBeenCalled();
  });

  it('calls appsAPI.start when the Start button is clicked on a stopped app', async () => {
    useApiState.data = {
      data: [fakeApp({ id: 'a1', name: 'alpha', status: 'stopped' })],
      total: 1,
    };
    useApiState.loading = false;
    renderApps();

    const card = screen.getByText('alpha').closest('a') as HTMLElement;
    fireEvent.click(findActionButton(card, 'lucide-play'));

    await waitFor(() => {
      expect(startMock).toHaveBeenCalledWith('a1');
    });
  });

  it('calls appsAPI.restart from the Restart button', async () => {
    useApiState.data = {
      data: [fakeApp({ id: 'a1', name: 'alpha', status: 'running' })],
      total: 1,
    };
    useApiState.loading = false;
    renderApps();

    const card = screen.getByText('alpha').closest('a') as HTMLElement;
    fireEvent.click(findActionButton(card, 'lucide-rotate-ccw'));

    await waitFor(() => {
      expect(restartMock).toHaveBeenCalledWith('a1');
    });
  });

  // Delete was moved from window.confirm() to an AlertDialog
  // (see Apps.tsx:425 — title "Delete Application", confirmLabel "Delete").
  // Click trash → dialog opens → click the dialog's Delete or Cancel.

  it('confirms before deleting and skips the API call when declined', () => {
    useApiState.data = {
      data: [fakeApp({ id: 'a1', name: 'alpha', status: 'running' })],
      total: 1,
    };
    useApiState.loading = false;
    renderApps();

    const card = screen.getByText('alpha').closest('a') as HTMLElement;
    fireEvent.click(findActionButton(card, 'lucide-trash2'));

    expect(screen.getByRole('heading', { name: /delete application/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }));

    expect(deleteMock).not.toHaveBeenCalled();
  });

  it('deletes when the confirm dialog is accepted', async () => {
    useApiState.data = {
      data: [fakeApp({ id: 'a1', name: 'alpha', status: 'running' })],
      total: 1,
    };
    useApiState.loading = false;
    renderApps();

    const card = screen.getByText('alpha').closest('a') as HTMLElement;
    fireEvent.click(findActionButton(card, 'lucide-trash2'));

    expect(screen.getByRole('heading', { name: /delete application/i })).toBeInTheDocument();
    const allDelete = screen.getAllByRole('button', { name: /^delete$/i });
    fireEvent.click(allDelete[allDelete.length - 1]);

    await waitFor(() => {
      expect(deleteMock).toHaveBeenCalledWith('a1');
    });
    expect(refetchMock).toHaveBeenCalled();
  });

  it('surfaces a toast error when an action fails', async () => {
    useApiState.data = {
      data: [fakeApp({ id: 'a1', name: 'alpha', status: 'running' })],
      total: 1,
    };
    useApiState.loading = false;

    stopMock.mockRejectedValueOnce(new Error('boom'));
    renderApps();

    const card = screen.getByText('alpha').closest('a') as HTMLElement;
    fireEvent.click(findActionButton(card, 'lucide-square'));

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith('Action failed');
    });
  });
});
