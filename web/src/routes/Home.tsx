import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { SearchBox } from '../components/SearchBox.tsx';
import { ApiError, listMetros, listRecent } from '../lib/api.ts';
import type { MetroSummary, Org } from '../lib/api.ts';
import { groupCountLabel } from '../lib/format.ts';
import { queryKeys } from '../lib/queryKeys.ts';

/** How many metros the home-page aside shows before linking to /browse. */
const METROS_LIMIT = 6;
/** How many recent orgs the home-page aside shows. */
const RECENT_LIMIT = 5;

/**
 * Home page. Two-column broadsheet: a lede column with the search
 * box and a drop-cap paragraph framing the project; an aside column
 * with two cards (browse-by-metro + recently-added) backed by
 * `/metros` and `/recent` respectively.
 *
 * The aside cards degrade gracefully — a downed dependency surfaces
 * "Temporarily unavailable" rather than breaking the rest of the
 * page — so the homepage stays usable when only the data tier is
 * troubled.
 */
export function Home() {
  const metros = useQuery<MetroSummary[], ApiError>({
    queryKey: queryKeys.metros(),
    queryFn: ({ signal }) => listMetros({ signal }),
  });
  const recent = useQuery<Org[], ApiError>({
    queryKey: queryKeys.recent(),
    queryFn: ({ signal }) => listRecent({ signal }),
  });

  return (
    <div className="broadsheet-body">
      <section className="col-lede" aria-labelledby="home-lede">
        <h2 id="home-lede" className="section-label">
          Start with a postal code
        </h2>
        <SearchBox />
        <p className="dropcap">
          The Urbanist Atlas catalogues the people in your city, your county,
          your region who are organizing — patiently, stubbornly, sometimes
          gloriously — for safer streets and better transit. Type a US ZIP or a
          Canadian postal code and we will name them for you.
        </p>
        <p>
          We index local and regional advocates only. National outfits do plenty
          of good work, but they are easy to find on their own; the harder
          search is for the neighbourhood committee three blocks from your
          door.
        </p>
      </section>
      <div className="broadsheet-gutter" aria-hidden="true" />
      <aside className="col-aside" aria-label="Other ways in">
        <AsideCard
          label="Browse by metro"
          query={metros}
          loadingText="Loading metros…"
          fallbackCopy={
            <p>
              A directory of metropolitan regions with the number of groups
              indexed in each. Useful when you want to wander rather than
              search.
            </p>
          }
          renderList={(data) => <MetrosList items={data.slice(0, METROS_LIMIT)} />}
        />
        <AsideCard
          label="Recently added"
          query={recent}
          loadingText="Loading recent…"
          fallbackCopy={
            <p>
              The newest organizations to clear the submission queue. A
              standing invitation to discover an effort you haven’t heard of
              yet.
            </p>
          }
          renderList={(data) => <RecentList items={data.slice(0, RECENT_LIMIT)} />}
        />
      </aside>
    </div>
  );
}

/**
 * A homepage aside card with the same loading / error / empty /
 * success state ladder. Renders the section label, then one of:
 *
 *   - loadingText alone (no status pill — the spinner copy is enough)
 *   - fallbackCopy + "Temporarily unavailable" pill on query error
 *   - fallbackCopy + "Nothing indexed yet" pill when the response is empty
 *   - renderList(data) on success
 *
 * Two card variants share this shape — extracting the helper keeps
 * the two paths honest about which states they show.
 */
function AsideCard<T>({
  label,
  query,
  loadingText,
  fallbackCopy,
  renderList,
}: {
  label: string;
  query: UseQueryResult<T[], ApiError>;
  loadingText: string;
  fallbackCopy: ReactNode;
  renderList: (data: T[]) => ReactNode;
}) {
  return (
    <div className="aside-card">
      <span className="section-label">{label}</span>
      {renderAsideBody(query, loadingText, fallbackCopy, renderList)}
    </div>
  );
}

function renderAsideBody<T>(
  query: UseQueryResult<T[], ApiError>,
  loadingText: string,
  fallbackCopy: ReactNode,
  renderList: (data: T[]) => ReactNode,
): ReactNode {
  if (query.isPending) {
    return <p>{loadingText}</p>;
  }
  if (query.isError) {
    return (
      <>
        {fallbackCopy}
        <span className="aside-card-status">Temporarily unavailable</span>
      </>
    );
  }
  if (query.data.length === 0) {
    return (
      <>
        {fallbackCopy}
        <span className="aside-card-status">Nothing indexed yet</span>
      </>
    );
  }
  return renderList(query.data);
}

function MetrosList({ items }: { items: MetroSummary[] }) {
  return (
    <>
      <ul className="entry-list" aria-label="Top metros">
        {items.map((m) => (
          <li key={m.region.slug} className="entry">
            <div className="entry-header">
              <h3 className="entry-name">
                <Link to={`/m/${m.region.slug}`}>{m.region.name}</Link>
              </h3>
              <span className="entry-domain">{groupCountLabel(m.org_count)}</span>
            </div>
          </li>
        ))}
      </ul>
      <p>
        <Link to="/browse">Browse all metros →</Link>
      </p>
    </>
  );
}

function RecentList({ items }: { items: Org[] }) {
  return (
    <ul className="entry-list" aria-label="Recently added">
      {items.map((org) => (
        <li key={org.id} className="entry">
          <div className="entry-header">
            <h3 className="entry-name">
              <a href={org.website_url} target="_blank" rel="noopener noreferrer">
                {org.name}
              </a>
            </h3>
          </div>
          <p className="entry-desc">{org.short_desc}</p>
        </li>
      ))}
    </ul>
  );
}
