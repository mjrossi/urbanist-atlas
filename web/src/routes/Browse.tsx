import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { ApiError, listMetros } from '../lib/api.ts';
import type { MetroSummary } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';

/**
 * `/browse` — single-column directory of every metro region with an
 * approved-org count. Reuses the same `.page` shell as the lookup
 * results page, but with a flat list of metros rather than the
 * Local/Regional split.
 *
 * Ordering matches what the API returns (`org_count` descending);
 * this component does not re-sort.
 */
export function Browse() {
  const query = useQuery<MetroSummary[], ApiError>({
    queryKey: queryKeys.metros(),
    queryFn: ({ signal }) => listMetros({ signal }),
  });

  return (
    <div className="page">
      <header className="page-header">
        <h1>Browse by metro</h1>
        <p>
          A directory of the metropolitan regions the Atlas has indexed so far,
          with the number of organizations active in each.
        </p>
        <p>
          Useful when you want to wander rather than search. Open a metro for
          the list of groups working there.
        </p>
      </header>
      <BrowseBody query={query} />
    </div>
  );
}

function BrowseBody({
  query,
}: {
  query: ReturnType<typeof useQuery<MetroSummary[], ApiError>>;
}) {
  if (query.isPending) {
    return (
      <p className="results-state" role="status">
        Loading metros…
      </p>
    );
  }
  if (query.isError) {
    const err = query.error;
    return (
      <p className="results-state error" role="alert">
        {err.message}
        {err.requestId ? (
          <span className="results-state-detail">request id: {err.requestId}</span>
        ) : null}
      </p>
    );
  }
  if (query.data.length === 0) {
    return (
      <p className="results-state">
        No metros indexed yet — try the search box at the top of every page.
      </p>
    );
  }
  return (
    <section aria-labelledby="browse-section">
      <h2 id="browse-section" className="section-label">
        Metros
      </h2>
      <ul className="entry-list">
        {query.data.map((m) => (
          <MetroRow key={m.region.slug} metro={m} />
        ))}
      </ul>
    </section>
  );
}

function MetroRow({ metro }: { metro: MetroSummary }) {
  const { region, org_count } = metro;
  return (
    <li className="entry">
      <div className="entry-header">
        <h3 className="entry-name">
          <Link to={`/m/${region.slug}`}>{region.name}</Link>
        </h3>
        <span className="entry-domain">{groupCountLabel(org_count)}</span>
      </div>
      <p className="entry-desc">{datelineFor(region)}</p>
    </li>
  );
}

function groupCountLabel(n: number): string {
  return n === 1 ? '1 group' : `${n} groups`;
}

function datelineFor(region: MetroSummary['region']): string {
  // Country first, parent slugs after, so a Portuguese metro reads
  // "PT · NUTS II Grande Lisboa" rather than the other way round. The
  // parent_slugs are just slugs (no names yet), so we render them in
  // small caps as a quiet hint about where this metro sits in the
  // graph; the canonical hierarchy lives on the metro detail page.
  const parts = [region.country];
  if (region.parent_slugs.length > 0) {
    parts.push(region.parent_slugs.join(' · '));
  }
  return parts.join(' · ');
}
