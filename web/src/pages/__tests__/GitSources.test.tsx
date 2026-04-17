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
  };
});

const connectMock = vi.fn();
const disconnectMock = vi.fn();
vi.mock('@/api/git-sources', async () => {
  const actual = await vi.importActual<typeof import('@/api/git-sources')>('@/api/git-sources');
  return {
    ...actual,
    gitSourcesAPI: {
      list: vi.fn(),
      connect: (data: unknown) => connectMock(data),
      disconnect: (id: string) => disconnectMock(id),
    },
  };
});

const toastSuccessMock = vi.fn();
const toastErrorMock = vi.fn();
vi.mock('@/stores/toastStore', () => ({
  toast: {
    success: (msg: string) => toastSuccessMock(msg),
    error: (msg: string) => toastErrorMock(msg),
    info: vi.fn(),
  },
}));

import { GitSources } from '../GitSources';

function fakeProvider(overrides: Partial<{
  id: string;
  name: string;
  type: string;
  connected: boolean;
  repo_count: number;
}> = {}) {
  return {
    id: 'gh-1',
    name: 'GitHub',
    type: 'github',
    connected: false,
    repo_count: 0,
    ...overrides,
  };
}

function renderGitSources() {
  return render(
    <MemoryRouter>
      <GitSources />
    </MemoryRouter>
  );
}

describe('GitSources page', () => {
  beforeEach(() => {
    useApiState.data = null;
    useApiState.loading = true;
    refetchMock.mockReset();
    connectMock.mockReset().mockResolvedValue(undefined);
    disconnectMock.mockReset().mockResolvedValue(undefined);
    toastSuccessMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the hero header and Connect Provider CTA', () => {
    useApiState.data = [fakeProvider()];
    useApiState.loading = false;
    renderGitSources();

    expect(screen.getByRole('heading', { name: /git sources/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /connect provider/i })).toBeInTheDocument();
  });

  it('renders the empty state when the provider list is empty', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderGitSources();

    expect(
      screen.getByRole('heading', { name: /no git providers connected/i })
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /connect your first provider/i })
    ).toBeInTheDocument();
  });

  it('renders a card per provider with Connected/Disconnected badge', () => {
    useApiState.data = [
      fakeProvider({ id: 'p1', name: 'My GH', type: 'github', connected: true, repo_count: 5 }),
      fakeProvider({ id: 'p2', name: 'My GL', type: 'gitlab', connected: false }),
    ];
    useApiState.loading = false;
    renderGitSources();

    expect(screen.getByText('My GH')).toBeInTheDocument();
    expect(screen.getByText('My GL')).toBeInTheDocument();
    expect(screen.getByText('Connected')).toBeInTheDocument();
    expect(screen.getByText('Disconnected')).toBeInTheDocument();
    expect(screen.getByText(/5 repos/i)).toBeInTheDocument();
  });

  it('opens the Connect dialog with a provider selection grid', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderGitSources();

    fireEvent.click(screen.getByRole('button', { name: /connect your first provider/i }));

    expect(
      screen.getByRole('heading', { name: /connect git provider/i })
    ).toBeInTheDocument();
    // Grid shows all 4 providers as buttons.
    expect(screen.getByRole('button', { name: /github/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /gitlab/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /gitea/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /bitbucket/i })).toBeInTheDocument();
  });

  it('moves to the token form when a provider card is clicked', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderGitSources();

    fireEvent.click(screen.getByRole('button', { name: /connect your first provider/i }));
    fireEvent.click(screen.getByRole('button', { name: /github/i }));

    expect(screen.getByRole('heading', { name: /connect github/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/access token/i)).toBeInTheDocument();
    // Instance URL is hidden for GitHub.
    expect(screen.queryByLabelText(/instance url/i)).not.toBeInTheDocument();
  });

  it('shows the Instance URL field for GitLab and Gitea', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderGitSources();

    fireEvent.click(screen.getByRole('button', { name: /connect your first provider/i }));
    fireEvent.click(screen.getByRole('button', { name: /gitlab/i }));

    expect(screen.getByLabelText(/instance url/i)).toBeInTheDocument();
  });

  it('submits the connect form through gitSourcesAPI.connect', async () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderGitSources();

    fireEvent.click(screen.getByRole('button', { name: /connect your first provider/i }));
    fireEvent.click(screen.getByRole('button', { name: /github/i }));

    fireEvent.change(screen.getByLabelText(/access token/i), {
      target: { value: 'ghp_testtoken' },
    });

    // The submit button in the token form has accessible name "Connect"
    // (plus the Link2 icon). The "Connect Provider" header button still
    // exists in the background of the page. Pick the dialog one.
    const connectBtns = screen.getAllByRole('button', { name: /^connect$/i });
    fireEvent.click(connectBtns[connectBtns.length - 1]);

    await waitFor(() =>
      expect(connectMock).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'github', token: 'ghp_testtoken' })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('GitHub connected successfully')
    );
    await waitFor(() => expect(refetchMock).toHaveBeenCalled());
  });

  it('surfaces the API error inline inside the token form', async () => {
    connectMock.mockRejectedValueOnce(new Error('bad token'));
    useApiState.data = [];
    useApiState.loading = false;
    renderGitSources();

    fireEvent.click(screen.getByRole('button', { name: /connect your first provider/i }));
    fireEvent.click(screen.getByRole('button', { name: /github/i }));

    fireEvent.change(screen.getByLabelText(/access token/i), {
      target: { value: 'broken' },
    });
    const connectBtns = screen.getAllByRole('button', { name: /^connect$/i });
    fireEvent.click(connectBtns[connectBtns.length - 1]);

    expect(await screen.findByText('bad token')).toBeInTheDocument();
  });

  // Disconnect now uses an AlertDialog (title "Disconnect Provider",
  // confirmLabel "Disconnect"). See GitSources.tsx:445. Click Disconnect
  // → dialog opens → click dialog Disconnect (or Cancel).

  it('calls gitSourcesAPI.disconnect when the Disconnect button is confirmed', async () => {
    useApiState.data = [fakeProvider({ id: 'p1', name: 'My GH', connected: true })];
    useApiState.loading = false;
    renderGitSources();

    fireEvent.click(screen.getByRole('button', { name: /^disconnect$/i }));

    expect(screen.getByRole('heading', { name: /disconnect provider/i })).toBeInTheDocument();
    const allDisconnect = screen.getAllByRole('button', { name: /^disconnect$/i });
    fireEvent.click(allDisconnect[allDisconnect.length - 1]);

    await waitFor(() => expect(disconnectMock).toHaveBeenCalledWith('p1'));
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Provider disconnected')
    );
  });

  it('skips the disconnect call when the confirm prompt is declined', () => {
    useApiState.data = [fakeProvider({ id: 'p1', connected: true })];
    useApiState.loading = false;
    renderGitSources();

    fireEvent.click(screen.getByRole('button', { name: /^disconnect$/i }));

    expect(screen.getByRole('heading', { name: /disconnect provider/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }));

    expect(disconnectMock).not.toHaveBeenCalled();
  });
});
