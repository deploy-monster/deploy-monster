import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// Mock the navigate function from react-router. We still need MemoryRouter so
// that <Link to="/register"> renders without blowing up, but we want to assert
// on programmatic navigation in handleSubmit without driving the router.
const navigateMock = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router');
  return {
    ...actual,
    useNavigate: () => navigateMock,
  };
});

// Mock the auth store. Login.tsx calls `useAuthStore((s) => s.login)`, so the
// hook needs to accept a selector and return whatever the selector picks out.
const loginMock = vi.fn();
vi.mock('../../stores/auth', () => ({
  useAuthStore: (selector: (s: { login: typeof loginMock }) => unknown) =>
    selector({ login: loginMock }),
}));

import { Login } from '../Login';

function renderLogin() {
  return render(
    <MemoryRouter>
      <Login />
    </MemoryRouter>
  );
}

describe('Login page', () => {
  beforeEach(() => {
    navigateMock.mockReset();
    loginMock.mockReset();
  });

  it('renders the email and password inputs and a submit button', () => {
    renderLogin();

    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('calls authStore.login with the entered credentials and navigates home on success', async () => {
    loginMock.mockResolvedValue(undefined);
    renderLogin();

    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'admin@deploy.monster' },
    });
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'correct-horse-battery-staple' },
    });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(loginMock).toHaveBeenCalledWith(
        'admin@deploy.monster',
        'correct-horse-battery-staple'
      );
    });
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith('/');
    });
  });

  it('shows the error message from login() when it throws', async () => {
    loginMock.mockRejectedValue(new Error('Invalid credentials'));
    renderLogin();

    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'bad@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'wrong' },
    });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    expect(await screen.findByText('Invalid credentials')).toBeInTheDocument();
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it('shows a generic message when login() throws a non-Error value', async () => {
    // If the caller ever rejects with a string, the component falls back to
    // "Login failed" — important for API clients that reject with JSON bodies.
    loginMock.mockRejectedValue('nope');
    renderLogin();

    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'a@b.co' },
    });
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'pw' },
    });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    expect(await screen.findByText('Login failed')).toBeInTheDocument();
  });

  it('disables the submit button and shows a loading label while login is pending', async () => {
    // Hold the login promise open so we can observe the loading state.
    let resolveLogin: () => void = () => {};
    loginMock.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveLogin = resolve;
        })
    );
    renderLogin();

    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'a@b.co' },
    });
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'pw' },
    });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    const button = await screen.findByRole('button', { name: /signing in/i });
    expect(button).toBeDisabled();

    resolveLogin();
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith('/');
    });
  });

  it('toggles password visibility when the eye button is clicked', () => {
    renderLogin();

    const passwordInput = screen.getByLabelText(/password/i) as HTMLInputElement;
    expect(passwordInput.type).toBe('password');

    // The visibility toggle is an icon button with no accessible label, so we
    // reach for it via its position next to the password input.
    const toggles = screen
      .getAllByRole('button')
      .filter((el) => el.getAttribute('type') === 'button');
    expect(toggles.length).toBeGreaterThan(0);
    fireEvent.click(toggles[0]);
    expect(passwordInput.type).toBe('text');

    fireEvent.click(toggles[0]);
    expect(passwordInput.type).toBe('password');
  });

  it('clears any prior error when the form is resubmitted', async () => {
    loginMock.mockRejectedValueOnce(new Error('Invalid credentials'));
    renderLogin();

    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'a@b.co' },
    });
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'wrong' },
    });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));
    expect(await screen.findByText('Invalid credentials')).toBeInTheDocument();

    // Second attempt succeeds — the error must disappear.
    loginMock.mockResolvedValueOnce(undefined);
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'right' },
    });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.queryByText('Invalid credentials')).not.toBeInTheDocument();
    });
    expect(navigateMock).toHaveBeenCalledWith('/');
  });
});
