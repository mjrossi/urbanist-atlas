import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import { Link, useParams, useSearchParams } from 'react-router';
import { ApiError, isSupportedCountry, lookup } from '../lib/api.ts';
import type { Country, LookupResult } from '../lib/api.ts';
import { normalizePostal } from '../lib/postal.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { EntryList } from '../components/EntryList.tsx';
import { QueryState } from '../components/QueryState.tsx';
import { RegionBreadcrumb } from '../components/RegionBreadcrumb.tsx';
import type { BreadcrumbItem } from '../components/RegionBreadcrumb.tsx';

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
  const breadcrumbPrefix: ReadonlyArray<BreadcrumbItem> = [
    { label: 'Atlas', to: '/' },
    { label: `Lookup · ${postalCode || '—'}` },
  ];

  // Resolved-ancestry walk: API returns leaf-first; the breadcrumb
  // wants root-first followed by the leaf as `current`. Reverse a
  // copy and split off the (originally first, now last) leaf.
  const ancestryRootFirst = query.data
    ? [...query.data.resolved_ancestry].slice(1).reverse()
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
      loading={<>Looking up groups near {postalCode}…</>}
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
  const placeLabel = data.resolved_place_label ?? postalCode;
  const { local, regional, resolved_ancestry } = data;
  const empty = local.length === 0 && regional.length === 0;

  // EntryList needs a slug -> display name map for its "Matched
  // via X" footer. Build it from the resolved-ancestry walk; this
  // covers every slug an Org's matched_region_slugs can reference
  // for this lookup.
  const regionNameBySlug = new Map(
    resolved_ancestry.map((r) => [r.slug, r.name]),
  );

  return (
    <>
      <div className="lede mt-48">
        <div className="eyebrow">
          § Postal-code lookup<span className="eyebrow-rule" />
        </div>
        <h1>
          {postalCode}
          <span className="accent">.</span>
        </h1>
        <p className="deck">
          {empty
            ? `Nothing indexed yet for ${placeLabel}. The Atlas grows one editorial decision at a time — file a tip if you know who's doing the work here.`
            : `Groups working in or around ${placeLabel}. Local entries are nearest; regional entries cover wider footprints that include this postal code.`}
        </p>
      </div>

      {empty ? (
        <div className="editors-note mt-24">
          <div className="label">No entries here yet</div>
          <p>
            Know an organization that should be in the Atlas for {placeLabel}?{' '}
            <Link to="/submit">File a tip at the submissions desk</Link> and
            we&rsquo;ll go look.
          </p>
        </div>
      ) : (
        <div className="mt-24">
          <EntryList local={local} regional={regional} regionNameBySlug={regionNameBySlug} />
        </div>
      )}
    </>
  );
}
