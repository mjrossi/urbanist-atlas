/**
 * Typed HTTP client for the Urbanist Atlas API.
 *
 * Every wire shape comes from `./api.gen.ts`, generated from
 * `../api/openapi.yaml`. Don't hand-roll types here: add them to the
 * spec and run `npm run generate:api`.
 */
import type { components } from './api.gen.ts';

export type Country = components['schemas']['Country'];
export type Region = components['schemas']['Region'];
export type Org = components['schemas']['Org'];
export type LookupOrg = components['schemas']['LookupOrg'];
export type LookupQuery = components['schemas']['LookupQuery'];
export type LookupResult = components['schemas']['LookupResult'];
export type RegionSummary = components['schemas']['RegionSummary'];
export type RegionSearchResult = components['schemas']['RegionSearchResult'];
export type RegionSearchResultsEnvelope =
  components['schemas']['RegionSearchResultsEnvelope'];
export type RegionDetail = components['schemas']['RegionDetail'];
export type Meta = components['schemas']['Meta'];
export type RegionSummariesEnvelope = components['schemas']['RegionSummariesEnvelope'];
export type RecentEnvelope = components['schemas']['RecentEnvelope'];
export type Stats = components['schemas']['Stats'];
export type CountryStats = components['schemas']['CountryStats'];
export type Submission = components['schemas']['Submission'];
export type SubmissionPayload = components['schemas']['SubmissionPayload'];
export type NewSubmissionRequest = components['schemas']['NewSubmissionRequest'];
export type ProblemDetails = components['schemas']['ProblemDetails'];

/**
 * A country the SPA exposes to users. The wire {@link Country} is an
 * open `string` (the API accepts any code the seed data defines), so
 * the UI gates on this narrower set and a stray `?country=…` renders
 * an error instead of silently coercing.
 */
export type SupportedCountry = 'US' | 'CA';

export const SUPPORTED_COUNTRIES: readonly SupportedCountry[] = ['US', 'CA'];

/** True if `raw` is one of {@link SUPPORTED_COUNTRIES}. */
export function isSupportedCountry(raw: string): raw is SupportedCountry {
  return (SUPPORTED_COUNTRIES as readonly string[]).includes(raw);
}

const DEFAULT_API_BASE = 'http://localhost:8080';

function resolveApiBase(): string {
  const raw = import.meta.env.VITE_API_BASE;
  if (raw !== undefined && raw !== '') return raw.replace(/\/+$/, '');
  // The CSP's connect-src does not allow localhost, so a production
  // bundle on the fallback would fail every request with no clear cause.
  if (import.meta.env.PROD) {
    throw new Error(
      'VITE_API_BASE is not set. Production builds must set it to the deployed API origin (see docs/deploy.md).',
    );
  }
  return DEFAULT_API_BASE;
}

/**
 * Where the API lives, from `VITE_API_BASE` with any trailing slash
 * stripped so `${apiBase}/api/v1/…` never doubles up. Throws at module
 * load in a production build that leaves it unset or empty.
 */
export const apiBase: string = resolveApiBase();

/**
 * URL of the served OpenAPI document, derived from {@link apiBase} so a
 * QA build links the QA API rather than production.
 */
export const openapiUrl = `${apiBase}/api/v1/openapi.yaml`;

/**
 * The Phase 1 `X-Atlas-Client` shared secret (CLAUDE.md § Launch
 * strategy). Empty, as in local dev, means the header isn't sent.
 */
const apiClientSecret: string = import.meta.env.VITE_API_CLIENT_SECRET ?? '';

/** Per-field validation messages from the problem+json `errors` extension. */
export type FieldErrors = Readonly<Record<string, string>>;

