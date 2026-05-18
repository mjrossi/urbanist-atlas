/**
 * Tests for the typed HTTP client. `fetch` is stubbed via `vi.stubGlobal`;
 * each test asserts the helper hits the right URL and parses the body
 * (or throws {@link ApiError} on a non-2xx response).
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, getMetro, listMetros, listRecent } from './api.ts';

describe('listMetros / getMetro / listRecent', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function jsonResponse(body: unknown): Response {
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  function problemResponse(status: number, body: unknown): Response {
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/problem+json' },
    });
  }

  it('listMetros calls GET /api/v1/metros and unwraps the envelope', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        meta: {
          license: 'ODbL-1.0',
          attribution_url: 'https://urbanistatlas.com',
          generated_at: '2026-05-18T12:00:00Z',
        },
        data: [],
      }),
    );
    const result = await listMetros();
    expect(result).toEqual([]);
    const [url] = fetchMock.mock.calls[0]!;
    expect(String(url)).toContain('/api/v1/metros');
  });

  it('listMetros returns the unwrapped data array on a non-empty body', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        meta: {
          license: 'ODbL-1.0',
          attribution_url: 'https://urbanistatlas.com',
          generated_at: '2026-05-18T12:00:00Z',
        },
        data: [
          {
            region: {
              id: 1,
              kind: 'us:metro',
              name: 'New York Metro',
              slug: 'nyc-metro',
              country: 'US',
              scope_tier: 'regional',
              parent_slugs: [],
            },
            org_count: 7,
          },
        ],
      }),
    );
    const result = await listMetros();
    expect(result).toHaveLength(1);
    expect(result[0]!.region.slug).toBe('nyc-metro');
    expect(result[0]!.org_count).toBe(7);
  });

  it('getMetro calls GET /api/v1/metros/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        region: {
          id: 1,
          kind: 'us:metro',
          name: 'New York Metro',
          slug: 'nyc-metro',
          country: 'US',
          scope_tier: 'regional',
          parent_slugs: [],
        },
        orgs: [],
      }),
    );
    const result = await getMetro('nyc-metro');
    expect(result.region.slug).toBe('nyc-metro');
    const [url] = fetchMock.mock.calls[0]!;
    expect(String(url)).toContain('/api/v1/metros/nyc-metro');
  });

  it('getMetro throws ApiError with status 404 when the API returns problem+json', async () => {
    fetchMock.mockResolvedValueOnce(
      problemResponse(404, {
        type: 'https://urbanistatlas.com/problems/not-found',
        title: 'Not Found',
        status: 404,
      }),
    );
    await expect(getMetro('totally-fake')).rejects.toBeInstanceOf(ApiError);
  });

  it('listRecent calls GET /api/v1/recent and unwraps the envelope', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        meta: {
          license: 'ODbL-1.0',
          attribution_url: 'https://urbanistatlas.com',
          generated_at: '2026-05-18T12:00:00Z',
        },
        data: [],
      }),
    );
    const result = await listRecent();
    expect(result).toEqual([]);
    const [url] = fetchMock.mock.calls[0]!;
    expect(String(url)).toContain('/api/v1/recent');
  });

  it('listRecent returns the unwrapped data array on a non-empty body', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        meta: {
          license: 'ODbL-1.0',
          attribution_url: 'https://urbanistatlas.com',
          generated_at: '2026-05-18T12:00:00Z',
        },
        data: [
          {
            id: 1,
            slug: 'transalt',
            name: 'Transportation Alternatives',
            short_desc: 'NYC advocacy',
            website_url: 'https://transalt.org',
            tags: ['transit'],
            regions: [
              {
                id: 1,
                kind: 'us:metro',
                name: 'New York Metro',
                slug: 'nyc-metro',
                country: 'US',
                scope_tier: 'regional',
                parent_slugs: [],
              },
            ],
          },
        ],
      }),
    );
    const result = await listRecent();
    expect(result).toHaveLength(1);
    expect(result[0]!.slug).toBe('transalt');
  });

  it('getMetro percent-encodes the slug', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        region: {
          id: 1,
          kind: 'pt:area-metropolitana',
          name: 'Área Metropolitana de Lisboa',
          slug: 'aml',
          country: 'PT',
          scope_tier: 'regional',
          parent_slugs: [],
        },
        orgs: [],
      }),
    );
    await getMetro('weird slug');
    const [url] = fetchMock.mock.calls[0]!;
    // Either '+' or '%20' is acceptable; the point is the space isn't
    // literally embedded in the URL path.
    expect(String(url)).not.toContain('weird slug');
    expect(String(url)).toMatch(/weird(%20|\+)slug/);
  });
});
