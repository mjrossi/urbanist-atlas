import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { type KeyboardEvent, useId, useRef, useState } from 'react';

import { type RegionSearchResult, searchRegions } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDebouncedValue } from '../lib/useDebouncedValue.ts';

interface RegionComboboxProps {
  /** Selected region slugs — the controlled form value. */
  value: string[];
  /** Called with the next slug list whenever the selection changes. */
  onChange: (next: string[]) => void;
  /** id for the text input so an external `<label htmlFor>` binds to it. */
  id?: string;
  /** Forwarded to the input for blur-driven validation (react-hook-form). */
  onBlur?: () => void;
  /** id of the hint/error element describing the field, for a11y. */
  describedById?: string;
  /** Marks the control invalid for assistive tech + styling. */
  invalid?: boolean;
}

const MIN_QUERY_LEN = 2;
const SEARCH_DEBOUNCE_MS = 200;
const SEARCH_LIMIT = 8;

/**
 * Accessible multi-select type-ahead for region slugs, backing the
 * `/submit` form's "Region served" field. Queries
 * `GET /api/v1/regions/search` (debounced) and lets the user pick one
 * or more regions; selections render as removable chips and the value
 * is the list of canonical slugs the API expects in `region_slugs`.
 *
 * Built on the WAI-ARIA combobox pattern (input `role="combobox"` +
 * `role="listbox"` popup) with plain CSS — no new dependencies. The
 * free-text "can't find it?" fallback lives in the parent form, not
 * here: this control only ever emits real slugs.
 */
export function RegionCombobox({
  value,
  onChange,
  id,
  onBlur,
  describedById,
  invalid,
}: RegionComboboxProps) {
  const [text, setText] = useState('');
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  // Display labels captured at selection time, so chips read "Queens ·
  // New York" without a second fetch. Keyed by slug.
  const [labels, setLabels] = useState<Record<string, { name: string; context: string }>>(
    {},
  );
  const listboxId = useId();
  const inputRef = useRef<HTMLInputElement>(null);

  const query = useDebouncedValue(text.trim(), SEARCH_DEBOUNCE_MS);
  const enabled = query.length >= MIN_QUERY_LEN;
  const { data: results = [], isError } = useQuery({
    queryKey: queryKeys.regionSearch(query),
    queryFn: ({ signal }) => searchRegions(query, SEARCH_LIMIT, { signal }),
    enabled,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });

  // Hide already-picked regions from the option list.
  const options = results.filter((r) => !value.includes(r.region.slug));
  const showList = open && enabled && options.length > 0;
  // A failed search (5xx / network drop) yields no options, which is
  // indistinguishable from "no matches" — surface it so the submitter
  // knows to fall back to the free-text field instead of silently
  // losing a region they could have picked. Debounce-driven request
  // cancellations reject as AbortError, which react-query does not flag
  // as an error, so this stays quiet during normal typing.
  const searchFailed = isError && enabled;

  function select(result: RegionSearchResult) {
    const slug = result.region.slug;
    if (!value.includes(slug)) {
      onChange([...value, slug]);
      setLabels((prev) => ({
        ...prev,
        [slug]: { name: result.region.name, context: result.context_label },
      }));
    }
    setText('');
    setActiveIndex(-1);
    setOpen(false);
    inputRef.current?.focus();
  }

  function remove(slug: string) {
    onChange(value.filter((s) => s !== slug));
    // Drop the cached label too, so the map doesn't accumulate entries
    // for regions no longer selected. A re-add re-captures it in select().
    setLabels((prev) => {
      if (!(slug in prev)) return prev;
      const { [slug]: _removed, ...next } = prev;
      return next;
    });
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setOpen(true);
        setActiveIndex((i) => Math.min(i + 1, options.length - 1));
        break;
      case 'ArrowUp':
        e.preventDefault();
        setActiveIndex((i) => Math.max(i - 1, 0));
        break;
      case 'Enter': {
        // While the suggestion list is open, Enter picks the highlighted
        // option — or the first one if none is highlighted yet — rather
        // than submitting the form. With the list closed, Enter falls
        // through to the native form submit.
        const active = showList ? (options[activeIndex] ?? options[0]) : undefined;
        if (active) {
          e.preventDefault();
          select(active);
        }
        break;
      }
      case 'Escape':
        setOpen(false);
        setActiveIndex(-1);
        break;
      case 'Backspace': {
        const last = value[value.length - 1];
        if (text === '' && last !== undefined) {
          remove(last);
        }
        break;
      }
    }
  }

  // Option ids key off the region slug, not the array index, so
  // aria-activedescendant keeps pointing at the same region even if
  // keepPreviousData briefly reorders results during a query change.
  const optionId = (slug: string) => `${listboxId}-opt-${slug}`;
  const activeOption = showList && activeIndex >= 0 ? options[activeIndex] : undefined;
  const activeOptionId = activeOption ? optionId(activeOption.region.slug) : undefined;

  return (
    <div className="combobox">
      {value.length > 0 ? (
        <ul className="combobox-chips" aria-label="Selected regions">
          {value.map((slug) => {
            const label = labels[slug];
            const name = label?.name ?? slug;
            return (
              <li key={slug} className="combobox-chip">
                <span className="combobox-chip-name">{name}</span>
                {label?.context ? (
                  <span className="combobox-chip-context">{label.context}</span>
                ) : null}
                <button
                  type="button"
                  className="combobox-chip-remove"
                  aria-label={`Remove ${name}`}
                  onClick={() => {
                    remove(slug);
                  }}
                >
                  ×
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}

      <input
        ref={inputRef}
        id={id}
        type="text"
        className="input"
        role="combobox"
        aria-expanded={showList}
        aria-controls={showList ? listboxId : undefined}
        aria-autocomplete="list"
        aria-activedescendant={activeOptionId}
        aria-describedby={describedById}
        aria-invalid={invalid ? true : undefined}
        autoComplete="off"
        placeholder={
          value.length > 0
            ? 'Add another region…'
            : 'Start typing a city, metro, or borough…'
        }
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setOpen(true);
          setActiveIndex(-1);
        }}
        onFocus={() => {
          setOpen(true);
        }}
        onBlur={() => {
          setOpen(false);
          onBlur?.();
        }}
        onKeyDown={onKeyDown}
      />

      {showList ? (
        <ul className="combobox-list" role="listbox" id={listboxId}>
          {options.map((r, i) => (
            <li
              key={r.region.slug}
              id={optionId(r.region.slug)}
              role="option"
              aria-selected={i === activeIndex}
              className={`combobox-option${i === activeIndex ? ' is-active' : ''}`}
              // onMouseDown (not onClick) fires before the input's blur
              // closes the list; preventDefault keeps focus in the input.
              onMouseDown={(e) => {
                e.preventDefault();
                select(r);
              }}
              onMouseEnter={() => {
                setActiveIndex(i);
              }}
            >
              <span className="combobox-option-name">{r.region.name}</span>
              {r.context_label ? (
                <span className="combobox-option-context">{r.context_label}</span>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}

      {searchFailed ? (
        <p className="combobox-status" role="status">
          Region search is unavailable right now — describe your region in the field below
          and we&rsquo;ll map it.
        </p>
      ) : null}
    </div>
  );
}
