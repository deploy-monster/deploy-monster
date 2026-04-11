import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// Mock useApi and useDebouncedValue. debounce is pass-through so the test can
// type into the search box and see the marketplaceData update synchronously.
const useApiState: { data: unknown; loading: boolean } = {
  data: null,
  loading: true,
};
vi.mock('../../hooks', async () => {
  const actual = await vi.importActual<typeof import('../../hooks')>('../../hooks');
  return {
    ...actual,
    useApi: () => ({
      data: useApiState.data,
      loading: useApiState.loading,
      error: null,
      refetch: vi.fn(),
    }),
    useDebouncedValue: (v: string) => v,
  };
});

const deployMock = vi.fn();
vi.mock('@/api/marketplace', async () => {
  const actual = await vi.importActual<typeof import('@/api/marketplace')>('@/api/marketplace');
  return {
    ...actual,
    marketplaceAPI: {
      list: vi.fn(),
      deploy: (req: unknown) => deployMock(req),
    },
  };
});

const navigateMock = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router');
  return {
    ...actual,
    useNavigate: () => navigateMock,
  };
});

import { Marketplace } from '../Marketplace';

function renderMarketplace() {
  return render(
    <MemoryRouter>
      <Marketplace />
    </MemoryRouter>
  );
}

// Each template card is a shadcn <Card> (plain <div>) with no data attr. We
// locate the card for a given template title by walking up from the title
// node until we find an ancestor that also contains a <button> — that's the
// CardFooter+card wrapper.
function findCardByTitle(title: string): HTMLElement {
  const el = screen.getByText(title);
  let cur: HTMLElement | null = el;
  while (cur) {
    if (cur.tagName === 'DIV' && cur.querySelector('button')) {
      return cur;
    }
    cur = cur.parentElement;
  }
  throw new Error(`no card ancestor with a button found for title "${title}"`);
}

function fakeTemplate(overrides: Partial<{
  slug: string;
  name: string;
  description: string;
  category: string;
  tags: string[];
  version: string;
  featured: boolean;
  verified: boolean;
}> = {}) {
  return {
    slug: 'wordpress',
    name: 'WordPress',
    description: 'The worlds most popular CMS.',
    category: 'cms',
    tags: ['php', 'mysql', 'cms'],
    version: '6.7',
    featured: false,
    verified: true,
    min_resources: { memory_mb: 512 },
    ...overrides,
  };
}

describe('Marketplace page', () => {
  beforeEach(() => {
    useApiState.data = null;
    useApiState.loading = true;
    deployMock.mockReset().mockResolvedValue({ app_id: 'app-123' });
    navigateMock.mockReset();
  });

  it('renders the hero, template count and one card per template', () => {
    useApiState.data = {
      data: [
        fakeTemplate({ slug: 'wp', name: 'WordPress', featured: true }),
        fakeTemplate({ slug: 'gh', name: 'Ghost', category: 'cms', featured: false }),
      ],
      categories: ['cms'],
    };
    useApiState.loading = false;
    renderMarketplace();

    expect(screen.getByRole('heading', { name: 'Marketplace' })).toBeInTheDocument();
    expect(screen.getByText(/2 templates/i)).toBeInTheDocument();
    expect(screen.getByText(/1 featured/i)).toBeInTheDocument();
    expect(screen.getByText('WordPress')).toBeInTheDocument();
    expect(screen.getByText('Ghost')).toBeInTheDocument();
  });

  it('shows the empty state when the marketplace returns no templates', () => {
    useApiState.data = { data: [], categories: [] };
    useApiState.loading = false;
    renderMarketplace();

    expect(screen.getByText(/no templates found/i)).toBeInTheDocument();
    expect(
      screen.getByText(/the marketplace is empty/i)
    ).toBeInTheDocument();
  });

  it('shows "Try adjusting" copy and a Clear-filters button when a search filter is active', () => {
    useApiState.data = { data: [], categories: ['cms'] };
    useApiState.loading = false;
    renderMarketplace();

    fireEvent.change(
      screen.getByPlaceholderText(/search templates by name/i),
      { target: { value: 'zzz-nothing-here' } }
    );

    expect(screen.getByText(/try adjusting your search/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /clear filters/i })).toBeInTheDocument();
  });

  it('opens the deploy dialog when the Deploy button on a card is clicked', () => {
    useApiState.data = {
      data: [fakeTemplate({ slug: 'ghost', name: 'Ghost' })],
      categories: ['cms'],
    };
    useApiState.loading = false;
    renderMarketplace();

    // There are potentially many "Deploy" buttons (card + dialog); click the
    // one on the card by scoping to the template card.
    const card = findCardByTitle('Ghost');
    const cardDeployBtn = within(card).getByRole('button', { name: /deploy/i });
    fireEvent.click(cardDeployBtn);

    // Dialog title: "Deploy Ghost"
    expect(screen.getByText('Deploy Ghost')).toBeInTheDocument();
    expect(screen.getByLabelText(/stack name/i)).toBeInTheDocument();
  });

  it('calls marketplaceAPI.deploy with the template slug and entered name', async () => {
    useApiState.data = {
      data: [fakeTemplate({ slug: 'wp', name: 'WordPress' })],
      categories: ['cms'],
    };
    useApiState.loading = false;
    renderMarketplace();

    const card = findCardByTitle('WordPress');
    const cardDeployBtn = within(card).getByRole('button', { name: /deploy/i });
    fireEvent.click(cardDeployBtn);

    const stackNameInput = screen.getByLabelText(/stack name/i);
    fireEvent.change(stackNameInput, { target: { value: 'my-wordpress' } });

    // The dialog renders a second Deploy button (with a Rocket icon). Every
    // "Deploy" button now visible: the card's and the dialog's. The dialog
    // one is the one we didn't click, so pick the one that is NOT the card's.
    const dialogDeploy = screen
      .getAllByRole('button', { name: /deploy/i })
      .find((btn) => btn !== cardDeployBtn);
    expect(dialogDeploy).toBeDefined();
    fireEvent.click(dialogDeploy!);

    await waitFor(() => {
      expect(deployMock).toHaveBeenCalledWith(
        expect.objectContaining({
          slug: 'wp',
          name: 'my-wordpress',
        })
      );
    });
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith('/apps/app-123');
    });
  });

  it('shows an error in the dialog when the deploy API rejects', async () => {
    deployMock.mockRejectedValueOnce(new Error('quota exceeded'));
    useApiState.data = {
      data: [fakeTemplate({ slug: 'wp', name: 'WordPress' })],
      categories: ['cms'],
    };
    useApiState.loading = false;
    renderMarketplace();

    const card = findCardByTitle('WordPress');
    const cardDeployBtn = within(card).getByRole('button', { name: /deploy/i });
    fireEvent.click(cardDeployBtn);

    // Trigger the dialog's Deploy button (any deploy button that isn't the
    // one on the card).
    const dialogDeploy = screen
      .getAllByRole('button', { name: /deploy/i })
      .find((btn) => btn !== cardDeployBtn);
    fireEvent.click(dialogDeploy!);

    expect(await screen.findByText('quota exceeded')).toBeInTheDocument();
    expect(navigateMock).not.toHaveBeenCalled();
  });
});
