import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { RegionDetail } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';

const { getRegionMock } = vi.hoisted(() => ({ getRegionMock: vi.fn() }));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return {
    ...actual,
    getRegion: getRegionMock,
  };
});

const { Region } = await import('./Region.tsx');

function renderAt(path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/region/:regionSlug" element={<Region />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function makeDetail(overrides: Partial<RegionDetail> = {}): RegionDetail {
  return {
    region: {
      id: 1,
      kind: 'us:metro',
      name: 'New York Metro',
      slug: 'nyc-metro',
      country: 'US',
      scope_tier: 'regional',
      parent_slugs: [],
    },
    orgs: [
      {
        id: 1001,
        slug: 'transitcenter',
        name: 'TransitCenter',
        short_desc: 'Foundation pushing for better transit.',
        website_url: 'https://transitcenter.org',
        tags: ['transit', 'policy'],
        regions: [],
      },
      {
        id: 1002,
        slug: 'riders-alliance',
        name: 'Riders Alliance',
        short_desc: 'Grassroots NYC transit riders.',
        website_url: 'https://www.ridersny.org',
        tags: ['transit', 'grassroots'],
        regions: [],
      },
    ],
    ancestry: [],
    ...overrides,
  };
}

describe('Region', () => {
  beforeEach(() => {
    getRegionMock.mockReset();
  });

  afterEach(() => {
    getRegionMock.mockReset();
  });

  it('renders the loading state while the query is pending', () => {
    getRegionMock.mockReturnValue(new Promise(() => {}));
    renderAt('/region/nyc-metro');
    expect(screen.getByRole('status').textContent).toMatch(/loading region/i);
  });

  it('renders the region name and its org list on success', async () => {
    getRegionMock.mockResolvedValueOnce(makeDetail());
    renderAt('/region/nyc-metro');

    await waitFor(() => {
      expect(screen.getByRole('link', { name: 'TransitCenter' })).toBeDefined();
    });
    const h1 = screen.getByRole('heading', { level: 1 });
    expect(h1.textContent).toMatch(/new york metro/i);
    expect(screen.getByRole('link', { name: 'Riders Alliance' })).toBeDefined();
    // Section heading present, mirroring the classified layout.
    expect(screen.getByText(/groups working in new york metro/i)).toBeDefined();
  });

  it('passes the URL slug through to getRegion', async () => {
    getRegionMock.mockResolvedValueOnce(makeDetail());
    renderAt('/region/nyc-metro');

    await waitFor(() => {
      expect(getRegionMock).toHaveBeenCalledWith('nyc-metro', expect.any(Object));
    });
  });

  // City-kind regions render via the same route; the kind label in
  // the eyebrow + rail should read "City", not "Metropolitan area".
  it('renders a city-kind region with a "City" kind label', async () => {
    getRegionMock.mockResolvedValueOnce(
      makeDetail({
        region: {
          id: 50,
          kind: 'us:city',
          name: 'Chicago',
          slug: 'chicago',
          country: 'US',
          scope_tier: 'local',
          parent_slugs: ['cook-county'],
        },
        orgs: [],
      }),
    );
    renderAt('/region/chicago');

    await waitFor(() => {
      const h1 = screen.getByRole('heading', { level: 1 });
      expect(h1.textContent).toMatch(/chicago/i);
    });
    // Eyebrow text includes "City report".
    expect(screen.getAllByText(/city report/i).length).toBeGreaterThan(0);
  });

  // State-kind regions resolve too (the broadened detail endpoint).
  // The eyebrow should read "State report".
  it('renders a state-kind region with a "State report" eyebrow', async () => {
    getRegionMock.mockResolvedValueOnce(
      makeDetail({
        region: {
          id: 60,
          kind: 'us:state',
          name: 'New York',
          slug: 'ny',
          country: 'US',
          scope_tier: 'regional',
          parent_slugs: [],
        },
        orgs: [],
      }),
    );
    renderAt('/region/ny');

    await waitFor(() => {
      const h1 = screen.getByRole('heading', { level: 1 });
      expect(h1.textContent).toMatch(/new york/i);
    });
    expect(screen.getAllByText(/state report/i).length).toBeGreaterThan(0);
  });

  // The breadcrumb kicker walks `ancestry` and renders each ancestor
  // as a clickable Link. The API returns ancestry closest-first;
  // the SPA reverses it so the breadcrumb reads root → leaf.
  it('renders ancestry as clickable breadcrumb links in the kicker', async () => {
    getRegionMock.mockResolvedValueOnce(
      makeDetail({
        region: {
          id: 99,
          kind: 'us:city',
          name: 'Brooklyn',
          slug: 'brooklyn-ny',
          country: 'US',
          scope_tier: 'local',
          parent_slugs: ['kings-county-ny'],
        },
        // API order: closest-first (direct parent → grandparent → …).
        ancestry: [
          {
            id: 2,
            kind: 'us:county',
            name: 'Kings County, NY',
            slug: 'kings-county-ny',
            country: 'US',
            scope_tier: 'local',
            parent_slugs: ['nyc-metro', 'ny'],
          },
          {
            id: 3,
            kind: 'us:metro',
            name: 'New York Metro',
            slug: 'nyc-metro',
            country: 'US',
            scope_tier: 'regional',
            parent_slugs: ['nyc-tristate'],
          },
          {
            id: 4,
            kind: 'us:state',
            name: 'NY',
            slug: 'ny',
            country: 'US',
            scope_tier: 'regional',
            parent_slugs: [],
          },
        ],
      }),
    );
    renderAt('/region/brooklyn-ny');

    await waitFor(() => {
      const h1 = screen.getByRole('heading', { level: 1 });
      expect(h1.textContent).toMatch(/brooklyn/i);
    });

    // Each ancestor renders as a link to /region/<slug>.
    const kings = screen.getByRole('link', { name: 'Kings County, NY' });
    expect(kings.getAttribute('href')).toBe('/region/kings-county-ny');
    const metro = screen.getByRole('link', { name: 'New York Metro' });
    expect(metro.getAttribute('href')).toBe('/region/nyc-metro');
    const ny = screen.getByRole('link', { name: 'NY' });
    expect(ny.getAttribute('href')).toBe('/region/ny');
  });

  it('renders the inline empty-state on 404 (not a crash)', async () => {
    getRegionMock.mockRejectedValueOnce(
      new ApiError(
        404,
        'Not Found',
        { type: 'about:blank', title: 'Not Found', status: 404 },
        'req-region-1',
      ),
    );
    renderAt('/region/totally-fake');

    await waitFor(() => {
      expect(screen.getByText(/isn.t in the atlas yet/i)).toBeDefined();
    });
    // Browse link is the suggested next step (one in breadcrumb, one in
    // the empty-state copy — at least one points at /browse).
    const browseLinks = screen
      .getAllByRole('link', { name: /browse/i })
      .filter((a) => a.getAttribute('href') === '/browse');
    expect(browseLinks.length).toBeGreaterThan(0);
  });

  it('renders a friendly empty state when orgs is an empty array', async () => {
    getRegionMock.mockResolvedValueOnce(makeDetail({ orgs: [] }));
    renderAt('/region/nyc-metro');

    await waitFor(() => {
      expect(screen.getByText(/no organizations indexed yet/i)).toBeDefined();
    });
  });

  it('sets the browser tab title to the region name on success', async () => {
    getRegionMock.mockResolvedValueOnce(makeDetail());
    renderAt('/region/nyc-metro');

    await waitFor(() => {
      expect(document.title).toMatch(/new york metro.*urbanist atlas/i);
    });
  });

  it('renders a non-404 ApiError as an error state, not the 404 empty-state', async () => {
    getRegionMock.mockRejectedValueOnce(
      new ApiError(
        500,
        'Database is on fire',
        { type: 'about:blank', title: 'Database is on fire', status: 500 },
        'req-region-2',
      ),
    );
    renderAt('/region/nyc-metro');

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      expect(alert.textContent).toContain('Database is on fire');
      expect(alert.textContent).toContain('req-region-2');
    });
  });
});
