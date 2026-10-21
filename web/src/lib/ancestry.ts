/**
 * Returns a root-first copy of the API's leaf-first ancestry list, the
 * order breadcrumbs read in.
 */
export function reverseAncestry<T>(ancestry: readonly T[]): T[] {
  return [...ancestry].reverse();
}
