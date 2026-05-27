import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { RegionSummary } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';

const { listRegionsMock } = vi.hoisted(() => ({ listRegionsMock: vi.fn() }));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return {
    ...actual,
    listRegions: listRegionsMock,
  };
});

const { Browse } = await import('./Browse.tsx');

function renderBrowse() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/browse']}>
        <Routes>
          <Route path="/browse" element={<Browse />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function makeRegion(
  overrides: Partial<RegionSummary['region']> & { browse_parent_slug?: string } = {},
  org_count = 4,
): RegionSummary {
  const { browse_parent_slug, ...regionOverrides } = overrides;
  const summary: RegionSummary = {
    region: {
      id: 1,
      kind: 'us:metro',
      name: 'New York Metro',
      slug: 'nyc-metro',
      country: 'US',
      scope_tier: 'regional',
      parent_slugs: [],
      ...regionOverrides,
    },
    org_count,
  };
  if (browse_parent_slug !== undefined) {
    summary.browse_parent_slug = browse_parent_slug;
  }
  return summary;
}

describe('Browse', () => {
  beforeEach(() => {
    listRegionsMock.mockReset();
  });

  afterEach(() => {
    listRegionsMock.mockReset();
  });

  it('renders the loading state while the query is pending', () => {
    listRegionsMock.mockReturnValue(new Promise(() => {}));
    renderBrowse();
    expect(screen.getByRole('status').textContent).toMatch(/loading the index/i);
  });

  it('renders provided regions as /region/:slug links across the country sections', async () => {
    listRegionsMock.mockResolvedValueOnce([
      makeRegion({ id: 1, slug: 'nyc-metro', name: 'New York Metro' }, 12),
      makeRegion({ id: 2, slug: 'sf-bay-area', name: 'San Francisco Bay Area' }, 7),
      makeRegion({ id: 3, slug: 'toronto-gta', name: 'Toronto GTA', country: 'CA' }, 3),
    ]);
    renderBrowse();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york metro/i })).toBeDefined();
    });
    expect(
      screen.getByRole('link', { name: /new york metro/i }).getAttribute('href'),
    ).toBe('/region/nyc-metro');
    expect(
      screen.getByRole('link', { name: /san francisco bay area/i }).getAttribute('href'),
    ).toBe('/region/sf-bay-area');
    expect(
      screen.getByRole('link', { name: /toronto gta/i }).getAttribute('href'),
    ).toBe('/region/toronto-gta');
  });

  it('each region row links to /region/:slug', async () => {
    listRegionsMock.mockResolvedValueOnce([
      makeRegion({ slug: 'nyc-metro', name: 'New York Metro' }, 12),
      makeRegion({ id: 2, slug: 'aml', name: 'AML' }, 3),
    ]);
    renderBrowse();

    await waitFor(() => {
      const nyc = screen.getByRole('link', { name: /new york metro/i });
      expect(nyc.getAttribute('href')).toBe('/region/nyc-metro');
    });
    expect(screen.getByRole('link', { name: /^AML/i }).getAttribute('href')).toBe('/region/aml');
  });

  // Cities whose `browse_parent_slug` points at a visible anchor in
  // the response nest under that anchor with the `.child` modifier
  // on the row. Pins the nested-layout fix for the duplicate-feeling
  // "Toronto" + "Toronto CMA" pairs.
  it('nests cities under their parent metro via browse_parent_slug', async () => {
    listRegionsMock.mockResolvedValueOnce([
      makeRegion({ id: 1, slug: 'chicago-metro', name: 'Chicago Metro', kind: 'us:metro' }, 4),
      makeRegion(
        {
          id: 2,
          slug: 'chicago',
          name: 'Chicago',
          kind: 'us:city',
          browse_parent_slug: 'chicago-metro',
        },
        3,
      ),
      makeRegion({ id: 3, slug: 'toronto-cma', name: 'Toronto CMA', kind: 'ca:cma', country: 'CA' }, 4),
      makeRegion(
        {
          id: 4,
          slug: 'toronto-on',
          name: 'Toronto',
          kind: 'ca:city',
          country: 'CA',
          browse_parent_slug: 'toronto-cma',
        },
        2,
      ),
    ]);
    const { container } = renderBrowse();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /chicago metro/i })).toBeDefined();
    });

    // Chicago renders as a child row of Chicago Metro.
    const chicagoLink = screen.getByRole('link', { name: /^ChicagoUS · City/i });
    expect(chicagoLink.getAttribute('href')).toBe('/region/chicago');
    expect(chicagoLink.classList.contains('child')).toBe(true);

    // Chicago Metro is the anchor (no `.child` class).
    const chicagoMetroLink = screen.getByRole('link', { name: /chicago metro/i });
    expect(chicagoMetroLink.classList.contains('child')).toBe(false);

    // Both Chicago Metro + Chicago live inside the same anchor group
    // (their DOM parent has class `.index-anchor-group`).
    const groups = Array.from(container.querySelectorAll('.index-anchor-group'));
    const chicagoGroup = groups.find((g) =>
      g.contains(chicagoMetroLink),
    );
    expect(chicagoGroup).toBeDefined();
    expect(chicagoGroup?.contains(chicagoLink)).toBe(true);

    // Toronto city nests under Toronto CMA the same way.
    const torontoLink = screen.getByRole('link', { name: /^TorontoCA · City/i });
    expect(torontoLink.getAttribute('href')).toBe('/region/toronto-on');
    expect(torontoLink.classList.contains('child')).toBe(true);
  });

  // Cities whose `browse_parent_slug` doesn't match a visible
  // anchor (parent absent from this response) fall back to
  // rendering as their own top-level anchor — no orphan rows.
  it('renders a city as top-level anchor when its parent slug is not in the response', async () => {
    listRegionsMock.mockResolvedValueOnce([
      // No nyc-metro row in this response — just NYC the city.
      makeRegion(
        {
          id: 10,
          slug: 'nyc',
          name: 'New York City',
          kind: 'us:city',
          browse_parent_slug: 'nyc-metro',
        },
        3,
      ),
    ]);
    renderBrowse();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york city/i })).toBeDefined();
    });
    const link = screen.getByRole('link', { name: /new york city/i });
    expect(link.classList.contains('child')).toBe(false);
  });

  // Legacy "duplicate" scenario: cities WITHOUT browse_parent_slug
  // (e.g. metros, or cities where the API didn't compute a parent)
  // continue to render flat next to their metros — pre-nesting
  // behavior preserved for the unchanged-data case.
  it('renders city-kind entries (us:city, ca:city) alongside metros', async () => {
    listRegionsMock.mockResolvedValueOnce([
      makeRegion({ id: 1, slug: 'chicago-metro', name: 'Chicago Metro', kind: 'us:metro' }, 4),
      makeRegion({ id: 2, slug: 'chicago', name: 'Chicago', kind: 'us:city' }, 3),
      makeRegion({ id: 3, slug: 'toronto-on', name: 'Toronto', kind: 'ca:city', country: 'CA' }, 2),
    ]);
    renderBrowse();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /chicago metro/i })).toBeDefined();
    });
    expect(
      screen.getByRole('link', { name: /chicago metro/i }).getAttribute('href'),
    ).toBe('/region/chicago-metro');
    // "Chicago" (city) link, distinct from "Chicago Metro".
    expect(
      screen.getByRole('link', { name: /^ChicagoUS · City/i }).getAttribute('href'),
    ).toBe('/region/chicago');
    // Toronto city, distinct from any Toronto CMA entry.
    expect(
      screen.getByRole('link', { name: /^TorontoCA · City/i }).getAttribute('href'),
    ).toBe('/region/toronto-on');
  });

  it('renders the org count for each region', async () => {
    listRegionsMock.mockResolvedValueOnce([
      makeRegion({ slug: 'nyc-metro', name: 'New York Metro' }, 12),
      makeRegion({ id: 2, slug: 'solo', name: 'Solo Metro' }, 1),
    ]);
    const { container } = renderBrowse();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york metro/i })).toBeDefined();
    });
    const counts = Array.from(container.querySelectorAll('.icount')).map(
      (n) => n.textContent ?? '',
    );
    const joined = counts.join(' | ');
    expect(joined).toMatch(/12\s*groups/);
    expect(joined).toMatch(/1\s*group(?!s)/);
  });

  it('renders an empty state when the list is empty', async () => {
    listRegionsMock.mockResolvedValueOnce([]);
    renderBrowse();

    await waitFor(() => {
      expect(screen.getByText(/no regions indexed yet/i)).toBeDefined();
    });
  });

  it('renders the error state on ApiError', async () => {
    listRegionsMock.mockRejectedValueOnce(
      new ApiError(
        500,
        'Database is on fire',
        { type: 'about:blank', title: 'Database is on fire', status: 500 },
        'req-browse-1',
      ),
    );
    renderBrowse();

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      expect(alert.textContent).toContain('Database is on fire');
    });
  });

  it('sets the browser tab title', async () => {
    listRegionsMock.mockReturnValue(new Promise(() => {}));
    renderBrowse();
    await waitFor(() => {
      expect(document.title).toMatch(/browse.*urbanist atlas/i);
    });
  });
});
