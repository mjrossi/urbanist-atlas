import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { SearchBox } from '../components/SearchBox.tsx';
import { ApiError, listMetros, listRecent } from '../lib/api.ts';
import type { MetroSummary, Org } from '../lib/api.ts';
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
 * Each aside card renders a "Coming soon" affordance on error so a
 * downed dependency degrades gracefully without breaking the rest of
 * the page.
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
        <MetrosCard query={metros} />
        <RecentCard query={recent} />
      </aside>
    </div>
  );
}

function MetrosCard({
  query,
}: {
  query: ReturnType<typeof useQuery<MetroSummary[], ApiError>>;
}) {
  return (
    <div className="aside-card">
      <span className="section-label">Browse by metro</span>
      {renderMetrosBody(query)}
    </div>
  );
}

function renderMetrosBody(
  query: ReturnType<typeof useQuery<MetroSummary[], ApiError>>,
) {
  if (query.isPending) {
    return (
      <>
        <p>Loading metros…</p>
        <span className="aside-card-status">Coming soon</span>
      </>
    );
  }
  if (query.isError) {
    return (
      <>
        <p>
          A directory of metropolitan regions with the number of groups
          indexed in each. Useful when you want to wander rather than search.
        </p>
        <span className="aside-card-status">Coming soon</span>
      </>
    );
  }
  const list = query.data.slice(0, METROS_LIMIT);
  if (list.length === 0) {
    return (
      <>
        <p>
          A directory of metropolitan regions with the number of groups
          indexed in each. Useful when you want to wander rather than search.
        </p>
        <span className="aside-card-status">Coming soon</span>
      </>
    );
  }
  return (
    <>
      <ul className="entry-list" aria-label="Top metros">
        {list.map((m) => (
          <li key={m.region.slug} className="entry">
            <div className="entry-header">
              <h3 className="entry-name">
                <Link to={`/m/${m.region.slug}`}>{m.region.name}</Link>
              </h3>
              <span className="entry-domain">
                {m.org_count === 1 ? '1 group' : `${m.org_count} groups`}
              </span>
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

function RecentCard({
  query,
}: {
  query: ReturnType<typeof useQuery<Org[], ApiError>>;
}) {
  return (
    <div className="aside-card">
      <span className="section-label">Recently added</span>
      {renderRecentBody(query)}
    </div>
  );
}

function renderRecentBody(query: ReturnType<typeof useQuery<Org[], ApiError>>) {
  if (query.isPending) {
    return (
      <>
        <p>Loading recent…</p>
        <span className="aside-card-status">Coming soon</span>
      </>
    );
  }
  if (query.isError) {
    return (
      <>
        <p>
          The newest organizations to clear the submission queue. A standing
          invitation to discover an effort you haven’t heard of yet.
        </p>
        <span className="aside-card-status">Coming soon</span>
      </>
    );
  }
  const list = query.data.slice(0, RECENT_LIMIT);
  if (list.length === 0) {
    return (
      <>
        <p>
          The newest organizations to clear the submission queue. A standing
          invitation to discover an effort you haven’t heard of yet.
        </p>
        <span className="aside-card-status">Coming soon</span>
      </>
    );
  }
  return (
    <ul className="entry-list" aria-label="Recently added">
      {list.map((org) => (
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
