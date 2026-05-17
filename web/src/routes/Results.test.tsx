import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { LookupResult } from '../lib/api.ts';
import { ApiError } from '../lib/api.ts';

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
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/r/:postalCode" element={<Results />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
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
    expect(screen.getByRole('status').textContent).toMatch(/looking up groups/i);
  });

  it('renders the dateline and grouped entries on success', async () => {
    lookupMock.mockResolvedValueOnce(makeResult());
    renderAt('/r/11217?country=US');

    await waitFor(() => {
      expect(
        screen.getByRole('link', { name: 'Transportation Alternatives' }),
      ).toBeDefined();
    });
    expect(screen.getByText('Brooklyn, NY')).toBeDefined();
    expect(screen.getByText('11217')).toBeDefined();
    expect(screen.getByRole('link', { name: 'Riders Alliance' })).toBeDefined();
    expect(screen.getByText('Local')).toBeDefined();
    expect(screen.getByText('Regional')).toBeDefined();
  });

  it('renders the empty prose when both tiers come back empty', async () => {
    lookupMock.mockResolvedValueOnce(makeResult({ local: [], regional: [] }));
    renderAt('/r/99999?country=US');

    await waitFor(() => {
      expect(screen.getByText(/no groups indexed yet for 99999/i)).toBeDefined();
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

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      expect(alert.textContent).toContain('Database is on fire');
      expect(alert.textContent).toContain('req-abc-123');
    });
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
});
