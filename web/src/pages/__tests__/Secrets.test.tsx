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
    useDebouncedValue: (v: string) => v,
  };
});

const createMock = vi.fn();
const deleteMock = vi.fn();
vi.mock('@/api/secrets', () => ({
  secretsAPI: {
    create: (data: unknown) => createMock(data),
    delete: (id: string) => deleteMock(id),
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

import { Secrets } from '../Secrets';

function fakeSecret(overrides: Partial<{
  id: string;
  name: string;
  scope: string;
  created_at: string;
  updated_at: string;
}> = {}) {
  return {
    id: 'sec-1',
    name: 'DB_PASSWORD',
    scope: 'tenant',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-02T00:00:00Z',
    ...overrides,
  };
}

function renderSecrets() {
  return render(
    <MemoryRouter>
      <Secrets />
    </MemoryRouter>
  );
}

describe('Secrets page', () => {
  beforeEach(() => {
    useApiState.data = null;
    useApiState.loading = true;
    refetchMock.mockReset();
    createMock.mockReset().mockResolvedValue(undefined);
    deleteMock.mockReset().mockResolvedValue(undefined);
    toastSuccessMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the Secrets header and Add Secret CTA', () => {
    useApiState.data = [fakeSecret()];
    useApiState.loading = false;
    renderSecrets();

    expect(screen.getByRole('heading', { name: /^secrets$/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /add secret/i })).toBeInTheDocument();
    expect(screen.getByText(/1 secret/i)).toBeInTheDocument();
  });

  it('renders the empty-vault state when the list is empty', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderSecrets();

    expect(screen.getByRole('heading', { name: /secret vault/i })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /add your first secret/i })
    ).toBeInTheDocument();
  });

  it('lists the secrets in the table with scope badges', () => {
    useApiState.data = [
      fakeSecret({ id: 's1', name: 'DB_URL', scope: 'tenant' }),
      fakeSecret({ id: 's2', name: 'API_KEY', scope: 'project' }),
    ];
    useApiState.loading = false;
    renderSecrets();

    expect(screen.getByText('DB_URL')).toBeInTheDocument();
    expect(screen.getByText('API_KEY')).toBeInTheDocument();
    // Scope labels show up twice: in the scope summary cards and in the
    // row badges. We just want at least one of each.
    expect(screen.getAllByText('Tenant').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Project').length).toBeGreaterThan(0);
  });

  it('filters the table as the user types in the search input', () => {
    useApiState.data = [
      fakeSecret({ id: 's1', name: 'STRIPE_KEY', scope: 'tenant' }),
      fakeSecret({ id: 's2', name: 'DB_URL', scope: 'tenant' }),
    ];
    useApiState.loading = false;
    renderSecrets();

    fireEvent.change(screen.getByPlaceholderText(/search secrets/i), {
      target: { value: 'stripe' },
    });

    expect(screen.getByText('STRIPE_KEY')).toBeInTheDocument();
    expect(screen.queryByText('DB_URL')).not.toBeInTheDocument();
  });

  it('posts the new secret through secretsAPI.create on dialog submit', async () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderSecrets();

    fireEvent.click(screen.getByRole('button', { name: /add your first secret/i }));

    fireEvent.change(screen.getByLabelText(/^name$/i), {
      target: { value: 'STRIPE_KEY' },
    });
    fireEvent.change(screen.getByLabelText(/^value$/i), {
      target: { value: 'sk_test_1234' },
    });
    // The submit button inside the dialog has name "Create Secret".
    fireEvent.click(screen.getByRole('button', { name: /create secret/i }));

    await waitFor(() =>
      expect(createMock).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'STRIPE_KEY',
          value: 'sk_test_1234',
          scope: 'tenant',
        })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Secret created successfully')
    );
  });

  it('sends the selected scope value when a different scope card is picked', async () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderSecrets();

    fireEvent.click(screen.getByRole('button', { name: /add your first secret/i }));

    // Click the Global scope card. Its accessible name is the label text
    // "Global" (there is only a KeyRound icon + text inside).
    fireEvent.click(screen.getByRole('button', { name: /^global$/i }));

    fireEvent.change(screen.getByLabelText(/^name$/i), {
      target: { value: 'TELEMETRY_TOKEN' },
    });
    fireEvent.change(screen.getByLabelText(/^value$/i), {
      target: { value: 'xyz' },
    });
    fireEvent.click(screen.getByRole('button', { name: /create secret/i }));

    await waitFor(() =>
      expect(createMock).toHaveBeenCalledWith(
        expect.objectContaining({ scope: 'global' })
      )
    );
  });

  it('surfaces the API error inside the dialog when create fails', async () => {
    createMock.mockRejectedValueOnce(new Error('name already exists'));
    useApiState.data = [];
    useApiState.loading = false;
    renderSecrets();

    fireEvent.click(screen.getByRole('button', { name: /add your first secret/i }));
    fireEvent.change(screen.getByLabelText(/^name$/i), {
      target: { value: 'DUP' },
    });
    fireEvent.change(screen.getByLabelText(/^value$/i), {
      target: { value: 'v' },
    });
    fireEvent.click(screen.getByRole('button', { name: /create secret/i }));

    expect(await screen.findByText('name already exists')).toBeInTheDocument();
  });

  // Delete uses AlertDialog (title "Delete Secret", confirmLabel "Delete")
  // — see Secrets.tsx:450.

  it('calls secretsAPI.delete when the delete confirm prompt is accepted', async () => {
    useApiState.data = [fakeSecret({ id: 's1', name: 'DB_URL' })];
    useApiState.loading = false;
    renderSecrets();

    const trashIcon = document.querySelector('svg.lucide-trash2');
    const btn = trashIcon?.closest('button');
    fireEvent.click(btn!);

    expect(screen.getByRole('heading', { name: /delete secret/i })).toBeInTheDocument();
    const allDelete = screen.getAllByRole('button', { name: /^delete$/i });
    fireEvent.click(allDelete[allDelete.length - 1]);

    await waitFor(() => expect(deleteMock).toHaveBeenCalledWith('s1'));
    await waitFor(() => expect(toastSuccessMock).toHaveBeenCalledWith('Secret deleted'));
  });

  it('skips the delete call if the confirm prompt is declined', () => {
    useApiState.data = [fakeSecret({ id: 's1' })];
    useApiState.loading = false;
    renderSecrets();

    const trashIcon = document.querySelector('svg.lucide-trash2');
    const btn = trashIcon?.closest('button');
    fireEvent.click(btn!);

    expect(screen.getByRole('heading', { name: /delete secret/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }));

    expect(deleteMock).not.toHaveBeenCalled();
  });
});
