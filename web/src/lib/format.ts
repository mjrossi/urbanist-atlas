/**
 * Picks the singular or plural noun for a count. The number itself is
 * the caller's to render, since several surfaces style it separately.
 */
export function pluralize(n: number, singular: string, plural: string): string {
  return n === 1 ? singular : plural;
}

/** Renders an org count with its noun, e.g. "1 group" or "42 groups". */
export function groupCountLabel(n: number): string {
  return `${n} ${pluralize(n, 'group', 'groups')}`;
}

/**
 * The bare host of a URL without a leading `www.`, or null for a
 * malformed URL.
 */
export function domainOf(url: string): string | null {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return null;
  }
}

/**
 * Renders a tag slug ('vision-zero') for display ('vision zero').
 * Hyphens are the only separator the seed data uses, though the loader
 * doesn't enforce it.
 */
export function prettyTag(tag: string): string {
  return tag.replace(/-/g, ' ');
}

/**
 * Formats an `added_at` date (YYYY-MM-DD) as "May 21, 2026". It parses
 * by hand because `new Date('2026-05-21')` is UTC midnight, which prints
 * as May 20 west of UTC.
 */
export function formatAddedAt(addedAt: string): string {
  const [yStr, mStr, dStr] = addedAt.split('-');
  const y = Number(yStr);
  const m = Number(mStr);
  const d = Number(dStr);
  if (!y || !m || !d) return addedAt;
  // Local noon, so no DST shift can move it to another calendar day.
  const dt = new Date(y, m - 1, d, 12, 0, 0);
  return dt.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  });
}
