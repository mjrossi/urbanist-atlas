import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { Link, useLocation } from 'react-router';

import type { ApiError, Stats } from '../lib/api.ts';
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

// The fallback only matters on routes no entry claims (e.g. /colophon).
function currentLabel(pathname: string): string {
  const active = NAV_ENTRIES.find((e) => isActive(e, pathname));
  return active ? active.label : NAV_ENTRIES[0].label;
}

export function BroadsheetNav() {
  const { pathname } = useLocation();
  // Keyed by the path it was opened on, so any route change closes it:
  // a nav link, a link in the page, or Back.
  const [openOn, setOpenOn] = useState<string | null>(null);
  // Cleared on the change itself, not just ignored: otherwise Back to
  // the opening path would reopen the drawer.
  if (openOn !== null && openOn !== pathname) setOpenOn(null);
  const menuOpen = openOn === pathname;
  const toggleRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!menuOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      setOpenOn(null);
      toggleRef.current?.focus();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [menuOpen]);

  // The masthead tally reads `/api/v1/stats` rather than summing over
  // `/api/v1/regions`. That list is the browseable subset (metros and
  // cities), so the old reduce here silently omitted every org attached
  // solely to a state, province, borough, or multi-state region — the
  // masthead under-reported the catalog by 30% on every page.
  //
  // Dropping the regions query also means routes that never render a
  // region list (/about, /colophon, /submit) no longer pull the whole
  // browse set just to print two numbers.
  const stats = useQuery<Stats, ApiError>({
    queryKey: queryKeys.stats(),
    queryFn: ({ signal }) => getStats({ signal }),
  });
  const orgCount = stats.data?.total_org_count ?? null;
  const placeCount = stats.data?.browse_region_count ?? null;

  return (
    <nav className={`nav${menuOpen ? ' nav-open' : ''}`} aria-label="Primary">
      <button
        ref={toggleRef}
        type="button"
        className="nav-toggle"
        aria-label={menuOpen ? 'Close menu' : 'Open menu'}
        aria-expanded={menuOpen}
        aria-controls="primary-nav-list"
        onClick={() => {
          setOpenOn(menuOpen ? null : pathname);
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
                onClick={() => {
                  setOpenOn(null);
                }}
                className={active ? 'current' : undefined}
                aria-current={
                  pathname === entry.to ? 'page' : active ? 'true' : undefined
                }
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
