import { useEffect } from 'react';
import { Link, useLocation } from 'react-router';
import { PageBreadcrumb } from '../components/PageBreadcrumb.tsx';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { apiBase } from '../lib/api.ts';

export function About() {
  useDocumentTitle('About — Urbanist Atlas');
  const { hash } = useLocation();

  // On desktop, all sections must render expanded. Chrome's UA stylesheet
  // hides closed-<details> body content through a mechanism that author
  // CSS — even with !important — can't override (the ::details-content
  // internal slot). The reliable fix is to keep [open] true at desktop
  // widths and prevent the summary click from toggling it back closed.
  useEffect(() => {
    const mq = window.matchMedia('(min-width: 769px)');
    const sections = Array.from(
      document.querySelectorAll<HTMLDetailsElement>('.method-section'),
    );
    const summaries = sections
      .map((d) => d.querySelector('summary'))
      .filter((s): s is HTMLElement => s !== null);

    const forceOpenIfDesktop = () => {
      if (!mq.matches) return;
      for (const d of sections) {
        if (!d.open) d.open = true;
      }
    };
    const preventToggleOnDesktop = (e: MouseEvent) => {
      if (mq.matches) e.preventDefault();
    };

    forceOpenIfDesktop();
    mq.addEventListener('change', forceOpenIfDesktop);
    summaries.forEach((s) => s.addEventListener('click', preventToggleOnDesktop));

    return () => {
      mq.removeEventListener('change', forceOpenIfDesktop);
      summaries.forEach((s) =>
        s.removeEventListener('click', preventToggleOnDesktop),
      );
    };
  }, []);

  // Open the surrounding <details> on mobile before the global
  // useScrollToTop lands on a kicker whose body is still collapsed.
  useEffect(() => {
    const id = decodeURIComponent(hash.replace(/^#/, ''));
    if (!id) return;
    const target = document.getElementById(id);
    if (!target) return;
    const details = target.closest('details');
    if (details && !details.open) {
      details.open = true;
    }
    const raf = requestAnimationFrame(() => {
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
    return () => cancelAnimationFrame(raf);
  }, [hash]);

  const openapiUrl = `${apiBase}/api/v1/openapi.yaml`;
  const atAGlance = (
    <div className="rail-block amber">
      <div className="rail-kicker">At a glance</div>
      <ul>
        <li>Transit + safe-streets advocacy only</li>
        <li>United States &amp; Canada (v1)</li>
        <li>Curated by hand, one entry at a time</li>
        <li>ODbL 1.0 — open data with attribution</li>
      </ul>
    </div>
  );
  return (
    <>
      <PageBreadcrumb
        prefix={[{ label: 'Atlas', to: '/' }]}
        current="About"
        meta="Volume I · 2026 Edition"
      />

      <div className="lede mt-48">
        <div className="eyebrow">
          § About the Atlas<span className="eyebrow-rule" />
        </div>
        <h1>
          A directory of <span className="accent">the people</span> doing the
          work.
        </h1>
        <p className="deck">
          The Urbanist Atlas indexes local and regional advocacy organizations
          working on transit and safe streets across the United States and
          Canada. Searchable by postal code. Curated by hand.
        </p>
      </div>

      <div className="spread">
        <main className="prose">
          <div className="glance-mobile">{atAGlance}</div>
          <div className="section-kicker" id="mission">§ I — Mission</div>
          <h2>Why this exists.</h2>
          <div className="h2-rule" />
          <p className="lead drop">
            The Urbanist Atlas exists for a narrow reason: when someone moves to
            a new city — or wakes up one morning angry about a dangerous
            intersection three blocks from home — they should be able to find
            the people already organizing for better streets and better transit
            nearby, in under a minute.
          </p>

          <details className="method-section" open>
            <summary>
              <div>
                <div className="section-kicker" id="methodology">
                  § II — Methodology
                </div>
                <h2>How we curate.</h2>
              </div>
              <div className="h2-rule" />
            </summary>
          <p>
            Every entry passes through an editor before it gets in. No
            scraper, no algorithm picking favorites — just the criteria
            below, applied case by case, as the time and the leads turn up.
          </p>
          <h3>What gets in.</h3>
          <div className="criteria">
            <div className="row">
              <p className="term">Geographic focus</p>
              <p className="def">
                A defined region — city, county, metro, state, or province.
                National-only outfits are filtered from local search.
              </p>
              <span className="verdict yes">Required</span>
            </div>
            <div className="row">
              <p className="term">Active advocacy</p>
              <p className="def">
                Visible public-facing work: campaigns, hearings, testimony,
                organizing. Not just a mailing list or a 990 on file.
              </p>
              <span className="verdict yes">Required</span>
            </div>
            <div className="row">
              <p className="term">On-topic</p>
              <p className="def">
                Transit advocacy or safe-streets advocacy. The two scopes the
                Atlas exists to index.
              </p>
              <span className="verdict yes">Required</span>
            </div>
            <div className="row">
              <p className="term">Government agencies</p>
              <p className="def">
                DOTs, transit boards, and their subsidiaries are out of scope —
                even when staffers are doing the right thing.
              </p>
              <span className="verdict no">Excluded</span>
            </div>
            <div className="row">
              <p className="term">Consultancies</p>
              <p className="def">
                Even pro-bono and B-Corp consultancies. The Atlas is for
                organizations whose primary product is advocacy, not consulting.
              </p>
              <span className="verdict no">Excluded</span>
            </div>
            <div className="row">
              <p className="term">Housing &amp; YIMBY</p>
              <p className="def">
                A neighbouring movement with overlapping priorities, but a
                tighter scope keeps local search useful here.
              </p>
              <span className="verdict no">Excluded</span>
            </div>
          </div>
          </details>

          <details className="method-section">
            <summary>
              <div>
                <div className="section-kicker" id="corrections">
                  § III — Independence and corrections
                </div>
                <h2>How we keep ourselves honest.</h2>
              </div>
              <div className="h2-rule" />
            </summary>
          <p>
            The Atlas is an independent reference work. It isn&rsquo;t
            affiliated with, endorsed by, or representing any of the
            organizations listed here. Entries draw on public sources and
            editorial judgment, and they go stale: a site lapses, a
            coalition reorganizes, a chapter folds.
          </p>
          <p>
            If you spot a broken link, an outdated description, an organization
            that no longer exists, or a listing that misrepresents the work of
            the people it indexes,{' '}
            <Link to="/submit">file a correction at the submissions desk</Link>.
            Corrections are read and applied by the same editor who maintains
            the directory.
          </p>
          </details>

          <details className="method-section">
            <summary>
              <div>
                <div className="section-kicker" id="for-developers">
                  § IV — For developers
                </div>
                <h2>The dataset is open.</h2>
              </div>
              <div className="h2-rule" />
            </summary>
          <p>
            The Atlas runs on a small Go service. Every endpoint and
            response shape lives in the OpenAPI document at{' '}
            <a href={openapiUrl}>
              <code>/api/v1/openapi.yaml</code>
            </a>
            . The dataset itself is licensed under the{' '}
            <a href="https://opendatacommons.org/licenses/odbl/">
              Open Database License (ODbL) 1.0
            </a>{' '}
            — open for reuse with attribution and share-alike. See the{' '}
            <Link to="/colophon">colophon</Link> for the full data-source
            and licensing picture.
          </p>
          <p>
            During Phase 1 the API sits behind a shared-secret gate while we
            shake out schema and query bugs. The
            Phase 2 program will open self-serve free keys. If you&rsquo;d like
            an early key before Phase 2 — to build a directory widget, a
            regional dashboard, anything — write to{' '}
            <a href="mailto:hello@urbanistatlas.com?subject=Atlas%20API%20early%20access">
              hello@urbanistatlas.com
            </a>{' '}
            and we&rsquo;ll set one up by hand.
          </p>
          </details>

          <details className="method-section">
            <summary>
              <div>
                <div className="section-kicker" id="acknowledgments">
                  § V — Acknowledgments
                </div>
                <h2>Who the directory rests on.</h2>
              </div>
              <div className="h2-rule" />
            </summary>
          <p>
            Above all, this directory rests on the work of the organizations it
            indexes — the volunteers, organizers, and staff who show up to
            their city&rsquo;s transportation meetings, week after week, and
            patiently argue for better. Postal-code geography comes from the{' '}
            <a href="https://www.census.gov">U.S. Census Bureau</a>{' '}
            (ZCTAs + the HUD USPS ZIP-to-County crosswalk) and{' '}
            <a href="https://www.statcan.gc.ca">Statistics Canada</a>&rsquo;s
            boundary files, both public-domain or open-licensed.
          </p>
          </details>
        </main>

        <aside className="rail">
          <div className="rail-block rail-toc">
            <div className="rail-kicker">On this page</div>
            <ul className="plain">
              <li>
                <a href="#mission">I &middot; Mission</a>
              </li>
              <li>
                <a href="#methodology">II &middot; Methodology</a>
              </li>
              <li>
                <a href="#corrections">III &middot; Independence</a>
              </li>
              <li>
                <a href="#for-developers">IV &middot; For developers</a>
              </li>
              <li>
                <a href="#acknowledgments">V &middot; Acknowledgments</a>
              </li>
            </ul>
          </div>
          <div className="glance-desktop">{atAGlance}</div>
          <div className="rail-block">
            <div className="rail-kicker">Get in touch</div>
            <p>
              For tips, corrections, or removal requests, the{' '}
              <Link to="/submit">submissions desk</Link> is the front door.
            </p>
            <p className="mb-0">
              For anything sensitive:{' '}
              <a href="mailto:hello@urbanistatlas.com">
                hello@urbanistatlas.com
              </a>
              .
            </p>
          </div>
          <div className="rail-block muted">
            <div className="rail-kicker">Colophon</div>
            <p className="text-sm">
              Set in <em>Fraunces</em> &amp; <em>Source Serif 4</em> for display
              and body, <em>Inter</em> for captions,{' '}
              <em>JetBrains Mono</em> for URLs and code.
            </p>
            <p className="text-sm mb-0">
              See the <Link to="/colophon">full colophon</Link> for sources,
              stack, and licensing.
            </p>
          </div>
        </aside>
      </div>
    </>
  );
}
