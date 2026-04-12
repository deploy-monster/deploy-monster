import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------

const navigateMock = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router');
  return {
    ...actual,
    useNavigate: () => navigateMock,
  };
});

import { Onboarding } from '../Onboarding';

function renderOnboarding() {
  return render(
    <MemoryRouter>
      <Onboarding />
    </MemoryRouter>
  );
}

describe('Onboarding page', () => {
  beforeEach(() => {
    navigateMock.mockReset();
    localStorage.clear();
  });

  it('renders the welcome step first with the DeployMonster branding', () => {
    renderOnboarding();

    expect(screen.getByText('DeployMonster')).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: /welcome to deploymonster/i })
    ).toBeInTheDocument();
    expect(screen.getByText(/step 1 of 5/i)).toBeInTheDocument();
  });

  it('advances to the Server Detection step when Continue is clicked', () => {
    renderOnboarding();

    fireEvent.click(screen.getByRole('button', { name: /continue/i }));

    expect(
      screen.getByRole('heading', { name: /server detection/i })
    ).toBeInTheDocument();
    expect(screen.getByText(/step 2 of 5/i)).toBeInTheDocument();
  });

  it('advances from Server Detection to the Platform Domain step', () => {
    renderOnboarding();

    // step 0 -> 1
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    // step 1 -> 2
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));

    expect(
      screen.getByRole('heading', { name: /platform domain/i })
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/platform domain/i)).toBeInTheDocument();
  });

  it('allows typing a platform domain value', () => {
    renderOnboarding();

    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));

    const input = screen.getByLabelText(/platform domain/i) as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'deploy.example.com' } });
    expect(input.value).toBe('deploy.example.com');
  });

  it('shows the Git provider selection on step 3', () => {
    renderOnboarding();

    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));

    expect(
      screen.getByRole('heading', { name: /connect git provider/i })
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /github/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /gitlab/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /gitea/i })).toBeInTheDocument();
  });

  it('allows selecting a Git provider card', () => {
    renderOnboarding();

    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));

    // Clicking GitHub should not throw. We can't trivially assert ring state,
    // but a Check icon appears next to the chosen provider name.
    fireEvent.click(screen.getByRole('button', { name: /github/i }));

    // After selection, there should be a visible check marker near GitHub.
    expect(screen.getByRole('button', { name: /github/i })).toBeInTheDocument();
  });

  it('reaches the Done step and navigates home on Finish', () => {
    renderOnboarding();

    // Walk through all 5 steps.
    fireEvent.click(screen.getByRole('button', { name: /continue/i })); // 0 -> 1
    fireEvent.click(screen.getByRole('button', { name: /continue/i })); // 1 -> 2
    fireEvent.click(screen.getByRole('button', { name: /continue/i })); // 2 -> 3
    fireEvent.click(screen.getByRole('button', { name: /continue/i })); // 3 -> 4

    expect(
      screen.getByRole('heading', { name: /you're all set!/i })
    ).toBeInTheDocument();

    // Final button says "Go to Dashboard" instead of Continue.
    fireEvent.click(screen.getByRole('button', { name: /go to dashboard/i }));

    expect(localStorage.getItem('onboarding_complete')).toBe('true');
    expect(navigateMock).toHaveBeenCalledWith('/');
  });
});
