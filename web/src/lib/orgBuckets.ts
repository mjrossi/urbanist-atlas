import type { LookupOrg } from './api.ts';

export interface OrgBuckets {
  local: readonly LookupOrg[];
  regional: readonly LookupOrg[];
  statewide: readonly LookupOrg[];
}

/** Total number of org entries across all three presentational tiers. */
export function totalEntries(buckets: OrgBuckets): number {
  return buckets.local.length + buckets.regional.length + buckets.statewide.length;
}
