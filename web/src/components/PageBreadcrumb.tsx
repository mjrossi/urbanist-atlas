import type { ReactNode } from 'react';

import { type BreadcrumbItem, BreadcrumbTrail } from './BreadcrumbTrail.tsx';

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
 * For region-shaped pages with a dynamic ancestor walk, use
 * `RegionBreadcrumb` instead — it accepts a `Region[]` and renders
 * each ancestor as a `/region/:slug` link. Both render the shared
 * {@link BreadcrumbTrail} markup + a11y contract.
 */
export function PageBreadcrumb({
  prefix = [],
  current,
  meta,
}: {
  prefix?: readonly BreadcrumbItem[];
  current: string;
  meta?: ReactNode;
}) {
  return <BreadcrumbTrail items={prefix} current={current} right={meta} />;
}
