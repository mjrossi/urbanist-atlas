import { keepPreviousData, useQuery } from '@tanstack/react-query';
import {
  type KeyboardEvent,
  type Ref,
  useCallback,
  useId,
  useRef,
  useState,
} from 'react';

import { type RegionSearchResult, searchRegions } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDebouncedValue } from '../lib/useDebouncedValue.ts';

interface RegionComboboxProps {
  /** Selected region slugs. */
  value: string[];
  onChange: (next: string[]) => void;
  /** The text input's id, for an external `<label htmlFor>`. */
  id?: string;
  onBlur?: () => void;
  describedById?: string;
  invalid?: boolean;
  /** Receives the text input, so a form can focus it. */
  ref?: Ref<HTMLInputElement>;
}

const MIN_QUERY_LEN = 2;
const SEARCH_DEBOUNCE_MS = 200;
const SEARCH_LIMIT = 8;

/**
 * A WAI-ARIA multi-select combobox over `GET /api/v1/regions/search`.
 * It only ever emits slugs the API returned.
 */
export function RegionCombobox({
  value,
  onChange,
  id,
  onBlur,
  describedById,
  invalid,
  ref,
}: RegionComboboxProps) {
  const [text, setText] = useState('');
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  // Captured at selection time, so chips need no second fetch.
  const [labels, setLabels] = useState<Record<string, { name: string; context: string }>>(
    {},
  );
  const listboxId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const setInputRef = useCallback(
    (el: HTMLInputElement | null) => {
      inputRef.current = el;
      if (typeof ref === 'function') ref(el);
      else if (ref) ref.current = el;
    },
    [ref],
  );

  const query = useDebouncedValue(text.trim(), SEARCH_DEBOUNCE_MS);
  const enabled = query.length >= MIN_QUERY_LEN;
  const {
    data: results = [],
    isError,
    isPlaceholderData,
  } = useQuery({
    queryKey: queryKeys.regionSearch(query),
    queryFn: ({ signal }) => searchRegions(query, SEARCH_LIMIT, { signal }),
    enabled,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });

  const options = results.filter((r) => !value.includes(r.region.slug));
  const showList = open && enabled && options.length > 0;
  // keepPreviousData leaves the last query's options on screen through the
  // debounce and the next fetch; they don't answer what is typed now.
  const resultsAreCurrent = query === text.trim() && !isPlaceholderData;
  // A failed search would otherwise look like "no matches".
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
        // Enter on a typed query picks the highlighted (or first) option,
        // and never submits the form, even while results are still stale.
        if (text.trim() === '') break;
        e.preventDefault();
        const active =
          showList && resultsAreCurrent
            ? (options[activeIndex] ?? options[0])
            : undefined;
        if (active) select(active);
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

  // Keyed by slug, not index, so aria-activedescendant follows the region
  // when the results change under it.
  const optionId = (slug: string) => `${listboxId}-opt-${slug}`;
  const activeOption = showList && activeIndex >= 0 ? options[activeIndex] : undefined;
  const activeOptionId = activeOption ? optionId(activeOption.region.slug) : undefined;

  let announcement = '';
  if (open && enabled && resultsAreCurrent && !isError) {
    announcement =
      options.length === 0
        ? 'No matching regions'
        : `${String(options.length)} matching ${options.length === 1 ? 'region' : 'regions'}`;
  }

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
        ref={setInputRef}
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

      <p className="sr-only" aria-live="polite">
        {announcement}
      </p>

      {searchFailed ? (
        <p className="combobox-status" role="status">
          Region search is unavailable right now — describe your region in the field below
          and we&rsquo;ll map it.
        </p>
      ) : null}
    </div>
  );
}
