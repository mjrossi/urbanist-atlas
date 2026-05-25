import { useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useParams, useSearchParams } from 'react-router';
import { Dateline } from '../components/Dateline.tsx';
import { EntryList } from '../components/EntryList.tsx';
import { ApiError, isSupportedCountry, lookup } from '../lib/api.ts';
import type { Country, LookupOrg, LookupResult, Region } from '../lib/api.ts';
import { normalizePostal } from '../lib/postal.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

/**
 * `/r/:postalCode` — resolves the postal code via `GET /api/v1/lookup`
 * and renders the classified-section list grouped into Local and
 * Regional. Country comes from the `?country=` search param
 * (defaults to `US`); the SearchBox always supplies one explicitly.
 */

/**
 * Map the raw `?country=` search param onto a supported `Country`.
 * Missing param falls back to US (the SearchBox always sets one
 * explicitly; this default just keeps a hand-typed `/r/11217` URL
 * working). An unsupported value returns `null` so the caller can
 * render an error instead of silently coercing.
 */
function parseCountry(raw: string | null): Country | null {
  if (raw === null) return 'US';
  return isSupportedCountry(raw) ? raw : null;
}

/**
 * Build a slug→name map from the resolved ancestry plus any regions
 * embedded in each org. The ancestry covers the common case; the org
 * regions handle the edge case where a matched_region_slug belongs to
 * a sibling region not in the ancestry chain.
 */
function buildRegionNameMap(
  ancestry: Region[],
  orgs: LookupOrg[],
): Map<string, string> {
  const map = new Map<string, string>();
  for (const region of ancestry) {
    map.set(region.slug, region.name);
  }
  for (const org of orgs) {
    for (const region of org.regions) {
      if (!map.has(region.slug)) {
        map.set(region.slug, region.name);
      }
    }
  }
  return map;
}

export function Results() {
  const params = useParams<{ postalCode: string }>();
  const [search] = useSearchParams();
  const postalCode = normalizePostal(params.postalCode ?? '');
  const rawCountry = search.get('country');
  const country = parseCountry(rawCountry);
  // `country` is `null` when the param is an unsupported value; the
  // `enabled` gate keeps the queryFn from running in that case, so the
  // fallback to 'US' here is just to give useQuery a concrete key/arg.
  // Keeping the gate as the single source of truth (rather than a cast)
  // means a future change to `enabled` can't silently leak an
  // unsupported-country fetch.
  const effectiveCountry: Country = country ?? 'US';

  const query = useQuery<LookupResult, ApiError>({
    queryKey: queryKeys.lookup(postalCode, effectiveCountry),
    queryFn: ({ signal }) => lookup(postalCode, effectiveCountry, { signal }),
    enabled: postalCode.length > 0 && country !== null,
  });

  useDocumentTitle(
    postalCode ? `${postalCode} — Urbanist Atlas` : 'Lookup — Urbanist Atlas',
  );
  // /r/:postalCode covers ~35k permutations; let crawlers follow the
  // org links out but keep the thin per-ZIP pages out of the index.
  useEffect(() => {
    const meta = document.createElement('meta');
    meta.name = 'robots';
    meta.content = 'noindex,follow';
    document.head.appendChild(meta);
    return () => {
      meta.remove();
    };
  }, []);

  const placeLabel = query.data?.resolved_place_label;
  const ancestry = query.data?.resolved_ancestry ?? [];

  return (
    <div className="page">
      <Dateline
        postalCode={postalCode || '—'}
        country={country ?? 'US'}
        placeLabel={placeLabel}
        ancestry={ancestry}
      />
      <ResultsBody
        query={query}
        postalCode={postalCode}
        country={country}
        rawCountry={rawCountry}
      />
    </div>
  );
}

function ResultsBody({
  query,
  postalCode,
  country,
  rawCountry,
}: {
  query: ReturnType<typeof useQuery<LookupResult, ApiError>>;
  postalCode: string;
  country: Country | null;
  rawCountry: string | null;
}) {
  // Hooks must run unconditionally before any early return, hence the
  // optional-chain into query.data and the empty-array fallbacks. When
  // query.data is undefined the memo result is a fresh empty Map; cheap
  // and discarded because we early-return before rendering EntryList.
  const ancestry = query.data?.resolved_ancestry;
  const local = query.data?.local;
  const regional = query.data?.regional;
  const regionNameBySlug = useMemo(
    () => buildRegionNameMap(ancestry ?? [], [...(local ?? []), ...(regional ?? [])]),
    [ancestry, local, regional],
  );

  if (postalCode.length === 0) {
    return <p className="results-state">No postal code in the URL.</p>;
  }
  if (country === null) {
    return (
      <p className="results-state error" role="alert">
        Country <code>{rawCountry}</code> isn’t supported yet. Try{' '}
        <code>?country=US</code> or <code>?country=CA</code>.
      </p>
    );
  }
  if (query.isPending) {
    return (
      <p className="results-state" role="status">
        Looking up groups near {postalCode}…
      </p>
    );
  }
  if (query.isError) {
    const err = query.error;
    return (
      <p className="results-state error" role="alert">
        {err.message}
        {err.requestId ? (
          <span className="results-state-detail">request id: {err.requestId}</span>
        ) : null}
      </p>
    );
  }
  if (query.data.local.length === 0 && query.data.regional.length === 0) {
    return (
      <p className="results-state">
        No groups indexed yet for {postalCode}. Know one?{' '}
        <a href="/submit">Submit it.</a>
      </p>
    );
  }
  return (
    <EntryList
      local={query.data.local}
      regional={query.data.regional}
      regionNameBySlug={regionNameBySlug}
    />
  );
}
