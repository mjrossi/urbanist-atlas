import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from 'react-router';
import { Entry } from '../components/Entry.tsx';
import { ApiError, getMetro } from '../lib/api.ts';
import type { LookupOrg, MetroDetail, Org } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';

/**
 * `/m/:metroSlug` — one metro region and the approved organizations
 * that serve it. Same classified-section vocabulary as the lookup
 * results page, but rooted at a metro rather than a postal code.
 *
 * 404 from the backend renders an inline empty-state with a link to
 * `/browse`; the dedicated 404 page is slice #15 territory.
 */
export function Metro() {
  const params = useParams<{ metroSlug: string }>();
  const slug = params.metroSlug ?? '';

  const query = useQuery<MetroDetail, ApiError>({
    queryKey: queryKeys.metro(slug),
    queryFn: ({ signal }) => getMetro(slug, { signal }),
    enabled: slug.length > 0,
  });

  return (
    <div className="page">
      <MetroBody query={query} />
    </div>
  );
}

function MetroBody({
  query,
}: {
  query: ReturnType<typeof useQuery<MetroDetail, ApiError>>;
}) {
  if (query.isPending) {
    return (
      <p className="results-state" role="status">
        Loading metro…
      </p>
    );
  }
  if (query.isError) {
    const err = query.error;
    if (err.status === 404) {
      return (
        <p className="results-state">
          This metro isn’t in the atlas yet — try{' '}
          <Link to="/browse">browse</Link> for the metros we have indexed.
        </p>
      );
    }
    return (
      <p className="results-state error" role="alert">
        {err.message}
        {err.requestId ? (
          <span className="results-state-detail">request id: {err.requestId}</span>
        ) : null}
      </p>
    );
  }

  const { region, orgs } = query.data;
  return (
    <>
      <MetroHeader region={region} />
      <section aria-labelledby="metro-orgs">
        <h2 id="metro-orgs" className="section-label">
          Organizations serving {region.name}
        </h2>
        {orgs.length === 0 ? (
          <p className="results-section-empty">
            No organizations indexed yet for {region.name}.{' '}
            <a href="/submit">Submit one.</a>
          </p>
        ) : (
          <ul className="entry-list">
            {orgs.map((org) => (
              <Entry
                key={org.id}
                org={asLookupOrg(org)}
                regionNameBySlug={emptyRegionMap}
              />
            ))}
          </ul>
        )}
      </section>
    </>
  );
}

function MetroHeader({ region }: { region: MetroDetail['region'] }) {
  return (
    <header className="page-header">
      <h1>{region.name}</h1>
      <p>
        {region.country}
        {region.parent_slugs.length > 0 ? ` · ${region.parent_slugs.join(' · ')}` : ''}
      </p>
    </header>
  );
}

/**
 * Project a base {@link Org} (returned by `/api/v1/metros/{slug}`)
 * onto the {@link LookupOrg} shape that {@link Entry} consumes. The
 * `matched_region_slugs` field is empty because this view isn't a
 * postal-code lookup — `Entry` already handles that case by omitting
 * the "via" subtitle (see `Entry.test.tsx`).
 *
 * Keeps `Entry`'s public signature untouched, per the slice's
 * non-goal of "no Entry/EntryList refactor".
 */
function asLookupOrg(org: Org): LookupOrg {
  return { ...org, matched_region_slugs: [] };
}

const emptyRegionMap = new Map<string, string>();
