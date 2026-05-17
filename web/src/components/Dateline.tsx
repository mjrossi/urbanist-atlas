import type { Country } from '../lib/api.ts';

/**
 * Newspaper-style dateline for the results header: the postal code
 * the user searched (as the kicker), the resolved place name as the
 * headline, and the country flush right.
 *
 * `placeLabel` is optional so callers can render the dateline while
 * the lookup is still in flight or has failed.
 */
export function Dateline({
  postalCode,
  country,
  placeLabel,
}: {
  postalCode: string;
  country: Country;
  placeLabel?: string;
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
    </header>
  );
}
