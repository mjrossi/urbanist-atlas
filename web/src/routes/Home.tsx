import type { UseQueryResult } from '@tanstack/react-query';
import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { Link } from 'react-router';

import { EmptyState } from '../components/EmptyState.tsx';
import { QueryState } from '../components/QueryState.tsx';
import { SearchBox } from '../components/SearchBox.tsx';
import type { Org, RegionSummary, Stats } from '../lib/api.ts';
import { ApiError, getStats, listRecent, listRegions } from '../lib/api.ts';
import { formatAddedAt, pluralize } from '../lib/format.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

const TOP_PLACES_LIMIT = 7;
const RECENT_LIMIT = 4;

const TOPIC_TAGS: readonly string[] = [
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
  const stats = useQuery<Stats, ApiError>({
    queryKey: queryKeys.stats(),
    queryFn: ({ signal }) => getStats({ signal }),
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
              Across the country, the county, and around the corner, people show up to
              transportation meetings patiently arguing for safer streets and better
              transit. The Urbanist Atlas helps you find these meetings and groups.
            </p>
            <p>
              How it works: type a US ZIP or a Canadian postal code and we&rsquo;ll name
              the ones working where you live.
            </p>
            <p>
              We&rsquo;ll index local and regional advocates only. National outfits do
              plenty of good work, but they&rsquo;re easier to find on their own. The hard
              search is finding those right around you. Who knows? They might be right
              behind your keyboard.
            </p>
            <div className="editors-note">
              <div className="label">Editor&rsquo;s note · Vol. I</div>
              <p>
                Every entry is reviewed against{' '}
                <Link to="/about#methodology">our criteria</Link> before it goes in.{' '}
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
              The most local thing on the internet is the meeting three blocks from your
              door.
            </p>
          </div>
        </aside>
      </div>

      <RecentlyFiled query={recent} />
      <ByTheNumbers places={places} recent={recent} stats={stats} />
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
                    {pluralize(p.org_count, 'group', 'groups')}
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
                <div className="added">Added {formatAddedAt(org.added_at)}</div>
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
  stats,
}: {
  places: UseQueryResult<RegionSummary[], ApiError>;
  recent: UseQueryResult<Org[], ApiError>;
  stats: UseQueryResult<Stats, ApiError>;
}) {
  // Counts come from `/api/v1/stats`, never from summing over
  // `places`. `/api/v1/regions` returns only the browseable subset
  // (metros and cities), so a sum of its direct_org_count drops every
  // org attached solely to a state, province, borough, or multi-state
  // region — 70 of them, which is how this panel came to advertise 166
  // organizations against a 236-org catalog.
  const totalOrgCount = stats.data?.total_org_count ?? null;
  const coveredPlaceCount = stats.data?.browse_region_count ?? null;
  const { usCount, caCount } = useMemo(() => {
    const rows = stats.data?.by_country;
    if (!rows) return { usCount: null, caCount: null };
    const find = (code: string) =>
      rows.find((r) => r.country === code)?.region_count ?? 0;
    return { usCount: find('US'), caCount: find('CA') };
  }, [stats.data]);
  // "Deepest coverage" is the single region whose scope contains the
  // most orgs (org_count is the descendant walk). ListRegions already
  // sorts by org_count DESC, so the head of the list is the answer —
  // no scan needed.
  const topRegion = places.data?.[0] ?? null;
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
          <div className="n">{formatNumber(coveredPlaceCount)}</div>
          {/*
            "Places with coverage", not "Regions indexed": this counts
            the browseable metros and cities carrying at least one org,
            not the ~628 regions in the graph. The old label claimed the
            whole index and showed a seventh of it.
          */}
          <div className="label">Places with coverage</div>
          <div className="sub">
            {usCount !== null ? `${usCount} US · ${caCount} Canada` : 'Two countries, v1'}
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

function TopicIndex({ tags }: { tags: readonly string[] }) {
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
            Tags are editorial labels — up to five per organization. Filtering by topic
            arrives with Phase 2; until then, <Link to="/browse">the region index</Link>{' '}
            is how you wander.
          </p>
        </div>
        <aside className="rail desktop-only">
          <div className="rail-block">
            <div className="rail-kicker">For developers</div>
            <p>
              The dataset is published under the Open Database License (ODbL 1.0). Phase 2
              opens self-serve free API keys; until then, ask and an editor will set one
              up by hand.
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
