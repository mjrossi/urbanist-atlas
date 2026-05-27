import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import { Link } from 'react-router';
import { SearchBox } from '../components/SearchBox.tsx';
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

      <div className="spread lede-first" style={{ marginTop: 40 }}>
        <div>
          <SearchBox />

          <div className="prose" style={{ marginTop: 48 }}>
            <p className="lead drop">
              The Urbanist Atlas catalogues the people in your city, your county,
              your region who are organizing — patiently, stubbornly, sometimes
              gloriously — for safer streets and better transit. Type a US ZIP or
              a Canadian postal code and we will name them for you.
            </p>
            <p>
              We index local and regional advocates only. National outfits do
              plenty of good work, but they are easy to find on their own; the
              harder search is for the neighbourhood committee three blocks from
              your door, the metro rider alliance two transfers away, the county
              Vision Zero coalition you have never heard of but should have.
            </p>
            <div className="editors-note">
              <div className="label">Editor&rsquo;s note · Vol. I</div>
              <p>
                Each entry is curated by hand against{' '}
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
            <Link to="/browse" className="read-on" style={{ marginTop: 14 }}>
              All places <span className="arrow">→</span>
            </Link>
          </div>
          <div className="rail-block amber desktop-only">
            <div className="rail-kicker">From the editors</div>
            <p className="pullquote-rail">
              National advocacy is easy to find. The harder search is the
              neighbourhood committee three blocks from your door.
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
  if (query.isPending) {
    return <p className="results-state">Loading places…</p>;
  }
  if (query.isError) {
    return <p className="results-state error">Region list is temporarily unavailable.</p>;
  }
  const items = query.data.slice(0, TOP_PLACES_LIMIT);
  if (items.length === 0) {
    return <p className="results-state">No places indexed yet.</p>;
  }
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
}

function RecentlyFiled({ query }: { query: UseQueryResult<Org[], ApiError> }) {
  return (
    <>
      <section className="section-break">
        <span className="num">II.</span>
        <h2 className="title">Recently filed.</h2>
        <span className="aside">From the editor&rsquo;s desk</span>
      </section>
      <RecentBody query={query} />
    </>
  );
}

function RecentBody({ query }: { query: UseQueryResult<Org[], ApiError> }) {
  if (query.isPending) {
    return <p className="results-state">Loading recent entries…</p>;
  }
  if (query.isError) {
    return <p className="results-state error">Recent entries are temporarily unavailable.</p>;
  }
  const items = query.data.slice(0, RECENT_LIMIT);
  if (items.length === 0) {
    return <p className="results-state">Nothing filed yet.</p>;
  }
  return (
    <div className="org-strip">
      {items.map((org) => {
        const where = primaryRegionLabel(org);
        return (
          <Link
            key={org.id}
            className="org-card"
            to={`/orgs/${encodeURIComponent(org.slug)}`}
          >
            <div className="added">+ Newly indexed</div>
            <h3 className="org-name">{org.name}</h3>
            {where ? <div className="org-where">{where}</div> : null}
            <p className="org-desc">{truncate(org.short_desc, 180)}</p>
          </Link>
        );
      })}
    </div>
  );
}

function ByTheNumbers({
  places,
  recent,
}: {
  places: UseQueryResult<RegionSummary[], ApiError>;
  recent: UseQueryResult<Org[], ApiError>;
}) {
  const placeCount = places.data?.length ?? null;
  const usCount = places.data?.filter((p) => p.region.country === 'US').length ?? null;
  const caCount = places.data?.filter((p) => p.region.country === 'CA').length ?? null;
  const totalOrgCount = places.data?.reduce((sum, p) => sum + p.org_count, 0) ?? null;
  const topRegion = places.data
    ? [...places.data].sort((a, b) => b.org_count - a.org_count)[0]
    : null;
  const recentCount = recent.data?.length ?? null;

  return (
    <>
      <section className="section-break" style={{ marginTop: 56 }}>
        <span className="num">III.</span>
        <h2 className="title">The Atlas, by the numbers.</h2>
        <span className="aside">From the live directory</span>
      </section>
      <div className="stats">
        <div className="stat">
          <div className="n">
            <span className="em">{formatNumber(totalOrgCount)}</span>
          </div>
          <div className="label">Org entries on file</div>
          <div className="sub">Across the US and Canada</div>
        </div>
        <div className="stat">
          <div className="n">{formatNumber(placeCount)}</div>
          <div className="label">Places indexed</div>
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
      <section className="section-break" style={{ marginTop: 56 }}>
        <span className="num">IV.</span>
        <h2 className="title">The topics the Atlas covers.</h2>
        <span className="aside">Editorial scope</span>
      </section>
      <div className="spread lede-first" style={{ marginTop: 12 }}>
        <div>
          <ul className="tag-list" style={{ gap: 12 }}>
            {tags.map((label) => (
              <li key={label}>
                <span className="tag">{label}</span>
              </li>
            ))}
          </ul>
          <p className="fineprint" style={{ marginTop: 22 }}>
            Tags are editorial labels, applied by hand. An organization can
            carry up to five. Per-topic filtering ships with Phase 2; until
            then, <Link to="/browse">the place index</Link> is the wander
            view.
          </p>
        </div>
        <aside className="rail desktop-only">
          <div className="rail-block">
            <div className="rail-kicker">For developers</div>
            <p>
              The dataset is published under the Open Database License (ODbL
              1.0). Phase 2 opens self-serve free API keys; until then, an
              editor will hand-issue one on request.
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
