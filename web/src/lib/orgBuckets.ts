/**
 * The three presentational tiers a postal-code lookup or a region
 * detail splits its organizations into — local (city / county),
 * regional (metro / CMA / multi-state / transit-federation), and
 * statewide (state / province). Both the Results and Region pages
 * bucket orgs this way; this module is the one place that knows how to
 * total them across all three tiers.
 */
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
