import type { Country, Region } from '../lib/api.ts';

/**
 * Newspaper-style dateline for the results header: the postal code
 * the user searched (as the kicker), the resolved place name as the
 * headline, and the country flush right.
 *
 * `placeLabel` is optional so callers can render the dateline while
 * the lookup is still in flight or has failed.
 *
 * `ancestry` is the resolved region chain from most-specific to
 * least-specific (as returned by the API). When non-empty a breadcrumb
 * row is rendered beneath the main dateline line.
 */
export function Dateline({
  postalCode,
  country,
  placeLabel,
  ancestry = [],
}: {
  postalCode: string;
  country: Country;
  placeLabel?: string;
  ancestry?: Region[];
}) {
  return (
    <header className="dateline">
      <span className="dateline-postal">{postalCode}</span>
      {placeLabel ? (
        <>
          <span className="dateline-sep" aria-hidden="true">
            ·
          </span>
          <span className="dateline-place">{placeLabel}</span>
        </>
      ) : null}
      <span className="dateline-country">{country}</span>
      {ancestry.length > 0 ? (
        <nav className="dateline-ancestry" aria-label="Region breadcrumb">
          {ancestry.map((region, i) => (
            <span key={region.slug} className="dateline-ancestry-crumb">
              {i > 0 ? (
                <span className="dateline-ancestry-sep" aria-hidden="true">
                  {' '}›{' '}
                </span>
              ) : null}
              {region.name}
            </span>
          ))}
        </nav>
      ) : null}
    </header>
  );
}
