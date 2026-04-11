import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------
//
// The Billing page fires two parallel useApi() calls: /billing/plans and
// /billing/usage. We route by path.

type ApiResponse = { data: unknown; loading: boolean };
const apiResponses: Record<string, ApiResponse> = {};

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
      return { data: res.data, loading: res.loading, error: null, refetch: vi.fn() };
    },
  };
});

import { Billing } from '../Billing';

function fakePlan(overrides: Partial<{
  id: string;
  name: string;
  description: string;
  price_cents: number;
  max_apps: number;
  max_containers: number;
  max_ram_mb: number;
  features: string[];
}> = {}) {
  return {
    id: 'free',
    name: 'Free',
    description: 'Starter tier',
    price_cents: 0,
    max_apps: 3,
    max_containers: 5,
    max_ram_mb: 1024,
    features: ['1 project'],
    ...overrides,
  };
}

function renderBilling() {
  return render(
    <MemoryRouter>
      <Billing />
    </MemoryRouter>
  );
}

describe('Billing page', () => {
  beforeEach(() => {
    clearApi();
  });

  it('renders the hero header with the Billing & Plans title', () => {
    setApi('/billing/plans', []);
    setApi('/billing/usage', {
      plan: { id: 'free', name: 'Free' },
      apps_used: 0,
      apps_limit: 3,
      containers_used: 0,
      containers_limit: 5,
      ram_used_mb: 0,
      ram_limit_mb: 1024,
    });
    renderBilling();

    expect(
      screen.getByRole('heading', { name: /billing & plans/i })
    ).toBeInTheDocument();
  });

  it('renders the current usage card with the three usage bars', () => {
    setApi('/billing/plans', []);
    setApi('/billing/usage', {
      plan: { id: 'pro', name: 'Pro' },
      apps_used: 2,
      apps_limit: 10,
      containers_used: 4,
      containers_limit: 20,
      ram_used_mb: 2048,
      ram_limit_mb: 8192,
    });
    renderBilling();

    expect(screen.getByText('Applications')).toBeInTheDocument();
    expect(screen.getByText('Containers')).toBeInTheDocument();
    expect(screen.getByText('RAM (MB)')).toBeInTheDocument();
    // Current Pro plan heading inside the usage card.
    expect(screen.getByRole('heading', { name: /pro plan/i })).toBeInTheDocument();
  });

  it('renders a card per available plan with price and feature list', () => {
    setApi('/billing/plans', [
      fakePlan({ id: 'free', name: 'Free', price_cents: 0 }),
      fakePlan({
        id: 'pro',
        name: 'Pro',
        price_cents: 1900,
        max_apps: 25,
        max_containers: 100,
        max_ram_mb: 16384,
        features: ['Priority support', 'Custom domains'],
      }),
    ]);
    setApi('/billing/usage', {
      plan: { id: 'free', name: 'Free' },
      apps_used: 0,
      apps_limit: 3,
      containers_used: 0,
      containers_limit: 5,
      ram_used_mb: 0,
      ram_limit_mb: 1024,
    });
    renderBilling();

    // Free and Pro plan names — the Free plan card + Pro plan card.
    // The usage card also shows "Free Plan" heading + "Free" badge, so
    // "Free" appears more than once. We just need the Pro plan to show up.
    expect(screen.getByText('Pro')).toBeInTheDocument();
    // Free plan price in the plan card = "Free"
    expect(screen.getAllByText('Free').length).toBeGreaterThan(0);
    // Pro plan price rendered as "$19"
    expect(screen.getByText('$19')).toBeInTheDocument();
    // Pro features
    expect(screen.getByText('Priority support')).toBeInTheDocument();
    expect(screen.getByText('Custom domains')).toBeInTheDocument();
  });

  it('marks the current plan with the "Current Plan" badge and disables its button', () => {
    setApi('/billing/plans', [
      fakePlan({ id: 'free', name: 'Free' }),
      fakePlan({ id: 'pro', name: 'Pro', price_cents: 1900 }),
    ]);
    setApi('/billing/usage', {
      plan: { id: 'pro', name: 'Pro' },
      apps_used: 0,
      apps_limit: 10,
      containers_used: 0,
      containers_limit: 20,
      ram_used_mb: 0,
      ram_limit_mb: 8192,
    });
    renderBilling();

    // The Pro card has a "Current Plan" badge + a disabled Current Plan button.
    const currentBtns = screen.getAllByRole('button', { name: /current plan/i });
    expect(currentBtns.length).toBeGreaterThan(0);
    expect(currentBtns[0]).toBeDisabled();
    // The Free plan should show a Downgrade button since its price is 0.
    expect(screen.getByRole('button', { name: /downgrade/i })).toBeInTheDocument();
  });

  it('shows an Upgrade button for higher-tier plans when on a free tier', () => {
    setApi('/billing/plans', [
      fakePlan({ id: 'free', name: 'Free', price_cents: 0 }),
      fakePlan({ id: 'pro', name: 'Pro', price_cents: 1900 }),
    ]);
    setApi('/billing/usage', {
      plan: { id: 'free', name: 'Free' },
      apps_used: 0,
      apps_limit: 3,
      containers_used: 0,
      containers_limit: 5,
      ram_used_mb: 0,
      ram_limit_mb: 1024,
    });
    renderBilling();

    expect(screen.getByRole('button', { name: /upgrade/i })).toBeInTheDocument();
  });

  it('shows the empty plans state when the plans list is empty', () => {
    setApi('/billing/plans', []);
    setApi('/billing/usage', {
      plan: { id: 'free', name: 'Free' },
      apps_used: 0,
      apps_limit: 3,
      containers_used: 0,
      containers_limit: 5,
      ram_used_mb: 0,
      ram_limit_mb: 1024,
    });
    renderBilling();

    expect(
      screen.getByRole('heading', { name: /no plans available/i })
    ).toBeInTheDocument();
  });

  it('renders the plan-card skeletons while the plans feed is loading', () => {
    setApi('/billing/plans', null, true);
    setApi('/billing/usage', null, true);
    renderBilling();

    // No plan names should be rendered while loading — just the header.
    expect(
      screen.getByRole('heading', { name: /billing & plans/i })
    ).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: /no plans available/i })).not.toBeInTheDocument();
  });
});
