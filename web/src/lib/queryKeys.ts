/**
 * Centralized factory for `@tanstack/react-query` keys. Importing
 * keys from one place keeps invalidation predictable: when an
 * endpoint's shape or args change, there's exactly one place to
 * update.
 *
 * Each factory returns a `readonly` tuple so React Query's structural
 * hashing stays stable and TypeScript can narrow on the discriminator.
 */
import type { Country } from './api.ts';

export const queryKeys = {
  lookup: (postal_code: string, country: Country) =>
    ['lookup', postal_code, country] as const,
  regions: () => ['regions'] as const,
  // Its own prefix, so invalidating `regions()` leaves searches cached.
  // The default limit matches RegionCombobox's page size.
  regionSearch: (q: string, limit = 8) => ['regionSearch', q, limit] as const,
  region: (slug: string) => ['region', slug] as const,
  org: (slug: string) => ['org', slug] as const,
  recent: () => ['recent'] as const,
  stats: () => ['stats'] as const,
} as const;
