import { screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { Stats } from '../lib/api.ts';
import { renderWithProviders } from '../test/renderWithProviders.tsx';

const { getStatsMock, listRegionsMock } = vi.hoisted(() => ({
  getStatsMock: vi.fn(),
  listRegionsMock: vi.fn(),
}));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return { ...actual, getStats: getStatsMock, listRegions: listRegionsMock };
});

const { BroadsheetNav } = await import('./BroadsheetNav.tsx');

function makeStats(overrides: Partial<Stats> = {}): Stats {
  return {
    total_org_count: 236,
    total_region_count: 628,
    browse_region_count: 92,
    by_country: [
      { country: 'CA', org_count: 40, region_count: 21 },
      { country: 'US', org_count: 196, region_count: 71 },
    ],
    ...overrides,
  };
}

describe('BroadsheetNav', () => {
  beforeEach(() => {
    getStatsMock.mockReset();
    listRegionsMock.mockReset();
    getStatsMock.mockResolvedValue(makeStats());
    listRegionsMock.mockResolvedValue([]);
  });

  afterEach(() => {
    getStatsMock.mockReset();
    listRegionsMock.mockReset();
  });

  it('marks the matching nav entry with aria-current="page"', () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/about'] });
    const aboutLink = screen.getByRole('link', { name: /about/i });
    expect(aboutLink.getAttribute('aria-current')).toBe('page');
  });

  it('leaves non-active entries without aria-current', () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/about'] });
    const submitLink = screen.getByRole('link', { name: /submit/i });
    expect(submitLink.getAttribute('aria-current')).toBeNull();
  });

  it('treats /browse, /region/*, /orgs/*, and /r/* as the Browse section', () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/region/seattle'] });
    const browseLink = screen.getByRole('link', { name: /browse/i });
    expect(browseLink.getAttribute('aria-current')).toBe('page');
  });

  it('keeps the active entry as a Link so it stays keyboard-focusable', () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/about'] });
    // Active entries used to render as a <span onClick> which had no
    // tab stop and announced no current-page state. Asserting role=link
    // pins down that fix.
    expect(screen.getByRole('link', { name: /about/i })).toBeDefined();
  });

  // The masthead tally renders on every route and previously had no
  // test at all — which is how it shipped showing 166 orgs against a
  // 236-org catalog. It used to sum direct_org_count over
  // /api/v1/regions, dropping every org attached solely to a state,
  // province, borough, or multi-state region.
  it('renders the server-side org and place totals', async () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/about'] });
    await waitFor(() => {
      expect(screen.getByText(/236 orgs/)).toBeDefined();
    });
    expect(screen.getByText(/92 places/)).toBeDefined();
  });

  it('reads the totals from /stats, never from the region list', async () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/about'] });
    await waitFor(() => {
      expect(getStatsMock).toHaveBeenCalled();
    });
    // Two claims in one: the tally can't be re-derived from the browse
    // subset (the bug), and a route rendering no region list no longer
    // pulls the whole browse set just to print two numbers.
    expect(listRegionsMock).not.toHaveBeenCalled();
  });

  it('omits the tally entirely while stats are unavailable', () => {
    getStatsMock.mockReturnValue(new Promise(() => {}));
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/about'] });
    expect(screen.queryByText(/orgs ·/)).toBeNull();
  });
});
