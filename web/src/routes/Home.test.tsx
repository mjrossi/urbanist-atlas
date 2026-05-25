import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { MetroSummary, Org } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';

const { listMetrosMock, listRecentMock } = vi.hoisted(() => ({
  listMetrosMock: vi.fn(),
  listRecentMock: vi.fn(),
}));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return {
    ...actual,
    listMetros: listMetrosMock,
    listRecent: listRecentMock,
  };
});

const { Home } = await import('./Home.tsx');

function renderHome() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/']}>
        <Home />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function makeMetro(slug: string, name: string, org_count: number): MetroSummary {
  return {
    region: {
      id: parseInt(slug.replace(/\D/g, '') || '0', 10) || 1,
      kind: 'us:metro',
      name,
      slug,
      country: 'US',
      scope_tier: 'regional',
      parent_slugs: [],
    },
    org_count,
  };
}

function makeOrg(id: number, slug: string, name: string): Org {
  return {
    id,
    slug,
    name,
    short_desc: `Org ${name}`,
    website_url: `https://${slug}.example.org`,
    tags: ['transit'],
    regions: [],
  };
}

describe('Home', () => {
  beforeEach(() => {
    listMetrosMock.mockReset();
    listRecentMock.mockReset();
  });

  afterEach(() => {
    listMetrosMock.mockReset();
    listRecentMock.mockReset();
  });

  it('renders the lede column unchanged with the search box', () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();
    // The lede column heading is the existing slice's responsibility;
    // assert it's still there so the aside wiring didn't disturb it.
    expect(screen.getByText(/start with a postal code/i)).toBeDefined();
  });

  it('renders the top 6 metros in the metros aside', async () => {
    const metros = [
      makeMetro('nyc-metro', 'New York Metro', 12),
      makeMetro('sf-bay-area', 'San Francisco Bay Area', 7),
      makeMetro('m3', 'Metro 3', 5),
      makeMetro('m4', 'Metro 4', 4),
      makeMetro('m5', 'Metro 5', 3),
      makeMetro('m6', 'Metro 6', 2),
      makeMetro('m7', 'Metro 7', 1),
    ];
    listMetrosMock.mockResolvedValueOnce(metros);
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york metro/i })).toBeDefined();
    });
    // 7 metros provided, 6 should render; metro 7 should not appear.
    expect(screen.queryByRole('link', { name: /metro 7/i })).toBeNull();
    expect(screen.getByRole('link', { name: /metro 6/i })).toBeDefined();
  });

  it('includes a "Browse all metros" link to /browse', async () => {
    listMetrosMock.mockResolvedValueOnce([
      makeMetro('nyc-metro', 'New York Metro', 12),
    ]);
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york metro/i })).toBeDefined();
    });
    const browseLink = screen.getByRole('link', { name: /browse all metros/i });
    expect(browseLink.getAttribute('href')).toBe('/browse');
  });

  it('renders the top 5 recent orgs in the recent aside', async () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockResolvedValueOnce([
      makeOrg(1, 'a', 'Org A'),
      makeOrg(2, 'b', 'Org B'),
      makeOrg(3, 'c', 'Org C'),
      makeOrg(4, 'd', 'Org D'),
      makeOrg(5, 'e', 'Org E'),
      makeOrg(6, 'f', 'Org F'),
    ]);
    renderHome();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: 'Org A' })).toBeDefined();
    });
    // 6 provided, 5 should render; Org F should not appear.
    expect(screen.queryByRole('link', { name: 'Org F' })).toBeNull();
    expect(screen.getByRole('link', { name: 'Org E' })).toBeDefined();
  });

  it('links each recent org name to its /orgs/:slug detail page', async () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockResolvedValueOnce([makeOrg(1, 'transalt', 'Transportation Alternatives')]);
    renderHome();

    await waitFor(() => {
      expect(
        screen.getByRole('link', { name: 'Transportation Alternatives' }),
      ).toBeDefined();
    });
    const orgLink = screen.getByRole('link', { name: 'Transportation Alternatives' });
    expect(orgLink.getAttribute('href')).toBe('/orgs/transalt');
    // Secondary external link to the website renders the domain.
    const domainLink = screen.getByRole('link', { name: 'transalt.example.org' });
    expect(domainLink.getAttribute('href')).toBe('https://transalt.example.org');
    expect(domainLink.getAttribute('target')).toBe('_blank');
    expect(domainLink.getAttribute('rel')).toContain('noopener');
    expect(domainLink.getAttribute('rel')).toContain('noreferrer');
  });

  it('renders subdued loading copy in both asides while the queries pend', () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();
    // Two loading affordances, one per card.
    const loading = screen.getAllByText(/loading/i);
    expect(loading.length).toBeGreaterThanOrEqual(2);
  });

  it('shows "Temporarily unavailable" in the metros aside on error', async () => {
    listMetrosMock.mockRejectedValueOnce(
      new ApiError(
        500,
        'metros 500',
        { type: 'about:blank', title: 'metros 500', status: 500 },
        'req-x',
      ),
    );
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      // Card still says "Browse by metro" + the descriptive fallback
      // copy, plus an honest status pill. The contract is the graceful
      // degradation, not the exact wording.
      const status = screen.getAllByText(/temporarily unavailable/i);
      expect(status.length).toBeGreaterThanOrEqual(1);
    });
  });

  it('shows "Temporarily unavailable" in the recent aside on error', async () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockRejectedValueOnce(
      new ApiError(
        500,
        'recent 500',
        { type: 'about:blank', title: 'recent 500', status: 500 },
        'req-y',
      ),
    );
    renderHome();

    await waitFor(() => {
      const status = screen.getAllByText(/temporarily unavailable/i);
      expect(status.length).toBeGreaterThanOrEqual(1);
    });
  });

  it('links each metro in the aside to /m/:slug', async () => {
    listMetrosMock.mockResolvedValueOnce([makeMetro('nyc-metro', 'New York Metro', 12)]);
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      const nyc = screen.getByRole('link', { name: /new york metro/i });
      expect(nyc.getAttribute('href')).toBe('/m/nyc-metro');
    });
  });

  it('sets the browser tab title', async () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();
    await waitFor(() => {
      expect(document.title).toMatch(/urbanist atlas/i);
    });
  });
});
