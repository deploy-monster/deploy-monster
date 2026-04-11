import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------
//
// The Admin page fires two parallel useApi() calls: /admin/system (system
// info + modules) and /admin/tenants (tenant list). We route by path so each
// feed can be primed independently.

type ApiResponse = { data: unknown; loading: boolean };
const apiResponses: Record<string, ApiResponse> = {};

function setApi(path: string, data: unknown, loading = false) {
  apiResponses[path] = { data, loading };
}

function clearApi() {
  for (const k of Object.keys(apiResponses)) delete apiResponses[k];
}

const systemRefetchMock = vi.fn();

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks');
  return {
    ...actual,
    useApi: (path: string) => {
      const res = apiResponses[path] ?? { data: null, loading: true };
      const refetch = path === '/admin/system' ? systemRefetchMock : vi.fn();
      return { data: res.data, loading: res.loading, error: null, refetch };
    },
  };
});

const saveSettingsMock = vi.fn();
vi.mock('@/api/admin', () => ({
  adminAPI: {
    saveSettings: (data: unknown) => saveSettingsMock(data),
    // keep the named type exports happy; actual calls routed through the mock.
    generateApiKey: vi.fn(),
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

import { Admin } from '../Admin';

const systemFixture = {
  version: '1.0.0',
  commit: 'deadbeefcafef00d',
  go: 'go1.26.0',
  os: 'linux',
  arch: 'amd64',
  goroutines: 42,
  memory: { alloc_mb: 128, sys_mb: 256 },
  modules: [
    { id: 'auth', status: 'ok' },
    { id: 'deploy', status: 'degraded' },
    { id: 'billing', status: 'down' },
  ],
  events: { published: 99, errors: 1, subscriptions: 12 },
};

function renderAdmin() {
  return render(
    <MemoryRouter>
      <Admin />
    </MemoryRouter>
  );
}

describe('Admin page', () => {
  beforeEach(() => {
    clearApi();
    systemRefetchMock.mockReset();
    saveSettingsMock.mockReset().mockResolvedValue(undefined);
    toastSuccessMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the hero header with the Admin Panel title', () => {
    setApi('/admin/system', systemFixture);
    setApi('/admin/tenants', []);
    renderAdmin();

    expect(screen.getByRole('heading', { name: /admin panel/i })).toBeInTheDocument();
    // Version badge only shows when system data is loaded.
    expect(screen.getByText('v1.0.0')).toBeInTheDocument();
  });

  it('renders the five system stat cards with values from /admin/system', () => {
    setApi('/admin/system', systemFixture);
    setApi('/admin/tenants', []);
    renderAdmin();

    // version card — the Version label maps to value "1.0.0"
    expect(screen.getByText('1.0.0')).toBeInTheDocument();
    // runtime card — "go1.26.0" and the os/arch suffix
    expect(screen.getByText('go1.26.0')).toBeInTheDocument();
    expect(screen.getByText('linux/amd64')).toBeInTheDocument();
    // memory, goroutines, events
    expect(screen.getByText('128 MB')).toBeInTheDocument();
    expect(screen.getByText('42')).toBeInTheDocument();
    expect(screen.getByText('99')).toBeInTheDocument();
  });

  it('renders the modules table with a row per module', () => {
    setApi('/admin/system', systemFixture);
    setApi('/admin/tenants', []);
    renderAdmin();

    expect(screen.getByText('auth')).toBeInTheDocument();
    expect(screen.getByText('deploy')).toBeInTheDocument();
    expect(screen.getByText('billing')).toBeInTheDocument();
    // Healthy count summary — 1 of 3 is "ok"
    expect(screen.getByText(/1 healthy/i)).toBeInTheDocument();
  });

  it('calls refetch on the system feed when Refresh is clicked', () => {
    setApi('/admin/system', systemFixture);
    setApi('/admin/tenants', []);
    renderAdmin();

    fireEvent.click(screen.getByRole('button', { name: /refresh/i }));
    expect(systemRefetchMock).toHaveBeenCalled();
  });

  it('shows the Tenants empty state when the tenant list is empty', () => {
    setApi('/admin/system', systemFixture);
    setApi('/admin/tenants', []);
    renderAdmin();

    fireEvent.click(screen.getByRole('tab', { name: /tenants/i }));

    expect(screen.getByRole('heading', { name: /no tenants/i })).toBeInTheDocument();
  });

  it('renders the tenant table rows when tenants exist', () => {
    setApi('/admin/system', systemFixture);
    setApi('/admin/tenants', [
      {
        id: 't1',
        name: 'Acme Inc',
        slug: 'acme',
        plan: 'pro',
        status: 'active',
        members_count: 7,
        created_at: new Date().toISOString(),
      },
      {
        id: 't2',
        name: 'Globex',
        slug: 'globex',
        plan: 'free',
        status: 'suspended',
        members_count: 1,
        created_at: new Date().toISOString(),
      },
    ]);
    renderAdmin();

    fireEvent.click(screen.getByRole('tab', { name: /tenants/i }));

    expect(screen.getByText('Acme Inc')).toBeInTheDocument();
    expect(screen.getByText('Globex')).toBeInTheDocument();
    expect(screen.getByText('pro')).toBeInTheDocument();
    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.getByText('Suspended')).toBeInTheDocument();
  });

  it('calls adminAPI.saveSettings when Save Settings is clicked', async () => {
    setApi('/admin/system', systemFixture);
    setApi('/admin/tenants', []);
    renderAdmin();

    fireEvent.click(screen.getByRole('tab', { name: /^settings$/i }));

    fireEvent.change(screen.getByLabelText(/registration mode/i), {
      target: { value: 'invite' },
    });
    fireEvent.change(screen.getByLabelText(/backup retention/i), {
      target: { value: '14' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save settings/i }));

    await waitFor(() =>
      expect(saveSettingsMock).toHaveBeenCalledWith(
        expect.objectContaining({
          registration_mode: 'invite',
          backup_retention_days: 14,
        })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Settings saved')
    );
  });

  it('surfaces a toast.error when saving settings fails', async () => {
    saveSettingsMock.mockRejectedValueOnce(new Error('nope'));
    setApi('/admin/system', systemFixture);
    setApi('/admin/tenants', []);
    renderAdmin();

    fireEvent.click(screen.getByRole('tab', { name: /^settings$/i }));
    fireEvent.click(screen.getByRole('button', { name: /save settings/i }));

    await waitFor(() =>
      expect(toastErrorMock).toHaveBeenCalledWith('Failed to save settings')
    );
  });

  it('toggles the Automatic SSL switch on the Settings tab', () => {
    setApi('/admin/system', systemFixture);
    setApi('/admin/tenants', []);
    renderAdmin();

    fireEvent.click(screen.getByRole('tab', { name: /^settings$/i }));

    const switches = screen.getAllByRole('switch');
    // First switch is auto_ssl (starts true). Toggle it off.
    const autoSsl = switches[0];
    expect(autoSsl.getAttribute('aria-checked')).toBe('true');
    fireEvent.click(autoSsl);
    expect(autoSsl.getAttribute('aria-checked')).toBe('false');
  });

  it('renders the stat-card skeletons while system data is loading', () => {
    setApi('/admin/system', null, true);
    setApi('/admin/tenants', []);
    renderAdmin();

    // Numeric totals should NOT be rendered while loading.
    expect(screen.queryByText('42')).not.toBeInTheDocument();
    expect(screen.queryByText('128 MB')).not.toBeInTheDocument();
    // Header heading is still rendered regardless of loading state.
    expect(screen.getByRole('heading', { name: /admin panel/i })).toBeInTheDocument();
  });
});
