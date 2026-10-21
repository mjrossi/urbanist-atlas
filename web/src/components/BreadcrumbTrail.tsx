import type { ReactNode } from 'react';
import { Link } from 'react-router';

/**
 * One crumb in a breadcrumb trail. `to` makes it a `<Link>`; without it
 * the label renders as plain text (for non-clickable context like a
 * postal code).
 */
export interface BreadcrumbItem {
  label: string;
  to?: string;
}

/**
 * The kicker breadcrumb markup shared by {@link PageBreadcrumb} and
 * `RegionBreadcrumb`: `items`, then a non-link `current` leaf marked
 * `aria-current="page"`, with an optional `right` slot.
 */
export function BreadcrumbTrail({
  items,
  current,
  right,
}: {
  items: readonly BreadcrumbItem[];
  current: string;
  right?: ReactNode;
}) {
  return (
    <div className="kicker">
      <nav aria-label="Breadcrumb">
        <ol className="crumb-trail">
          {items.map((item) => (
            <li key={item.to ?? item.label}>
              {item.to ? (
                <Link to={item.to}>{item.label}</Link>
              ) : (
                <span>{item.label}</span>
              )}
              <span className="crumb-sep" aria-hidden="true">
                /
              </span>
            </li>
          ))}
          <li>
            <span className="crumb-here" aria-current="page">
              {current}
            </span>
          </li>
        </ol>
      </nav>
      {right !== undefined ? <div>{right}</div> : null}
    </div>
  );
}
