import { screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { LookupResult } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';
import { renderWithProviders } from '../test/renderWithProviders.tsx';

const { lookupMock } = vi.hoisted(() => ({ lookupMock: vi.fn() }));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return {
    ...actual,
    lookup: lookupMock,
  };
});

// Import Results AFTER the mock is registered so it picks up our stub.
const { Results } = await import('./Results.tsx');

function renderAt(path: string) {
  return renderWithProviders(<Results />, {
    initialEntries: [path],
    routePath: '/r/:postalCode',
  });
}

function makeResult(overrides: Partial<LookupResult> = {}): LookupResult {
  return {
    query: { postal_code: '11217', country: 'US' },
    resolved_place_label: 'Brooklyn, NY',
    resolved_ancestry: [],
    local: [
      {
        id: 1,
        slug: 'transalt',
        name: 'Transportation Alternatives',
        short_desc: 'NYC-wide advocacy.',
        website_url: 'https://www.transalt.org',
        tags: ['transit'],
        regions: [],
        matched_region_slugs: [],
        added_at: '2026-05-17',
      },
    ],
    regional: [
      {
        id: 2,
        slug: 'riders-alliance',
        name: 'Riders Alliance',
        short_desc: 'NY State transit riders.',
        website_url: 'https://www.ridersny.org',
        tags: ['transit', 'policy'],
        regions: [],
        matched_region_slugs: [],
        added_at: '2026-05-17',
      },
    ],
    statewide: [
      {
        id: 3,
        slug: 'ny-lcv',
        name: 'NY League of Conservation Voters',
        short_desc: 'Statewide transportation policy.',
        website_url: 'https://www.nylcv.org',
        tags: ['policy'],
        regions: [],
        matched_region_slugs: [],
        added_at: '2026-05-17',
      },
    ],
    ...overrides,
  };
}

