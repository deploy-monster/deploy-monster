import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------
//
// The Team page fires two parallel useApi() calls: /team/members and
// /team/audit-log. We route by path so each can be primed independently.

type ApiResponse = { data: unknown; loading: boolean };
const apiResponses: Record<string, ApiResponse> = {};
const refetchMembersMock = vi.fn();

function setApi(path: string, data: unknown, loading = false) {
  apiResponses[path] = { data, loading };
}
function clearApi() {
  for (const k of Object.keys(apiResponses)) delete apiResponses[k];
}

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks');
  return {
    ...actual,
    useApi: (path: string) => {
      const res = apiResponses[path] ?? { data: null, loading: true };
      const refetch = path === '/team/members' ? refetchMembersMock : vi.fn();
      return { data: res.data, loading: res.loading, error: null, refetch };
    },
  };
});

const inviteMock = vi.fn();
const removeMemberMock = vi.fn();
vi.mock('@/api/team', () => ({
  teamAPI: {
    invite: (data: unknown) => inviteMock(data),
    removeMember: (id: string) => removeMemberMock(id),
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

import { Team } from '../Team';

function fakeMember(overrides: Partial<{
  id: string;
  name: string;
  email: string;
  role: string;
  joined_at: string;
}> = {}) {
  return {
    id: 'm1',
    name: 'Alice Zhang',
    email: 'alice@example.com',
    role: 'role_admin',
    joined_at: '2025-01-01T00:00:00Z',
    ...overrides,
  };
}

function renderTeam() {
  return render(
    <MemoryRouter>
      <Team />
    </MemoryRouter>
  );
}

describe('Team page', () => {
  beforeEach(() => {
    clearApi();
    refetchMembersMock.mockReset();
    inviteMock.mockReset().mockResolvedValue(undefined);
    removeMemberMock.mockReset().mockResolvedValue(undefined);
    toastSuccessMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the hero header and Invite Member CTA', () => {
    setApi('/team/members', [fakeMember()]);
    setApi('/team/audit-log', []);
    renderTeam();

    expect(screen.getByRole('heading', { name: /team management/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /invite member/i })).toBeInTheDocument();
    // Badge shows member count when > 0
    expect(screen.getByText(/1 member/i)).toBeInTheDocument();
  });

  it('renders the team members empty state when the list is empty', () => {
    setApi('/team/members', []);
    setApi('/team/audit-log', []);
    renderTeam();

    expect(screen.getByRole('heading', { name: /no team members yet/i })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /invite your first member/i })
    ).toBeInTheDocument();
  });

  it('lists team members with their names, emails, and role badges', () => {
    setApi('/team/members', [
      fakeMember({ id: 'm1', name: 'Alice Zhang', email: 'alice@example.com', role: 'role_admin' }),
      fakeMember({ id: 'm2', name: 'Bob Lee', email: 'bob@example.com', role: 'role_developer' }),
    ]);
    setApi('/team/audit-log', []);
    renderTeam();

    expect(screen.getByText('Alice Zhang')).toBeInTheDocument();
    expect(screen.getByText('alice@example.com')).toBeInTheDocument();
    expect(screen.getByText('Bob Lee')).toBeInTheDocument();
    expect(screen.getByText('Admin')).toBeInTheDocument();
    expect(screen.getByText('Developer')).toBeInTheDocument();
  });

  it('opens the invite dialog when Invite Member is clicked', () => {
    setApi('/team/members', [fakeMember()]);
    setApi('/team/audit-log', []);
    renderTeam();

    fireEvent.click(screen.getByRole('button', { name: /invite member/i }));

    expect(screen.getByRole('heading', { name: /invite team member/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/email address/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/^role$/i)).toBeInTheDocument();
  });

  it('calls teamAPI.invite with the email and role when Send Invite is clicked', async () => {
    setApi('/team/members', [fakeMember()]);
    setApi('/team/audit-log', []);
    renderTeam();

    fireEvent.click(screen.getByRole('button', { name: /invite member/i }));
    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'new@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/^role$/i), {
      target: { value: 'role_operator' },
    });
    fireEvent.click(screen.getByRole('button', { name: /send invite/i }));

    await waitFor(() =>
      expect(inviteMock).toHaveBeenCalledWith(
        expect.objectContaining({ email: 'new@example.com', role_id: 'role_operator' })
      )
    );
    await waitFor(() => expect(toastSuccessMock).toHaveBeenCalledWith('Invite sent'));
    await waitFor(() => expect(refetchMembersMock).toHaveBeenCalled());
  });

  it('keeps Send Invite disabled until an email is entered', () => {
    setApi('/team/members', [fakeMember()]);
    setApi('/team/audit-log', []);
    renderTeam();

    fireEvent.click(screen.getByRole('button', { name: /invite member/i }));

    const sendBtn = screen.getByRole('button', { name: /send invite/i });
    expect(sendBtn).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'x@example.com' },
    });
    expect(sendBtn).not.toBeDisabled();
  });

  it('surfaces a toast.error when teamAPI.invite rejects', async () => {
    inviteMock.mockRejectedValueOnce(new Error('nope'));
    setApi('/team/members', [fakeMember()]);
    setApi('/team/audit-log', []);
    renderTeam();

    fireEvent.click(screen.getByRole('button', { name: /invite member/i }));
    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'fail@example.com' },
    });
    fireEvent.click(screen.getByRole('button', { name: /send invite/i }));

    await waitFor(() =>
      expect(toastErrorMock).toHaveBeenCalledWith('Failed to send invite')
    );
  });

  it('calls teamAPI.removeMember when the delete prompt is confirmed', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    setApi('/team/members', [fakeMember({ id: 'm1' })]);
    setApi('/team/audit-log', []);
    renderTeam();

    // The remove button is an icon-only Trash2; scope via the lucide class.
    const trashIcon = document.querySelector('svg.lucide-trash2');
    const btn = trashIcon?.closest('button');
    expect(btn).not.toBeNull();
    fireEvent.click(btn!);

    await waitFor(() => expect(removeMemberMock).toHaveBeenCalledWith('m1'));
    await waitFor(() => expect(toastSuccessMock).toHaveBeenCalledWith('Member removed'));
    confirmSpy.mockRestore();
  });

  it('skips the removeMember call if the confirm prompt is declined', () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
    setApi('/team/members', [fakeMember({ id: 'm1' })]);
    setApi('/team/audit-log', []);
    renderTeam();

    const trashIcon = document.querySelector('svg.lucide-trash2');
    const btn = trashIcon?.closest('button');
    fireEvent.click(btn!);

    expect(removeMemberMock).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it('shows the audit-log empty state on the Audit Log tab when no entries exist', () => {
    setApi('/team/members', [fakeMember()]);
    setApi('/team/audit-log', []);
    renderTeam();

    fireEvent.click(screen.getByRole('tab', { name: /audit log/i }));

    expect(
      screen.getByRole('heading', { name: /no audit log entries/i })
    ).toBeInTheDocument();
  });

  it('renders the audit log timeline entries when the list is populated', () => {
    setApi('/team/members', [fakeMember()]);
    setApi('/team/audit-log', [
      {
        id: 1,
        action: 'login',
        user_name: 'Alice',
        resource_type: 'session',
        resource_id: 'sess-1',
        ip_address: '10.0.0.1',
        created_at: new Date().toISOString(),
      },
      {
        id: 2,
        action: 'deploy',
        user_name: 'Bob',
        resource_type: 'app',
        resource_id: 'app-42',
        ip_address: '10.0.0.2',
        created_at: new Date().toISOString(),
      },
    ]);
    renderTeam();

    fireEvent.click(screen.getByRole('tab', { name: /audit log/i }));

    expect(screen.getByText('Alice')).toBeInTheDocument();
    expect(screen.getByText('Bob')).toBeInTheDocument();
    expect(screen.getByText('sess-1')).toBeInTheDocument();
    expect(screen.getByText('app-42')).toBeInTheDocument();
  });
});