/**
 * Error thrown for any failed API request. `status` is the HTTP status
 * of a non-2xx response, or `0` when no response arrived (network
 * failure, CORS/CSP block, or the request timed out); the original
 * error is then kept as `cause`. `problem` is the parsed RFC 9457
 * document when the server sent one.
 *
 * `retryAfterSeconds` is the parsed `Retry-After` on a 429;
 * `fieldErrors` is the `errors` extension when present and non-empty.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly problem: ProblemDetails | undefined;
  readonly requestId: string | undefined;
  readonly retryAfterSeconds: number | undefined;
  readonly fieldErrors: FieldErrors | undefined;

  constructor(
    status: number,
    message: string,
    problem: ProblemDetails | undefined,
    requestId: string | undefined,
    extras?: {
      retryAfterSeconds?: number;
      fieldErrors?: FieldErrors;
      cause?: unknown;
    },
  ) {
    super(message, extras?.cause === undefined ? undefined : { cause: extras.cause });
    this.name = 'ApiError';
    this.status = status;
    this.problem = problem;
    this.requestId = requestId;
    this.retryAfterSeconds = extras?.retryAfterSeconds;
    this.fieldErrors = extras?.fieldErrors;
  }
}

/**
 * Parses a `Retry-After` value (delta-seconds or an RFC 9110 HTTP-date)
 * into non-negative seconds. Missing or malformed values give
 * `undefined`, and a past date gives `0`.
 */
export function parseRetryAfter(
  header: string | null,
  now: () => number = Date.now,
): number | undefined {
  if (header === null) return undefined;
  const trimmed = header.trim();
  if (trimmed === '') return undefined;
  if (/^\d+$/.test(trimmed)) {
    const n = Number.parseInt(trimmed, 10);
    return Number.isFinite(n) && n >= 0 ? n : undefined;
  }
  const t = Date.parse(trimmed);
  if (Number.isNaN(t)) return undefined;
  const delta = Math.ceil((t - now()) / 1000);
  return delta > 0 ? delta : 0;
}

/**
 * Keeps only non-empty string entries of the `errors` extension: the
 * body is parsed with a cast, so its shape is not guaranteed.
 */
function extractFieldErrors(
  problem: ProblemDetails | undefined,
): FieldErrors | undefined {
  const raw: unknown = problem?.errors;
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return undefined;
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw)) {
    if (k !== '' && typeof v === 'string' && v !== '') out[k] = v;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

// `detail` is the occurrence-specific text the spec marks safe to show
// end users; `title` is the generic per-type summary.
function problemMessage(status: number, problem: ProblemDetails | undefined): string {
  if (problem?.detail) return problem.detail;
  if (problem?.title) return problem.title;
  return `HTTP ${status}`;
}

const REQUEST_TIMEOUT_MS = 15_000;

/**
 * Aborts when either signal does. `AbortSignal.any` would do this, but it
 * needs Safari 17.4, and react-query passes a signal on every query, so
 * older iOS would fail every request.
 */
function anySignal(a: AbortSignal, b: AbortSignal): AbortSignal {
  const controller = new AbortController();
  for (const signal of [a, b]) {
    if (signal.aborted) {
      controller.abort(signal.reason);
      break;
    }
    signal.addEventListener(
      'abort',
      () => {
        controller.abort(signal.reason);
      },
      {
        once: true,
        signal: controller.signal,
      },
    );
  }
  return controller.signal;
}

/**
 * Maps a rejection from `fetch` or a body read to an {@link ApiError}
 * with status 0. A caller-initiated abort and a JSON `SyntaxError` are
 * returned untouched.
 */
function transportError(
  err: unknown,
  callerSignal: AbortSignal | null | undefined,
  timeoutSignal: AbortSignal,
): unknown {
  if (callerSignal?.aborted) return err;
  if (timeoutSignal.aborted) {
    return new ApiError(
      0,
      'The Urbanist Atlas API timed out. Try again in a moment.',
      undefined,
      undefined,
      { cause: err },
    );
  }
  if (err instanceof SyntaxError) return err;
  return new ApiError(
    0,
    "Couldn't reach the Urbanist Atlas API. Check your connection and try again.",
    undefined,
    undefined,
    { cause: err },
  );
}

/**
 * Fetches `path` under {@link apiBase} with a 15s timeout and returns the
 * parsed JSON body. Throws {@link ApiError} for a non-2xx response or no
 * response; rethrows the caller's own abort as-is.
 */
export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (!headers.has('Accept')) {
    headers.set('Accept', 'application/json');
  }
  if (apiClientSecret && !headers.has('X-Atlas-Client')) {
    headers.set('X-Atlas-Client', apiClientSecret);
  }

  const callerSignal = init?.signal;
  const timeoutSignal = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
  const signal = callerSignal ? anySignal(callerSignal, timeoutSignal) : timeoutSignal;

  let response: Response;
  try {
    response = await fetch(`${apiBase}${path}`, { ...init, headers, signal });
  } catch (err) {
    throw transportError(err, callerSignal, timeoutSignal);
  }
  const requestId = response.headers.get('X-Request-ID') ?? undefined;

  if (!response.ok) {
    let problem: ProblemDetails | undefined;
    const contentType = response.headers.get('Content-Type') ?? '';
    if (contentType.includes('application/problem+json')) {
      try {
        problem = (await response.json()) as ProblemDetails;
      } catch {
        // An unreadable problem body still yields a bare-status error.
      }
    }
    const retryAfterSeconds =
      response.status === 429
        ? parseRetryAfter(response.headers.get('Retry-After'))
        : undefined;
    throw new ApiError(
      response.status,
      problemMessage(response.status, problem),
      problem,
      requestId,
      {
        retryAfterSeconds,
        fieldErrors: extractFieldErrors(problem),
      },
    );
  }

  try {
    return (await response.json()) as T;
  } catch (err) {
    throw transportError(err, callerSignal, timeoutSignal);
  }
}

