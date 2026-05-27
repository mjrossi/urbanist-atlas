import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import { Link, useParams } from 'react-router';
import { ApiError, getOrg } from '../lib/api.ts';
import type { Org as OrgT } from '../lib/api.ts';
import { domainOf } from '../lib/format.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { isBrowseableKind, isMetroKind } from '../lib/regionKind.ts';
import { QueryState } from '../components/QueryState.tsx';

export function Org() {
  const params = useParams<{ slug: string }>();
  const slug = params.slug ?? '';
  const query = useQuery<OrgT, ApiError>({
    queryKey: queryKeys.org(slug),
    queryFn: ({ signal }) => getOrg(slug, { signal }),
    enabled: slug.length > 0,
  });

  useDocumentTitle(
    query.data
      ? `${query.data.name} — Urbanist Atlas`
      : 'Loading organization — Urbanist Atlas',
  );

  return (
    <>
      <OrgKicker query={query} />
      <OrgBody query={query} />
    </>
  );
}

function OrgKicker({ query }: { query: UseQueryResult<OrgT, ApiError> }) {
  const org = query.data;
  return (
    <div className="kicker">
      <div>
        <Link to="/">Atlas</Link>
        <span className="crumb-sep">/</span>
        <Link to="/browse">Browse</Link>
        <span className="crumb-sep">/</span>
        <span className="crumb-here">{org ? org.name : 'Organization'}</span>
      </div>
      <div>{org ? `Slug · ${org.slug}` : 'Organization file'}</div>
    </div>
  );
}

function OrgBody({ query }: { query: UseQueryResult<OrgT, ApiError> }) {
  return (
    <QueryState
      query={query}
      loading="Loading organization…"
      marginTop={48}
      error={(e) =>
        e.status === 404 ? (
          <div className="lede" style={{ marginTop: 48 }}>
            <div className="eyebrow">
              § Organization file<span className="eyebrow-rule" />
            </div>
            <h1>
              This organization <span className="accent">isn&rsquo;t in the atlas yet.</span>
            </h1>
            <p className="deck">
              Try <Link to="/browse">browse</Link> for the metros we have indexed,
              or <Link to="/submit">file a tip</Link> if you know who&rsquo;s
              doing the work here.
            </p>
          </div>
        ) : undefined
      }
    >
      {(org) => <OrgContent org={org} />}
    </QueryState>
  );
}

