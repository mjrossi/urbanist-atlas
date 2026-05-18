/**
 * Postal-code canonicalization, mirroring the API's
 * `atlas.NormalizePostalCode` (`api/pkg/atlas/postal.go`). The wire
 * contract treats the canonical form as authoritative — same input
 * should produce the same lookup. Country-specific truncation
 * (CA → FSA, UK → outward, PT → hyphen-stripped) is handled
 * server-side; the client only does the generic uppercase + strip
 * pass so the URL and query string match what the server stores.
 */
export function normalizePostal(raw: string): string {
  return raw.replace(/\s+/g, '').toUpperCase();
}
