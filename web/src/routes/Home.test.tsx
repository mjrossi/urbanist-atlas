import { screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { Org, RegionSummary } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';
import { renderWithProviders } from '../test/renderWithProviders.tsx';

const { listRegionsMock, listRecentMock } = vi.hoisted(() => ({
  listRegionsMock: vi.fn(),
  listRecentMock: vi.fn(),
}));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return {
    ...actual,
    listRegions: listRegionsMock,
    listRecent: listRecentMock,
  };
});

const { Home } = await import('./Home.tsx');

function renderHome() {
  return renderWithProviders(<Home />);
}

function makeRegion(
  slug: string,
  name: string,
  org_count: number,
  direct_org_count: number = org_count,
): RegionSummary {
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
    direct_org_count,
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
    added_at: '2026-05-21',
  };
}

describe('Home', () => {
  beforeEach(() => {
    listRegionsMock.mockReset();
    listRecentMock.mockReset();
  });

  afterEach(() => {
    listRegionsMock.mockReset();
    listRecentMock.mockReset();
  });

  it('renders the lede column unchanged with the search box', async () => {
    listRegionsMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();
    // The lede column still hosts the lookup card.
    expect(screen.getByLabelText(/postal code/i)).toBeDefined();
    expect(screen.getByText(/look it up in the atlas/i)).toBeDefined();
    await waitFor(() => {
      expect(document.title).toMatch(/urbanist atlas/i);
    });
  });

  it('renders the top 7 places in the places rail', async () => {
    const places = [
      makeRegion('nyc-metro', 'New York Metro', 12),
      makeRegion('sf-bay-area', 'San Francisco Bay Area', 7),
      makeRegion('m3', 'Metro 3', 5),
      makeRegion('m4', 'Metro 4', 4),
      makeRegion('m5', 'Metro 5', 3),
      makeRegion('m6', 'Metro 6', 2),
      makeRegion('m7', 'Metro 7', 1),
      makeRegion('m8', 'Metro 8', 1),
    ];
    listRegionsMock.mockResolvedValueOnce(places);
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york metro/i })).toBeDefined();
    });
    // 8 places provided, 7 should render; metro 8 should not appear.
    expect(screen.queryByRole('link', { name: /metro 8/i })).toBeNull();
    expect(screen.getByRole('link', { name: /metro 7/i })).toBeDefined();
  });

  it('includes an "All regions" link to /browse', async () => {
    listRegionsMock.mockResolvedValueOnce([
      makeRegion('nyc-metro', 'New York Metro', 12),
    ]);
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york metro/i })).toBeDefined();
    });
    const browseLink = screen.getByRole('link', { name: /all regions/i });
    expect(browseLink.getAttribute('href')).toBe('/browse');
  });

  it('renders the top 4 recent orgs in the recent strip', async () => {
    listRegionsMock.mockReturnValue(new Promise(() => {}));
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
    listRegionsMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockResolvedValueOnce([
      makeOrg(1, 'transalt', 'Transportation Alternatives'),
    ]);
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
    listRegionsMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();
    // Two loading affordances, one per card.
    const loading = screen.getAllByText(/loading/i);
    expect(loading.length).toBeGreaterThanOrEqual(2);
  });

  it('shows the region-list unavailable message on regions error', async () => {
    listRegionsMock.mockRejectedValueOnce(
      new ApiError(
        500,
        'places 500',
        { type: 'about:blank', title: 'places 500', status: 500 },
        'req-x',
      ),
    );
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      expect(screen.getByText(/region list isn.t loading right now/i)).toBeDefined();
    });
  });

  it('shows the recent-entries unavailable message on recent error', async () => {
    listRegionsMock.mockReturnValue(new Promise(() => {}));
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
      expect(screen.getByText(/recent entries aren.t loading right now/i)).toBeDefined();
    });
  });

  it('links each region in the aside to /region/:slug', async () => {
    listRegionsMock.mockResolvedValueOnce([
      makeRegion('nyc-metro', 'New York Metro', 12),
    ]);
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();

    await waitFor(() => {
      const nyc = screen.getByRole('link', { name: /new york metro/i });
      expect(nyc.getAttribute('href')).toBe('/region/nyc-metro');
    });
  });

  it('sets the browser tab title', async () => {
    listRegionsMock.mockReturnValue(new Promise(() => {}));
    listRecentMock.mockReturnValue(new Promise(() => {}));
    renderHome();
    await waitFor(() => {
      expect(document.title).toMatch(/urbanist atlas/i);
    });
  });
});
