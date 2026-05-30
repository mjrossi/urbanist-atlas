import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import type { Org as OrgT } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';
import { renderWithProviders } from '../test/renderWithProviders.tsx';

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
  return renderWithProviders(<Org />, {
    initialEntries: [path],
    routePath: '/orgs/:slug',
  });
}

function makeOrg(overrides: Partial<OrgT> = {}): OrgT {
  return {
    id: 1,
    slug: 'transalt',
    name: 'Transportation Alternatives',
    short_desc: 'NYC-wide advocacy for walking, biking, and public transit.',
    website_url: 'https://www.transalt.org',
    tags: ['transit', 'safe-streets'],
    added_at: '2026-05-17',
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
      screen.getAllByText(/NYC-wide advocacy for walking, biking, and public transit/i)
        .length,
    ).toBeGreaterThan(0);
    // Tags rendered as chips, prettified (hyphens → spaces). Both the
    // dateline ("§ transit · safe streets") and the tag-list spans show
    // the prettified label.
    expect(screen.getAllByText('transit').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/safe streets/i).length).toBeGreaterThan(0);
    // Region with a place kind links to /region/:slug (multiple links share
    // the name — meta-strip + regions list + companion rail).
    const placeLinks = screen
      .getAllByRole('link', { name: 'New York Metro' })
      .filter((a) => a.getAttribute('href') === '/region/nyc-metro');
    expect(placeLinks.length).toBeGreaterThan(0);
    // Boroughs are browseable (server resolves /region/{slug} for
    // any non-national kind) so they render as a clickable link too.
    const boroughLinks = screen
      .getAllByRole('link', { name: 'Brooklyn' })
      .filter((a) => a.getAttribute('href') === '/region/brooklyn');
    expect(boroughLinks.length).toBeGreaterThan(0);
  });

  it('renders the website domain as an external link', async () => {
    getOrgMock.mockResolvedValueOnce(makeOrg());
    renderAt('/orgs/transalt');

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { level: 1, name: 'Transportation Alternatives' }),
      ).toBeDefined();
    });
    // The website domain appears as a link in the org-feature header
    // and again in the prose paragraph that points to the same URL.
    const links = screen
      .getAllByRole('link', { name: 'transalt.org' })
      .filter((a) => a.getAttribute('href') === 'https://www.transalt.org');
    expect(links.length).toBeGreaterThan(0);
    for (const link of links) {
      expect(link.getAttribute('target')).toBe('_blank');
      expect(link.getAttribute('rel')).toContain('noopener');
    }
  });

  // The isBrowseableKind helper in web/src/lib/regionKind.ts is the
  // single source of truth for which kinds render as /region/<slug>
  // links. The server resolves any non-national region by slug, so
  // every kind in the labels map qualifies (metros, cities, states,
  // counties, boroughs, multi-state coalitions, provinces, …).
  // These tests pin the set so a one-sided edit fails CI loudly.
  it('links ca:regional-district regions to /region/:slug', async () => {
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
      const links = screen
        .getAllByRole('link', { name: 'Metro Vancouver' })
        .filter((a) => a.getAttribute('href') === '/region/metro-vancouver');
      expect(links.length).toBeGreaterThan(0);
    });
  });

  it('links us:multi-state regions to /region/:slug', async () => {
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
      const links = screen
        .getAllByRole('link', { name: 'NYC Tri-State' })
        .filter((a) => a.getAttribute('href') === '/region/nyc-tristate');
      expect(links.length).toBeGreaterThan(0);
    });
  });

  it('renders a contact link when contact_url is present', async () => {
    getOrgMock.mockResolvedValueOnce(
      makeOrg({ contact_url: 'https://www.transalt.org/contact' }),
    );
    renderAt('/orgs/transalt');

    await waitFor(() => {
      // Contact URL lives in the meta-strip as an "open form →" link.
      // Find it by href since the visible label is generic.
      const contactLink = screen
        .getAllByRole('link')
        .find((a) => a.getAttribute('href') === 'https://www.transalt.org/contact');
      expect(contactLink).toBeDefined();
      expect(contactLink?.getAttribute('target')).toBe('_blank');
    });
  });

  it('renders the page breadcrumb as a navigation landmark with the org name as current page', async () => {
    getOrgMock.mockResolvedValueOnce(makeOrg());
    renderAt('/orgs/transalt');

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { level: 1, name: 'Transportation Alternatives' }),
      ).toBeDefined();
    });

    const nav = screen.getByRole('navigation', { name: /breadcrumb/i });
    const current = within(nav).getByText('Transportation Alternatives');
    expect(current.getAttribute('aria-current')).toBe('page');
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
    const browseLinks = screen
      .getAllByRole('link', { name: /browse/i })
      .filter((a) => a.getAttribute('href') === '/browse');
    expect(browseLinks.length).toBeGreaterThan(0);
  });

  it('sets the browser tab title to the org name on success', async () => {
    getOrgMock.mockResolvedValueOnce(makeOrg());
    renderAt('/orgs/transalt');

    await waitFor(() => {
      expect(document.title).toMatch(/transportation alternatives.*urbanist atlas/i);
    });
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
