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

/**
 * Extracts the bare host of a URL, stripping any leading `www.`.
 * Returns null for malformed input so callers can fall back to the
 * raw URL string. Used by org entries on both the Results list and
 * the Org detail page to render compact "by their site" links.
 */
export function domainOf(url: string): string | null {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return null;
  }
}
