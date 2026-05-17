import { useQuery } from '@tanstack/react-query';
import { useParams, useSearchParams } from 'react-router';
import { Dateline } from '../components/Dateline.tsx';
import { EntryList } from '../components/EntryList.tsx';
import { ApiError, isSupportedCountry, lookup } from '../lib/api.ts';
import type { Country, LookupOrg, LookupResult, Region } from '../lib/api.ts';
import { normalizePostal } from '../lib/postal.ts';
import { queryKeys } from '../lib/queryKeys.ts';

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

  const query = useQuery<LookupResult, ApiError>({
    queryKey: queryKeys.lookup(postalCode, country ?? 'US'),
    queryFn: ({ signal }) => lookup(postalCode, country as Country, { signal }),
    enabled: postalCode.length > 0 && country !== null,
  });

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
  const { local, regional, resolved_ancestry } = query.data;
  if (local.length === 0 && regional.length === 0) {
    return (
      <p className="results-state">
        No groups indexed yet for {postalCode}. Know one?{' '}
        <a href="/submit">Submit it.</a>
      </p>
    );
  }
  const regionNameBySlug = buildRegionNameMap(resolved_ancestry, [...local, ...regional]);
  return <EntryList local={local} regional={regional} regionNameBySlug={regionNameBySlug} />;
}
