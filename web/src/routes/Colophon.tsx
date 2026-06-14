import { Link } from 'react-router';

import {
  MrsPeacockIllustration,
  PadThaiIllustration,
} from '../components/NewsroomCats.tsx';
import { PageBreadcrumb } from '../components/PageBreadcrumb.tsx';
import { openapiUrl } from '../lib/api.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

export function Colophon() {
  useDocumentTitle('Colophon — Urbanist Atlas');
  return (
    <>
      <PageBreadcrumb
        prefix={[{ label: 'Atlas', to: '/' }]}
        current="Colophon"
        meta="Volume I · 2026 Edition"
      />

      <div className="lede mt-48">
        <div className="eyebrow">
          § The colophon
          <span className="eyebrow-rule" />
        </div>
        <h1>
          What the Atlas <span className="accent">is built from.</span>
        </h1>
        <p className="deck">
          A short page of receipts. The upstream data, the stack, the type, the licensing
          — and the share-alike obligation the API carries in-band on every response.
        </p>
      </div>

      <div className="spread">
        <main className="prose">
          <div className="section-kicker">§ I — Data sources</div>
          <h2>Where the geography comes from.</h2>
          <div className="h2-rule" />
          <p>
            United States postal-code geography comes from the{' '}
            <a href="https://www.census.gov">U.S. Census Bureau</a>&rsquo;s ZIP Code
            Tabulation Area crosswalks (2020 vintage), backfilled by the{' '}
            <a href="https://www.huduser.gov/portal/dataset/uspszip-api.html">
              HUD USPS ZIP-to-County crosswalk
            </a>{' '}
            (2025 Q4 release) for the ~9,000 operational ZIPs that exist only as P.O.
            boxes, single buildings, or APO/FPO military codes. Metropolitan regions come
            from the Census Bureau&rsquo;s CBSA delineation file (July 2023).
          </p>
          <p>
            Canadian postal-code geography and metropolitan regions come from{' '}
            <a href="https://www.statcan.gc.ca">Statistics Canada</a>&rsquo;s Forward
            Sortation Area and Census Metropolitan Area boundary files (2021 census).
          </p>
          <p>
            Organizations, editorial overrides, and the curated city/borough region graph
            are hand-maintained in TOML files in the{' '}
            <a href="https://github.com/mjrossi/urbanist-atlas">public repository</a>.
            There is no algorithmic ingestion. The directory grows one editorial decision
            at a time.
          </p>

          <div className="section-kicker">§ II — Stack</div>
          <h2>How the Atlas runs.</h2>
          <div className="h2-rule" />
          <p>
            The API is a small Go service — standard-library-first, with chi for HTTP
            routing. The read path is stateless: at boot it loads the bundled seed data
            (TOML for the region graph and organizations, CSV for the postal-code
            crosswalks) into an in-memory FileStore and serves every lookup from memory.
            The write path — public submissions — lands in a small SQLite database on a
            Fly volume. It runs on <a href="https://fly.io">Fly.io</a> in Virginia, with
            nightly backups of the SQLite store to Cloudflare R2 on a thirty-day retention
            window.
          </p>
          <p>
            The web app is a React + Vite SPA, with TanStack Query for server state and
            React Router for navigation. It deploys to Cloudflare Workers + Pages as
            static assets with SPA fallback. The wire contract between the two halves
            lives in a single OpenAPI document that both sides codegen from — no
            hand-rolled types on either edge.
          </p>

          <div className="section-kicker">§ III — Type</div>
          <h2>The broadsheet vocabulary.</h2>
          <div className="h2-rule" />
          <p>
            Headlines are set in <strong>Fraunces</strong>, the variable-axis serif used
            in <a href="https://mjrossi.com/blog">Urbanist Lexicon</a> (this
            project&rsquo;s sibling publication, from which the broadsheet identity is
            inherited). Body copy is <strong>Source Serif 4</strong>; user-interface
            chrome is <strong>Inter</strong>; URLs and code are{' '}
            <strong>JetBrains Mono</strong>. All four families are self-hosted via the
            Fontsource variable packages — no external font requests on any page.
          </p>

          <div className="section-kicker">§ IV — Licensing</div>
          <h2>What you can take, what you owe back.</h2>
          <div className="h2-rule" />
          <p>
            The code is released under <strong>Apache 2.0</strong>. Documentation and
            prose are <strong>CC BY-SA 4.0</strong>. The dataset itself — every
            organization entry, every region row, every postal-code mapping — is released
            under the{' '}
            <a href="https://opendatacommons.org/licenses/odbl/">
              Open Database License (ODbL) 1.0
            </a>
            .
          </p>
          <p>
            Every successful response from the public API carries the attribution in-band
            as HTTP headers, and collection responses also carry it in a <code>meta</code>{' '}
            envelope, so downstream consumers can&rsquo;t miss the share-alike obligation.
            The block as it appears on the wire:
          </p>
          <pre className="codeblock">
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

          <div className="section-kicker">§ V — Editorial cadence</div>
          <h2>How the directory stays current.</h2>
          <div className="h2-rule" />
          <p>
            Listings drift — websites lapse, coalitions reorganize, chapters fold. The
            editor runs a link-rot pass with the bundled <code>just linkcheck</code> tool
            before each significant editorial update, triages the report, and ships
            corrections as data-only commits. Corrections from readers are read and
            applied by the same editor; the channel is the{' '}
            <Link to="/submit">submissions desk</Link> today, and a moderated in-app queue
            ships with the Phase 2 API-key program.
          </p>

          <div className="section-kicker">§ VI — Keeping it running</div>
          <h2>Who pays for the servers.</h2>
          <div className="h2-rule" />
          <p>
            The Atlas is independent and unfunded. Hosting is cheap but not free — the
            Fly.io instance, the Cloudflare plan, and the domain run a few dollars a
            month, paid out of pocket. If you&rsquo;d like to help cover the bill,
            there&rsquo;s a{' '}
            <a
              href="https://liberapay.com/urbanist-atlas/"
              target="_blank"
              rel="noopener noreferrer"
            >
              Liberapay
            </a>{' '}
            tip jar.
          </p>
          <p>
            Two caveats, both non-negotiable. First: give to your local advocacy
            organization before you give here — they&rsquo;re the entire point of this
            directory, and a few dollars go much further with them than with our server
            bill. Second: donations buy nothing on this side. They don&rsquo;t influence
            which organizations are listed, how they&rsquo;re ordered, or anything else
            editorial. The directory grows one editorial decision at a time, and money is
            not one of the inputs.
          </p>

          <div className="section-kicker">§ VII — Field staff</div>
          <h2>Two surveyors who know the ground.</h2>
          <div className="h2-rule" />
          <p>
            No atlas is surveyed from a desk alone, and this one keeps two who know the
            territory — both paid in salmon, neither inclined to file a report.{' '}
            <strong>Pad Thai</strong>, an all-black Bombay, is our authority on sprawl: he
            occupies the maximum possible desk area, belly to the ceiling, in open
            defiance of every density guideline we publish. <strong>Mrs Peacock</strong>,
            a dilute tortoiseshell, runs the walkability audit — she tests each surface on
            foot, the keyboard especially, and reroutes the day around her chosen desire
            line. Their findings are non-negotiable.
          </p>
          <div className="newsroom-cats">
            <figure className="newsroom-cat">
              <PadThaiIllustration className="cat-illustration" />
              <figcaption>
                <span className="cat-name">Pad Thai</span> · Sprawl
              </figcaption>
            </figure>
            <figure className="newsroom-cat">
              <MrsPeacockIllustration className="cat-illustration" />
              <figcaption>
                <span className="cat-name">Mrs Peacock</span> · Walkability
              </figcaption>
            </figure>
          </div>
        </main>

        <aside className="rail">
          <div className="rail-block">
            <div className="rail-kicker">Quick links</div>
            <ul className="plain">
              <li>
                <a href={openapiUrl}>OpenAPI spec</a>
              </li>
              <li>
                <a href="https://github.com/mjrossi/urbanist-atlas">Source on GitHub</a>
              </li>
              <li>
                <Link to="/about#for-developers">Developer preview</Link>
              </li>
              <li>
                <Link to="/submit">Submissions desk</Link>
              </li>
            </ul>
          </div>
          <div className="rail-block amber">
            <div className="rail-kicker">Licensing at a glance</div>
            <ul>
              <li>Code · Apache 2.0</li>
              <li>Prose · CC BY-SA 4.0</li>
              <li>Dataset · ODbL 1.0 (share-alike)</li>
              <li>Attribution required in headers</li>
            </ul>
          </div>
          <div className="rail-block amber">
            <div className="rail-kicker">Support the server bill</div>
            <p className="text-sm">
              <a
                className="read-on"
                href="https://liberapay.com/urbanist-atlas/"
                target="_blank"
                rel="noopener noreferrer"
              >
                Chip in on Liberapay →
              </a>
            </p>
            <p className="text-sm">Local orgs first. Hosting only — never editorial.</p>
          </div>
          <div className="rail-block muted">
            <div className="rail-kicker">Type</div>
            <p className="text-sm">
              Set in <em>Fraunces</em>, <em>Source Serif 4</em>, <em>Inter</em>, and{' '}
              <em>JetBrains Mono</em>. Cream <code>#FDF6EC</code>, amber{' '}
              <code>#8F5520</code>, ink <code>#1A1612</code>.
            </p>
          </div>
          <div className="rail-block muted">
            <div className="rail-kicker">Field staff</div>
            <ul>
              <li>Pad Thai · Sprawl</li>
              <li>Mrs Peacock · Walkability</li>
              <li>The territory disagrees at its peril.</li>
            </ul>
          </div>
        </aside>
      </div>
    </>
  );
}
