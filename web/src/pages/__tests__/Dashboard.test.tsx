import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------
//
// Dashboard fires four parallel useApi() calls (stats, apps, activity,
// announcements). We route each by its path so one test fixture can stand in
// for the whole page. Unknown paths return `{ data: null, loading: true }` so
// nothing throws if the real page later adds a fifth call.

type ApiResponse = { data: unknown; loading: boolean };

const apiResponses: Record<string, ApiResponse> = {};

function setApi(path: string, data: unknown, loading = false) {
  apiResponses[path] = { data, loading };
}

function clearApi() {
  for (const k of Object.keys(apiResponses)) delete apiResponses[k];
}

vi.mock('../../hooks', async () => {
  const actual = await vi.importActual<typeof import('../../hooks')>('../../hooks');
  return {
    ...actual,
    useApi: (path: string) => {
      const res = apiResponses[path] ?? { data: null, loading: true };
      return {
        data: res.data,
        loading: res.loading,
        error: null,
        refetch: vi.fn(),
      };
    },
  };
});

import { Dashboard } from '../Dashboard';

function renderDashboard() {
  return render(
    <MemoryRouter>
      <Dashboard />
    </MemoryRouter>
  );
}

const statsFixture = {
  apps: { total: 12 },
  containers: { running: 8, stopped: 2, total: 10 },
  domains: 4,
  projects: 3,
  events: { published: 256, errors: 0 },
};

describe('Dashboard page', () => {
  beforeEach(() => {
    clearApi();
  });

  it('renders the welcome greeting and deploy CTA', () => {
    setApi('/dashboard/stats', statsFixture);
    setApi('/apps?page=1&per_page=5', { data: [] });
    setApi('/activity?limit=10', { data: [] });
    setApi('/announcements', { data: [] });

    renderDashboard();

    // The greeting varies by hour — just assert on the suffix that is stable.
    expect(screen.getByText(/, admin$/i)).toBeInTheDocument();
    expect(
      screen.getByText(/here is what is happening across your platform/i)
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /deploy new app/i })).toBeInTheDocument();
  });

  it('shows the stat totals pulled from /dashboard/stats', () => {
    setApi('/dashboard/stats', statsFixture);
    setApi('/apps?page=1&per_page=5', { data: [] });
    setApi('/activity?limit=10', { data: [] });
    setApi('/announcements', { data: [] });

    renderDashboard();

    // 12 apps, 10 containers, 4 domains, 3 projects — all rendered as
    // standalone tabular numbers inside stat cards.
    expect(screen.getByText('12')).toBeInTheDocument();
    expect(screen.getByText('10')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
  });

  it('renders the stat card skeletons while stats are loading', () => {
    setApi('/dashboard/stats', null, true);
    setApi('/apps?page=1&per_page=5', { data: [] });
    setApi('/activity?limit=10', { data: [] });
    setApi('/announcements', { data: [] });

    renderDashboard();

    // While loading we must not render the numeric totals.
    expect(screen.queryByText('12')).not.toBeInTheDocument();
    // And the CTA is still rendered regardless of loading state.
    expect(screen.getByRole('link', { name: /deploy new app/i })).toBeInTheDocument();
  });

  it('renders the announcement banner when announcements are present', () => {
    setApi('/dashboard/stats', statsFixture);
    setApi('/apps?page=1&per_page=5', { data: [] });
    setApi('/activity?limit=10', { data: [] });
    setApi('/announcements', {
      data: [
        {
          id: 'a1',
          title: 'Maintenance window Friday',
          body: 'Expect a short outage between 02:00-02:30 UTC.',
          type: 'info',
        },
      ],
    });

    renderDashboard();

    expect(screen.getByText('Maintenance window Friday')).toBeInTheDocument();
    expect(
      screen.getByText(/expect a short outage between 02:00-02:30 UTC/i)
    ).toBeInTheDocument();
  });

  it('does not render the announcement banner when the list is empty', () => {
    setApi('/dashboard/stats', statsFixture);
    setApi('/apps?page=1&per_page=5', { data: [] });
    setApi('/activity?limit=10', { data: [] });
    setApi('/announcements', { data: [] });

    renderDashboard();

    expect(screen.queryByText(/maintenance window friday/i)).not.toBeInTheDocument();
  });

  it('updates the search input as the user types', () => {
    setApi('/dashboard/stats', statsFixture);
    setApi('/apps?page=1&per_page=5', { data: [] });
    setApi('/activity?limit=10', { data: [] });
    setApi('/announcements', { data: [] });

    renderDashboard();

    // Both desktop and mobile search inputs share the same placeholder; take
    // the first one.
    const inputs = screen.getAllByPlaceholderText(/search apps, domains/i);
    expect(inputs.length).toBeGreaterThan(0);
    fireEvent.change(inputs[0], { target: { value: 'my-app' } });
    expect((inputs[0] as HTMLInputElement).value).toBe('my-app');
  });
});
