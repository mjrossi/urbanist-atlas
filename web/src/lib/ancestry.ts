/**
 * Ancestry helpers shared between the postal-code Results page and
 * the Region detail page.
 *
 * The API returns region ancestry leaf-first (closest ancestor at
 * index 0). The SPA's breadcrumbs read left-to-right root → leaf,
 * which means every consumer of the ancestry array first reverses
 * a copy. Keeping that one-liner in a named helper makes the
 * convention discoverable and ensures both pages stay in sync if
 * the API ever flips the contract.
 */

/**
 * Returns a root-first copy of a leaf-first ancestry list. The
 * input is never mutated (the array is spread before reversing).
 * The element type is generic so both `Region` and `RegionSummary`
 * lists pass through unchanged.
 */
export function reverseAncestry<T>(ancestry: ReadonlyArray<T>): T[] {
  return [...ancestry].reverse();
}
