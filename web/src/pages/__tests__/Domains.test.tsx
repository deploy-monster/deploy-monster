import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------

const refetchMock = vi.fn();
const useApiState: { data: unknown; loading: boolean } = {
  data: null,
  loading: true,
};
vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks');
  return {
    ...actual,
    useApi: () => ({
      data: useApiState.data,
      loading: useApiState.loading,
      error: null,
      refetch: refetchMock,
    }),
    useDebouncedValue: (value: string) => value,
  };
});

const apiPostMock = vi.fn();
const apiDeleteMock = vi.fn();
vi.mock('@/api/client', () => ({
  api: {
    post: (path: string, body: unknown) => apiPostMock(path, body),
    delete: (path: string) => apiDeleteMock(path),
  },
}));

const toastSuccessMock = vi.fn();
const toastErrorMock = vi.fn();
vi.mock('@/stores/toastStore', () => ({
  toast: {
    success: (msg: string) => toastSuccessMock(msg),
    error: (msg: string) => toastErrorMock(msg),
    info: vi.fn(),
  },
}));

import { Domains } from '../Domains';

function fakeDomain(overrides: Partial<{
  id: string;
  fqdn: string;
  app_id: string;
  type: string;
  dns_provider: string;
  dns_synced: boolean;
  verified: boolean;
  created_at: string;
}> = {}) {
  return {
    id: 'dom-1',
    fqdn: 'app.example.com',
    app_id: 'app-1',
    type: 'custom',
    dns_provider: 'manual',
    dns_synced: false,
    verified: true,
    created_at: '2025-01-01T00:00:00Z',
    ...overrides,
  };
}

function renderDomains() {
  return render(
    <MemoryRouter>
      <Domains />
    </MemoryRouter>
  );
}

