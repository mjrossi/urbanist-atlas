import { useSyncExternalStore } from 'react';

export type ThemeChoice = 'auto' | 'light' | 'dark';

const STORAGE_KEY = 'theme';

/**
 * The `data-theme` attribute on `<html>` is the single source of truth.
 * `/theme.js` sets it before first paint and re-applies it when another
 * tab changes the stored choice or the page returns from the
 * back/forward cache; clicks here set it too. Observing the attribute
 * keeps the pressed state right in every one of those cases without a
 * second copy of the logic.
 */
function readChoice(): ThemeChoice {
  const value = document.documentElement.getAttribute('data-theme');
  return value === 'light' || value === 'dark' ? value : 'auto';
}

function subscribe(onChange: () => void): () => void {
  const observer = new MutationObserver(onChange);
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-theme'],
  });
  return () => {
    observer.disconnect();
  };
}

function choose(next: ThemeChoice) {
  try {
    if (next === 'auto') localStorage.removeItem(STORAGE_KEY);
    else localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // Storage refused (private mode, blocked site data): the choice
    // still applies to this page, it just won't follow the reader.
  }
  const root = document.documentElement;
  if (next === 'auto') root.removeAttribute('data-theme');
  else root.setAttribute('data-theme', next);
}

const OPTIONS: readonly { choice: ThemeChoice; label: string; title: string }[] = [
  { choice: 'auto', label: 'Match system theme', title: 'Match system' },
  { choice: 'light', label: 'Light theme', title: 'Light' },
  { choice: 'dark', label: 'Dark theme', title: 'Dark' },
];

function Glyph({ choice }: { choice: ThemeChoice }) {
  switch (choice) {
    case 'auto':
      return (
        <>
          <circle
            cx="8"
            cy="8"
            r="5.75"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
          />
          <path d="M8 2.25a5.75 5.75 0 0 1 0 11.5Z" fill="currentColor" />
        </>
      );
    case 'light':
      return (
        <>
          <circle
            cx="8"
            cy="8"
            r="3"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
          />
          <path
            d="M8 1v1.75M8 13.25V15M1 8h1.75M13.25 8H15M3.05 3.05l1.24 1.24M11.71 11.71l1.24 1.24M3.05 12.95l1.24-1.24M11.71 4.29l1.24-1.24"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
          />
        </>
      );
    case 'dark':
      return (
        <path
          d="M13.5 9.6A5.75 5.75 0 1 1 6.4 2.5a4.5 4.5 0 0 0 7.1 7.1Z"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinejoin="round"
        />
      );
  }
}

/**
 * Auto · Light · Dark, as three small glyphs: a half-filled disc
 * (follow the system), a sun, a moon. The Lexicon Family core's
 * ThemeToggle — rendered once, in the masthead folio, the one line of
 * chrome on screen at every width. Icon-only, so each button carries
 * its own aria-label, and a title for the pointer.
 */
export function ThemeToggle() {
  const current = useSyncExternalStore(subscribe, readChoice, () => 'auto' as const);
  return (
    <span className="theme-toggle" role="group" aria-label="Colour theme">
      {OPTIONS.map(({ choice, label, title }) => (
        <button
          key={choice}
          type="button"
          aria-pressed={current === choice}
          aria-label={label}
          title={title}
          onClick={() => {
            choose(choice);
          }}
        >
          <svg
            viewBox="0 0 16 16"
            width="14"
            height="14"
            aria-hidden="true"
            focusable="false"
          >
            <Glyph choice={choice} />
          </svg>
        </button>
      ))}
    </span>
  );
}
