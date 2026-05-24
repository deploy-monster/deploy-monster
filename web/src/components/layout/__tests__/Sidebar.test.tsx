import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { Sidebar } from '../Sidebar';
import { useAuthStore } from '@/stores/auth';

vi.mock('@/hooks', () => ({
  useApi: () => ({ data: null, loading: false, error: null, refetch: vi.fn() }),
}));

function renderSidebar(role: string) {
  useAuthStore.setState({
    user: { id: 'u1', email: 'a@example.com', name: 'Alice', role, tenant_id: 't1' },
    isAuthenticated: true,
    isLoading: false,
  });

  render(
    <MemoryRouter>
      <Sidebar />
    </MemoryRouter>,
  );
}

describe('Sidebar', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows the Admin navigation item only to super admins', () => {
    renderSidebar('role_super_admin');

    expect(screen.getAllByRole('link', { name: /admin/i }).length).toBeGreaterThan(0);
  });

  it('hides the Admin navigation item from tenant admins', () => {
    renderSidebar('role_admin');

    expect(screen.queryByRole('link', { name: /admin/i })).not.toBeInTheDocument();
  });
});
