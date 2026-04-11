import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { NotFound } from '../NotFound';

describe('NotFound page', () => {
  it('renders the 404 heading and supporting copy', () => {
    render(
      <MemoryRouter>
        <NotFound />
      </MemoryRouter>
    );

    expect(screen.getByText('404')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /page not found/i })).toBeInTheDocument();
    expect(
      screen.getByText(/the page you are looking for does not exist/i)
    ).toBeInTheDocument();
  });

  it('exposes a Dashboard link that routes to the root', () => {
    render(
      <MemoryRouter>
        <NotFound />
      </MemoryRouter>
    );

    const dashboardLink = screen.getByRole('link', { name: /dashboard/i });
    expect(dashboardLink).toHaveAttribute('href', '/');
  });

  it('calls window.history.back when the Go Back button is clicked', () => {
    const backSpy = vi.spyOn(window.history, 'back').mockImplementation(() => {});
    render(
      <MemoryRouter>
        <NotFound />
      </MemoryRouter>
    );

    fireEvent.click(screen.getByRole('button', { name: /go back/i }));
    expect(backSpy).toHaveBeenCalled();
    backSpy.mockRestore();
  });
});
