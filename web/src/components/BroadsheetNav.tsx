import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Link, useLocation } from 'react-router';

import type { Stats } from '../lib/api.ts';
import { getStats } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';

interface NavEntry {
  to: string;
  roman: string;
  label: string;
}

const NAV_ENTRIES = [
  { to: '/', roman: 'i.', label: 'Front' },
  { to: '/browse', roman: 'ii.', label: 'Browse' },
  { to: '/submit', roman: 'iii.', label: 'Submit' },
  { to: '/about', roman: 'iv.', label: 'About' },
] as const satisfies readonly NavEntry[];

function isActive(entry: NavEntry, pathname: string): boolean {
  if (entry.to === '/') return pathname === '/';
  if (entry.to === '/browse') {
    return (
      pathname.startsWith('/browse') ||
      pathname.startsWith('/region/') ||
      pathname.startsWith('/orgs/') ||
      pathname.startsWith('/r/')
    );
  }
  return pathname === entry.to || pathname.startsWith(`${entry.to}/`);
}

// The drawer label echoes the current page when collapsed and reads
// "Close" when expanded. The fallback to NAV_ENTRIES[0] only matters on
// routes that don't match any entry (e.g. /colophon, /r/*); NAV_ENTRIES
// is a non-empty tuple, so element 0 is always defined.
function currentLabel(pathname: string): string {
  const active = NAV_ENTRIES.find((e) => isActive(e, pathname));
  return active ? active.label : NAV_ENTRIES[0].label;
}

export function BroadsheetNav() {
  const { pathname } = useLocation();
  const [menuOpen, setMenuOpen] = useState(false);
  const closeMenu = () => {
    setMenuOpen(false);
  };

  // The masthead tally reads `/api/v1/stats` rather than summing over
  // `/api/v1/regions`. That list is the browseable subset (metros and
  // cities), so the old reduce here silently omitted every org attached
  // solely to a state, province, borough, or multi-state region — the
  // masthead under-reported the catalog by 30% on every page.
  //
  // Dropping the regions query also means routes that never render a
  // region list (/about, /colophon, /submit) no longer pull the whole
  // browse set just to print two numbers.
  const stats = useQuery<Stats>({
    queryKey: queryKeys.stats(),
    queryFn: ({ signal }) => getStats({ signal }),
  });
  const orgCount = stats.data?.total_org_count ?? null;
  const placeCount = stats.data?.browse_region_count ?? null;

  return (
    <nav className={`nav${menuOpen ? ' nav-open' : ''}`} aria-label="Primary">
      <button
        type="button"
        className="nav-toggle"
        aria-label={menuOpen ? 'Close menu' : 'Open menu'}
        aria-expanded={menuOpen}
        aria-controls="primary-nav-list"
        onClick={() => {
          setMenuOpen((o) => !o);
        }}
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
              <Link
                to={entry.to}
                onClick={closeMenu}
                className={active ? 'current' : undefined}
                aria-current={active ? 'page' : undefined}
              >
                <span className="roman">{entry.roman}</span>
                {entry.label}
              </Link>
            </li>
          );
        })}
      </ul>
      <div className="nav-right">
        <span className="live">
          <strong>Live directory</strong>
        </span>
        {placeCount !== null && orgCount !== null ? (
          <span>
            {orgCount} orgs · {placeCount} places
          </span>
        ) : null}
      </div>
    </nav>
  );
}
