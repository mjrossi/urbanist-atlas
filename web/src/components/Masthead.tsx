import { Link, useLocation } from 'react-router';

/**
 * Newspaper-style masthead for every page of the Atlas.
 *
 * The structure mirrors the portfolio's `<header class="masthead full">`
 * exactly so the ported CSS works without per-component rules: a name
 * row with the wordmark on the left and an edition block on the right,
 * then a tagline row with the italic tagline bracketed by two
 * horizontal rules.
 *
 * "Atlas" in the wordmark carries `.surname` so it picks up the amber
 * `--accent-surname` token, mirroring the "Rossi" treatment on
 * mjrossi.com. The wordmark is a heading on the home page and a link
 * back to / on every other page (matches the portfolio's `isHome`
 * branch).
 */
export function Masthead() {
  const { pathname } = useLocation();
  const isHome = pathname === '/';

  return (
    <header className="masthead full">
      <div className="masthead-inner">
        <div className="masthead-name-row">
          {isHome ? (
            <h1 className="masthead-name">
              Urbanist <span className="surname">Atlas</span>
            </h1>
          ) : (
            <Link to="/" className="masthead-name masthead-name-link">
              Urbanist <span className="surname">Atlas</span>
            </Link>
          )}
          <div className="masthead-meta">
            <span className="masthead-meta-loc">United States &middot; Canada</span>
            <span className="masthead-meta-edition">Vol. I &middot; 2026 Edition</span>
          </div>
        </div>
        <div className="masthead-tagline-row">
          <span className="masthead-rule" aria-hidden="true" />
          <span className="masthead-tagline">
            Find the people fighting for better streets where you live
          </span>
          <span className="masthead-rule" aria-hidden="true" />
        </div>
      </div>
    </header>
  );
}
