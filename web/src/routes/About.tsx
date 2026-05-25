import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

/**
 * `/about` — mission / methodology / criteria / acknowledgments for
 * the Urbanist Atlas. Pure content; no data fetching.
 *
 * Uses the existing `.page` single-column treatment from global.css
 * (originally introduced in slice #14 for the Browse/Metro pages).
 * The four `<section>` blocks pick up the section-divider border
 * and the small-caps `<h2>` styling automatically.
 */
export function About() {
  useDocumentTitle('About — Urbanist Atlas');
  return (
    <div className="page">
      <header className="page-header">
        <h1>About the Urbanist Atlas</h1>
        <p>
          A directory of transit and safe-streets advocacy organizations,
          searchable by US ZIP or Canadian postal code. A companion volume to{' '}
          <a href="https://mjrossi.com/blog">Urbanist Lexicon</a>.
        </p>
      </header>

      <section>
        <h2>Mission</h2>
        <p>
          The Urbanist Atlas exists for a narrow reason: when someone moves to a
          new city — or wakes up one morning angry about a dangerous
          intersection three blocks from home — they should be able to find the
          people already organizing for better streets and better transit
          nearby, in under a minute.
        </p>
        <p>
          National advocacy outfits do plenty of good work, but they are easy
          to find on their own. The harder search is for the neighbourhood
          committee, the metro-area rider alliance, the county-level Vision
          Zero coalition. The Atlas indexes those.
        </p>
      </section>

      <section>
        <h2>Methodology</h2>
        <p>
          Entries are curated by hand. Each organization in the Atlas was
          reviewed by an editor against the criteria below before being added.
          There is no algorithmic ingestion and no automated scraping; the
          directory grows one editorial decision at a time.
        </p>
        <p>
          The underlying dataset is licensed under the{' '}
          <a href="https://opendatacommons.org/licenses/odbl/">
            Open Database License (ODbL) 1.0
          </a>{' '}
          and will be available for download once the API opens to the public.
          A public submission flow arrives with the Phase 2 API-key program;
          until then, suggestions and corrections are welcome as issues on the{' '}
          <a href="https://github.com/mjrossi/urbanist-atlas">
            public repository
          </a>
          .
        </p>
      </section>

      <section>
        <h2>Independence and corrections</h2>
        <p>
          The Atlas is an independent reference work. It is not affiliated
          with, endorsed by, or representing any of the organizations listed
          here. Listings are based on publicly available information and
          editorial judgment, and they can go stale — a website lapses, a
          coalition reorganizes, a chapter folds.
        </p>
        <p>
          If you spot a broken link, an outdated description, an organization
          that no longer exists, or a listing that misrepresents the work of
          the people it indexes, please{' '}
          <a href="https://github.com/mjrossi/urbanist-atlas/issues/new">
            open an issue on GitHub
          </a>
          . Corrections are read and applied by the same editor who maintains
          the directory.
        </p>
      </section>

      <section>
        <h2>Criteria for inclusion</h2>
        <p>
          The Atlas indexes organizations whose primary work is{' '}
          <strong>transit advocacy</strong> (riders’ alliances, bus and rail
          coalitions, transit committees) or{' '}
          <strong>safe-streets advocacy</strong> (Vision Zero groups,
          neighbourhood traffic-calming coalitions, pedestrian and cycling
          alliances).
        </p>
        <p>
          Out of scope, deliberately: housing and YIMBY organizations, even
          when their priorities overlap. The directory’s job is to be useful,
          not exhaustive, and a tighter scope makes the local-search
          experience cleaner. Consultancies, think tanks, and academic centres
          are also out of scope unless they double as a membership advocacy
          organization.
        </p>
      </section>

      <section id="for-developers">
        <h2>For developers</h2>
        <p>
          The Atlas runs on a small Go service whose entire surface is
          described by the OpenAPI document at{' '}
          <a href="https://api.urbanistatlas.com/api/v1/openapi.yaml">
            <code>/api/v1/openapi.yaml</code>
          </a>
          . It is deliberately small — postal-code lookup, metro browse, org
          detail — and the dataset is licensed under the{' '}
          <a href="https://opendatacommons.org/licenses/odbl/">
            Open Database License (ODbL) 1.0
          </a>{' '}
          for downstream reuse with attribution and share-alike. See the{' '}
          <a href="/colophon">colophon</a> for the full data-source and
          licensing picture.
        </p>
        <p>
          During the Phase 1 dogfood window the API sits behind a shared-secret
          gate while we shake out schema and query bugs against the QA
          frontend. The Phase 2 program will open self-serve free keys. If
          you’d like an early key before Phase 2 — to build a directory
          widget, a regional dashboard, anything — write to{' '}
          <a href="mailto:hello@urbanistatlas.com?subject=Atlas%20API%20early%20access">
            hello@urbanistatlas.com
          </a>{' '}
          and we’ll set one up by hand.
        </p>
      </section>

      <section>
        <h2>Acknowledgments</h2>
        <p>
          Postal-code geography in the United States comes from the{' '}
          <a href="https://www.census.gov">U.S. Census Bureau</a>’s ZIP Code
          Tabulation Areas (ZCTAs); Canadian postal-code geography comes from{' '}
          <a href="https://www.statcan.gc.ca">Statistics Canada</a>’s Postal
          Code Conversion File. Both are public-domain or open-licensed.
        </p>
        <p>
          Above all, this directory is built on the work of the organizations
          it indexes — the volunteers, organizers, and staff who show up to
          their city’s transportation meetings, week after week, and patiently
          argue for better.
        </p>
      </section>
    </div>
  );
}
