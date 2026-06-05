import type { ReactNode } from 'react';
import { Link } from 'react-router';

import type { Region } from '../lib/api.ts';

/**
 * The broadsheet `.kicker` row at the top of a region-shaped page.
 * Renders a breadcrumb of region ancestors (each linkable) and an
 * optional right-side metadata slot. Used by both the Region detail
 * page and the postal-code Results page so navigation between
 * Browse → Region → ancestor regions stays one consistent
 * affordance.
 *
 * `prefix` is the path between the home link and the ancestry walk
 * — typically `[{label: 'Atlas', to: '/'}, {label: 'Browse', to:
 * '/browse'}]` for Region or `[{label: 'Atlas', to: '/'}, {label:
 * 'Lookup · 11217'}]` for Results. A prefix entry with no `to`
 * renders as a plain `<span>`, so callers can include non-clickable
 * context (like the postal code itself).
 *
 * `ancestors` is the upward DAG walk between the prefix and the
 * current region — ordered ROOT-FIRST so the breadcrumb reads
 * left-to-right "broad → narrow." The caller is responsible for
 * the order (Region.tsx reverses the closest-first `ancestry`
 * field from `RegionDetail`; Results.tsx reverses the
 * `resolved_ancestry` from `LookupResult`, dropping the leaf).
 *
 * `current` is the rendered crumb-here label — usually the
 * region's name, but Results passes the leaf region's name when
 * the lookup resolved.
 *
 * A11y: the crumb chain renders as `<nav aria-label="Breadcrumb">
 * <ol class="crumb-trail">...</ol></nav>`. The trailing crumb is
 * marked `aria-current="page"` and visual `/` separators are
 * `aria-hidden` so screen readers don't speak punctuation.
 */
export interface BreadcrumbItem {
  label: string;
  to?: string;
}

export function RegionBreadcrumb({
  prefix,
  ancestors,
  current,
  metaRight,
}: {
  prefix: readonly BreadcrumbItem[];
  ancestors: readonly Region[];
  current: string;
  metaRight?: ReactNode;
}) {
  return (
    <div className="kicker">
      <nav aria-label="Breadcrumb">
        <ol className="crumb-trail">
          {prefix.map((item) => (
            <CrumbLi key={item.to ?? item.label} item={item} />
          ))}
          {ancestors.map((r) => (
            <CrumbLi
              key={`a-${r.slug}`}
              item={{ label: r.name, to: `/region/${encodeURIComponent(r.slug)}` }}
            />
          ))}
          <li>
            <span className="crumb-here" aria-current="page">
              {current}
            </span>
          </li>
        </ol>
      </nav>
      {metaRight !== undefined ? <div>{metaRight}</div> : null}
    </div>
  );
}

function CrumbLi({ item }: { item: BreadcrumbItem }) {
  return (
    <li>
      {item.to ? <Link to={item.to}>{item.label}</Link> : <span>{item.label}</span>}
      <span className="crumb-sep" aria-hidden="true">
        /
      </span>
    </li>
  );
}