describe('Results', () => {
  beforeEach(() => {
    lookupMock.mockReset();
  });

  afterEach(() => {
    lookupMock.mockReset();
  });

  it('renders the loading state while the query is pending', () => {
    lookupMock.mockReturnValue(new Promise(() => {}));
    renderAt('/r/11217?country=US');
    expect(screen.getByRole('status').textContent).toMatch(/finding organizations/i);
  });

  it('renders the dateline and grouped entries on success', async () => {
    lookupMock.mockResolvedValueOnce(makeResult());
    renderAt('/r/11217?country=US');

    await waitFor(() => {
      expect(
        screen.getByRole('link', { name: 'Transportation Alternatives' }),
      ).toBeDefined();
    });
    // Place label appears in the deck copy.
    expect(screen.getByText(/Brooklyn, NY/)).toBeDefined();
    // Postal code is the h1.
    const h1 = screen.getByRole('heading', { level: 1 });
    expect(h1.textContent).toMatch(/11217/);
    expect(screen.getByRole('link', { name: 'Riders Alliance' })).toBeDefined();
    expect(
      screen.getByRole('link', { name: 'NY League of Conservation Voters' }),
    ).toBeDefined();
    // Local + Regional + State / Provincial section h2s.
    const h2s = screen.getAllByRole('heading', { level: 2 });
    const h2Text = h2s.map((h) => h.textContent).join(' | ');
    expect(h2Text).toMatch(/local/i);
    expect(h2Text).toMatch(/regional/i);
    expect(h2Text).toMatch(/state \/ provincial/i);
  });

  it('renders the empty prose when all three tiers come back empty', async () => {
    lookupMock.mockResolvedValueOnce(
      makeResult({ local: [], regional: [], statewide: [] }),
    );
    renderAt('/r/99999?country=US');

    await waitFor(() => {
      // Empty deck mentions the resolved place label and the
      // map-fills-in framing.
      expect(screen.getByText(/no entries for brooklyn, ny yet/i)).toBeDefined();
    });
  });

  it('renders the ApiError message and request id on failure', async () => {
    lookupMock.mockRejectedValueOnce(
      new ApiError(
        500,
        'Database is on fire',
        { type: 'about:blank', title: 'Database is on fire', status: 500 },
        'req-abc-123',
      ),
    );
    renderAt('/r/11217?country=US');

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('Database is on fire');
    expect(alert.textContent).toContain('req-abc-123');
  });

  it('renders the backend not-found copy in a friendly card with a link back (not an alert)', async () => {
    lookupMock.mockRejectedValueOnce(
      new ApiError(
        404,
        'Postal Code Not Found',
        {
          type: 'https://urbanistatlas.com/problems/not-found',
          title: 'Postal Code Not Found',
          detail:
            'No region is mapped to that postal code. Try a nearby code, or file a tip if you know an organization there.',
          status: 404,
        },
        'req-nf-1',
      ),
    );
    renderAt('/r/00000?country=US');

    // The card renders the server-supplied detail verbatim as the body —
    // the frontend doesn't re-author the sentence. The label is a fixed
    // small-caps eyebrow (not the backend problem title).
    await waitFor(() => {
      expect(screen.getByText(/No region is mapped to that postal code/i)).toBeDefined();
    });
    expect(screen.getByText('No match for that postal code')).toBeDefined();
    // Friendly, not a red error: no alert role, and a link back to the
    // lookup is offered as navigation chrome.
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.getByRole('link', { name: /another code/i })).toBeDefined();
  });

  it('renders the backend military-ZIP copy without the frontend knowing the type', async () => {
    lookupMock.mockRejectedValueOnce(
      new ApiError(
        404,
        'Military or Diplomatic ZIP Code',
        {
          type: 'https://urbanistatlas.com/problems/military-postal-code',
          title: 'Military or Diplomatic ZIP Code',
          detail:
            "APO, FPO, and DPO ZIP codes are military and diplomatic addresses that aren't tied to a local region. Enter a residential ZIP code to find organizations near you.",
          status: 404,
        },
        'req-mil-1',
      ),
    );
    renderAt('/r/09000?country=US');

    // Same 404 code path as not-found; only the server's copy differs.
    await waitFor(() => {
      expect(screen.getByText(/APO, FPO, and DPO ZIP codes/i)).toBeDefined();
    });
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.getByText(/Enter a residential ZIP code/i)).toBeDefined();
  });

  it('falls back to a generic card body on a 404 with no problem body', async () => {
    lookupMock.mockRejectedValueOnce(
      new ApiError(404, 'Not Found', undefined, 'req-nf-2'),
    );
    renderAt('/r/00000?country=US');

    // No problem+json body (e.g. a proxy-injected error page): the card
    // still renders friendly chrome — the fixed eyebrow, a generic body
    // naming the code, and the nav link — without an alert role.
    await waitFor(() => {
      expect(screen.getByText('No match for that postal code')).toBeDefined();
    });
    expect(screen.getByText(/couldn.t find a match for 00000/i)).toBeDefined();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.getByRole('link', { name: /another code/i })).toBeDefined();
  });

  it('defaults country to US when the search param is missing', async () => {
    lookupMock.mockResolvedValueOnce(makeResult({ local: [], regional: [] }));
    renderAt('/r/11217');

    await waitFor(() => {
      expect(lookupMock).toHaveBeenCalledWith('11217', 'US', expect.any(Object));
    });
  });

  it('passes country=CA through to the lookup call', async () => {
    lookupMock.mockResolvedValueOnce(makeResult({ local: [], regional: [] }));
    renderAt('/r/M5V?country=CA');

    await waitFor(() => {
      expect(lookupMock).toHaveBeenCalledWith('M5V', 'CA', expect.any(Object));
    });
  });

  it('renders an unsupported-country error without calling the API', () => {
    renderAt('/r/11217?country=DE');

    const alert = screen.getByRole('alert');
    expect(alert.textContent).toMatch(/country.*DE.*isn.t supported/i);
    expect(lookupMock).not.toHaveBeenCalled();
  });

  it('sets the browser tab title to the postal code', async () => {
    lookupMock.mockReturnValue(new Promise(() => {}));
    renderAt('/r/11217?country=US');
    await waitFor(() => {
      expect(document.title).toMatch(/11217.*urbanist atlas/i);
    });
  });

  it('adds a noindex,follow robots meta tag while mounted', async () => {
    lookupMock.mockReturnValue(new Promise(() => {}));
    const { unmount } = renderAt('/r/11217?country=US');
    await waitFor(() => {
      const meta = document.head.querySelector('meta[name="robots"]');
      expect(meta?.getAttribute('content')).toBe('noindex,follow');
    });
    unmount();
    expect(document.head.querySelector('meta[name="robots"]')).toBeNull();
  });
});