function OrgContent({ org }: { org: OrgT }) {
  const domain = domainOf(org.website_url);
  const primaryMetro = org.regions.find((r) => isMetroKind(r.kind));
  const tagsTopline = org.tags.slice(0, 3).map(prettyTag).join(' · ');

  const atAGlance = (
    <div className="rail-block amber">
      <div className="rail-kicker">At a glance</div>
      <ul>
        <li>
          <strong>{org.regions.length}</strong>{' '}
          {org.regions.length === 1 ? 'region' : 'regions'} served
        </li>
        <li>
          <strong>{org.tags.length}</strong> editorial{' '}
          {org.tags.length === 1 ? 'tag' : 'tags'}
        </li>
        <li>
          <strong>
            <code>{org.slug}</code>
          </strong>{' '}
          directory slug
        </li>
        {primaryMetro ? (
          <li>
            Primary metro · <strong>{primaryMetro.name}</strong>
          </li>
        ) : null}
      </ul>
    </div>
  );

  return (
    <>
      <header className="org-feature">
        <div className="dateline">
          {tagsTopline ? (
            <>
              <span>§ {tagsTopline}</span>
              <span className="sep">·</span>
            </>
          ) : null}
          <span>
            {primaryMetro
              ? `${primaryMetro.name}, ${primaryMetro.country}`
              : org.regions[0]
                ? `${org.regions[0].name}, ${org.regions[0].country}`
                : 'See entry'}
          </span>
        </div>
        <h1 className="name">{org.name}</h1>
        <p className="url-line">
          →{' '}
          <a href={org.website_url} target="_blank" rel="noopener noreferrer">
            {domain ?? org.website_url}
          </a>
        </p>
        <div className="deck-row">
          <p className="deck">{org.short_desc}</p>
          <div style={{ paddingTop: 6 }}>
            {org.tags.length > 0 ? (
              <ul className="tag-list">
                {org.tags.map((tag, i) => (
                  <li key={tag}>
                    <span className={`tag${i === 0 ? ' solid' : ''}`}>
                      {prettyTag(tag)}
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        </div>
        <div className="meta-strip">
          {primaryMetro ? (
            <div className="item">
              <div>Primary metro</div>
              <span className="val">
                <Link to={`/region/${encodeURIComponent(primaryMetro.slug)}`}>
                  {primaryMetro.name}
                </Link>
              </span>
            </div>
          ) : null}
          <div className="item">
            <div>Country</div>
            <span className="val">
              {primaryMetro?.country ?? org.regions[0]?.country ?? '—'}
            </span>
          </div>
          <div className="item">
            <div>Regions served</div>
            <span className="val">{org.regions.length}</span>
          </div>
          <div className="item">
            <div>Tags</div>
            <span className="val">{org.tags.length}</span>
          </div>
          <div className="item">
            <div>Slug</div>
            <span className="val">
              <code style={{ fontFamily: 'var(--mono)', fontSize: 13 }}>
                {org.slug}
              </code>
            </span>
          </div>
          {org.contact_url ? (
            <div className="item">
              <div>Contact</div>
              <span className="val amber">
                <a href={org.contact_url} target="_blank" rel="noopener noreferrer">
                  open form →
                </a>
              </span>
            </div>
          ) : null}
        </div>
      </header>

      <div className="spread" style={{ marginTop: 0 }}>
        <main className="prose">
          <div className="glance-mobile">{atAGlance}</div>
          <div className="section-kicker">§ I — The entry</div>
          <h2>The directory record.</h2>
          <div className="h2-rule" />
          <p className="lead drop">{org.short_desc}</p>
          <p>
            For current campaigns and ways to plug in, open{' '}
            <a href={org.website_url} target="_blank" rel="noopener noreferrer">
              {domain ?? org.website_url}
            </a>
            .{' '}
            {org.regions.length > 0
              ? `Below: the regions the Atlas files ${org.name} under.`
              : `${org.name} doesn’t have a region attached yet — file a tip if you can place them.`}
          </p>

          {org.regions.length > 0 ? (
            <>
              <div className="section-kicker">§ II — Where they work</div>
              <h2>Regions served.</h2>
              <div className="h2-rule" />
              <ul className="org-regions-list">
                {org.regions.map((region) => (
                  <li key={region.id} className="org-region-item">
                    <div>
                      <div className="name">
                        {isBrowseableKind(region.kind) ? (
                          <Link to={`/region/${encodeURIComponent(region.slug)}`}>
                            {region.name}
                          </Link>
                        ) : (
                          region.name
                        )}
                      </div>
                      <div className="meta">
                        {region.country} · {region.kind}
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            </>
          ) : null}

          <div className="editors-note" style={{ marginTop: 16 }}>
            <div className="label">Something off?</div>
            <p>
              We try to verify every entry on a rolling cadence. If a campaign
              listed here has wrapped, leadership has changed, or a fact looks
              wrong — <Link to="/submit">file a correction</Link> and
              we&rsquo;ll fix it.
            </p>
          </div>
        </main>

        <aside className="rail">
          <div className="glance-desktop">{atAGlance}</div>
          <div className="rail-block">
            <div className="rail-kicker">Filed by</div>
            <p>
              An entry in the Urbanist Atlas — curated by hand against the{' '}
              <Link to="/about#methodology">inclusion criteria</Link>, verified
              against public sources. Spot something off?{' '}
              <Link to="/submit">File a correction</Link>.
            </p>
          </div>
          <div className="rail-block muted">
            <div className="rail-kicker">Companion pages</div>
            <ul className="plain">
              <li>
                <Link to="/browse">Browse the atlas</Link>
              </li>
              {primaryMetro ? (
                <li>
                  <Link to={`/region/${encodeURIComponent(primaryMetro.slug)}`}>
                    Other groups in {primaryMetro.name}
                  </Link>
                </li>
              ) : null}
              <li>
                <Link to="/about">About the Atlas</Link>
              </li>
            </ul>
          </div>
        </aside>
      </div>
    </>
  );
}

function prettyTag(tag: string): string {
  return tag.replace(/-/g, ' ');
}
