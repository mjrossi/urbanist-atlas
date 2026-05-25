import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

/**
 * `/colophon` — the receipts page. Pinned data-source vintages, the
 * stack lineup, the typography credit, and the dataset attribution
 * block shown verbatim as the API returns it. Pure content; no data
 * fetching. Same `.page` shell as About.
 */
export function Colophon() {
  useDocumentTitle('Colophon — Urbanist Atlas');
  return (
    <div className="page">
      <header className="page-header">
        <h1>Colophon</h1>
        <p>
          <em>
            What the Atlas is built from — the upstream data, the stack, the
            type, the licensing. A short page of receipts for the curious.
          </em>
        </p>
      </header>

      <section>
        <h2>Data sources</h2>
        <p>
          United States postal-code geography comes from the{' '}
          <a href="https://www.census.gov">U.S. Census Bureau</a>’s ZIP Code
          Tabulation Area crosswalks (2020 vintage), backfilled by the{' '}
          <a href="https://www.huduser.gov/portal/dataset/uspszip-api.html">
            HUD USPS ZIP-to-County crosswalk
          </a>{' '}
          (2026 Q1 release) for the ~9,000 operational ZIPs that exist only as
          P.O. boxes, single buildings, or APO/FPO military codes. Metropolitan
          regions come from the Census Bureau’s CBSA delineation file (July
          2023).
        </p>
        <p>
          Canadian postal-code geography and metropolitan regions come from{' '}
          <a href="https://www.statcan.gc.ca">Statistics Canada</a>’s Forward
          Sortation Area and Census Metropolitan Area boundary files (2021
          census).
        </p>
        <p>
          Organizations, editorial overrides, and the curated city/borough
          region graph are hand-maintained in TOML files in the{' '}
          <a href="https://github.com/mjrossi/urbanist-atlas">public repository</a>.
          There is no algorithmic ingestion. The directory grows one editorial
          decision at a time.
        </p>
      </section>

      <section>
        <h2>Stack</h2>
        <p>
          The API is a small Go service — standard-library-first, with chi for
          HTTP routing, sqlc for type-safe SQL, pgx for the Postgres driver,
          and goose for migrations. It runs on{' '}
          <a href="https://fly.io">Fly.io</a> in Virginia, with a sibling
          Postgres 17 app on a 1 GB volume reached over Fly’s private network.
          Nightly <code>pg_dump</code> backups land in Cloudflare R2 with
          thirty-day retention.
        </p>
        <p>
          The web app is a React + Vite SPA, with TanStack Query for server
          state and React Router for navigation. It deploys to Cloudflare
          Workers + Pages as static assets with SPA fallback. The wire
          contract between the two halves lives in a single OpenAPI document
          that both sides codegen from — no hand-rolled types on either edge.
        </p>
        <p>
          Continuous integration and deploys run on GitHub Actions. The
          maintainer pushes to <code>main</code>; the API redeploys
          automatically after migrations run, the web app builds and uploads,
          and a post-deploy smoke test verifies the live API. The roadmap and
          the editorial decision log are in the public repository alongside
          the code.
        </p>
      </section>

      <section>
        <h2>Type</h2>
        <p>
          Headlines are set in <strong>Fraunces</strong>, the variable-axis
          serif used in <a href="https://mjrossi.com/blog">Urbanist Lexicon</a>{' '}
          (this project’s sibling publication, from which the broadsheet
          identity is inherited). Body copy is{' '}
          <strong>Source Serif 4</strong>; user-interface chrome (the
          navigation, the search box, the form fields) is{' '}
          <strong>Inter</strong>. All three families are self-hosted via the
          Fontsource variable packages — no external font requests on any
          page.
        </p>
      </section>

      <section>
        <h2>Licensing</h2>
        <p>
          The code is released under <strong>Apache 2.0</strong>. Documentation
          and prose are <strong>CC BY-SA 4.0</strong>. The dataset itself —
          every organization entry, every region row, every postal-code
          mapping — is released under the{' '}
          <a href="https://opendatacommons.org/licenses/odbl/">
            Open Database License (ODbL) 1.0
          </a>
          .
        </p>
        <p>
          Every successful response from the public API carries the
          attribution in-band as HTTP headers, and collection responses also
          carry it in a <code>meta</code> envelope, so downstream consumers
          can’t miss the share-alike obligation. The block as it appears on
          the wire:
        </p>
        <pre className="colophon-attribution">
          <code>
{`X-Data-License: ODbL-1.0
X-Data-Attribution: https://urbanistatlas.com

{
  "meta": {
    "license": "ODbL-1.0",
    "attribution_url": "https://urbanistatlas.com",
    "generated_at": "..."
  },
  "data": [...]
}`}
          </code>
        </pre>
      </section>

      <section>
        <h2>Editorial cadence</h2>
        <p>
          The Atlas indexes organizations whose primary work is transit
          advocacy or safe-streets advocacy, in the United States and Canada
          (v1). National umbrella organizations are filtered from the default
          local search; they’re easy to find on their own, and the Atlas
          earns its keep on the harder local-and-regional layer. Housing,
          consultancies, and academic centres are out of scope unless they
          double as a membership advocacy organization. The criteria are
          spelled out in detail on the{' '}
          <a href="/about">About page</a>.
        </p>
        <p>
          Listings drift — websites lapse, coalitions reorganize, chapters
          fold. The editor runs a link-rot pass with the bundled{' '}
          <code>just linkcheck</code> tool before each significant editorial
          update, triages the report, and ships corrections as data-only
          commits. Corrections from readers are read and applied by the same
          editor; the channel is the{' '}
          <a href="https://github.com/mjrossi/urbanist-atlas/issues/new">
            issue tracker
          </a>{' '}
          today, and a moderated submission queue ships with the Phase 2
          API-key program.
        </p>
      </section>
    </div>
  );
}
