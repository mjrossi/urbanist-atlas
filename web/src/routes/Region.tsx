import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import { Link, useParams } from 'react-router';
import { ApiError, getRegion } from '../lib/api.ts';
import type { LookupOrg, RegionDetail } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { regionKindLabel } from '../lib/regionKind.ts';
import { EntryList } from '../components/EntryList.tsx';
import { QueryState } from '../components/QueryState.tsx';
import { RegionBreadcrumb } from '../components/RegionBreadcrumb.tsx';
import type { BreadcrumbItem } from '../components/RegionBreadcrumb.tsx';

const BREADCRUMB_PREFIX: ReadonlyArray<BreadcrumbItem> = [
  { label: 'Atlas', to: '/' },
  { label: 'Browse', to: '/browse' },
];

export function Region() {
  const params = useParams<{ regionSlug: string }>();
  const slug = params.regionSlug ?? '';
  const query = useQuery<RegionDetail, ApiError>({
    queryKey: queryKeys.region(slug),
    queryFn: ({ signal }) => getRegion(slug, { signal }),
    enabled: slug.length > 0,
  });

  useDocumentTitle(
    query.data
      ? `${query.data.region.name} — Urbanist Atlas`
      : 'Loading region — Urbanist Atlas',
  );

  // Breadcrumb wants ancestors broad-first (left-to-right reads
  // root → leaf), but the API hands them back closest-first.
  // Reverse a copy so the API contract stays leaf-centric and the
  // SPA owns its display ordering.
  const ancestorsRootFirst = query.data
    ? [...query.data.ancestry].reverse()
    : [];
  const currentLabel = query.data ? query.data.region.name : 'Region';
  const totalOrgs = query.data
    ? query.data.local.length + query.data.regional.length
    : 0;
  const metaRight = query.data
    ? `${totalOrgs} ${totalOrgs === 1 ? 'org' : 'orgs'} indexed`
    : 'Region report';

  return (
    <>
      <RegionBreadcrumb
        prefix={BREADCRUMB_PREFIX}
        ancestors={ancestorsRootFirst}
        current={currentLabel}
        metaRight={metaRight}
      />
      <RegionBody query={query} />
    </>
  );
}

function RegionBody({ query }: { query: UseQueryResult<RegionDetail, ApiError> }) {
  return (
    <QueryState
      query={query}
      loading="Loading region…"
      className="mt-48"
      error={(e) =>
        e.status === 404 ? (
          <div className="lede mt-48">
            <div className="eyebrow">
              § Region report<span className="eyebrow-rule" />
            </div>
            <h1>
              This region <span className="accent">isn&rsquo;t in the atlas yet.</span>
            </h1>
            <p className="deck">
              Try <Link to="/browse">browse</Link> for the regions we have indexed,
              or <Link to="/submit">file a tip</Link> if you know advocates here.
            </p>
          </div>
        ) : undefined
      }
    >
      {(data) => <RegionContent data={data} />}
    </QueryState>
  );
}

