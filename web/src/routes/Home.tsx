import { SearchBox } from '../components/SearchBox.tsx';

/**
 * Home page. Two-column broadsheet: a lede column with the search
 * box and a drop-cap paragraph framing the project, and an aside
 * column with two placeholder cards for features that land in later
 * roadmap slices.
 *
 * The placeholders are intentionally static — the `/metros` and
 * `/recent` endpoints they'll eventually call don't exist yet
 * (slice #6).
 */
export function Home() {
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
        <div className="aside-card">
          <span className="section-label">Browse by metro</span>
          <p>
            A directory of metropolitan regions with the number of groups
            indexed in each. Useful when you want to wander rather than
            search.
          </p>
          <span className="aside-card-status">Coming soon</span>
        </div>
        <div className="aside-card">
          <span className="section-label">Recently added</span>
          <p>
            The newest organizations to clear the submission queue. A standing
            invitation to discover an effort you haven't heard of yet.
          </p>
          <span className="aside-card-status">Coming soon</span>
        </div>
      </aside>
    </div>
  );
}
