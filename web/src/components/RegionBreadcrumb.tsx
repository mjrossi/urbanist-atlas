import type { ReactNode } from 'react';

import type { Region } from '../lib/api.ts';
import { type BreadcrumbItem, BreadcrumbTrail } from './BreadcrumbTrail.tsx';

// Re-exported so region pages can keep importing the crumb-item type
// from here (its historical home) rather than reaching into
// BreadcrumbTrail.
export type { BreadcrumbItem };

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
 * The markup + a11y contract (nav landmark, `aria-hidden` separators,
 * trailing `aria-current="page"`) lives in the shared
 * {@link BreadcrumbTrail}; this component only maps the region
 * ancestors into crumb items.
 */
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
  const items: BreadcrumbItem[] = [
    ...prefix,
    ...ancestors.map((r) => ({
      label: r.name,
      to: `/region/${encodeURIComponent(r.slug)}`,
    })),
  ];
  return <BreadcrumbTrail items={items} current={current} right={metaRight} />;
}
