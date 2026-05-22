import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from 'react-router';
import { TagChip } from '../components/TagChip.tsx';
import { ApiError, getOrg } from '../lib/api.ts';
import type { Org as OrgT, Region } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';

/**
 * `/orgs/:slug` — the addressable detail page for one organization.
 * Single-column broadsheet `.page` treatment (same one `/about` and
 * `/m/:metroSlug` use). The view is mostly content-first: name,
 * website, short description, optional contact link, tags, and a list
 * of every region the org serves with metro-equivalent regions linked
 * to `/m/:slug` so a reader can pivot to the metro index.
 *
 * 404 from the backend renders the same "not in this edition" voice as
 * /m/:metroSlug, with a link back to /browse for orientation.
 */
export function Org() {
  const params = useParams<{ slug: string }>();
  const slug = params.slug ?? '';

  const query = useQuery<OrgT, ApiError>({
    queryKey: queryKeys.org(slug),
    queryFn: ({ signal }) => getOrg(slug, { signal }),
    enabled: slug.length > 0,
  });

  return (
    <div className="page">
      <OrgBody query={query} />
    </div>
  );
}

function OrgBody({
  query,
}: {
  query: ReturnType<typeof useQuery<OrgT, ApiError>>;
}) {
  if (query.isPending) {
    return (
      <p className="results-state" role="status">
        Loading organization…
      </p>
    );
  }
  if (query.isError) {
    const err = query.error;
    // useQuery<_, ApiError> is a TS hint only — react-query passes
    // whatever the queryFn rejected. Narrow with instanceof so a stray
    // non-ApiError (network TypeError, AbortError) doesn't render
    // `undefined` from missing fields.
    const apiErr = err instanceof ApiError ? err : null;
    if (apiErr?.status === 404) {
      return (
        <p className="results-state">
          This organization isn’t in the atlas yet — try{' '}
          <Link to="/browse">browse</Link> for the metros we have indexed.
        </p>
      );
    }
    return (
      <p className="results-state error" role="alert">
        {apiErr?.message ?? 'Something went wrong loading this organization.'}
        {apiErr?.requestId ? (
          <span className="results-state-detail">request id: {apiErr.requestId}</span>
        ) : null}
      </p>
    );
  }

  const org = query.data;
  const domain = domainOf(org.website_url);
  return (
    <>
      <header className="page-header">
        <h1>{org.name}</h1>
        <p>
          {/* Org data is admin-curated, so we render the website as a
              link even when domainOf can't extract a hostname (e.g. a
              scheme-less URL slipped past validation). The link text
              falls back to the raw URL, making bad data visible. */}
          <a href={org.website_url} target="_blank" rel="noopener noreferrer">
            {domain ?? org.website_url}
          </a>
          {org.contact_url ? (
            <>
              {' · '}
              <a href={org.contact_url} target="_blank" rel="noopener noreferrer">
                contact
              </a>
            </>
          ) : null}
        </p>
      </header>

      <section>
        <p className="entry-desc">{org.short_desc}</p>
        {org.tags.length > 0 ? (
          <ul className="entry-tags" aria-label="Tags">
            {org.tags.map((tag) => (
              <li key={tag}>
                <TagChip label={tag} />
              </li>
            ))}
          </ul>
        ) : null}
      </section>

      {org.regions.length > 0 ? (
        <section aria-labelledby="org-regions">
          <h2 id="org-regions" className="section-label">
            Serves
          </h2>
          <ul className="entry-list">
            {org.regions.map((r) => (
              <li key={r.id} className="entry">
                <div className="entry-header">
                  <h3 className="entry-name">{regionLink(r)}</h3>
                </div>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </>
  );
}

/**
 * Region kinds that map to a `/m/:metroSlug` index page. MUST stay in
 * lockstep with the server-side metroKinds map in
 * `api/pkg/atlas/metro_kinds.go` — the `/api/v1/metros/{slug}` endpoint
 * only serves regions whose kind is in that map, so a drift here
 * either dead-links (web has a kind the server rejects) or hides a
 * valid pivot (server serves a kind we don't link).
 */
const METRO_KINDS = new Set<string>([
  'us:metro',
  'ca:cma',
  'ca:regional-district',
  'pt:area-metropolitana',
]);

function regionLink(r: Region) {
  if (METRO_KINDS.has(r.kind)) {
    return <Link to={`/m/${encodeURIComponent(r.slug)}`}>{r.name}</Link>;
  }
  return <span>{r.name}</span>;
}

/** Same defensive URL→hostname helper as `Entry.tsx`, kept local to
 *  avoid a circular import between routes and components. */
function domainOf(url: string): string | null {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return null;
  }
}
