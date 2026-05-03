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

import { Servers } from '../Servers';

function fakeServer(overrides: Partial<{
  id: string;
  hostname: string;
  provider: string;
  region: string;
  size: string;
  role: string;
  status: string;
  ip_address: string;
  created_at: string;
}> = {}) {
  return {
    id: 'srv-1',
    hostname: 'web-01',
    provider: 'hetzner',
    region: 'fsn1',
    size: 'medium',
    role: 'worker',
    status: 'active',
    ip_address: '10.0.0.1',
    created_at: '2025-01-01T00:00:00Z',
    ...overrides,
  };
}

function renderServers() {
  return render(
    <MemoryRouter>
      <Servers />
    </MemoryRouter>
  );
}

describe('Servers page', () => {
  beforeEach(() => {
    useApiState.data = null;
    useApiState.loading = true;
    refetchMock.mockReset();
    apiPostMock.mockReset().mockResolvedValue(undefined);
    toastSuccessMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the hero header and Add Server CTA', () => {
    useApiState.data = [fakeServer()];
    useApiState.loading = false;
    renderServers();

    expect(screen.getByRole('heading', { name: /^servers$/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /add server/i })).toBeInTheDocument();
  });

  it('always renders the localhost master card even if the server list is empty', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderServers();

    expect(screen.getByText('localhost')).toBeInTheDocument();
    expect(screen.getByText('127.0.0.1')).toBeInTheDocument();
    expect(screen.getByText(/master node/i)).toBeInTheDocument();
  });

  it('does not duplicate the built-in localhost card if the API returns local server metadata', () => {
    useApiState.data = [
      fakeServer({
        id: 'local',
        hostname: 'localhost',
        provider: 'local',
        region: 'local',
        size: 'local',
        role: 'master',
        status: 'active',
        ip_address: '127.0.0.1',
      }),
    ];
    useApiState.loading = false;
    renderServers();

    expect(screen.getAllByText('localhost')).toHaveLength(1);
    expect(screen.getByText(/1 server/i)).toBeInTheDocument();
    expect(screen.getByText(/1 active/i)).toBeInTheDocument();
  });

  it('renders a card per remote server with provider badge', () => {
    useApiState.data = [
      fakeServer({ id: 'srv-a', hostname: 'web-01', provider: 'hetzner', status: 'active' }),
      fakeServer({
        id: 'srv-b',
        hostname: 'db-01',
        provider: 'digitalocean',
        status: 'stopped',
        ip_address: '10.0.0.2',
      }),
    ];
    useApiState.loading = false;
    renderServers();

    expect(screen.getByText('web-01')).toBeInTheDocument();
    expect(screen.getByText('db-01')).toBeInTheDocument();
    expect(screen.getByText('10.0.0.2')).toBeInTheDocument();
    // Provider label badges
    expect(screen.getByText('Hetzner Cloud')).toBeInTheDocument();
    expect(screen.getByText('DigitalOcean')).toBeInTheDocument();
    // Status badges: at least one Active (localhost + srv-a) and the Stopped srv-b
    expect(screen.getAllByText('Active').length).toBeGreaterThan(0);
    expect(screen.getByText('Stopped')).toBeInTheDocument();
  });

  it('opens the Add Server dialog and posts /servers with provider defaults', async () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderServers();

    fireEvent.click(screen.getByRole('button', { name: /add server/i }));
    fireEvent.click(screen.getByRole('button', { name: /hetzner cloud/i }));

    fireEvent.change(screen.getByLabelText(/hostname/i), {
      target: { value: 'my-new-server' },
    });
    // Pick a region so the body matches
    fireEvent.change(screen.getByLabelText(/region/i), {
      target: { value: 'fsn1' },
    });

    // Submit button is "Provision" for cloud providers
    fireEvent.click(screen.getByRole('button', { name: /^provision$/i }));

    await waitFor(() =>
      expect(apiPostMock).toHaveBeenCalledWith(
        '/servers',
        expect.objectContaining({
          hostname: 'my-new-server',
          provider: 'hetzner',
          region: 'fsn1',
          size: 'small',
        })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Server provisioning started')
    );
    await waitFor(() => expect(refetchMock).toHaveBeenCalled());
  });

  it('switches the dialog to IP/custom mode when Custom SSH is selected', async () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderServers();

    fireEvent.click(screen.getByRole('button', { name: /add server/i }));

    // Click the Custom SSH provider card. Accessible name is "Custom SSH".
    fireEvent.click(screen.getByRole('button', { name: /custom ssh/i }));

    // Region select should now be gone, IP Address input should be present.
    expect(screen.queryByLabelText(/region/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/ip address/i)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/hostname/i), {
      target: { value: 'existing-host' },
    });
    fireEvent.change(screen.getByLabelText(/ip address/i), {
      target: { value: '203.0.113.10' },
    });

    // The primary button label becomes "Connect" in custom mode.
    fireEvent.click(screen.getByRole('button', { name: /^connect$/i }));

    await waitFor(() =>
      expect(apiPostMock).toHaveBeenCalledWith(
        '/servers',
        expect.objectContaining({
          hostname: 'existing-host',
          provider: 'custom',
          ip_address: '203.0.113.10',
          region: '',
          size: '',
        })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Server connected')
    );
  });

  it('surfaces API errors inline inside the dialog', async () => {
    apiPostMock.mockRejectedValueOnce(new Error('provider down'));
    useApiState.data = [];
    useApiState.loading = false;
    renderServers();

    fireEvent.click(screen.getByRole('button', { name: /add server/i }));
    fireEvent.click(screen.getByRole('button', { name: /hetzner cloud/i }));
    fireEvent.change(screen.getByLabelText(/hostname/i), {
      target: { value: 'boom' },
    });
    fireEvent.change(screen.getByLabelText(/region/i), {
      target: { value: 'fsn1' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^provision$/i }));

    expect(await screen.findByText('provider down')).toBeInTheDocument();
  });

  it('keeps Connect button disabled until hostname and IP are entered', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderServers();

    fireEvent.click(screen.getByRole('button', { name: /add server/i }));

    const btn = screen.getByRole('button', { name: /^connect$/i });
    expect(btn).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/hostname/i), {
      target: { value: 'x' },
    });
    expect(btn).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/ip address/i), {
      target: { value: '203.0.113.10' },
    });
    expect(btn).not.toBeDisabled();
  });
});
