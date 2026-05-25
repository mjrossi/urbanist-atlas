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

  it('renders the lede column unchanged with the search box', async () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();
    // The lede column still hosts the lookup card.
    expect(screen.getByLabelText(/postal code/i)).toBeDefined();
    expect(screen.getByText(/look it up in the atlas/i)).toBeDefined();
    await waitFor(() => {
      expect(document.title).toMatch(/urbanist atlas/i);
    });
  });

  it('renders the top 7 metros in the metros rail', async () => {
    const metros = [
      makeMetro('nyc-metro', 'New York Metro', 12),
      makeMetro('sf-bay-area', 'San Francisco Bay Area', 7),
      makeMetro('m3', 'Metro 3', 5),
      makeMetro('m4', 'Metro 4', 4),
      makeMetro('m5', 'Metro 5', 3),
      makeMetro('m6', 'Metro 6', 2),
      makeMetro('m7', 'Metro 7', 1),
      makeMetro('m8', 'Metro 8', 1),
    ];
    listMetrosMock.mockResolvedValueOnce(metros);
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york metro/i })).toBeDefined();
    });
    // 8 metros provided, 7 should render; metro 8 should not appear.
    expect(screen.queryByRole('link', { name: /metro 8/i })).toBeNull();
    expect(screen.getByRole('link', { name: /metro 7/i })).toBeDefined();
  });

  it('includes an "All metros" link to /browse', async () => {
    listMetrosMock.mockResolvedValueOnce([
      makeMetro('nyc-metro', 'New York Metro', 12),
    ]);
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york metro/i })).toBeDefined();
    });
    const browseLink = screen.getByRole('link', { name: /all metros/i });
    expect(browseLink.getAttribute('href')).toBe('/browse');
  });

  it('renders the top 4 recent orgs in the recent strip', async () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockResolvedValueOnce([
      makeOrg(1, 'a', 'Org A'),
      makeOrg(2, 'b', 'Org B'),
      makeOrg(3, 'c', 'Org C'),
      makeOrg(4, 'd', 'Org D'),
      makeOrg(5, 'e', 'Org E'),
    ]);
    renderHome();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /Org A/ })).toBeDefined();
    });
    // 5 provided, 4 should render; Org E should not appear.
    expect(screen.queryByRole('link', { name: /Org E/ })).toBeNull();
    expect(screen.getByRole('link', { name: /Org D/ })).toBeDefined();
  });

  it('links each recent org name to its /orgs/:slug detail page', async () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockResolvedValueOnce([makeOrg(1, 'transalt', 'Transportation Alternatives')]);
    renderHome();

    await waitFor(() => {
      expect(
        screen.getByRole('link', { name: /Transportation Alternatives/ }),
      ).toBeDefined();
    });
    const orgLink = screen.getByRole('link', { name: /Transportation Alternatives/ });
    expect(orgLink.getAttribute('href')).toBe('/orgs/transalt');
  });

  it('renders subdued loading copy in both asides while the queries pend', () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();
    // Two loading affordances, one per card.
    const loading = screen.getAllByText(/loading/i);
    expect(loading.length).toBeGreaterThanOrEqual(2);
  });

  it('shows the metro-list temporarily-unavailable message on metros error', async () => {
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
      expect(
        screen.getByText(/metro list is temporarily unavailable/i),
      ).toBeDefined();
    });
  });

  it('shows the recent-entries temporarily-unavailable message on recent error', async () => {
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
      expect(
        screen.getByText(/recent entries are temporarily unavailable/i),
      ).toBeDefined();
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
