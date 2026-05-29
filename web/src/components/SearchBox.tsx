import { useId, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { Link, useNavigate } from 'react-router';
import type { Country } from '../lib/api.ts';
import { normalizePostal } from '../lib/postal.ts';

type DetectedCountry = Country | null;

// TODO(third-country): the digit→US, letter→CA heuristic is fine for
// the v1 US/CA shipping decision. When PT/ES/UK go user-facing, swap
// this for a per-country regex map (or reintroduce an explicit country
// pick); lib/api.ts' SUPPORTED_COUNTRIES needs updating in lockstep.
function detectCountry(raw: string): DetectedCountry {
  const trimmed = raw.trim();
  if (trimmed.length === 0) return null;
  const first = trimmed[0];
  if (first === undefined) return null;
  if (/[0-9]/.test(first)) return 'US';
  if (/[A-Za-z]/.test(first)) return 'CA';
  return null;
}

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

const SUGGESTIONS: ReadonlyArray<{ label: string; postal: string; country: Country }> = [
  { label: '10027', postal: '10027', country: 'US' },
  { label: '94110', postal: '94110', country: 'US' },
  { label: 'M5V 1J1', postal: 'M5V1J1', country: 'CA' },
  { label: '02139', postal: '02139', country: 'US' },
  { label: '60607', postal: '60607', country: 'US' },
];

export function SearchBox() {
  const navigate = useNavigate();
  const [raw, setRaw] = useState('');
  const [submitError, setSubmitError] = useState<string | null>(null);
  const inputId = useId();
  const effectiveCountry: Country = detectCountry(raw) ?? 'US';
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
    <div className="lookup">
      <div className="lookup-eyebrow">§ Look it up in the Atlas</div>
      <h2 className="lookup-title">Find local advocates by postal code.</h2>
      <form className="lookup-row" onSubmit={handleSubmit} noValidate>
        <label htmlFor={inputId} className="sr-only">
          Postal code
        </label>
        <input
          id={inputId}
          type="text"
          name="postalCode"
          className="lookup-input"
          placeholder="94110 · M5V 1J1 · 10027"
          autoComplete="postal-code"
          spellCheck={false}
          value={raw}
          onChange={(event) => {
            setRaw(event.target.value);
            if (submitError) setSubmitError(null);
          }}
          aria-invalid={submitError !== null}
          aria-describedby={submitError ? `${inputId}-error` : `${inputId}-hint`}
        />
        <button type="submit" className="btn-primary">
          Look up <span className="arrow">→</span>
        </button>
      </form>
      {submitError ? (
        <p id={`${inputId}-error`} className="lookup-hint error" role="alert">
          {submitError}
        </p>
      ) : (
        <p id={`${inputId}-hint`} className="lookup-hint">
          US ZIP (5 digits) or Canadian postal code (FSA or full). We&rsquo;ll
          name your metro and the groups working there.
        </p>
      )}
      <div className="lookup-suggestions">
        <span className="label">Try one</span>
        {SUGGESTIONS.map((s) => (
          <Link key={s.postal} to={`/r/${s.postal}?country=${s.country}`}>
            {s.label}
          </Link>
        ))}
      </div>
    </div>
  );
}
