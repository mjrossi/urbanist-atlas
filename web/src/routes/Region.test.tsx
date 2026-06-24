import { screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { LookupOrg, Region as RegionT, RegionDetail } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';
import { renderWithProviders } from '../test/renderWithProviders.tsx';

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
  return renderWithProviders(<Region />, {
    initialEntries: [path],
    routePath: '/region/:regionSlug',
  });
}

function makeOrg(overrides: Partial<LookupOrg> = {}): LookupOrg {
  return {
    id: 1001,
    slug: 'transitcenter',
    name: 'TransitCenter',
    short_desc: 'Foundation pushing for better transit.',
    website_url: 'https://transitcenter.org',
    tags: ['transit', 'policy'],
    regions: [],
    matched_region_slugs: [],
    added_at: '2026-05-17',
    ...overrides,
  };
}

function makeRegion(overrides: Partial<RegionT> = {}): RegionT {
  return {
    id: 1,
    kind: 'us:metro',
    name: 'New York Metro',
    slug: 'nyc-metro',
    country: 'US',
    scope_tier: 'regional',
    parent_slugs: [],
    ...overrides,
  };
}

function makeDetail(overrides: Partial<RegionDetail> = {}): RegionDetail {
  return {
    region: makeRegion(),
    local: [],
    regional: [
      makeOrg({
        id: 1001,
        slug: 'transitcenter',
        name: 'TransitCenter',
        matched_region_slugs: ['nyc-metro'],
      }),
      makeOrg({
        id: 1002,
        slug: 'riders-alliance',
        name: 'Riders Alliance',
        short_desc: 'Grassroots NYC transit riders.',
        website_url: 'https://www.ridersny.org',
        tags: ['transit', 'grassroots'],
        matched_region_slugs: ['nyc-metro'],
      }),
    ],
    statewide: [],
    ancestry: [],
    descendant_region_names: {},
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
    // Regional section label rendered (since regional bucket is non-empty).
    expect(screen.getByText('Regional')).toBeDefined();
  });

  it('passes the URL slug through to getRegion', async () => {
    getRegionMock.mockResolvedValueOnce(makeDetail());
    renderAt('/region/nyc-metro');

    await waitFor(() => {
      expect(getRegionMock).toHaveBeenCalledWith('nyc-metro', expect.any(Object));
    });
  });

  // Local/Regional bucketing surfaces the new scope semantics: a
  // city-tagged org shows up in Local, a metro-tagged org in
  // Regional. The kicker reads "City report" for a us:city.
  it('renders Local + Regional sections from the bucketed scope', async () => {
    getRegionMock.mockResolvedValueOnce(
      makeDetail({
        region: makeRegion({
          id: 50,
          kind: 'us:city',
          name: 'Chicago',
          slug: 'chicago',
          scope_tier: 'local',
          parent_slugs: ['cook-county'],
        }),
        local: [
          makeOrg({
            id: 10,
            slug: 'better-streets-chicago',
            name: 'Better Streets Chicago',
            matched_region_slugs: ['chicago'],
          }),
        ],
        regional: [
          makeOrg({
            id: 11,
            slug: 'active-trans',
            name: 'Active Transportation Alliance',
            matched_region_slugs: ['chicago-metro'],
          }),
        ],
        ancestry: [
          {
            id: 60,
            kind: 'us:county',
            name: 'Cook County',
            slug: 'cook-county',
            country: 'US',
            scope_tier: 'local',
            parent_slugs: ['chicago-metro'],
          },
          {
            id: 61,
            kind: 'us:metro',
            name: 'Chicago Metro',
            slug: 'chicago-metro',
            country: 'US',
            scope_tier: 'regional',
            parent_slugs: ['chicagoland'],
          },
        ],
      }),
    );
    renderAt('/region/chicago');

    await waitFor(() => {
      const h1 = screen.getByRole('heading', { level: 1 });
      expect(h1.textContent).toMatch(/chicago/i);
    });

    // Eyebrow + kicker reflect the kind label.
    expect(screen.getAllByText(/city report/i).length).toBeGreaterThan(0);

    // Both section labels rendered (EntryList only renders a section
    // when it has at least one entry).
    expect(screen.getByText('Local')).toBeDefined();
    expect(screen.getByText('Regional')).toBeDefined();

    // The city-tagged org is in Local; the metro-tagged org is in
    // Regional. We assert by role (both rendered as org-name links).
    expect(screen.getByRole('link', { name: 'Better Streets Chicago' })).toBeDefined();
    expect(
      screen.getByRole('link', { name: 'Active Transportation Alliance' }),
    ).toBeDefined();
  });

  it('renders the State / Provincial section for state-attached orgs', async () => {
    getRegionMock.mockResolvedValueOnce(
      makeDetail({
        region: makeRegion({
          id: 70,
          kind: 'us:state',
          name: 'Michigan',
          slug: 'mi',
          scope_tier: 'regional',
          parent_slugs: [],
        }),
        local: [],
        regional: [
          makeOrg({
            id: 20,
            slug: 'detroit-greenways',
            name: 'Detroit Greenways Coalition',
            matched_region_slugs: ['detroit-mi-metro'],
          }),
        ],
        statewide: [
          makeOrg({
            id: 21,
            slug: 'league-of-michigan-bicyclists',
            name: 'League of Michigan Bicyclists',
            matched_region_slugs: ['mi'],
          }),
        ],
      }),
    );
    renderAt('/region/mi');

    await waitFor(() => {
      const h1 = screen.getByRole('heading', { level: 1 });
      expect(h1.textContent).toMatch(/michigan/i);
    });

    // Regional and State / Provincial sections both render.
    expect(screen.getByText('Regional')).toBeDefined();
    expect(screen.getByText('State / Provincial')).toBeDefined();
    expect(
      screen.getByRole('link', { name: 'Detroit Greenways Coalition' }),
    ).toBeDefined();
    expect(
      screen.getByRole('link', { name: 'League of Michigan Bicyclists' }),
    ).toBeDefined();
  });

  it('renders a state-kind region with a "State report" eyebrow', async () => {
    getRegionMock.mockResolvedValueOnce(
      makeDetail({
        region: makeRegion({
          id: 60,
          kind: 'us:state',
          name: 'New York',
          slug: 'ny',
          parent_slugs: [],
        }),
        local: [],
        regional: [],
        ancestry: [],
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
        region: makeRegion({
          id: 99,
          kind: 'us:city',
          name: 'Brooklyn',
          slug: 'brooklyn-ny',
          scope_tier: 'local',
          parent_slugs: ['kings-county-ny'],
        }),
        local: [],
        regional: [],
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

    const kings = screen.getByRole('link', { name: 'Kings County, NY' });
    expect(kings.getAttribute('href')).toBe('/region/kings-county-ny');
    const metro = screen.getByRole('link', { name: 'New York Metro' });
    expect(metro.getAttribute('href')).toBe('/region/nyc-metro');
    const ny = screen.getByRole('link', { name: 'NY' });
    expect(ny.getAttribute('href')).toBe('/region/ny');
  });

  it('renders the backend not-found copy on 404 with Browse chrome (not a crash)', async () => {
    getRegionMock.mockRejectedValueOnce(
      new ApiError(
        404,
        'Region Not Found',
        {
          type: 'https://urbanistatlas.com/problems/not-found',
          title: 'Region Not Found',
          detail:
            "We don't have this region in the atlas yet. It may not be indexed, or the link you followed may be out of date.",
          status: 404,
        },
        'req-region-1',
      ),
    );
    renderAt('/region/totally-fake');

    // Server-supplied title + detail render verbatim; the frontend
    // doesn't author the message.
    await waitFor(() => {
      expect(screen.getByText(/don.t have this region in the atlas yet/i)).toBeDefined();
    });
    // Navigation links remain as frontend chrome.
    const browseLinks = screen
      .getAllByRole('link', { name: /browse/i })
      .filter((a) => a.getAttribute('href') === '/browse');
    expect(browseLinks.length).toBeGreaterThan(0);
  });

  it('falls back to a generic deck on a 404 with no problem body', async () => {
    getRegionMock.mockRejectedValueOnce(
      new ApiError(404, 'Not Found', undefined, 'req-region-3'),
    );
    renderAt('/region/totally-fake');

    // No problem+json body (e.g. a proxy-injected error page): the deck
    // falls back to a generic line that doesn't duplicate — and so can't
    // drift from — the API's authoritative copy. Browse chrome remains.
    await waitFor(() => {
      expect(screen.getByText(/this page isn.t available/i)).toBeDefined();
    });
    const browseLinks = screen
      .getAllByRole('link', { name: /browse/i })
      .filter((a) => a.getAttribute('href') === '/browse');
    expect(browseLinks.length).toBeGreaterThan(0);
  });

  it('renders a friendly empty state when all three buckets are empty', async () => {
    getRegionMock.mockResolvedValueOnce(
      makeDetail({ local: [], regional: [], statewide: [] }),
    );
    renderAt('/region/nyc-metro');

    await waitFor(() => {
      expect(screen.getByText(/it belongs in the atlas/i)).toBeDefined();
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

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('Database is on fire');
    expect(alert.textContent).toContain('req-region-2');
  });
});
