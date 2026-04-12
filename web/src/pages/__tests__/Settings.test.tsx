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
  role: 'role_admin',
};
vi.mock('@/stores/auth', () => ({
  useAuthStore: (selector: (s: { user: typeof fakeUser }) => unknown) =>
    selector({ user: fakeUser }),
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

// API client — Settings calls api.patch('/auth/me', ...) and
// api.post('/auth/change-password', ...).
const apiPatchMock = vi.fn();
const apiPostMock = vi.fn();
vi.mock('@/api/client', () => ({
  api: {
    patch: (path: string, body: unknown) => apiPatchMock(path, body),
    post: (path: string, body: unknown) => apiPostMock(path, body),
  },
}));

// adminAPI.generateApiKey is the only admin call.
const generateApiKeyMock = vi.fn();
vi.mock('@/api/admin', () => ({
  adminAPI: {
    generateApiKey: () => generateApiKeyMock(),
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
    setThemeMock.mockReset();
    apiPatchMock.mockReset().mockResolvedValue(undefined);
    apiPostMock.mockReset().mockResolvedValue(undefined);
    generateApiKeyMock.mockReset();
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

  it('shows a 2FA confirmation banner after flipping the Enable 2FA switch', () => {
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    // Before toggling: confirmation copy is absent.
    expect(
      screen.queryByText(/two-factor authentication is enabled/i)
    ).not.toBeInTheDocument();

    // There's exactly one switch on the Security tab (Enable 2FA).
    fireEvent.click(screen.getByRole('switch'));

    expect(
      screen.getByText(/two-factor authentication is enabled/i)
    ).toBeInTheDocument();
  });

  it('reveals the generated key after adminAPI.generateApiKey resolves', async () => {
    generateApiKeyMock.mockResolvedValueOnce({ key: 'dm_test_abc123' });
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));

    fireEvent.click(screen.getByRole('button', { name: /generate new key/i }));

    expect(await screen.findByText('dm_test_abc123')).toBeInTheDocument();
    expect(toastSuccessMock).toHaveBeenCalledWith('API key generated -- save it now!');
  });

  it('revokes the generated key and restores the empty state', async () => {
    generateApiKeyMock.mockResolvedValueOnce({ key: 'dm_test_xyz789' });
    renderSettings();
    fireEvent.click(screen.getByRole('tab', { name: /security/i }));
    fireEvent.click(screen.getByRole('button', { name: /generate new key/i }));

    expect(await screen.findByText('dm_test_xyz789')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /revoke key/i }));

    expect(screen.queryByText('dm_test_xyz789')).not.toBeInTheDocument();
    // Empty state header
    expect(screen.getByText(/no api keys/i)).toBeInTheDocument();
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
