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

/**
 * Renders an editorial tag slug for display. Seed data stores tags
 * as hyphen-lowercase ('vision-zero', 'rider-union', 'safe-streets')
 * and the API ships them unchanged; this helper swaps hyphens for
 * spaces so the chip reads naturally.
 *
 * Hyphens are the only separator the seed file uses today — the
 * loader (`api/internal/seedfiles/orgs.go`) bounds tag length and
 * non-emptiness but doesn't constrain charset, so this stays a
 * one-line replace rather than a full slug-to-Title-Case routine.
 * Bump the regex if underscores or other separators ever start
 * appearing in `api/seed/orgs.toml`.
 */
export function prettyTag(tag: string): string {
  return tag.replace(/-/g, ' ');
}

/**
 * Formats an `added_at` date-only string (YYYY-MM-DD, per the wire
 * contract — see `openapi.yaml`'s Org schema) as an editorial
 * dateline. Returns the natural broadsheet form, e.g.
 *
 *   "May 21, 2026"
 *
 * Parsing is intentionally manual (not `new Date(s)`) because the
 * Date constructor reads bare-date inputs as UTC midnight and then
 * renders them in the local timezone — so for a viewer west of UTC,
 * "2026-05-21" would print as "May 20". Splitting on `-` keeps the
 * displayed day calendar-correct everywhere.
 */
export function formatAddedAt(addedAt: string): string {
  const [yStr, mStr, dStr] = addedAt.split('-');
  const y = Number(yStr);
  const m = Number(mStr);
  const d = Number(dStr);
  if (!y || !m || !d) return addedAt;
  // Local-noon construction sidesteps the same UTC drift on the
  // Intl.DateTimeFormat side: noon in every IANA zone is the same
  // calendar day regardless of how the timezone offset rounds.
  const dt = new Date(y, m - 1, d, 12, 0, 0);
  return dt.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  });
}
