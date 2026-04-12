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

const createMock = vi.fn();
const restoreMock = vi.fn();
vi.mock('@/api/backups', async () => {
  const actual = await vi.importActual<typeof import('@/api/backups')>('@/api/backups');
  return {
    ...actual,
    backupsAPI: {
      list: vi.fn(),
      create: (data: unknown) => createMock(data),
      restore: (key: string) => restoreMock(key),
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

import { Backups } from '../Backups';

function fakeBackup(overrides: Partial<{
  key: string;
  size: number;
  type: string;
  status: string;
  created_at: number;
}> = {}) {
  return {
    key: 'backup-2025-01-01.tar.gz',
    size: 1024 * 1024 * 500, // 500 MB
    type: 'full',
    status: 'completed',
    created_at: Math.floor(Date.now() / 1000) - 3600, // 1 hour ago
    ...overrides,
  };
}

function renderBackups() {
  return render(
    <MemoryRouter>
      <Backups />
    </MemoryRouter>
  );
}

describe('Backups page', () => {
  beforeEach(() => {
    useApiState.data = null;
    useApiState.loading = true;
    refetchMock.mockReset();
    createMock.mockReset().mockResolvedValue(undefined);
    restoreMock.mockReset().mockResolvedValue(undefined);
    toastSuccessMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the hero header and Create Backup CTA', () => {
    useApiState.data = [fakeBackup()];
    useApiState.loading = false;
    renderBackups();

    expect(screen.getByRole('heading', { name: /^backups$/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /create backup/i })).toBeInTheDocument();
    expect(screen.getByText(/1 backup/i)).toBeInTheDocument();
  });

  it('renders the empty state with the first-backup CTA when the list is empty', () => {
    useApiState.data = [];
    useApiState.loading = false;
    renderBackups();

    expect(screen.getByRole('heading', { name: /no backups yet/i })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /create your first backup/i })
    ).toBeInTheDocument();
  });

  it('renders the summary cards and a row per backup when the list is populated', () => {
    useApiState.data = [
      fakeBackup({ key: 'full-2025-01-01.tar', type: 'full', status: 'completed' }),
      fakeBackup({
        key: 'db-2025-01-02.sql',
        type: 'database',
        status: 'failed',
        size: 1024 * 2,
      }),
    ];
    useApiState.loading = false;
    renderBackups();

    expect(screen.getByText('full-2025-01-01.tar')).toBeInTheDocument();
    expect(screen.getByText('db-2025-01-02.sql')).toBeInTheDocument();
    // Type labels in the rows
    expect(screen.getByText('Full')).toBeInTheDocument();
    expect(screen.getByText('Database')).toBeInTheDocument();
    // Status badges — "Completed" label also shows in the summary card.
    expect(screen.getAllByText('Completed').length).toBeGreaterThan(0);
    expect(screen.getByText('Failed')).toBeInTheDocument();
  });

  it('calls backupsAPI.create when Create Backup is clicked from the header', async () => {
    useApiState.data = [fakeBackup()];
    useApiState.loading = false;
    renderBackups();

    fireEvent.click(screen.getByRole('button', { name: /create backup/i }));

    await waitFor(() =>
      expect(createMock).toHaveBeenCalledWith(
        expect.objectContaining({ source_type: 'full', source_id: 'all' })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Backup started')
    );
    await waitFor(() => expect(refetchMock).toHaveBeenCalled());
  });

  it('surfaces a toast.error when create fails', async () => {
    createMock.mockRejectedValueOnce(new Error('bucket full'));
    useApiState.data = [];
    useApiState.loading = false;
    renderBackups();

    fireEvent.click(screen.getByRole('button', { name: /create your first backup/i }));

    await waitFor(() =>
      expect(toastErrorMock).toHaveBeenCalledWith('Failed to create backup')
    );
  });

  it('opens the restore confirmation dialog when the restore icon button is clicked', () => {
    useApiState.data = [fakeBackup({ key: 'restore-me.tar' })];
    useApiState.loading = false;
    renderBackups();

    // The restore icon uses lucide-rotate-ccw.
    const restoreIcon = document.querySelector('svg.lucide-rotate-ccw');
    const btn = restoreIcon?.closest('button');
    fireEvent.click(btn!);

    expect(
      screen.getByRole('heading', { name: /restore backup/i })
    ).toBeInTheDocument();
    // The key shows in both the row and the dialog confirmation body.
    expect(screen.getAllByText('restore-me.tar').length).toBeGreaterThan(1);
  });

  it('calls backupsAPI.restore when the restore dialog is confirmed', async () => {
    useApiState.data = [fakeBackup({ key: 'confirm-me.tar' })];
    useApiState.loading = false;
    renderBackups();

    const rowIcon = document.querySelector('svg.lucide-rotate-ccw');
    fireEvent.click(rowIcon!.closest('button')!);

    // Two buttons match /^restore backup$/i now — the dialog submit button
    // (declared first in the JSX tree) and the row icon button (rendered
    // later inside the table). Pick the dialog one.
    const buttons = screen.getAllByRole('button', { name: /^restore backup$/i });
    const dialogBtn = buttons.find((b) => b.querySelector('svg.lucide-rotate-ccw') !== null
      && b.textContent?.trim() === 'Restore Backup') ?? buttons[0];
    fireEvent.click(dialogBtn);

    await waitFor(() => expect(restoreMock).toHaveBeenCalledWith('confirm-me.tar'));
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Restore started')
    );
  });

  it('surfaces a toast.error when restore fails', async () => {
    restoreMock.mockRejectedValueOnce(new Error('missing'));
    useApiState.data = [fakeBackup({ key: 'busted.tar' })];
    useApiState.loading = false;
    renderBackups();

    const rowIcon = document.querySelector('svg.lucide-rotate-ccw');
    fireEvent.click(rowIcon!.closest('button')!);

    const buttons = screen.getAllByRole('button', { name: /^restore backup$/i });
    const dialogBtn = buttons.find((b) => b.querySelector('svg.lucide-rotate-ccw') !== null
      && b.textContent?.trim() === 'Restore Backup') ?? buttons[0];
    fireEvent.click(dialogBtn);

    await waitFor(() =>
      expect(toastErrorMock).toHaveBeenCalledWith('Failed to restore backup')
    );
  });
});