/**
 * react-query `retry` predicate: never retries a 4xx (the request
 * itself is wrong, or rate-limited), and retries anything else once.
 */
export function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
    return false;
  }
  return failureCount < 1;
}

/**
 * `GET /api/v1/lookup`: the orgs serving a postal code. An unresolvable
 * code throws an {@link ApiError} with status 404.
 */
export function lookup(
  postal_code: string,
  country: Country,
  init?: RequestInit,
): Promise<LookupResult> {
  const params = new URLSearchParams({ postal_code, country });
  return apiFetch<LookupResult>(`/api/v1/lookup?${params.toString()}`, init);
}

/**
 * `GET /api/v1/regions`: the browse set (metros and cities) with their
 * org counts, unwrapped from the `{ meta, data }` envelope.
 */
export function listRegions(init?: RequestInit): Promise<RegionSummary[]> {
  return apiFetch<RegionSummariesEnvelope>('/api/v1/regions', init).then(
    (env) => env.data,
  );
}

/**
 * `GET /api/v1/regions/search`: type-ahead over every region kind,
 * unwrapped from the `{ meta, data }` envelope. A blank `q` returns `[]`.
 */
export function searchRegions(
  q: string,
  limit?: number,
  init?: RequestInit,
): Promise<RegionSearchResult[]> {
  const params = new URLSearchParams({ q });
  if (limit !== undefined) params.set('limit', String(limit));
  return apiFetch<RegionSearchResultsEnvelope>(
    `/api/v1/regions/search?${params.toString()}`,
    init,
  ).then((env) => env.data);
}

/**
 * `GET /api/v1/regions/{slug}`: one region and the orgs serving it.
 * Throws an {@link ApiError} with status 404 for an unknown or
 * national-tier slug.
 */
export function getRegion(slug: string, init?: RequestInit): Promise<RegionDetail> {
  return apiFetch<RegionDetail>(`/api/v1/regions/${encodeURIComponent(slug)}`, init);
}

/**
 * `GET /api/v1/orgs/{slug}`: one org with the regions it serves. Throws
 * an {@link ApiError} with status 404 for an unknown slug.
 */
export function getOrg(slug: string, init?: RequestInit): Promise<Org> {
  return apiFetch<Org>(`/api/v1/orgs/${encodeURIComponent(slug)}`, init);
}

/**
 * `GET /api/v1/recent`: recently approved orgs, newest first, unwrapped
 * from the `{ meta, data }` envelope.
 */
export function listRecent(init?: RequestInit): Promise<Org[]> {
  return apiFetch<RecentEnvelope>('/api/v1/recent', init).then((env) => env.data);
}

/**
 * `GET /api/v1/stats`: atlas-wide org and region counts, in total and
 * per country. Use it for every atlas-wide number: summing
 * {@link listRegions} counts misses orgs outside the browse set.
 */
export function getStats(init?: RequestInit): Promise<Stats> {
  return apiFetch<Stats>('/api/v1/stats', init);
}

/**
 * `POST /api/v1/submissions`: queue an org for editorial review.
 * Validation errors are 400; rate limits and a full queue are 429 with
 * `Retry-After`.
 */
export function createSubmission(
  body: NewSubmissionRequest,
  init?: RequestInit,
): Promise<Submission> {
  const headers = new Headers(init?.headers);
  headers.set('Content-Type', 'application/json');
  return apiFetch<Submission>('/api/v1/submissions', {
    ...init,
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });
}
