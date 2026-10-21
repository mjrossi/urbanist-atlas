/**
 * The generic half of the API's `atlas.NormalizePostalCode`: uppercase
 * with whitespace stripped. Country-specific truncation (ZIP+4, CA FSA)
 * stays server-side.
 */
export function normalizePostal(raw: string): string {
  return raw.replace(/\s+/g, '').toUpperCase();
}
