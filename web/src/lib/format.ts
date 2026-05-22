/**
 * Shared formatting helpers for user-facing strings. Keep these
 * trivial — when a function grows beyond a one-liner, it usually
 * belongs in a component-local helper or a domain-specific module
 * (postal.ts, etc.) instead.
 */

/**
 * Renders an org count with the matching singular / plural noun.
 * Used in the homepage Browse aside and on the /browse listing —
 * one canonical wording so the two surfaces don't drift.
 */
export function groupCountLabel(n: number): string {
  return n === 1 ? '1 group' : `${n} groups`;
}
