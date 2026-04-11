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

const apiPostMock = vi.fn();
vi.mock('@/api/client', () => ({
  api: {
    post: (path: string, body: unknown) => apiPostMock(path, body),
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

import { Databases } from '../Databases';

function fakeDb(overrides: Partial<{
  id: string;
  name: string;
  engine: string;
  version: string;
  status: string;
  connection_string: string;
  size_mb: number;
  created_at: string;
}> = {}) {
  return {
    id: 'db-1',
    name: 'my-pg',
    engine: 'postgres',
    version: '17',
    status: 'running',
    connection_string: 'postgres://user:pass@host:5432/my-pg',
    size_mb: 128,
    created_at: '2025-01-01T00:00:00Z',
    ...overrides,
  };
}

function renderDatabases() {
  return render(
    <MemoryRouter>
      <Databases />
    </MemoryRouter>
  );
}

describe('Databases page', () => {
  beforeEach(() => {
    useApiState.data = null;
    useApiState.loading = true;
    refetchMock.mockReset();
    apiPostMock.mockReset().mockResolvedValue(undefined);
    toastSuccessMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the hero header with the Databases title and count', () => {
    useApiState.data = [fakeDb()];
    useApiState.loading = false;
    renderDatabases();

    expect(screen.getByRole('heading', { name: /^databases$/i })).toBeInTheDocument();
    expect(screen.getByText(/1 database/i)).toBeInTheDocument();
  });

  it('renders the empty state with the first-database CTA', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderDatabases();

    expect(screen.getByRole('heading', { name: /no databases yet/i })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /create your first database/i })
    ).toBeInTheDocument();
  });

  it('renders one card per database with engine label and status badge', () => {
    useApiState.data = [
      fakeDb({ id: 'db-1', name: 'orders', engine: 'postgres', version: '17' }),
      fakeDb({ id: 'db-2', name: 'cache', engine: 'redis', version: '7', status: 'stopped' }),
    ];
    useApiState.loading = false;
    renderDatabases();

    expect(screen.getByText('orders')).toBeInTheDocument();
    expect(screen.getByText('cache')).toBeInTheDocument();
    expect(screen.getByText(/postgresql v17/i)).toBeInTheDocument();
    expect(screen.getByText(/redis v7/i)).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
    expect(screen.getByText('Stopped')).toBeInTheDocument();
  });

  it('filters the database grid based on the search input', () => {
    useApiState.data = [
      fakeDb({ id: 'db-1', name: 'orders' }),
      fakeDb({ id: 'db-2', name: 'billing' }),
    ];
    useApiState.loading = false;
    renderDatabases();

    fireEvent.change(screen.getByPlaceholderText(/search databases/i), {
      target: { value: 'bill' },
    });

    expect(screen.queryByText('orders')).not.toBeInTheDocument();
    expect(screen.getByText('billing')).toBeInTheDocument();
  });

  it('shows a "no databases found" message when the search matches nothing', () => {
    useApiState.data = [fakeDb({ name: 'orders' })];
    useApiState.loading = false;
    renderDatabases();

    fireEvent.change(screen.getByPlaceholderText(/search databases/i), {
      target: { value: 'zzz' },
    });

    expect(screen.getByText(/no databases found/i)).toBeInTheDocument();
  });

  it('posts /databases when the create dialog is submitted with a name', async () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderDatabases();

    fireEvent.click(screen.getByRole('button', { name: /create your first database/i }));

    fireEvent.change(screen.getByLabelText(/database name/i), {
      target: { value: 'my-new-db' },
    });

    // Dialog's primary button accessible name is exactly "Create" (the icon
    // Plus has no label).
    const createBtn = screen.getByRole('button', { name: /^create$/i });
    fireEvent.click(createBtn);

    await waitFor(() =>
      expect(apiPostMock).toHaveBeenCalledWith(
        '/databases',
        expect.objectContaining({
          name: 'my-new-db',
          engine: 'postgres',
          version: '17',
        })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Database created successfully')
    );
    await waitFor(() => expect(refetchMock).toHaveBeenCalled());
  });

  it('surfaces the API error inline inside the dialog on create failure', async () => {
    apiPostMock.mockRejectedValueOnce(new Error('name taken'));
    useApiState.data = [];
    useApiState.loading = false;
    renderDatabases();

    fireEvent.click(screen.getByRole('button', { name: /create your first database/i }));
    fireEvent.change(screen.getByLabelText(/database name/i), {
      target: { value: 'dup' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^create$/i }));

    expect(await screen.findByText('name taken')).toBeInTheDocument();
  });

  it('switches the engine in the dialog when another engine card is clicked', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderDatabases();

    fireEvent.click(screen.getByRole('button', { name: /create your first database/i }));

    // The MySQL card uses a <button> whose accessible name includes both
    // the letter marker ("MY") and the engine name ("MySQL").
    const mysqlBtn = screen.getByRole('button', { name: /mysql/i });
    fireEvent.click(mysqlBtn);

    // Version select should now show the MySQL options (8.4 as the default).
    const select = screen.getByLabelText(/version/i) as HTMLSelectElement;
    expect(select.value).toBe('8.4');
  });

  it('copies the connection string when the copy button on a card is clicked', () => {
    const writeTextMock = vi.fn().mockResolvedValue(undefined);
    // jsdom doesn't include a clipboard; stub one in.
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextMock },
      configurable: true,
    });
    useApiState.data = [
      fakeDb({
        id: 'db-1',
        name: 'orders',
        connection_string: 'postgres://u:p@h/orders',
      }),
    ];
    useApiState.loading = false;
    renderDatabases();

    const copyIcon = document.querySelector('svg.lucide-copy');
    const btn = copyIcon?.closest('button');
    fireEvent.click(btn!);

    expect(writeTextMock).toHaveBeenCalledWith('postgres://u:p@h/orders');
    expect(toastSuccessMock).toHaveBeenCalledWith('Connection string copied');
  });
});
