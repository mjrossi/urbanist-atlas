import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import { Link } from 'react-router';
import { EmptyState } from '../components/EmptyState.tsx';
import { SearchBox } from '../components/SearchBox.tsx';
import { QueryState } from '../components/QueryState.tsx';
import { ApiError, listRegions, listRecent } from '../lib/api.ts';
import type { RegionSummary, Org } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

const TOP_PLACES_LIMIT = 7;
const RECENT_LIMIT = 4;

const TOPIC_TAGS: ReadonlyArray<string> = [
  'Transit',
  'Safe streets',
  'Vision Zero',
  'Cycling',
  'Walkability',
  'Bus & rapid transit',
  'Rail',
  'Policy & research',
  'Political & PAC',
  'Rider unions',
];

export function Home() {
  useDocumentTitle('Urbanist Atlas — Transit and safe-streets advocacy near you');
  const places = useQuery<RegionSummary[], ApiError>({
    queryKey: queryKeys.regions(),
    queryFn: ({ signal }) => listRegions({ signal }),
  });
  const recent = useQuery<Org[], ApiError>({
    queryKey: queryKeys.recent(),
    queryFn: ({ signal }) => listRecent({ signal }),
  });

  return (
    <>
      <div className="kicker">
        <div>
          The front page<span className="crumb-sep">·</span>
          <span className="crumb-here">Index by postal code</span>
        </div>
        <div>Vol. I · 2026 Edition</div>
      </div>

      <div className="spread lede-first mt-40">
        <div>
          <SearchBox />

          <div className="prose mt-48">
            <p className="lead drop">
              Within a few blocks of you, people show up to transportation
              meetings week after week, patiently arguing for safer streets and
              better transit. The Urbanist Atlas helps you find them. Type a US
              ZIP or a Canadian postal code and we&rsquo;ll name the ones
              working where you live.
            </p>
            <p>
              We index local and regional advocates only. National outfits do
              plenty of good work, but they are easy to find on their own; the
              harder search is for the neighborhood committee three blocks from
              your door, the metro rider alliance two transfers away, the county
              Vision Zero coalition you have never heard of but should have.
            </p>
            <div className="editors-note">
              <div className="label">Editor&rsquo;s note · Vol. I</div>
              <p>
                Every entry is picked by hand, weighed against{' '}
                <Link to="/about#methodology">our criteria</Link>.{' '}
                <Link to="/submit">File a tip</Link> if your region is missing.
              </p>
            </div>
          </div>
        </div>

        <aside className="rail">
          <div className="rail-block">
            <div className="rail-kicker">Browse the atlas</div>
            <TopPlaces query={places} />
            <Link to="/browse" className="read-on mt-14">
              All regions <span className="arrow">→</span>
            </Link>
          </div>
          <div className="rail-block amber desktop-only">
            <div className="rail-kicker">From the editors</div>
            <p className="pullquote-rail">
              National advocacy is easy to find. The harder search is the
              neighborhood committee three blocks from your door.
            </p>
          </div>
        </aside>
      </div>

      <RecentlyFiled query={recent} />
      <ByTheNumbers places={places} recent={recent} />
      <TopicIndex tags={TOPIC_TAGS} />
    </>
  );
}

