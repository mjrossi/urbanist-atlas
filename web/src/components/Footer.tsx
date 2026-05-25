import { Link } from 'react-router';

export function Footer() {
  return (
    <>
      <footer className="site-foot">
        <div>
          <h3 className="colophon-title">
            Urbanist <span className="em">Atlas</span>
          </h3>
          <p className="colophon-tag">
            A directory of the people fighting for better streets, where you live.
          </p>
          <p className="colophon-note">
            An independent informational directory. Not affiliated with the
            organizations listed.{' '}
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
              <Link to="/browse">Browse by metro</Link>
            </li>
            <li>
              <Link to="/submit">File a submission</Link>
            </li>
            <li>
              <Link to="/about">About the Atlas</Link>
            </li>
            <li>
              <Link to="/colophon">Colophon</Link>
            </li>
          </ul>
        </div>
        <div>
          <h4>Methodology</h4>
          <ul>
            <li>
              <Link to="/about">Inclusion criteria</Link>
            </li>
            <li>
              <Link to="/about">Sources &amp; verification</Link>
            </li>
            <li>
              <Link to="/about#for-developers">For developers</Link>
            </li>
            <li>
              <a href="https://api.urbanistatlas.com/api/v1/openapi.yaml">
                OpenAPI spec
              </a>
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
              <a href="https://github.com/mjrossi/urbanist-atlas">
                Source on GitHub
              </a>
            </li>
          </ul>
        </div>
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
