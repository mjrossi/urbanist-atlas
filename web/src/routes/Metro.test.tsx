import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { MetroDetail } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';

const { getMetroMock } = vi.hoisted(() => ({ getMetroMock: vi.fn() }));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return {
    ...actual,
    getMetro: getMetroMock,
  };
});

const { Metro } = await import('./Metro.tsx');

function renderAt(path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/m/:metroSlug" element={<Metro />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function makeDetail(overrides: Partial<MetroDetail> = {}): MetroDetail {
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
    ...overrides,
  };
}

describe('Metro', () => {
  beforeEach(() => {
    getMetroMock.mockReset();
  });

  afterEach(() => {
    getMetroMock.mockReset();
  });

  it('renders the loading state while the query is pending', () => {
    getMetroMock.mockReturnValue(new Promise(() => {}));
    renderAt('/m/nyc-metro');
    expect(screen.getByRole('status').textContent).toMatch(/loading metro/i);
  });

  it('renders the metro name and its org list on success', async () => {
    getMetroMock.mockResolvedValueOnce(makeDetail());
    renderAt('/m/nyc-metro');

    await waitFor(() => {
      expect(screen.getByRole('link', { name: 'TransitCenter' })).toBeDefined();
    });
    expect(screen.getByText('New York Metro')).toBeDefined();
    expect(screen.getByRole('link', { name: 'Riders Alliance' })).toBeDefined();
    // Section heading present, mirroring the classified layout.
    expect(screen.getByText(/organizations serving new york metro/i)).toBeDefined();
  });

  it('passes the URL slug through to getMetro', async () => {
    getMetroMock.mockResolvedValueOnce(makeDetail());
    renderAt('/m/nyc-metro');

    await waitFor(() => {
      expect(getMetroMock).toHaveBeenCalledWith('nyc-metro', expect.any(Object));
    });
  });

  it('renders the inline empty-state on 404 (not a crash)', async () => {
    getMetroMock.mockRejectedValueOnce(
      new ApiError(
        404,
        'Not Found',
        { type: 'about:blank', title: 'Not Found', status: 404 },
        'req-metro-1',
      ),
    );
    renderAt('/m/totally-fake');

    await waitFor(() => {
      expect(screen.getByText(/isn.t in the atlas yet/i)).toBeDefined();
    });
    // Browse link is the suggested next step.
    expect(
      screen.getByRole('link', { name: /browse/i }).getAttribute('href'),
    ).toBe('/browse');
  });

  it('renders a friendly empty state when orgs is an empty array', async () => {
    getMetroMock.mockResolvedValueOnce(makeDetail({ orgs: [] }));
    renderAt('/m/nyc-metro');

    await waitFor(() => {
      expect(screen.getByText(/no organizations indexed yet/i)).toBeDefined();
    });
  });

  it('renders a non-404 ApiError as an error state, not the 404 empty-state', async () => {
    getMetroMock.mockRejectedValueOnce(
      new ApiError(
        500,
        'Database is on fire',
        { type: 'about:blank', title: 'Database is on fire', status: 500 },
        'req-metro-2',
      ),
    );
    renderAt('/m/nyc-metro');

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      expect(alert.textContent).toContain('Database is on fire');
      expect(alert.textContent).toContain('req-metro-2');
    });
  });
});