function TopPlaces({ query }: { query: UseQueryResult<RegionSummary[], ApiError> }) {
  return (
    <QueryState
      query={query}
      loading="Loading regions…"
      error={() => (
        <p className="results-state error">
          The region list isn&rsquo;t loading right now. Refresh to try again.
        </p>
      )}
      empty={{
        when: (data) => data.length === 0,
        render: <p className="results-state">No regions indexed yet.</p>,
      }}
    >
      {(data) => {
        const items = data.slice(0, TOP_PLACES_LIMIT);
        return (
          <ul className="metros compact">
            {items.map((p) => (
              <li key={p.region.slug}>
                <Link className="metro" to={`/region/${p.region.slug}`}>
                  <div>
                    <p className="name">{p.region.name}</p>
                    <div className="meta">
                      {p.region.country}
                      {p.region.parent_slugs[0] ? ` · ${p.region.parent_slugs[0]}` : ''}
                    </div>
                  </div>
                  <div className="count">
                    <span className="n">{p.org_count}</span>
                    {p.org_count === 1 ? 'group' : 'groups'}
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        );
      }}
    </QueryState>
  );
}

function RecentlyFiled({ query }: { query: UseQueryResult<Org[], ApiError> }) {
  return (
    <>
      <section className="section-break">
        <span className="num">II.</span>
        <h2 className="title">Recently indexed.</h2>
        <span className="aside">From the editor&rsquo;s desk</span>
      </section>
      <RecentBody query={query} />
    </>
  );
}

function RecentBody({ query }: { query: UseQueryResult<Org[], ApiError> }) {
  return (
    <QueryState
      query={query}
      loading="Loading recent entries…"
      error={() => (
        <p className="results-state error">
          Recent entries aren&rsquo;t loading right now. Refresh, or browse the full
          index.
        </p>
      )}
      empty={{
        when: (data) => data.length === 0,
        render: (
          <EmptyState
            title="Nothing filed yet"
            body="The editor's desk has been quiet lately."
            cta={<Link to="/submit">File the first tip.</Link>}
          />
        ),
      }}
    >
      {(data) => (
        <div className="org-strip">
          {data.slice(0, RECENT_LIMIT).map((org) => {
            const where = primaryRegionLabel(org);
            return (
              <Link
                key={org.id}
                className="org-card"
                to={`/orgs/${encodeURIComponent(org.slug)}`}
              >
                <div className="added">Newly indexed</div>
                <h3 className="org-name">{org.name}</h3>
                {where ? <div className="org-where">{where}</div> : null}
                <p className="org-desc">{truncate(org.short_desc, 180)}</p>
              </Link>
            );
          })}
        </div>
      )}
    </QueryState>
  );
}

function ByTheNumbers({
  places,
  recent,
}: {
  places: UseQueryResult<RegionSummary[], ApiError>;
  recent: UseQueryResult<Org[], ApiError>;
}) {
  // Stat aggregates over the (potentially large) regions list.
  // The cost is negligible today, but useMemo declares the intent
  // and stops the work from re-running when an unrelated parent
  // state change re-renders this section.
  const { placeCount, usCount, caCount, totalOrgCount, topRegion } = useMemo(() => {
    const data = places.data;
    if (!data) {
      return {
        placeCount: null,
        usCount: null,
        caCount: null,
        totalOrgCount: null,
        topRegion: null,
      };
    }
    let us = 0;
    let ca = 0;
    let total = 0;
    let top: RegionSummary | null = null;
    for (const p of data) {
      if (p.region.country === 'US') us += 1;
      else if (p.region.country === 'CA') ca += 1;
      // direct_org_count avoids double-counting orgs that surface
      // under both a metro and its child cities.
      total += p.direct_org_count;
      // "Deepest coverage" intentionally tracks org_count (descendant
      // walk) — single-row max showing how much advocacy depth one
      // region's scope contains.
      if (!top || p.org_count > top.org_count) top = p;
    }
    return {
      placeCount: data.length,
      usCount: us,
      caCount: ca,
      totalOrgCount: total,
      topRegion: top,
    };
  }, [places.data]);
  const recentCount = recent.data?.length ?? null;

  return (
    <>
      <section className="section-break mt-56">
        <span className="num">III.</span>
        <h2 className="title">The Atlas, by the numbers.</h2>
        <span className="aside">From the live directory</span>
      </section>
      <div className="stats">
        <div className="stat">
          <div className="n">
            <span className="em">{formatNumber(totalOrgCount)}</span>
          </div>
          <div className="label">Organizations on file</div>
          <div className="sub">Across the US and Canada</div>
        </div>
        <div className="stat">
          <div className="n">{formatNumber(placeCount)}</div>
          <div className="label">Regions indexed</div>
          <div className="sub">
            {usCount !== null && caCount !== null
              ? `${usCount} US · ${caCount} Canada`
              : 'Two countries, v1'}
          </div>
        </div>
        <div className="stat">
          <div className="n">{formatNumber(recentCount)}</div>
          <div className="label">Recently filed</div>
          <div className="sub">Latest editorial additions</div>
        </div>
        <div className="stat">
          <div className="n">
            <span className="em">{topRegion ? topRegion.org_count : '—'}</span>
          </div>
          <div className="label">Deepest coverage</div>
          <div className="sub">
            {topRegion ? `${topRegion.region.name}, today` : 'Loading…'}
          </div>
        </div>
      </div>
    </>
  );
}

function TopicIndex({ tags }: { tags: ReadonlyArray<string> }) {
  return (
    <>
      <section className="section-break mt-56">
        <span className="num">IV.</span>
        <h2 className="title">The topics the Atlas covers.</h2>
        <span className="aside">Editorial scope</span>
      </section>
      <div className="spread lede-first mt-12">
        <div>
          <ul className="tag-list gap-12">
            {tags.map((label) => (
              <li key={label}>
                <span className="tag">{label}</span>
              </li>
            ))}
          </ul>
          <p className="fineprint mt-22">
            Tags are editorial labels, applied by hand — up to five per
            organization. Filtering by topic arrives with Phase 2; until then,{' '}
            <Link to="/browse">the region index</Link> is how you wander.
          </p>
        </div>
        <aside className="rail desktop-only">
          <div className="rail-block">
            <div className="rail-kicker">For developers</div>
            <p>
              The dataset is published under the Open Database License (ODbL
              1.0). Phase 2 opens self-serve free API keys; until then, ask and
              an editor will set one up by hand.
            </p>
            <Link to="/about#for-developers" className="read-on">
              Developer preview <span className="arrow">→</span>
            </Link>
          </div>
        </aside>
      </div>
    </>
  );
}

function primaryRegionLabel(org: Org): string | null {
  const r = org.regions[0];
  if (!r) return null;
  return `${r.name} · ${r.country}`;
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s;
  return `${s.slice(0, n - 1).trimEnd()}…`;
}

function formatNumber(n: number | null): string {
  if (n === null) return '—';
  return n.toLocaleString('en-US');
}
