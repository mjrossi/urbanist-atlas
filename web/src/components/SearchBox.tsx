import { useId, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { useNavigate } from 'react-router';
import type { Country } from '../lib/api.ts';
import { normalizePostal } from '../lib/postal.ts';

/**
 * Postal-code search input. Accepts a US 5-digit ZIP or a Canadian
 * postal code (3-char FSA or full 6-char). On submit it normalizes
 * the input (uppercase + strip whitespace) and navigates to
 * `/r/:postalCode?country=US|CA`.
 *
 * Country is detected heuristically from the first character (digit
 * → US, letter → CA); the small `<select>` lets the user override
 * when the heuristic guesses wrong.
 */

type DetectedCountry = Country | null;

// TODO(third-country): the digit→US, letter→CA heuristic only works
// while the UI exposes exactly two countries. Before adding a third
// (e.g. PT/ES going user-facing), either retire the auto-detect and
// require an explicit country pick, or replace this with a per-country
// regex map. The `<select>` options below and lib/api.ts'
// SUPPORTED_COUNTRIES need updating in lockstep.
function detectCountry(raw: string): DetectedCountry {
  const trimmed = raw.trim();
  if (trimmed.length === 0) return null;
  const first = trimmed[0];
  if (first === undefined) return null;
  if (/[0-9]/.test(first)) return 'US';
  if (/[A-Za-z]/.test(first)) return 'CA';
  return null;
}

/**
 * Client-side sanity check. The API does authoritative validation;
 * this just prevents the obviously-junk submissions from making a
 * round trip and gives the user immediate feedback.
 */
function validate(normalized: string, country: Country): string | null {
  if (normalized.length === 0) return 'Enter a ZIP or postal code.';
  if (country === 'US') {
    if (!/^\d{5}$/.test(normalized)) {
      return 'US ZIP codes are 5 digits.';
    }
  } else {
    if (!/^[A-Z]\d[A-Z](\d[A-Z]\d)?$/.test(normalized)) {
      return 'Canadian postal codes look like A1A or A1A 1A1.';
    }
  }
  return null;
}

export function SearchBox() {
  const navigate = useNavigate();
  const [raw, setRaw] = useState('');
  const [countryOverride, setCountryOverride] = useState<Country | ''>('');
  const [submitError, setSubmitError] = useState<string | null>(null);
  const inputId = useId();
  const countryId = useId();

  const effectiveCountry: Country =
    countryOverride !== '' ? countryOverride : (detectCountry(raw) ?? 'US');

  const normalized = useMemo(() => normalizePostal(raw), [raw]);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const err = validate(normalized, effectiveCountry);
    if (err) {
      setSubmitError(err);
      return;
    }
    setSubmitError(null);
    const search = new URLSearchParams({ country: effectiveCountry }).toString();
    navigate(`/r/${encodeURIComponent(normalized)}?${search}`);
  }

  return (
    <form className="search-box" onSubmit={handleSubmit} noValidate>
      <label htmlFor={inputId} className="section-label">
        Find local groups
      </label>
      <div className="search-box-row">
        <input
          id={inputId}
          type="text"
          name="postalCode"
          className="search-box-input"
          placeholder="11217 or M5V 2T6"
          autoComplete="postal-code"
          inputMode="text"
          spellCheck={false}
          value={raw}
          onChange={(event) => {
            setRaw(event.target.value);
            if (submitError) setSubmitError(null);
          }}
          aria-invalid={submitError !== null}
          aria-describedby={submitError ? `${inputId}-error` : `${inputId}-hint`}
        />
        <label htmlFor={countryId} className="visually-hidden" hidden>
          Country
        </label>
        <select
          id={countryId}
          name="country"
          className="search-box-country"
          value={countryOverride}
          onChange={(event) => setCountryOverride(event.target.value as Country | '')}
          aria-label="Country"
        >
          <option value="">Auto</option>
          <option value="US">US</option>
          <option value="CA">CA</option>
        </select>
        <button type="submit" className="search-box-submit">
          Look up
        </button>
      </div>
      {submitError ? (
        <p id={`${inputId}-error`} className="search-box-error" role="alert">
          {submitError}
        </p>
      ) : (
        <p id={`${inputId}-hint`} className="search-box-hint">
          US ZIP (5 digits) or Canadian postal code (FSA or full).
        </p>
      )}
    </form>
  );
}
