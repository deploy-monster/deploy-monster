import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// DeployWizard calls `api.post('/apps', ...)` directly rather than going
// through a hook or store, so we mock the shared client module.
const apiPostMock = vi.fn();
vi.mock('@/api/client', () => ({
  api: {
    post: (path: string, body: unknown) => apiPostMock(path, body),
  },
}));

const navigateMock = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router');
  return {
    ...actual,
    useNavigate: () => navigateMock,
  };
});

import { DeployWizard } from '../DeployWizard';

function renderWizard() {
  return render(
    <MemoryRouter>
      <DeployWizard />
    </MemoryRouter>
  );
}

describe('DeployWizard page', () => {
  beforeEach(() => {
    apiPostMock.mockReset();
    navigateMock.mockReset();
  });

  it('starts on step 1 (Source) with Next disabled until a source is picked', () => {
    renderWizard();

    expect(screen.getByRole('heading', { name: /deploy new application/i })).toBeInTheDocument();
    expect(screen.getByText(/choose deployment source/i)).toBeInTheDocument();
    expect(screen.getByText('Git Repository')).toBeInTheDocument();
    expect(screen.getByText('Docker Image')).toBeInTheDocument();
    expect(screen.getByText('Marketplace')).toBeInTheDocument();

    const next = screen.getByRole('button', { name: /next/i });
    expect(next).toBeDisabled();
  });

  it('advances to step 2 after selecting a source and clicking Next', () => {
    renderWizard();

    fireEvent.click(screen.getByText('Git Repository'));
    const next = screen.getByRole('button', { name: /next/i });
    expect(next).not.toBeDisabled();
    fireEvent.click(next);

    expect(screen.getByText(/configure your application/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/application name/i)).toBeInTheDocument();
    // Git source unlocks the repository + branch fields.
    expect(screen.getByLabelText(/repository url/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/branch/i)).toBeInTheDocument();
  });

  it('shows the Docker image field for the image source and hides the git fields', () => {
    renderWizard();
    fireEvent.click(screen.getByText('Docker Image'));
    fireEvent.click(screen.getByRole('button', { name: /next/i }));

    expect(screen.getByLabelText(/docker image/i)).toBeInTheDocument();
    expect(screen.queryByLabelText(/repository url/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/^branch/i)).not.toBeInTheDocument();
  });

  it('requires a name before advancing from step 2 to step 3', () => {
    renderWizard();
    fireEvent.click(screen.getByText('Git Repository'));
    fireEvent.click(screen.getByRole('button', { name: /next/i }));

    // Empty name keeps Next disabled.
    const next = screen.getByRole('button', { name: /next/i });
    expect(next).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/application name/i), {
      target: { value: 'my-app' },
    });
    expect(next).not.toBeDisabled();
  });

  it('renders the review summary on step 3 with the values entered in step 2', () => {
    renderWizard();
    fireEvent.click(screen.getByText('Git Repository'));
    fireEvent.click(screen.getByRole('button', { name: /next/i }));

    fireEvent.change(screen.getByLabelText(/application name/i), {
      target: { value: 'review-app' },
    });
    fireEvent.change(screen.getByLabelText(/repository url/i), {
      target: { value: 'https://github.com/example/review-app.git' },
    });
    fireEvent.click(screen.getByRole('button', { name: /next/i }));

    expect(screen.getByText(/review and deploy/i)).toBeInTheDocument();
    expect(screen.getByText('review-app')).toBeInTheDocument();
    expect(screen.getByText('git')).toBeInTheDocument();
    expect(screen.getByText('https://github.com/example/review-app.git')).toBeInTheDocument();
    // Branch default survives into the review panel.
    expect(screen.getByText('main')).toBeInTheDocument();
    // Port default survives into the review panel.
    expect(screen.getByText('3000')).toBeInTheDocument();
  });

  it('allows stepping back from step 2 to step 1 preserving the selected source', () => {
    renderWizard();
    fireEvent.click(screen.getByText('Git Repository'));
    fireEvent.click(screen.getByRole('button', { name: /next/i }));

    fireEvent.click(screen.getByRole('button', { name: /back/i }));

    expect(screen.getByText(/choose deployment source/i)).toBeInTheDocument();
    // Coming back, the Next button should still be enabled because sourceType
    // was preserved in state.
    expect(screen.getByRole('button', { name: /next/i })).not.toBeDisabled();
  });

  it('calls api.post and navigates to the new app page on successful deploy', async () => {
    apiPostMock.mockResolvedValue({ id: 'app-new-1' });
    renderWizard();

    fireEvent.click(screen.getByText('Git Repository'));
    fireEvent.click(screen.getByRole('button', { name: /next/i }));
    fireEvent.change(screen.getByLabelText(/application name/i), {
      target: { value: 'deploy-me' },
    });
    fireEvent.change(screen.getByLabelText(/repository url/i), {
      target: { value: 'https://github.com/example/deploy-me.git' },
    });
    fireEvent.click(screen.getByRole('button', { name: /next/i }));
    fireEvent.click(screen.getByRole('button', { name: /deploy application/i }));

    await waitFor(() => {
      expect(apiPostMock).toHaveBeenCalledWith(
        '/apps',
        expect.objectContaining({
          name: 'deploy-me',
          source_type: 'git',
          source_url: 'https://github.com/example/deploy-me.git',
          branch: 'main',
        })
      );
    });
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith('/apps/app-new-1');
    });
  });

  it('surfaces the API error message in the review pane on deploy failure', async () => {
    apiPostMock.mockRejectedValue(new Error('port already taken'));
    renderWizard();

    fireEvent.click(screen.getByText('Docker Image'));
    fireEvent.click(screen.getByRole('button', { name: /next/i }));
    fireEvent.change(screen.getByLabelText(/application name/i), {
      target: { value: 'broken' },
    });
    fireEvent.click(screen.getByRole('button', { name: /next/i }));
    fireEvent.click(screen.getByRole('button', { name: /deploy application/i }));

    expect(await screen.findByText('port already taken')).toBeInTheDocument();
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it('falls back to "Deploy failed" when the API rejects with a non-Error value', async () => {
    apiPostMock.mockRejectedValue('nope');
    renderWizard();

    fireEvent.click(screen.getByText('Docker Image'));
    fireEvent.click(screen.getByRole('button', { name: /next/i }));
    fireEvent.change(screen.getByLabelText(/application name/i), {
      target: { value: 'broken' },
    });
    fireEvent.click(screen.getByRole('button', { name: /next/i }));
    fireEvent.click(screen.getByRole('button', { name: /deploy application/i }));

    expect(await screen.findByText(/deploy failed/i)).toBeInTheDocument();
  });
});
