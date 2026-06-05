import type { ReactNode } from 'react';
import { Link } from 'react-router';

export interface PageBreadcrumbItem {
  label: string;
  to?: string;
}

/**
 * The broadsheet `.kicker` row at the top of a static-route page.
 * Renders a semantic breadcrumb that ends in a non-link "current
 * page" leaf marked `aria-current="page"`.
 *
 * `prefix` is the navigation trail leading to the current page —
 * typically `[{label: 'Atlas', to: '/'}]` for top-level routes,
 * or `[{label: 'Atlas', to: '/'}, {label: 'Browse', to: '/browse'}]`
 * for detail pages reached through Browse. A prefix entry with no
 * `to` renders as plain text so callers can include non-clickable
 * context (e.g. the "Lookup · 11217" header used by Results).
 *
 * `current` is the leaf label — the name of the page being shown.
 * `meta` is the optional right-side metadata slot (volume number,
 * region count, etc.).
 *
 * A11y: renders as `<nav aria-label="Breadcrumb"><ol class="crumb-trail">...</ol></nav>`.
 * The trailing crumb is marked `aria-current="page"`; visual `/`
 * separators are `aria-hidden` so screen readers don't speak
 * punctuation.
 *
 * For region-shaped pages with a dynamic ancestor walk, use
 * `RegionBreadcrumb` instead — it accepts a `Region[]` and renders
 * each ancestor as a `/region/:slug` link.
 */
export function PageBreadcrumb({
  prefix = [],
  current,
  meta,
}: {
  prefix?: readonly PageBreadcrumbItem[];
  current: string;
  meta?: ReactNode;
}) {
  return (
    <div className="kicker">
      <nav aria-label="Breadcrumb">
        <ol className="crumb-trail">
          {prefix.map((item) => (
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
      {meta !== undefined ? <div>{meta}</div> : null}
    </div>
  );
}
