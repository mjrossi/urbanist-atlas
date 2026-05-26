import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, useLocation } from 'react-router';
import { listMetros } from '../lib/api.ts';
import type { MetroSummary } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';

interface NavEntry {
  to: string;
  roman: string;
  label: string;
}

const NAV_ENTRIES: ReadonlyArray<NavEntry> = [
  { to: '/', roman: 'i.', label: 'Front' },
  { to: '/browse', roman: 'ii.', label: 'Browse' },
  { to: '/submit', roman: 'iii.', label: 'Submit' },
  { to: '/about', roman: 'iv.', label: 'About' },
];

function isActive(entry: NavEntry, pathname: string): boolean {
  if (entry.to === '/') return pathname === '/';
  if (entry.to === '/browse') {
    return (
      pathname.startsWith('/browse') ||
      pathname.startsWith('/m/') ||
      pathname.startsWith('/orgs/') ||
      pathname.startsWith('/r/')
    );
  }
  return pathname === entry.to || pathname.startsWith(`${entry.to}/`);
}

// The drawer label echoes the current page when collapsed and reads
// "Close" when expanded. The fallback to NAV_ENTRIES[0] only matters on
// routes that don't match any entry (e.g. /colophon, /r/*); the array
// is a top-level const, so the non-null assertion is safe.
function currentLabel(pathname: string): string {
  const active = NAV_ENTRIES.find((e) => isActive(e, pathname));
  return active ? active.label : NAV_ENTRIES[0]!.label;
}

export function BroadsheetNav() {
  const { pathname } = useLocation();
  const [menuOpen, setMenuOpen] = useState(false);
  const closeMenu = () => setMenuOpen(false);

  const metros = useQuery<MetroSummary[]>({
    queryKey: queryKeys.metros(),
    queryFn: ({ signal }) => listMetros({ signal }),
  });
  const metroCount = metros.data?.length ?? null;
  const orgCount = metros.data
    ? metros.data.reduce((sum, m) => sum + m.org_count, 0)
    : null;

  return (
    <nav className={`nav${menuOpen ? ' nav-open' : ''}`} aria-label="Primary">
      <button
        type="button"
        className="nav-toggle"
        aria-label={menuOpen ? 'Close menu' : 'Open menu'}
        aria-expanded={menuOpen}
        aria-controls="primary-nav-list"
        onClick={() => setMenuOpen((o) => !o)}
      >
        <span className="nav-toggle-icon" aria-hidden="true">
          <span />
          <span />
          <span />
        </span>
        <span className="nav-toggle-label">
          {menuOpen ? 'Close' : currentLabel(pathname)}
        </span>
      </button>
      <ul id="primary-nav-list" className="nav-list">
        {NAV_ENTRIES.map((entry) => {
          const active = isActive(entry, pathname);
          return (
            <li key={entry.to} className="nav-item">
              {active ? (
                <span className="current" onClick={closeMenu}>
                  <span className="roman">{entry.roman}</span>
                  {entry.label}
                </span>
              ) : (
                <Link to={entry.to} onClick={closeMenu}>
                  <span className="roman">{entry.roman}</span>
                  {entry.label}
                </Link>
              )}
            </li>
          );
        })}
      </ul>
      <div className="nav-right">
        <span className="live">
          <strong>Indexed &amp; current</strong>
        </span>
        {metroCount !== null && orgCount !== null ? (
          <span>
            {orgCount} orgs · {metroCount} metros
          </span>
        ) : null}
      </div>
    </nav>
  );
}
