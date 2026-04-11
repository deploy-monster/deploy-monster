import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

const navigateMock = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router');
  return {
    ...actual,
    useNavigate: () => navigateMock,
  };
});

const registerMock = vi.fn();
vi.mock('../../stores/auth', () => ({
  useAuthStore: (selector: (s: { register: typeof registerMock }) => unknown) =>
    selector({ register: registerMock }),
}));

import { Register } from '../Register';

function renderRegister() {
  return render(
    <MemoryRouter>
      <Register />
    </MemoryRouter>
  );
}

// The component exposes two password fields ("Password" and "Confirm
// password") and `getByLabelText(/password/i)` matches both. Grab them by id
// to keep the tests unambiguous.
function getPasswordInput(): HTMLInputElement {
  return document.getElementById('password') as HTMLInputElement;
}
function getConfirmInput(): HTMLInputElement {
  return document.getElementById('confirm-password') as HTMLInputElement;
}

describe('Register page', () => {
  beforeEach(() => {
    navigateMock.mockReset();
    registerMock.mockReset();
  });

  it('renders name, email, password and confirm password inputs', () => {
    renderRegister();

    expect(screen.getByLabelText(/name/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
    expect(getPasswordInput()).toBeInTheDocument();
    expect(getConfirmInput()).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /create account/i })).toBeInTheDocument();
  });

  it('calls authStore.register with entered values and navigates home on success', async () => {
    registerMock.mockResolvedValue(undefined);
    renderRegister();

    fireEvent.change(screen.getByLabelText(/name/i), {
      target: { value: 'Ada Lovelace' },
    });
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'ada@example.com' },
    });
    fireEvent.change(getPasswordInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.change(getConfirmInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));

    await waitFor(() => {
      // Register store takes (email, password, name) — order matters.
      expect(registerMock).toHaveBeenCalledWith(
        'ada@example.com',
        'Valid-Pass-8',
        'Ada Lovelace'
      );
    });
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith('/');
    });
  });

  it('rejects mismatched passwords before hitting the store', async () => {
    renderRegister();

    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'Ada' } });
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'ada@example.com' },
    });
    fireEvent.change(getPasswordInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.change(getConfirmInput(), { target: { value: 'Different!' } });
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));

    // The validation error comes straight from the submit handler, not from a
    // mocked rejection — asserting on the alert div text confirms we short-
    // circuited before calling register().
    const alertError = await screen.findAllByText(/passwords do not match/i);
    expect(alertError.length).toBeGreaterThan(0);
    expect(registerMock).not.toHaveBeenCalled();
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it('rejects passwords shorter than 8 characters', async () => {
    renderRegister();

    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'Ada' } });
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'ada@example.com' },
    });
    fireEvent.change(getPasswordInput(), { target: { value: 'short' } });
    fireEvent.change(getConfirmInput(), { target: { value: 'short' } });
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));

    expect(
      await screen.findByText(/password must be at least 8 characters/i)
    ).toBeInTheDocument();
    expect(registerMock).not.toHaveBeenCalled();
  });

  it('surfaces errors thrown by the store', async () => {
    registerMock.mockRejectedValue(new Error('email already taken'));
    renderRegister();

    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'Ada' } });
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'ada@example.com' },
    });
    fireEvent.change(getPasswordInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.change(getConfirmInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));

    expect(await screen.findByText('email already taken')).toBeInTheDocument();
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it('falls back to a generic message for non-Error rejections', async () => {
    registerMock.mockRejectedValue('not-an-error');
    renderRegister();

    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'Ada' } });
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'ada@example.com' },
    });
    fireEvent.change(getPasswordInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.change(getConfirmInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));

    expect(await screen.findByText('Registration failed')).toBeInTheDocument();
  });

  it('shows a loading label and disables the button while register is pending', async () => {
    let resolveRegister: () => void = () => {};
    registerMock.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveRegister = resolve;
        })
    );
    renderRegister();

    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'Ada' } });
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'ada@example.com' },
    });
    fireEvent.change(getPasswordInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.change(getConfirmInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));

    const loadingBtn = await screen.findByRole('button', { name: /creating account/i });
    expect(loadingBtn).toBeDisabled();

    resolveRegister();
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith('/');
    });
  });

  it('does not render the strength indicator until a password is typed', () => {
    renderRegister();

    // No label exists for the empty state — none of "Weak/Fair/Strong" should
    // be in the document before the user types anything.
    expect(screen.queryByText(/^weak$/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/^fair$/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/^strong$/i)).not.toBeInTheDocument();
  });

  it('classifies passwords into weak / fair / strong tiers', () => {
    renderRegister();
    const pw = getPasswordInput();

    fireEvent.change(pw, { target: { value: 'ab' } });
    expect(screen.getByText(/^weak$/i)).toBeInTheDocument();

    fireEvent.change(pw, { target: { value: 'Abcdefgh' } });
    expect(screen.getByText(/^fair$/i)).toBeInTheDocument();

    fireEvent.change(pw, { target: { value: 'Abcdefgh1!XY' } });
    expect(screen.getByText(/^strong$/i)).toBeInTheDocument();
  });

  it('shows inline "Passwords do not match" under the confirm field while typing', () => {
    renderRegister();

    fireEvent.change(getPasswordInput(), { target: { value: 'Valid-Pass-8' } });
    fireEvent.change(getConfirmInput(), { target: { value: 'Val' } });

    // Only the inline field-level message should be visible — the top alert
    // banner (which shares the same copy) only appears after submit.
    const matches = screen.getAllByText(/passwords do not match/i);
    expect(matches.length).toBe(1);

    fireEvent.change(getConfirmInput(), { target: { value: 'Valid-Pass-8' } });
    expect(screen.queryByText(/passwords do not match/i)).not.toBeInTheDocument();
  });
});