function RegionContent({ data }: { data: RegionDetail }) {
  const { region, local, regional, ancestry, descendant_region_names } = data;
  const kindLabel = regionKindLabel(region.kind);
  const totalOrgs = local.length + regional.length;

  // Build a slug → display-name map so Entry can render "Matched
  // via Brooklyn" instead of "Matched via brooklyn-ny". Seeded from
  // the focus region, its ancestry walk, and the server-provided
  // descendant_region_names map (which covers any descendant slug
  // referenced by matched_region_slugs on the bucketed orgs).
  const regionNameBySlug = new Map<string, string>();
  regionNameBySlug.set(region.slug, region.name);
  for (const r of ancestry) {
    regionNameBySlug.set(r.slug, r.name);
  }
  for (const [slug, name] of Object.entries(descendant_region_names)) {
    regionNameBySlug.set(slug, name);
  }

  // Parent names for the rail copy. Pulled from ancestry (which
  // carries resolved display names) by intersecting with the focus
  // region's direct parents. Falls back to empty when there's no
  // overlap (top-of-hierarchy regions).
  const parentNames = ancestry
    .filter((r) => region.parent_slugs.includes(r.slug))
    .map((r) => r.name);

  return (
    <>
      <div className="lede mt-48">
        <div className="eyebrow">
          § {kindLabel} report · {region.country}
          <span className="eyebrow-rule" />
        </div>
        <h1>
          {region.name}
          <span className="accent">.</span>
        </h1>
        <p className="deck">
          {totalOrgs === 0
            ? `The Atlas hasn’t indexed any organizations in scope for ${region.name} yet — but the region is on the map.`
            : `${totalOrgs} indexed ${totalOrgs === 1 ? 'group' : 'groups'} working in or covering ${region.name}. Local entries are nearest; regional entries cover wider footprints that include this region.`}
        </p>
        <div className="byline">
          <span>{region.country}</span>
          <span className="crumb-sep">·</span>
          <span>
            Region slug <span className="em">{region.slug}</span>
          </span>
        </div>
      </div>

      <div className="spread mt-32">
        <main>
          {totalOrgs === 0 ? (
            <p className="results-state mt-24">
              No organizations indexed yet for {region.name}.{' '}
              <Link to="/submit">File a tip.</Link>
            </p>
          ) : (
            <div className="mt-24">
              <EntryList
                local={local}
                regional={regional}
                regionNameBySlug={regionNameBySlug}
              />
            </div>
          )}
          <div className="editors-note mt-32">
            <div className="label">Know a group we&rsquo;re missing?</div>
            <p>
              The Atlas adds groups one editorial decision at a time.{' '}
              <Link to="/submit">File a tip</Link> if a coalition is missing —
              see <Link to="/about#methodology">our criteria</Link>.
            </p>
          </div>
        </main>

        <aside className="rail">
          <div className="rail-block">
            <div className="rail-kicker">About this {kindLabel.toLowerCase()}</div>
            <p>
              The Atlas indexes {region.name} as a {kindLabel.toLowerCase()}
              {parentNames.length > 0
                ? `, sitting under ${parentNames.join(' · ')}.`
                : '.'}{' '}
              This page lists orgs anchored to {region.name} itself or to
              regions it contains. For the wider footprint above (state,
              multi-state coalitions), use the front-page postal lookup.
            </p>
            <p className="mb-0">
              Looking up by postal code?{' '}
              <Link to="/">Use the front-page lookup</Link>.
            </p>
          </div>
          {totalOrgs > 0 ? (
            <div className="rail-block amber">
              <div className="rail-kicker">By the numbers</div>
              <ul>
                <li>
                  <strong>{local.length}</strong> local{' '}
                  {local.length === 1 ? 'entry' : 'entries'}
                </li>
                <li>
                  <strong>{regional.length}</strong> regional{' '}
                  {regional.length === 1 ? 'entry' : 'entries'}
                </li>
                <li>
                  <strong>{countTags([...local, ...regional])}</strong> distinct editorial tags
                </li>
                <li>
                  Region kind{' '}
                  <strong>
                    <code>{region.kind}</code>
                  </strong>
                </li>
              </ul>
            </div>
          ) : null}
          <div className="rail-block muted">
            <div className="rail-kicker">Companion pages</div>
            <ul className="plain">
              <li>
                <Link to="/browse">Browse the atlas</Link>
              </li>
              <li>
                <Link to="/about">About the Atlas</Link>
              </li>
              <li>
                <Link to="/submit">Submissions desk</Link>
              </li>
            </ul>
          </div>
        </aside>
      </div>
    </>
  );
}

function countTags(orgs: ReadonlyArray<LookupOrg>): number {
  const s = new Set<string>();
  for (const o of orgs) for (const t of o.tags) s.add(t);
  return s.size;
}
