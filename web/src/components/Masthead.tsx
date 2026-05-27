import { Link, useLocation } from 'react-router';

const FOLIO_LABELS: ReadonlyArray<[RegExp, string]> = [
  [/^\/$/, "Today's reading"],
  [/^\/about$/, 'About the Atlas'],
  [/^\/browse$/, 'The index'],
  [/^\/submit$/, 'The submissions desk'],
  [/^\/colophon$/, 'The colophon'],
  [/^\/region\//, 'Region report'],
  [/^\/orgs\//, 'Organization file'],
  [/^\/r\//, 'Postal lookup'],
];

function folioRight(pathname: string): string {
  for (const [pattern, label] of FOLIO_LABELS) {
    if (pattern.test(pathname)) return label;
  }
  return 'Reader edition';
}

export function Masthead() {
  const { pathname } = useLocation();
  const isHome = pathname === '/';
  return (
    <header className={`masthead${isHome ? '' : ' interior'}`}>
      <div className="masthead-folio">
        <span>
          A directory<span className="dot">·</span>Vol. I
        </span>
        <span>{folioRight(pathname)}</span>
      </div>
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
          <span>United States · Canada</span>
        </div>
      </div>
      <div className="masthead-tagline-row">
        <span className="masthead-rule" aria-hidden="true" />
        <span className="masthead-tagline">
          Find the people fighting for better streets where you live
        </span>
        <span className="masthead-rule" aria-hidden="true" />
      </div>
    </header>
  );
}
