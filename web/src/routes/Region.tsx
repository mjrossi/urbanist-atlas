import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import { Link, useParams } from 'react-router';
import { ApiError, getRegion } from '../lib/api.ts';
import type { RegionDetail, Org } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { regionKindLabel } from '../lib/regionKind.ts';

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

  const regionReport = query.data
    ? `${regionKindLabel(query.data.region.kind)} report`
    : 'Region report';

  return (
    <>
      <div className="kicker">
        <div>
          <Link to="/">Atlas</Link>
          <span className="crumb-sep">/</span>
          <Link to="/browse">Browse</Link>
          <span className="crumb-sep">/</span>
          <span className="crumb-here">
            {query.data ? query.data.region.name : 'Region'}
          </span>
        </div>
        <div>
          {query.data
            ? `${query.data.orgs.length} ${query.data.orgs.length === 1 ? 'org' : 'orgs'} indexed`
            : regionReport}
        </div>
      </div>
      <RegionBody query={query} />
    </>
  );
}

function RegionBody({ query }: { query: UseQueryResult<RegionDetail, ApiError> }) {
  if (query.isPending) {
    return (
      <p className="results-state" role="status" style={{ marginTop: 48 }}>
        Loading region…
      </p>
    );
  }
  if (query.isError) {
    if (query.error.status === 404) {
      return (
        <div className="lede" style={{ marginTop: 48 }}>
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
      );
    }
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

  const { region, orgs } = query.data;
  const kindLabel = regionKindLabel(region.kind);

  return (
    <>
      <div className="lede" style={{ marginTop: 48 }}>
        <div className="eyebrow">
          § {kindLabel} report · {region.country}
          <span className="eyebrow-rule" />
        </div>
        <h1>
          {region.name}
          <span className="accent">.</span>
        </h1>
        <p className="deck">
          {orgs.length === 0
            ? `The Atlas hasn’t indexed any organizations in ${region.name} yet — but the region is on the map.`
            : `${orgs.length} indexed ${orgs.length === 1 ? 'group' : 'groups'} pushing for safer streets and better transit across ${region.name}.`}
        </p>
        <div className="byline">
          <span>{region.country}</span>
          <span className="crumb-sep">·</span>
          <span>
            Region slug <span className="em">{region.slug}</span>
          </span>
          {region.parent_slugs.length > 0 ? (
            <>
              <span className="crumb-sep">·</span>
              <span>Parent {region.parent_slugs.join(' · ')}</span>
            </>
          ) : null}
        </div>
      </div>

      <div className="spread" style={{ marginTop: 32 }}>
        <main>
          <header className="section-break" style={{ marginTop: 0 }}>
            <span className="num">I.</span>
            <h2 className="title">Groups working in {region.name}.</h2>
            <span className="aside">From the directory</span>
          </header>
          {orgs.length === 0 ? (
            <p className="results-state">
              No organizations indexed yet for {region.name}.{' '}
              <Link to="/submit">File a tip.</Link>
            </p>
          ) : (
            orgs.map((org) => <OrgRow key={org.id} org={org} />)
          )}
          <div className="editors-note" style={{ marginTop: 32 }}>
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
              {region.parent_slugs.length > 0
                ? `, sitting under ${region.parent_slugs.join(' · ')}.`
                : '.'}{' '}
              Orgs tagged to sub-regions (cities, boroughs, counties)
              roll up here too.
            </p>
            <p style={{ marginBottom: 0 }}>
              Looking up by postal code?{' '}
              <Link to="/">Use the front-page lookup</Link>.
            </p>
          </div>
          {orgs.length > 0 ? (
            <div className="rail-block amber">
              <div className="rail-kicker">By the numbers</div>
              <ul>
                <li>
                  <strong>{orgs.length}</strong> indexed{' '}
                  {orgs.length === 1 ? 'group' : 'groups'}
                </li>
                <li>
                  <strong>{countTags(orgs)}</strong> distinct editorial tags
                </li>
                <li>
                  Region kind{' '}
                  <strong>
                    <code>{region.kind}</code>
                  </strong>
                </li>
                <li>
                  Slug{' '}
                  <strong>
                    <code>{region.slug}</code>
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

function OrgRow({ org }: { org: Org }) {
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
        <div className="foot">
          {org.contact_url ? (
            <a
              href={org.contact_url}
              target="_blank"
              rel="noopener noreferrer"
              style={{ color: 'var(--amber)', textDecoration: 'none' }}
            >
              Contact →
            </a>
          ) : null}
          <Link
            to={`/orgs/${encodeURIComponent(org.slug)}`}
            style={{ color: 'var(--amber)', textDecoration: 'none' }}
          >
            Open the org file →
          </Link>
        </div>
      </div>
    </article>
  );
}

function countTags(orgs: Org[]): number {
  const s = new Set<string>();
  for (const o of orgs) for (const t of o.tags) s.add(t);
  return s.size;
}

function domainOf(url: string): string | null {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return null;
  }
}
