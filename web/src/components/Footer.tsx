import { Link } from 'react-router';

export function Footer() {
  return (
    <>
      <footer className="site-foot desktop-only">
        <div>
          <h3 className="colophon-title">
            Urbanist <span className="em">Atlas</span>
          </h3>
          <p className="colophon-tag">
            A directory of the people fighting for better streets, where you live.
          </p>
          <p className="colophon-note">
            An independent informational directory. Not affiliated with the organizations
            listed.{' '}
            <a href="https://mjrossi.com/blog">
              A companion to <em>Urbanist Lexicon</em>
            </a>
            .
          </p>
        </div>
        <div>
          <h4>The desks</h4>
          <ul>
            <li>
              <Link to="/">Front page</Link>
            </li>
            <li>
              <Link to="/browse">Browse the atlas</Link>
            </li>
            <li>
              <Link to="/submit">File a submission</Link>
            </li>
            <li>
              <Link to="/about">About the Atlas</Link>
            </li>
          </ul>
        </div>
        <div>
          <h4>Methodology</h4>
          <ul>
            <li>
              <Link to="/about#methodology">Inclusion criteria</Link>
            </li>
            <li>
              <Link to="/about#corrections">Independence &amp; corrections</Link>
            </li>
            <li>
              <Link to="/about#for-developers">For developers</Link>
            </li>
            <li>
              <Link to="/colophon">Colophon &amp; sources</Link>
            </li>
            <li>
              <a href="https://api.urbanistatlas.com/api/v1/openapi.yaml">OpenAPI spec</a>
            </li>
          </ul>
        </div>
        <div>
          <h4>Contact</h4>
          <ul>
            <li>
              <a href="mailto:hello@urbanistatlas.com">hello@urbanistatlas.com</a>
            </li>
            <li>
              <a href="https://github.com/mjrossi/urbanist-atlas/issues/new">
                File an issue on GitHub
              </a>
            </li>
            <li>
              <a href="https://github.com/mjrossi/urbanist-atlas">Source on GitHub</a>
            </li>
          </ul>
        </div>
      </footer>
      {/* Phones get a single compact block: wordmark, tagline, three
          essential links, and the colophon strip below. Everything else
          (full link lists, methodology, contact handles) lives one tap
          away on /about and /colophon. */}
      <footer className="site-foot-compact mobile-only">
        <h3 className="colophon-title">
          Urbanist <span className="em">Atlas</span>
        </h3>
        <p className="colophon-tag">
          A directory of the people fighting for better streets, where you live.
        </p>
        <p className="colophon-note">
          A companion to{' '}
          <a href="https://mjrossi.com/blog">
            <em>Urbanist Lexicon</em>
          </a>
          .
        </p>
        <ul className="site-foot-links">
          <li>
            <Link to="/about">About</Link>
          </li>
          <li>
            <Link to="/submit">Submit a tip</Link>
          </li>
          <li>
            <a href="mailto:hello@urbanistatlas.com">Contact</a>
          </li>
        </ul>
      </footer>
      <div className="colophon-strip">
        <span>
          The Atlas · Vol. I · <span className="em">2026 Edition</span>
        </span>
        <span>Set in Fraunces, Source Serif 4, Inter &amp; JetBrains Mono</span>
        <span>
          Printed on cream — <span className="em">#FDF6EC</span>
        </span>
      </div>
    </>
  );
}
