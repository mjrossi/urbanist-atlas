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

function makeRegion(overrides: Partial<RegionSummary['region']> = {}, org_count = 4): RegionSummary {
  return {
    region: {
      id: 1,
      kind: 'us:metro',
      name: 'New York Metro',
      slug: 'nyc-metro',
      country: 'US',
      scope_tier: 'regional',
      parent_slugs: [],
      ...overrides,
    },
    org_count,
  };
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

  // City entries surface alongside their parent metros under the
  // default browse set. Pins the broadened-kind behavior.
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