describe('Domains page', () => {
  beforeEach(() => {
    useApiState.data = null;
    useApiState.loading = true;
    refetchMock.mockReset();
    apiPostMock.mockReset().mockResolvedValue(undefined);
    apiDeleteMock.mockReset().mockResolvedValue(undefined);
    toastSuccessMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the hero header and Add Domain CTA', () => {
    useApiState.data = [fakeDomain()];
    useApiState.loading = false;
    renderDomains();

    expect(screen.getByRole('heading', { name: /^domains$/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /add domain/i })).toBeInTheDocument();
  });

  it('renders the skeleton table while loading', () => {
    useApiState.data = null;
    useApiState.loading = true;
    renderDomains();

    // The table header row isn't rendered during load (Card skeleton only)
    expect(screen.queryByText('SSL Status')).not.toBeInTheDocument();
  });

  it('renders the empty state with the first-domain CTA when the list is empty', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderDomains();

    expect(
      screen.getByRole('heading', { name: /no domains configured/i })
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /add your first domain/i })
    ).toBeInTheDocument();
  });

  it('renders the table with domain rows and Active/Pending SSL badges', () => {
    useApiState.data = [
      fakeDomain({ id: 'd1', fqdn: 'a.example.com', verified: true }),
      fakeDomain({ id: 'd2', fqdn: 'b.example.com', verified: false }),
    ];
    useApiState.loading = false;
    renderDomains();

    expect(screen.getByText('a.example.com')).toBeInTheDocument();
    expect(screen.getByText('b.example.com')).toBeInTheDocument();
    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.getByText('Pending')).toBeInTheDocument();
  });

  it('filters the table by the search input', () => {
    useApiState.data = [
      fakeDomain({ id: 'd1', fqdn: 'alpha.example.com' }),
      fakeDomain({ id: 'd2', fqdn: 'beta.example.com' }),
    ];
    useApiState.loading = false;
    renderDomains();

    fireEvent.change(screen.getByPlaceholderText(/search domains/i), {
      target: { value: 'beta' },
    });

    expect(screen.queryByText('alpha.example.com')).not.toBeInTheDocument();
    expect(screen.getByText('beta.example.com')).toBeInTheDocument();
  });

  it('shows the "no results" copy when the search matches nothing', () => {
    useApiState.data = [fakeDomain({ fqdn: 'a.example.com' })];
    useApiState.loading = false;
    renderDomains();

    fireEvent.change(screen.getByPlaceholderText(/search domains/i), {
      target: { value: 'zzz' },
    });

    expect(screen.getByText(/no domains found/i)).toBeInTheDocument();
  });

  it('opens the Add Domain dialog and posts /domains when submitted', async () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderDomains();

    // Use the empty-state CTA to open the dialog.
    fireEvent.click(screen.getByRole('button', { name: /add your first domain/i }));

    fireEvent.change(screen.getByLabelText(/domain.*fqdn/i), {
      target: { value: 'new.example.com' },
    });
    fireEvent.change(screen.getByLabelText(/application id/i), {
      target: { value: 'app-42' },
    });

    // The dialog's submit button is the second button named /add domain/i —
    // pick the one that is NOT the header's add button (they coexist now).
    const addBtns = screen.getAllByRole('button', { name: /^add domain$/i });
    fireEvent.click(addBtns[addBtns.length - 1]);

    await waitFor(() =>
      expect(apiPostMock).toHaveBeenCalledWith(
        '/domains',
        expect.objectContaining({ fqdn: 'new.example.com', app_id: 'app-42' })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Domain added successfully')
    );
    await waitFor(() => expect(refetchMock).toHaveBeenCalled());
  });

  it('surfaces the API error message inside the dialog on add failure', async () => {
    apiPostMock.mockRejectedValueOnce(new Error('duplicate domain'));
    useApiState.data = [];
    useApiState.loading = false;
    renderDomains();

    fireEvent.click(screen.getByRole('button', { name: /add your first domain/i }));
    fireEvent.change(screen.getByLabelText(/domain.*fqdn/i), {
      target: { value: 'dup.example.com' },
    });

    const addBtns = screen.getAllByRole('button', { name: /^add domain$/i });
    fireEvent.click(addBtns[addBtns.length - 1]);

    expect(await screen.findByText('duplicate domain')).toBeInTheDocument();
  });

  it('calls POST /domains/:id/verify when clicking the verify button for a pending row', async () => {
    useApiState.data = [fakeDomain({ id: 'dp', fqdn: 'p.example.com', verified: false })];
    useApiState.loading = false;
    renderDomains();

    // The verify button is an icon button only shown for unverified rows.
    // It uses CheckCircle (lucide-circle-check-big).
    const icon =
      document.querySelector('svg.lucide-circle-check-big') ||
      document.querySelector('svg.lucide-check-circle');
    const verifyBtn = icon?.closest('button');
    expect(verifyBtn).not.toBeNull();
    fireEvent.click(verifyBtn!);

    await waitFor(() =>
      expect(apiPostMock).toHaveBeenCalledWith('/domains/dp/verify', expect.anything())
    );
    await waitFor(() => expect(toastSuccessMock).toHaveBeenCalledWith('Domain verified'));
  });

  it('calls DELETE /domains/:id when the delete confirm prompt is accepted', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    useApiState.data = [fakeDomain({ id: 'dd', fqdn: 'x.example.com', verified: true })];
    useApiState.loading = false;
    renderDomains();

    const trashIcon = document.querySelector('svg.lucide-trash2');
    const deleteBtn = trashIcon?.closest('button');
    fireEvent.click(deleteBtn!);

    await waitFor(() => expect(apiDeleteMock).toHaveBeenCalledWith('/domains/dd'));
    await waitFor(() => expect(toastSuccessMock).toHaveBeenCalledWith('Domain removed'));
    confirmSpy.mockRestore();
  });

  it('skips the delete API call when the confirm prompt is declined', () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
    useApiState.data = [fakeDomain({ id: 'dd', verified: true })];
    useApiState.loading = false;
    renderDomains();

    const trashIcon = document.querySelector('svg.lucide-trash2');
    const deleteBtn = trashIcon?.closest('button');
    fireEvent.click(deleteBtn!);

    expect(apiDeleteMock).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });
});
