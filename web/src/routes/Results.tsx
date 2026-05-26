import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import { Link, useParams, useSearchParams } from 'react-router';
import { ApiError, isSupportedCountry, lookup } from '../lib/api.ts';
import type { Country, LookupOrg, LookupResult } from '../lib/api.ts';
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
  // ~35k postal-code permutations; let crawlers follow the org links out but
  // keep the thin per-ZIP pages out of the index.
  useEffect(() => {
    const meta = document.createElement('meta');
    meta.name = 'robots';
    meta.content = 'noindex,follow';
    document.head.appendChild(meta);
    return () => {
      meta.remove();
    };
  }, []);

  return (
    <>
      <div className="kicker">
        <div>
          <Link to="/">Atlas</Link>
          <span className="crumb-sep">/</span>
          <span className="crumb-here">
            Lookup · {postalCode || '—'}
          </span>
        </div>
        <div>
          {country ? `${country} · postal-code lookup` : 'Postal-code lookup'}
        </div>
      </div>
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
  if (postalCode.length === 0) {
    return (
      <p className="results-state" style={{ marginTop: 48 }}>
        No postal code in the URL. <Link to="/">Try the lookup</Link>.
      </p>
    );
  }
  if (country === null) {
    return (
      <p className="results-state error" role="alert" style={{ marginTop: 48 }}>
        Country <code>{rawCountry}</code> isn&rsquo;t supported yet. Try{' '}
        <code>?country=US</code> or <code>?country=CA</code>.
      </p>
    );
  }
  if (query.isPending) {
    return (
      <p className="results-state" role="status" style={{ marginTop: 48 }}>
        Looking up groups near {postalCode}…
      </p>
    );
  }
  if (query.isError) {
    return (
      <p className="results-state error" role="alert" style={{ marginTop: 48 }}>
        {query.error.message}
        {query.error.requestId ? (
          <span className="results-state-detail">
            request id: {query.error.requestId}
          </span>
        ) : null}
      </p>
    );
  }

  const placeLabel = query.data.resolved_place_label ?? postalCode;
  const ancestry = query.data.resolved_ancestry;
  const { local, regional } = query.data;
  const empty = local.length === 0 && regional.length === 0;

  return (
    <>
      <div className="lede" style={{ marginTop: 48 }}>
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
        {ancestry.length > 0 ? (
          <div className="byline">
            <span>{country}</span>
            <span className="crumb-sep">·</span>
            <span>
              Resolved to{' '}
              <span className="em">
                {ancestry.map((r) => r.name).join(' → ')}
              </span>
            </span>
          </div>
        ) : null}
      </div>

      {empty ? (
        <div className="editors-note" style={{ marginTop: 24 }}>
          <div className="label">No entries here yet</div>
          <p>
            Know an organization that should be in the Atlas for {placeLabel}?{' '}
            <Link to="/submit">File a tip at the submissions desk</Link> and
            we&rsquo;ll go look.
          </p>
        </div>
      ) : (
        <div style={{ marginTop: 24 }}>
          <ResultSection title="Local" orgs={local} kind="local" />
          <ResultSection title="Regional" orgs={regional} kind="regional" />
        </div>
      )}
    </>
  );
}

function ResultSection({
  title,
  orgs,
  kind,
}: {
  title: string;
  orgs: LookupOrg[];
  kind: 'local' | 'regional';
}) {
  if (orgs.length === 0) return null;
  return (
    <section className="org-section" style={{ marginTop: 32 }}>
      <header className="section-break" style={{ marginTop: 0 }}>
        <span className="num">{kind === 'local' ? 'I.' : 'II.'}</span>
        <h2 className="title">
          {title}
          <span className="accent" style={{ color: 'var(--amber)' }}>
            .
          </span>
        </h2>
        <span className="aside">
          {orgs.length} {orgs.length === 1 ? 'entry' : 'entries'}
        </span>
      </header>
      {orgs.map((org) => (
        <ResultEntry key={org.id} org={org} />
      ))}
    </section>
  );
}

function ResultEntry({ org }: { org: LookupOrg }) {
  const domain = domainOf(org.website_url);
  return (
    <article className="org-entry">
      <div>
        <div className="head">
          <h3 className="name">
            <Link to={`/orgs/${encodeURIComponent(org.slug)}`}>{org.name}</Link>
          </h3>
        </div>
        {org.website_url ? (
          <div className="url">
            <a href={org.website_url} target="_blank" rel="noopener noreferrer">
              {domain ?? org.website_url}
            </a>
          </div>
        ) : null}
        <p className="desc">{org.short_desc}</p>
        {org.tags.length > 0 ? (
          <ul className="tag-list">
            {org.tags.map((tag) => (
              <li key={tag}>
                <span className="tag">{tag.replace(/-/g, ' ')}</span>
              </li>
            ))}
          </ul>
        ) : null}
        {org.matched_region_slugs.length > 0 ? (
          <div className="foot">
            <span className="via">
              Matched via{' '}
              <span className="em">{org.matched_region_slugs.join(' · ')}</span>
            </span>
          </div>
        ) : null}
      </div>
    </article>
  );
}

function domainOf(url: string): string | null {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return null;
  }
}
