import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { Org as OrgT } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';

const { getOrgMock } = vi.hoisted(() => ({ getOrgMock: vi.fn() }));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return {
    ...actual,
    getOrg: getOrgMock,
  };
});

const { Org } = await import('./Org.tsx');

function renderAt(path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/orgs/:slug" element={<Org />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function makeOrg(overrides: Partial<OrgT> = {}): OrgT {
  return {
    id: 1,
    slug: 'transalt',
    name: 'Transportation Alternatives',
    short_desc: 'NYC-wide advocacy for walking, biking, and public transit.',
    website_url: 'https://www.transalt.org',
    tags: ['transit', 'safe-streets'],
    regions: [
      {
        id: 10,
        kind: 'us:metro',
        name: 'New York Metro',
        slug: 'nyc-metro',
        country: 'US',
        scope_tier: 'regional',
        parent_slugs: [],
      },
      {
        id: 11,
        kind: 'us:borough',
        name: 'Brooklyn',
        slug: 'brooklyn',
        country: 'US',
        scope_tier: 'local',
        parent_slugs: ['nyc'],
      },
    ],
    ...overrides,
  };
}

describe('Org', () => {
  beforeEach(() => {
    getOrgMock.mockReset();
  });

  afterEach(() => {
    getOrgMock.mockReset();
  });

  it('renders the loading state while the query is pending', () => {
    getOrgMock.mockReturnValue(new Promise(() => {}));
    renderAt('/orgs/transalt');
    expect(screen.getByRole('status').textContent).toMatch(/loading organization/i);
  });

  it('renders the org name, description, tags, and regions on success', async () => {
    getOrgMock.mockResolvedValueOnce(makeOrg());
    renderAt('/orgs/transalt');

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { level: 1, name: 'Transportation Alternatives' }),
      ).toBeDefined();
    });
    expect(
      screen.getByText(/NYC-wide advocacy for walking, biking, and public transit/i),
    ).toBeDefined();
    // Tags rendered as chips.
    expect(screen.getByText('transit')).toBeDefined();
    expect(screen.getByText('safe-streets')).toBeDefined();
    // Region with a metro kind links to /m/:slug.
    const metroLink = screen.getByRole('link', { name: 'New York Metro' });
    expect(metroLink.getAttribute('href')).toBe('/m/nyc-metro');
    // Non-metro region (borough) renders as plain text, not a link.
    expect(screen.queryByRole('link', { name: 'Brooklyn' })).toBeNull();
    expect(screen.getByText('Brooklyn')).toBeDefined();
  });

  it('renders the website domain as an external link', async () => {
    getOrgMock.mockResolvedValueOnce(makeOrg());
    renderAt('/orgs/transalt');

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { level: 1, name: 'Transportation Alternatives' }),
      ).toBeDefined();
    });
    const link = screen.getByRole('link', { name: 'transalt.org' });
    expect(link.getAttribute('href')).toBe('https://www.transalt.org');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toContain('noopener');
  });

  // METRO_KINDS in Org.tsx must stay in lockstep with the server's
  // metroKinds map (api/pkg/atlas/metro_kinds.go). These tests pin the
  // currently-known set so a one-sided edit fails CI loudly.
  it('links ca:regional-district regions to /m/:slug', async () => {
    getOrgMock.mockResolvedValueOnce(
      makeOrg({
        regions: [
          {
            id: 20,
            kind: 'ca:regional-district',
            name: 'Metro Vancouver',
            slug: 'metro-vancouver',
            country: 'CA',
            scope_tier: 'regional',
            parent_slugs: [],
          },
        ],
      }),
    );
    renderAt('/orgs/transalt');

    await waitFor(() => {
      const link = screen.getByRole('link', { name: 'Metro Vancouver' });
      expect(link.getAttribute('href')).toBe('/m/metro-vancouver');
    });
  });

  it('does NOT link us:multi-state regions (server /metros does not serve them)', async () => {
    getOrgMock.mockResolvedValueOnce(
      makeOrg({
        regions: [
          {
            id: 30,
            kind: 'us:multi-state',
            name: 'NYC Tri-State',
            slug: 'nyc-tristate',
            country: 'US',
            scope_tier: 'regional',
            parent_slugs: [],
          },
        ],
      }),
    );
    renderAt('/orgs/transalt');

    await waitFor(() => {
      expect(screen.getByText('NYC Tri-State')).toBeDefined();
    });
    expect(screen.queryByRole('link', { name: 'NYC Tri-State' })).toBeNull();
  });

  it('renders a contact link when contact_url is present', async () => {
    getOrgMock.mockResolvedValueOnce(
      makeOrg({ contact_url: 'https://www.transalt.org/contact' }),
    );
    renderAt('/orgs/transalt');

    await waitFor(() => {
      const link = screen.getByRole('link', { name: /contact/i });
      expect(link.getAttribute('href')).toBe('https://www.transalt.org/contact');
    });
  });

  it('passes the URL slug through to getOrg', async () => {
    getOrgMock.mockResolvedValueOnce(makeOrg());
    renderAt('/orgs/transalt');

    await waitFor(() => {
      expect(getOrgMock).toHaveBeenCalledWith('transalt', expect.any(Object));
    });
  });

  it('renders the inline empty-state on 404 (not a crash)', async () => {
    getOrgMock.mockRejectedValueOnce(
      new ApiError(
        404,
        'Not Found',
        {
          type: 'https://urbanistatlas.com/problems/not-found',
          title: 'Not Found',
          status: 404,
        },
        'req-org-1',
      ),
    );
    renderAt('/orgs/totally-fake');

    await waitFor(() => {
      expect(screen.getByText(/isn.t in the atlas yet/i)).toBeDefined();
    });
    expect(
      screen.getByRole('link', { name: /browse/i }).getAttribute('href'),
    ).toBe('/browse');
  });

  it('renders a non-404 ApiError as an error state', async () => {
    getOrgMock.mockRejectedValueOnce(
      new ApiError(
        500,
        'Database is on fire',
        { type: 'about:blank', title: 'Database is on fire', status: 500 },
        'req-org-2',
      ),
    );
    renderAt('/orgs/transalt');

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      expect(alert.textContent).toContain('Database is on fire');
      expect(alert.textContent).toContain('req-org-2');
    });
  });
});
