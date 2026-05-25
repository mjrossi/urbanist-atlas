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

export function BroadsheetNav() {
  const { pathname } = useLocation();
  const metros = useQuery<MetroSummary[]>({
    queryKey: queryKeys.metros(),
    queryFn: ({ signal }) => listMetros({ signal }),
  });
  const metroCount = metros.data?.length ?? null;
  const orgCount = metros.data
    ? metros.data.reduce((sum, m) => sum + m.org_count, 0)
    : null;

  return (
    <nav className="nav" aria-label="Primary">
      <ul className="nav-list">
        {NAV_ENTRIES.map((entry) => {
          const active = isActive(entry, pathname);
          return (
            <li key={entry.to} className="nav-item">
              {active ? (
                <span className="current">
                  <span className="roman">{entry.roman}</span>
                  {entry.label}
                </span>
              ) : (
                <Link to={entry.to}>
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
