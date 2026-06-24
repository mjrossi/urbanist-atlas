import type { UseQueryResult } from '@tanstack/react-query';
import { useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { Link, useParams, useSearchParams } from 'react-router';

import { EmptyState } from '../components/EmptyState.tsx';
import { EntryList } from '../components/EntryList.tsx';
import { QueryState } from '../components/QueryState.tsx';
import type { BreadcrumbItem } from '../components/RegionBreadcrumb.tsx';
import { RegionBreadcrumb } from '../components/RegionBreadcrumb.tsx';
import { reverseAncestry } from '../lib/ancestry.ts';
import type { Country, LookupResult } from '../lib/api.ts';
import { ApiError, isSupportedCountry, lookup } from '../lib/api.ts';
import { totalEntries } from '../lib/orgBuckets.ts';
import { normalizePostal } from '../lib/postal.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

function parseCountry(raw: string | null): Country | null {
  if (raw === null) return 'US';
  return isSupportedCountry(raw) ? raw : null;
}

export function Results() {
  const params = useParams<{ postalCode: string }>();
  const [search] = useSearchParams();
  const postalCode = normalizePostal(params.postalCode ?? '');
  const rawCountry = search.get('country');
  const country = parseCountry(rawCountry);
  const effectiveCountry: Country = country ?? 'US';

  const query = useQuery<LookupResult, ApiError>({
    queryKey: queryKeys.lookup(postalCode, effectiveCountry),
    queryFn: ({ signal }) => lookup(postalCode, effectiveCountry, { signal }),
    enabled: postalCode.length > 0 && country !== null,
  });

  useDocumentTitle(
    postalCode ? `${postalCode} — Urbanist Atlas` : 'Lookup — Urbanist Atlas',
  );
  // ~35k postal-code permutations; let crawlers follow the org links
  // out but keep the thin per-ZIP pages out of the index.
  useEffect(() => {
    const meta = document.createElement('meta');
    meta.name = 'robots';
    meta.content = 'noindex,follow';
    document.head.appendChild(meta);
    return () => {
      meta.remove();
    };
  }, []);

  // Breadcrumb prefix: "Atlas / Lookup · <ZIP>". The ZIP renders as
  // a non-clickable span (no `to`) so it shows context without
  // implying a clickable lookup-only landing page.
  const breadcrumbPrefix: readonly BreadcrumbItem[] = [
    { label: 'Atlas', to: '/' },
    { label: `Lookup · ${postalCode || '—'}` },
  ];

  // Resolved-ancestry walk: API returns leaf-first; the breadcrumb
  // wants root-first followed by the leaf as `current`. Reverse
  // the ancestry and drop the (now-last) leaf, which the breadcrumb
  // renders separately via `currentLabel`.
  const ancestryRootFirst = query.data
    ? reverseAncestry(query.data.resolved_ancestry).slice(0, -1)
    : [];
  const leafName = query.data?.resolved_ancestry[0]?.name;
  const currentLabel = leafName ?? (postalCode || '—');
  const metaRight = country ? `${country} · postal-code lookup` : 'Postal-code lookup';

  return (
    <>
      <RegionBreadcrumb
        prefix={breadcrumbPrefix}
        ancestors={ancestryRootFirst}
        current={currentLabel}
        metaRight={metaRight}
      />
      <ResultsBody
        query={query}
        postalCode={postalCode}
        country={country}
        rawCountry={rawCountry}
      />
    </>
  );
}

function ResultsBody({
  query,
  postalCode,
  country,
  rawCountry,
}: {
  query: UseQueryResult<LookupResult, ApiError>;
  postalCode: string;
  country: Country | null;
  rawCountry: string | null;
}) {
  // Pre-query gates: URL hasn't reached a state where a lookup makes sense yet.
  if (postalCode.length === 0) {
    return (
      <p className="results-state mt-48">
        No postal code in the URL. <Link to="/">Try the lookup</Link>.
      </p>
    );
  }
  if (country === null) {
    return (
      <p className="results-state error mt-48" role="alert">
        Country <code>{rawCountry}</code> isn&rsquo;t supported yet. Try{' '}
        <code>?country=US</code> or <code>?country=CA</code>.
      </p>
    );
  }

  return (
    <QueryState
      query={query}
      loading={<>Finding organizations for {postalCode}…</>}
      error={(e) =>
        // Any 404 on /lookup is an unresolved postal code — a not-found
        // or a military/diplomatic ZIP. The API owns the consumer-facing
        // sentence (problem.detail); we render it as the card body and add
        // the navigation links as chrome rather than re-authoring the copy.
        // The card label is a fixed small-caps eyebrow (the EmptyState
        // `.label` slot is uppercased and tracked — designed for a terse
        // eyebrow, not a backend problem title), so the server's title is
        // not routed there. Genuine errors (401/429/500) return undefined
        // and fall through to QueryState's default red alert with the
        // request id.
        e.status === 404 ? (
          <EmptyState
            className="mt-48"
            title="No match for that postal code"
            body={e.problem?.detail ?? `We couldn’t find a match for ${postalCode}.`}
            cta={
              <>
                Try <Link to="/">another code</Link>, or{' '}
                <Link to="/submit">file a tip</Link>.
              </>
            }
          />
        ) : undefined
      }
      className="mt-48"
    >
      {(data) => <ResultsContent data={data} postalCode={postalCode} />}
    </QueryState>
  );
}

function ResultsContent({
  data,
  postalCode,
}: {
  data: LookupResult;
  postalCode: string;
}) {
  const placeLabel = data.resolved_place_label;
  const { local, regional, statewide, resolved_ancestry } = data;
  const empty = totalEntries(data) === 0;

  // EntryList needs a slug -> display name map for its "Matched
  // via X" footer. Build it from the resolved-ancestry walk; this
  // covers every slug an Org's matched_region_slugs can reference
  // for this lookup.
  const regionNameBySlug = new Map(resolved_ancestry.map((r) => [r.slug, r.name]));

  return (
    <>
      <div className="lede mt-48">
        <div className="eyebrow">
          § Postal-code lookup
          <span className="eyebrow-rule" />
        </div>
        <h1>
          {postalCode}
          <span className="accent">.</span>
        </h1>
        <p className="deck">
          {empty
            ? `No entries for ${placeLabel} yet. The map fills in metro by metro, as the leads turn up — this corner just hasn't been reached.`
            : `Groups working in or around ${placeLabel}. Local entries are nearest; regional and state / provincial entries cover wider footprints that include this postal code.`}
        </p>
      </div>

      {empty ? (
        <EmptyState
          className="mt-24"
          title="No entries here yet"
          body={
            <>
              Know an organization that should be in the Atlas for {placeLabel}?{' '}
              <Link to="/submit">File a tip at the submissions desk</Link> and we&rsquo;ll
              go look.
            </>
          }
        />
      ) : (
        <div className="mt-24">
          <EntryList
            local={local}
            regional={regional}
            statewide={statewide}
            regionNameBySlug={regionNameBySlug}
          />
        </div>
      )}
    </>
  );
}
