/**
 * Tests for the typed HTTP client. `fetch` is stubbed via `vi.stubGlobal`;
 * each test asserts the helper hits the right URL and parses the body
 * (or throws {@link ApiError} on a non-2xx response).
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ApiError,
  apiFetch,
  getMetro,
  isSupportedCountry,
  listMetros,
  listRecent,
  lookup,
} from './api.ts';

function jsonResponse(
  body: unknown,
  init?: { headers?: Record<string, string> },
): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });
}

function problemResponse(
  status: number,
  body: unknown,
  init?: { headers?: Record<string, string> },
): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/problem+json', ...init?.headers },
  });
}

describe('listMetros / getMetro / listRecent', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

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
    // Tighter than `toContain('/api/v1/metros')` so it can't
    // accidentally match a detail route like `/api/v1/metros/some-slug`.
    expect(String(url)).toMatch(/\/api\/v1\/metros($|\?)/);
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
    expect(String(url)).toMatch(/\/api\/v1\/metros\/nyc-metro($|\?)/);
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
    expect(String(url)).toMatch(/\/api\/v1\/recent($|\?)/);
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

describe('lookup()', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function lookupBody() {
    return {
      query: { postal_code: '11217', country: 'US' },
      resolved_place_label: 'Brooklyn',
      resolved_ancestry: [],
      local: [],
      regional: [],
    };
  }

  it('builds the URL with postal_code and country query params', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(lookupBody()));
    await lookup('11217', 'US');
    const [url] = fetchMock.mock.calls[0]!;
    const s = String(url);
    expect(s).toContain('/api/v1/lookup?');
    expect(s).toContain('postal_code=11217');
    expect(s).toContain('country=US');
  });

  it('returns the parsed LookupResult on 200', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        query: { postal_code: '11217', country: 'US' },
        resolved_place_label: 'Brooklyn — New York Metro',
        resolved_ancestry: [],
        local: [],
        regional: [],
      }),
    );
    const result = await lookup('11217', 'US');
    expect(result.query.postal_code).toBe('11217');
    expect(result.query.country).toBe('US');
    expect(result.resolved_place_label).toBe('Brooklyn — New York Metro');
    expect(result.local).toEqual([]);
    expect(result.regional).toEqual([]);
  });

  it('throws ApiError with .status === 404 on a 404 problem+json body', async () => {
    fetchMock.mockResolvedValueOnce(
      problemResponse(404, {
        type: 'https://urbanistatlas.com/problems/not-found',
        title: 'Not Found',
        status: 404,
        detail: 'No region found for postal code 99999.',
      }),
    );
    // try/catch + caught: a chained `.catch` would swallow any inner
    // `expect` throw and let the test pass silently. The pattern below
    // mirrors the rest of the suite (see ApiError tests at L353+).
    let caught: unknown;
    try {
      await lookup('99999', 'US');
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    const apiErr = caught as ApiError;
    expect(apiErr.status).toBe(404);
  });

  it('URL-encodes a CA postal code that contains a space', async () => {
    // URLSearchParams encodes a space as '+'. A future refactor that
    // switched to encodeURIComponent (which uses '%20') would break
    // the wire contract that the API parses.
    fetchMock.mockResolvedValueOnce(jsonResponse(lookupBody()));
    await lookup('M5V 3A8', 'CA');
    const [url] = fetchMock.mock.calls[0]!;
    const s = String(url);
    expect(s).toContain('postal_code=M5V+3A8');
    // Belt-and-suspenders: the literal space must not appear.
    expect(s).not.toContain('M5V 3A8');
  });
});

describe('apiFetch — happy path', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sets Accept: application/json on the outgoing request', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
    await apiFetch('/api/v1/anything');
    const [, init] = fetchMock.mock.calls[0]!;
    const headers = new Headers((init as RequestInit).headers);
    expect(headers.get('Accept')).toBe('application/json');
  });

  it('prepends apiBase to relative paths', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
    await apiFetch('/api/v1/anything');
    const [url] = fetchMock.mock.calls[0]!;
    // Default apiBase is http://localhost:8080 in tests (no
    // VITE_API_BASE set). Strict equality pins both the scheme/host
    // (so a regression that quietly changes DEFAULT_API_BASE is caught)
    // and the prepended path.
    expect(String(url)).toBe('http://localhost:8080/api/v1/anything');
  });

  it('leaves absolute URLs un-prepended', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
    await apiFetch('https://example.com/somewhere');
    const [url] = fetchMock.mock.calls[0]!;
    expect(String(url)).toBe('https://example.com/somewhere');
  });

  it('returns the parsed body as T', async () => {
    interface Shape {
      hello: string;
      n: number;
    }
    fetchMock.mockResolvedValueOnce(jsonResponse({ hello: 'world', n: 42 }));
    const got = await apiFetch<Shape>('/api/v1/anything');
    expect(got).toEqual({ hello: 'world', n: 42 });
  });

  it('does not overwrite a caller-provided Accept header', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
    await apiFetch('/api/v1/anything', {
      headers: { Accept: 'application/foo' },
    });
    const [, init] = fetchMock.mock.calls[0]!;
    const headers = new Headers((init as RequestInit).headers);
    expect(headers.get('Accept')).toBe('application/foo');
  });
});

describe('ApiError — rich shape from problem+json', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('populates status, problem fields, and requestId from response', async () => {
    fetchMock.mockResolvedValueOnce(
      problemResponse(
        400,
        {
          type: 'https://urbanistatlas.com/problems/validation',
          title: 'Bad Request',
          status: 400,
          detail: 'postal_code is required',
        },
        { headers: { 'X-Request-ID': 'req-abc-123' } },
      ),
    );

    let caught: unknown;
    try {
      await apiFetch('/api/v1/lookup');
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    const apiErr = caught as ApiError;
    expect(apiErr.status).toBe(400);
    expect(apiErr.problem).toBeDefined();
    expect(apiErr.problem?.type).toBe('https://urbanistatlas.com/problems/validation');
    expect(apiErr.problem?.title).toBe('Bad Request');
    expect(apiErr.problem?.detail).toBe('postal_code is required');
    expect(apiErr.requestId).toBe('req-abc-123');
  });

  it('uses problem.title as the Error message when present', async () => {
    fetchMock.mockResolvedValueOnce(
      problemResponse(400, {
        type: 'https://urbanistatlas.com/problems/validation',
        title: 'Bad Request',
        status: 400,
        detail: 'postal_code is required',
      }),
    );
    let caught: unknown;
    try {
      await apiFetch('/api/v1/lookup');
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).message).toBe('Bad Request');
  });

  it('falls back to problem.detail when title is absent', async () => {
    // Note: the fallback chain uses `??`, so it only triggers on
    // undefined/null — an explicit empty string `title: ''` would
    // win as the message. The wire shape (api.gen.ts) marks `title`
    // as required, so in practice the server always sends one; this
    // test pins the defensive fallback that triggers when the body
    // omits it entirely (e.g. proxy mangles JSON, future spec lets
    // it become optional).
    fetchMock.mockResolvedValueOnce(
      problemResponse(400, {
        type: 'https://urbanistatlas.com/problems/validation',
        status: 400,
        detail: 'postal_code is required',
      }),
    );
    let caught: unknown;
    try {
      await apiFetch('/api/v1/lookup');
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).message).toBe('postal_code is required');
  });

  it('falls back to `HTTP <status>` when title and detail are both absent', async () => {
    fetchMock.mockResolvedValueOnce(
      problemResponse(418, {
        type: 'https://urbanistatlas.com/problems/teapot',
        status: 418,
      }),
    );
    let caught: unknown;
    try {
      await apiFetch('/api/v1/teapot');
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).message).toBe('HTTP 418');
  });

  it('preserves the exact unauthorized problem URI on a 401 (slice #23)', async () => {
    // Pins the wire shape the backend's X-Atlas-Client middleware
    // emits when the shared-secret header is missing/wrong. The exact
    // string must stay in lockstep with `problemUnauthorized` in
    // api/internal/httpapi/problem.go.
    fetchMock.mockResolvedValueOnce(
      problemResponse(401, {
        type: 'https://urbanistatlas.com/problems/unauthorized',
        title: 'Unauthorized',
        status: 401,
        // Exact server-emitted string — matches the `writeProblem`
        // call in api/internal/httpapi/clientsecret.go:31-33
        // (lowercase, no trailing period). A future reader copying
        // this fixture should see what the wire actually carries.
        detail: 'missing or invalid X-Atlas-Client header',
      }),
    );
    let caught: unknown;
    try {
      await apiFetch('/api/v1/lookup?postal_code=11217&country=US');
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    const apiErr = caught as ApiError;
    expect(apiErr.status).toBe(401);
    expect(apiErr.problem?.type).toBe(
      'https://urbanistatlas.com/problems/unauthorized',
    );
    expect(apiErr.problem?.title).toBe('Unauthorized');
    // Pin the detail too so the fixture is actually load-bearing —
    // a future drift in the server-emitted string will fail here.
    expect(apiErr.problem?.detail).toBe(
      'missing or invalid X-Atlas-Client header',
    );
  });

  it('non-2xx with non-problem body: problem is undefined, message is `HTTP <status>`', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response('something went wrong', {
        status: 500,
        headers: { 'Content-Type': 'text/plain' },
      }),
    );
    let caught: unknown;
    try {
      await apiFetch('/api/v1/anything');
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    const apiErr = caught as ApiError;
    expect(apiErr.status).toBe(500);
    expect(apiErr.problem).toBeUndefined();
    expect(apiErr.message).toBe('HTTP 500');
  });

  it('4xx with malformed problem+json body still throws ApiError with HTTP fallback', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response('not { valid json ::', {
        status: 422,
        headers: { 'Content-Type': 'application/problem+json' },
      }),
    );
    let caught: unknown;
    try {
      await apiFetch('/api/v1/anything');
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    const apiErr = caught as ApiError;
    expect(apiErr.status).toBe(422);
    expect(apiErr.problem).toBeUndefined();
    expect(apiErr.message).toBe('HTTP 422');
  });
});

describe('X-Atlas-Client header injection (env-gated)', () => {
  // `apiClientSecret` is captured at module-load time from
  // `import.meta.env.VITE_API_CLIENT_SECRET`, so each test resets the
  // module registry, stubs the env, then dynamically re-imports `./api.ts`
  // so the captured value reflects the stub. Without resetModules the
  // const in `api.ts` would still hold whatever value was visible the
  // first time the file was imported (when the test suite loaded).
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.resetModules();
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
  });

  function lookupBody() {
    return {
      query: { postal_code: '11217', country: 'US' },
      resolved_place_label: 'Brooklyn',
      resolved_ancestry: [],
      local: [],
      regional: [],
    };
  }

  it('injects X-Atlas-Client when VITE_API_CLIENT_SECRET is set', async () => {
    vi.stubEnv('VITE_API_CLIENT_SECRET', 'the-secret');
    const { lookup } = await import('./api.ts');
    fetchMock.mockResolvedValueOnce(jsonResponse(lookupBody()));
    await lookup('11217', 'US');
    const [, init] = fetchMock.mock.calls[0]!;
    const headers = new Headers((init as RequestInit).headers);
    expect(headers.get('X-Atlas-Client')).toBe('the-secret');
    expect(headers.get('Accept')).toBe('application/json');
  });

  it('does NOT inject X-Atlas-Client when env var is empty', async () => {
    vi.stubEnv('VITE_API_CLIENT_SECRET', '');
    const { lookup } = await import('./api.ts');
    fetchMock.mockResolvedValueOnce(jsonResponse(lookupBody()));
    await lookup('11217', 'US');
    const [, init] = fetchMock.mock.calls[0]!;
    const headers = new Headers((init as RequestInit).headers);
    expect(headers.has('X-Atlas-Client')).toBe(false);
  });

  it('does NOT inject X-Atlas-Client when env var is unset', async () => {
    // Production has two distinct shapes for "no secret":
    //   1. VITE_API_CLIENT_SECRET= in .env (empty string — covered above)
    //   2. The var omitted entirely from .env (undefined — covered here)
    // The `?? ''` fallback at api.ts:64 collapses both to empty, so the
    // module-level apiClientSecret is '' in either case. This test pins
    // the second branch — if the fallback ever disappears, only the
    // unset case would break.
    vi.stubEnv('VITE_API_CLIENT_SECRET', undefined);
    const { lookup } = await import('./api.ts');
    fetchMock.mockResolvedValueOnce(jsonResponse(lookupBody()));
    await lookup('11217', 'US');
    const [, init] = fetchMock.mock.calls[0]!;
    const headers = new Headers((init as RequestInit).headers);
    expect(headers.has('X-Atlas-Client')).toBe(false);
  });

  it('does NOT overwrite caller-provided X-Atlas-Client header', async () => {
    vi.stubEnv('VITE_API_CLIENT_SECRET', 'env-secret');
    const { apiFetch } = await import('./api.ts');
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
    await apiFetch('/api/v1/something', {
      headers: { 'X-Atlas-Client': 'caller-override' },
    });
    const [, init] = fetchMock.mock.calls[0]!;
    const headers = new Headers((init as RequestInit).headers);
    expect(headers.get('X-Atlas-Client')).toBe('caller-override');
  });
});

describe('isSupportedCountry', () => {
  // The wire `Country` type is open (the API accepts any code the seed
  // data defines), but the UI gates on a narrower set so a stray
  // `?country=…` query renders an error instead of silently coercing.
  // Keep this in lockstep with SUPPORTED_COUNTRIES in api.ts.
  it('returns true for currently-shipping countries', () => {
    expect(isSupportedCountry('US')).toBe(true);
    expect(isSupportedCountry('CA')).toBe(true);
  });

  it('returns false for unsupported codes', () => {
    expect(isSupportedCountry('GB')).toBe(false);
    expect(isSupportedCountry('PT')).toBe(false);
    expect(isSupportedCountry('')).toBe(false);
  });

  it('is case-sensitive — lowercase does not match', () => {
    // SUPPORTED_COUNTRIES is uppercase. Lowercase must NOT coerce,
    // because the API expects uppercase ISO codes and a silent
    // coercion here would mask a UI bug.
    expect(isSupportedCountry('us')).toBe(false);
    expect(isSupportedCountry('ca')).toBe(false);
  });
});
