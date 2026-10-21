import type { ReactNode } from 'react';

import { type BreadcrumbItem, BreadcrumbTrail } from './BreadcrumbTrail.tsx';

/**
 * The kicker breadcrumb for a static-route page: `prefix`, then the
 * `current` leaf, with `meta` on the right.
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
