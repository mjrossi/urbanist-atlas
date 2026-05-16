import { NavLink } from 'react-router';

/**
 * Primary nav strip rendered immediately below the masthead. Matches
 * the portfolio's `<nav class="broadsheet-nav">` shape: a flex row of
 * page links with hover/active styling pulled from global.css.
 *
 * The four destinations correspond to slices later in the roadmap
 * (#11 Home, #14 Browse, #13 Submit, #15 About). The routes don't
 * exist yet, so following the links produces a router 404 — that's
 * fine for the layout shell and will be filled in by those slices.
 */
const navLinks = [
  { to: '/', label: 'Home', end: true },
  { to: '/browse', label: 'Browse', end: false },
  { to: '/submit', label: 'Submit', end: false },
  { to: '/about', label: 'About', end: false },
] as const;

export function BroadsheetNav() {
  return (
    <nav className="broadsheet-nav" aria-label="Primary">
      <div className="broadsheet-nav-inner">
        <div className="page-links">
          {navLinks.map((link) => (
            <NavLink
              key={link.to}
              to={link.to}
              end={link.end}
              className={({ isActive }) => (isActive ? 'active' : '')}
            >
              {link.label}
            </NavLink>
          ))}
        </div>
      </div>
    </nav>
  );
}
