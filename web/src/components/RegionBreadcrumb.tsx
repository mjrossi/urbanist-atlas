import type { ReactNode } from 'react';

import type { Region } from '../lib/api.ts';
import { type BreadcrumbItem, BreadcrumbTrail } from './BreadcrumbTrail.tsx';

export type { BreadcrumbItem };

/**
 * The kicker breadcrumb for a region-shaped page: `prefix`, then each
 * of `ancestors` as a region link, then `current`. `ancestors` must be
 * root-first.
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
