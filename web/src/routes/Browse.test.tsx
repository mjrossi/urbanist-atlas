import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { MetroSummary } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';

const { listMetrosMock } = vi.hoisted(() => ({ listMetrosMock: vi.fn() }));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return {
    ...actual,
    listMetros: listMetrosMock,
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

function makeMetro(overrides: Partial<MetroSummary['region']> = {}, org_count = 4): MetroSummary {
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
    listMetrosMock.mockReset();
  });

  afterEach(() => {
    listMetrosMock.mockReset();
  });

  it('renders the loading state while the query is pending', () => {
    listMetrosMock.mockReturnValue(new Promise(() => {}));
    renderBrowse();
    expect(screen.getByRole('status').textContent).toMatch(/loading metros/i);
  });

  it('renders metro rows in the order returned by the API', async () => {
    listMetrosMock.mockResolvedValueOnce([
      makeMetro({ id: 1, slug: 'nyc-metro', name: 'New York Metro' }, 12),
      makeMetro({ id: 2, slug: 'sf-bay-area', name: 'San Francisco Bay Area' }, 7),
      makeMetro({ id: 3, slug: 'aml', name: 'Área Metropolitana de Lisboa', country: 'PT' }, 3),
    ]);
    renderBrowse();

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /new york metro/i })).toBeDefined();
    });
    const links = screen.getAllByRole('link');
    const names = links.map((a) => a.textContent ?? '');
    expect(names[0]).toMatch(/new york metro/i);
    expect(names[1]).toMatch(/san francisco bay area/i);
    expect(names[2]).toMatch(/lisboa/i);
  });

  it('each metro row links to /m/:slug', async () => {
    listMetrosMock.mockResolvedValueOnce([
      makeMetro({ slug: 'nyc-metro', name: 'New York Metro' }, 12),
      makeMetro({ id: 2, slug: 'aml', name: 'AML', country: 'PT' }, 3),
    ]);
    renderBrowse();

    await waitFor(() => {
      const nyc = screen.getByRole('link', { name: /new york metro/i });
      expect(nyc.getAttribute('href')).toBe('/m/nyc-metro');
    });
    expect(screen.getByRole('link', { name: /^AML/i }).getAttribute('href')).toBe('/m/aml');
  });

  it('renders the org count for each metro', async () => {
    listMetrosMock.mockResolvedValueOnce([
      makeMetro({ slug: 'nyc-metro', name: 'New York Metro' }, 12),
      makeMetro({ id: 2, slug: 'solo', name: 'Solo Metro' }, 1),
    ]);
    renderBrowse();

    await waitFor(() => {
      expect(screen.getByText(/12 groups/i)).toBeDefined();
    });
    expect(screen.getByText(/^1 group$/i)).toBeDefined();
  });

  it('renders an empty state when the list is empty', async () => {
    listMetrosMock.mockResolvedValueOnce([]);
    renderBrowse();

    await waitFor(() => {
      expect(screen.getByText(/no metros indexed yet/i)).toBeDefined();
    });
  });

  it('renders the error state on ApiError', async () => {
    listMetrosMock.mockRejectedValueOnce(
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
});
