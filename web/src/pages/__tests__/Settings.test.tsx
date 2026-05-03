import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------

// Auth store — Settings calls `useAuthStore((s) => s.user)`, so the mock has
// to accept and invoke the selector with a fake state.
const fakeUser = {
  id: 'u1',
  email: 'alice@example.com',
  name: 'Alice',
  role: 'role_super_admin',
};
const updateUserMock = vi.fn();
vi.mock('@/stores/auth', () => ({
  useAuthStore: (selector: (s: { user: typeof fakeUser; updateUser: typeof updateUserMock }) => unknown) =>
    selector({ user: fakeUser, updateUser: updateUserMock }),
}));

// Theme store — called as `useThemeStore()` returning { theme, setTheme }.
// We keep a module-scoped copy of the current theme so the component renders
// the active styling but setTheme is a plain mock (no matchMedia side-effect).
const setThemeMock = vi.fn();
let themeValue: 'light' | 'dark' | 'system' = 'system';
vi.mock('@/stores/theme', () => ({
  useThemeStore: () => ({
    theme: themeValue,
    setTheme: (t: 'light' | 'dark' | 'system') => {
      themeValue = t;
      setThemeMock(t);
    },
  }),
}));

// API client — Settings calls profile, password and TOTP endpoints.
const apiGetMock = vi.fn();
const apiPatchMock = vi.fn();
const apiPostMock = vi.fn();
vi.mock('@/api/client', () => ({
  api: {
    get: (path: string) => apiGetMock(path),
    patch: (path: string, body: unknown) => apiPatchMock(path, body),
    post: (path: string, body: unknown) => apiPostMock(path, body),
  },
}));

