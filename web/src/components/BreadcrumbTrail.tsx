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
 * The shared broadsheet `.kicker` breadcrumb: a `<nav aria-label=
 * "Breadcrumb">` wrapping an `<ol className="crumb-trail">`, an optional
 * right-side metadata slot, and a trailing non-link leaf marked
 * `aria-current="page"`. Every non-leaf crumb gets an `aria-hidden`
 * visual `/` separator; the leaf gets none.
 *
 * This is the primitive behind both {@link PageBreadcrumb} (static
 * routes — `items` is just the prefix) and `RegionBreadcrumb` (region
 * pages — `items` is the prefix followed by the resolved ancestor
 * walk). Callers build the `items` list; this component owns the markup
 * and a11y contract so the two breadcrumb flavors can't drift.
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
