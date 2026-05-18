/**
 * Tests for the typed HTTP client.
 *
 * Two flavours of test:
 *
 * 1. Fixture-mode tests (`describe('with VITE_USE_FIXTURES=true', ...)`):
 *    stub `import.meta.env.VITE_USE_FIXTURES` via `vi.stubEnv` and
 *    confirm `listMetros`/`getMetro`/`listRecent` short-circuit to
 *    the fixture module rather than calling `fetch`.
 * 2. Network-mode tests (`describe('with VITE_USE_FIXTURES unset', ...)`):
 *    stub `globalThis.fetch` and confirm the helpers hit the right URLs
 *    and parse the JSON body.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, getMetro, listMetros, listRecent } from './api.ts';

describe('listMetros / getMetro / listRecent', () => {
  describe('with VITE_USE_FIXTURES=true', () => {
    beforeEach(() => {
      vi.stubEnv('VITE_USE_FIXTURES', 'true');
    });

    afterEach(() => {
      vi.unstubAllEnvs();
    });

    it('listMetros returns the fixture metros ordered descending by org_count', async () => {
      const metros = await listMetros();
      expect(metros.length).toBeGreaterThan(0);
      // The fixture file is already sorted; assert the ordering is
      // preserved end-to-end (regression guard for accidental reordering
      // in the fixture-mode branch).
      for (let i = 1; i < metros.length; i++) {
        expect(metros[i - 1].org_count).toBeGreaterThanOrEqual(metros[i].org_count);
      }
    });

    it('getMetro returns the fixture detail for a known slug', async () => {
      const detail = await getMetro('nyc-metro');
      expect(detail.region.slug).toBe('nyc-metro');
      expect(detail.orgs.length).toBeGreaterThan(0);
    });

    it('getMetro throws ApiError with status 404 for an unknown slug', async () => {
      await expect(getMetro('totally-fake')).rejects.toBeInstanceOf(ApiError);
      try {
        await getMetro('totally-fake');
      } catch (err) {
        expect(err).toBeInstanceOf(ApiError);
        expect((err as ApiError).status).toBe(404);
      }
    });

    it('listRecent returns at most 10 entries (one screenful)', async () => {
      const recent = await listRecent();
      expect(recent.length).toBeGreaterThan(0);
      expect(recent.length).toBeLessThanOrEqual(10);
    });
  });

  describe('with VITE_USE_FIXTURES unset', () => {
    let fetchMock: ReturnType<typeof vi.fn>;

    beforeEach(() => {
      vi.stubEnv('VITE_USE_FIXTURES', '');
      fetchMock = vi.fn();
      vi.stubGlobal('fetch', fetchMock);
    });

    afterEach(() => {
      vi.unstubAllEnvs();
      vi.unstubAllGlobals();
    });

    function jsonResponse(body: unknown): Response {
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }

    it('listMetros calls GET /api/v1/metros and returns the parsed body', async () => {
      fetchMock.mockResolvedValueOnce(jsonResponse([]));
      const result = await listMetros();
      expect(result).toEqual([]);
      const [url] = fetchMock.mock.calls[0];
      expect(String(url)).toContain('/api/v1/metros');
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
      const [url] = fetchMock.mock.calls[0];
      expect(String(url)).toContain('/api/v1/metros/nyc-metro');
    });

    it('listRecent calls GET /api/v1/recent', async () => {
      fetchMock.mockResolvedValueOnce(jsonResponse([]));
      const result = await listRecent();
      expect(result).toEqual([]);
      const [url] = fetchMock.mock.calls[0];
      expect(String(url)).toContain('/api/v1/recent');
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
      const [url] = fetchMock.mock.calls[0];
      // Either '+' or '%20' is acceptable; the point is the space isn't
      // literally embedded in the URL path.
      expect(String(url)).not.toContain('weird slug');
      expect(String(url)).toMatch(/weird(%20|\+)slug/);
    });
  });
});