// Admin API helpers for key generation/revocation.
const generateApiKeyMock = vi.fn();
const revokeApiKeyMock = vi.fn();
vi.mock('@/api/admin', () => ({
  adminAPI: {
    generateApiKey: () => generateApiKeyMock(),
    revokeApiKey: (prefix: string) => revokeApiKeyMock(prefix),
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

import { Settings } from '../Settings';

function renderSettings() {
  return render(
    <MemoryRouter>
      <Settings />
    </MemoryRouter>
  );
}

describe('Settings page', () => {
  beforeEach(() => {
    themeValue = 'system';
    fakeUser.role = 'role_super_admin';
    setThemeMock.mockReset();
    updateUserMock.mockReset();
    apiGetMock.mockReset().mockImplementation((path: string) => {
      if (path === '/auth/totp/status') return Promise.resolve({ enabled: false });
      if (path === '/admin/api-keys') return Promise.resolve([]);
      return Promise.resolve(null);
    });
    apiPatchMock.mockReset().mockResolvedValue(undefined);
    apiPostMock.mockReset().mockResolvedValue(undefined);
    generateApiKeyMock.mockReset();
    revokeApiKeyMock.mockReset().mockResolvedValue(undefined);
    toastSuccessMock.mockReset();
    toastErrorMock.mockReset();
  });

  it('renders the page header and both tabs', () => {
    renderSettings();

    expect(screen.getByRole('heading', { name: /^settings$/i })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /profile/i })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /security/i })).toBeInTheDocument();
  });

  it('pre-populates the display name and email from the auth store user', () => {
    renderSettings();

    const nameInput = screen.getByLabelText(/display name/i) as HTMLInputElement;
    expect(nameInput.value).toBe('Alice');
    // The email input is disabled — grab by display value since it has no label id.
    expect(screen.getByDisplayValue('alice@example.com')).toBeInTheDocument();
  });

  it('calls api.patch with the updated name when Save Profile is clicked', async () => {
    renderSettings();

    fireEvent.change(screen.getByLabelText(/display name/i), {
      target: { value: 'Alice Zhang' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save profile/i }));

    await waitFor(() =>
      expect(apiPatchMock).toHaveBeenCalledWith(
        '/auth/me',
        expect.objectContaining({ name: 'Alice Zhang' })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Profile updated')
    );
    expect(updateUserMock).toHaveBeenCalledWith({ name: 'Alice Zhang' });
  });

  it('surfaces a toast.error when the profile save rejects', async () => {
    apiPatchMock.mockRejectedValueOnce(new Error('nope'));
    renderSettings();

    fireEvent.click(screen.getByRole('button', { name: /save profile/i }));

    await waitFor(() =>
      expect(toastErrorMock).toHaveBeenCalledWith('Failed to update profile')
    );
  });

  it('invokes setTheme when a different theme card is clicked', () => {
    renderSettings();

    // The three theme cards are plain <button>s with the label as text. The
    // Profile tab is active by default, so all three are mounted.
    fireEvent.click(screen.getByRole('button', { name: /^dark$/i }));

    expect(setThemeMock).toHaveBeenCalledWith('dark');
  });

  it('toggles a notification preference via the Switch role', () => {
    renderSettings();

    // Four switches under Notifications; the first corresponds to "Email
    // notifications", which defaults to checked. Clicking should flip it.
    const switches = screen.getAllByRole('switch');
    expect(switches.length).toBeGreaterThanOrEqual(4);
    const emailSwitch = switches[0];
    expect(emailSwitch.getAttribute('aria-checked')).toBe('true');
    fireEvent.click(emailSwitch);
    expect(emailSwitch.getAttribute('aria-checked')).toBe('false');
  });

  it('keeps Change Password disabled until both password fields are filled', () => {
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    const btn = screen.getByRole('button', { name: /^change password$/i });
    expect(btn).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/current password/i), {
      target: { value: 'old-secret' },
    });
    // Still missing new password.
    expect(btn).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/new password/i), {
      target: { value: 'brand-new' },
    });
    expect(btn).not.toBeDisabled();
  });

  it('calls api.post on change-password with the two password fields', async () => {
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    fireEvent.change(screen.getByLabelText(/current password/i), {
      target: { value: 'old-secret' },
    });
    fireEvent.change(screen.getByLabelText(/new password/i), {
      target: { value: 'brand-new' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^change password$/i }));

    await waitFor(() =>
      expect(apiPostMock).toHaveBeenCalledWith(
        '/auth/change-password',
        expect.objectContaining({
          current_password: 'old-secret',
          new_password: 'brand-new',
        })
      )
    );
    await waitFor(() =>
      expect(toastSuccessMock).toHaveBeenCalledWith('Password changed')
    );
  });

  it('shows the change-password error returned by the API', async () => {
    apiPostMock.mockRejectedValueOnce(new Error('current password invalid'));
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    fireEvent.change(screen.getByLabelText(/current password/i), {
      target: { value: 'x' },
    });
    fireEvent.change(screen.getByLabelText(/new password/i), {
      target: { value: 'y' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^change password$/i }));

    await waitFor(() =>
      expect(toastErrorMock).toHaveBeenCalledWith('current password invalid')
    );
  });

  it('starts 2FA enrollment and confirms it with an authentication code', async () => {
    apiPostMock.mockResolvedValueOnce({
      provisioning_uri: 'otpauth://totp/DeployMonster:alice@example.com?secret=abc',
    }).mockResolvedValueOnce({ status: 'enabled' });
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    // Before toggling: confirmation copy is absent.
    expect(
      screen.queryByText(/two-factor authentication is enabled/i)
    ).not.toBeInTheDocument();

    // There's exactly one switch on the Security tab (Enable 2FA).
    fireEvent.click(screen.getByRole('switch'));

    await waitFor(() =>
      expect(apiPostMock).toHaveBeenCalledWith('/auth/totp/enroll', {})
    );
    expect(screen.getByDisplayValue(/otpauth:\/\/totp/i)).toBeInTheDocument();
    expect(toastSuccessMock).toHaveBeenCalledWith('Two-factor authentication setup started');

    fireEvent.change(screen.getByLabelText(/authentication code/i), {
      target: { value: '123456' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^verify$/i }));

    await waitFor(() =>
      expect(apiPostMock).toHaveBeenCalledWith('/auth/totp/enroll', { code: '123456' })
    );
    expect(await screen.findByText(/two-factor authentication is enabled/i)).toBeInTheDocument();
    expect(toastSuccessMock).toHaveBeenCalledWith('Two-factor authentication enabled');
  });

  it('disables 2FA through the API when an authentication code is supplied', async () => {
    apiGetMock.mockResolvedValue({ enabled: true });
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    expect(await screen.findByText(/two-factor authentication is enabled/i)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/authentication code/i), {
      target: { value: '123456' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^disable$/i }));

    await waitFor(() =>
      expect(apiPostMock).toHaveBeenCalledWith('/auth/totp/disable', { code: '123456' })
    );
    expect(toastSuccessMock).toHaveBeenCalledWith('Two-factor authentication disabled');
  });

  it('reveals the generated key after adminAPI.generateApiKey resolves', async () => {
    generateApiKeyMock.mockResolvedValueOnce({ key: 'dm_test_abc123', prefix: 'dm_test' });
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    fireEvent.click(screen.getByRole('button', { name: /generate new key/i }));

    expect(await screen.findByText('dm_test_abc123')).toBeInTheDocument();
    expect(toastSuccessMock).toHaveBeenCalledWith('API key generated -- save it now!');
  });

  it('hides API keys for non-super-admin users and skips the admin endpoint', () => {
    fakeUser.role = 'role_admin';
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    expect(screen.queryByRole('button', { name: /generate new key/i })).not.toBeInTheDocument();
    expect(apiGetMock).not.toHaveBeenCalledWith('/admin/api-keys');
  });

  it('revokes the generated key through the API and restores the empty state', async () => {
    generateApiKeyMock.mockResolvedValueOnce({ key: 'dm_test_xyz789', prefix: 'dm_test' });
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));
    fireEvent.click(screen.getByRole('button', { name: /generate new key/i }));

    expect(await screen.findByText('dm_test_xyz789')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /revoke key/i }));

    await waitFor(() => expect(revokeApiKeyMock).toHaveBeenCalledWith('dm_test'));
    await waitFor(() => expect(screen.queryByText('dm_test_xyz789')).not.toBeInTheDocument());
    // Empty state header
    expect(screen.getByText(/no api keys/i)).toBeInTheDocument();
  });

  it('lists existing API key prefixes and revokes them through the API', async () => {
    apiGetMock.mockImplementation((path: string) => {
      if (path === '/auth/totp/status') return Promise.resolve({ enabled: false });
      if (path === '/admin/api-keys') {
        return Promise.resolve([
          {
            prefix: 'dm_live_abcd',
            type: 'platform',
            created_by: 'u1',
            created_at: '2026-01-02T03:04:05Z',
          },
        ]);
      }
      return Promise.resolve(null);
    });
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    expect(await screen.findByText('dm_live_abcd')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^revoke$/i }));

    await waitFor(() => expect(revokeApiKeyMock).toHaveBeenCalledWith('dm_live_abcd'));
    expect(toastSuccessMock).toHaveBeenCalledWith('API key revoked');
  });

  it('falls back to a generic error toast when key generation rejects', async () => {
    generateApiKeyMock.mockRejectedValueOnce(new Error('forbidden'));
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    fireEvent.click(screen.getByRole('button', { name: /generate new key/i }));

    await waitFor(() =>
      expect(toastErrorMock).toHaveBeenCalledWith('Failed to generate API key')
    );
  });
});
