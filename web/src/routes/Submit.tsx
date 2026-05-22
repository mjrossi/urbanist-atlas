/**
 * `/submit` — placeholder page advertised in the nav but not yet
 * wired to a backend submission endpoint. The real submission flow
 * lands with the Phase 2 API-key program (roadmap slices #5 + #13);
 * until then this page sets honest expectations and points
 * contributors at GitHub issues, which mirrors the About page's
 * contact line.
 *
 * Why a real placeholder instead of falling through to the 404: the
 * "Page not in this edition" treatment is reserved for genuinely
 * unknown URLs. Using it as a stand-in for a known-future route
 * confuses readers and undermines the broadsheet metaphor.
 */
export function Submit() {
  return (
    <div className="page">
      <header className="page-header">
        <h1>The submissions desk opens with Phase 2.</h1>
        <p>
          <em>
            Public submissions arrive alongside the free-key API program —
            the next slice on the roadmap after this one.
          </em>
        </p>
      </header>

      <section>
        <h2>What’s coming</h2>
        <p>
          A short form here that takes an organization’s name, website, a
          one-line description, the region(s) it serves, and the tags that
          apply. Submissions land in a moderation queue; entries that
          clear editorial review become part of the directory and appear
          on the matching region pages.
        </p>
        <p>
          Phase 2 also opens self-serve API keys with a free tier, so
          third parties can build on top of the dataset without going
          through the project frontend.
        </p>
      </section>

      <section>
        <h2>In the meantime</h2>
        <p>
          If you know of an organization the Atlas should index, or you
          spot a correction in an existing entry, please file an issue on
          the{' '}
          <a href="https://github.com/mjrossi/urbanist-atlas">
            public repository
          </a>
          . Until the submissions queue exists, that’s the staffed channel
          and the one that won’t get lost.
        </p>
      </section>
    </div>
  );
}
