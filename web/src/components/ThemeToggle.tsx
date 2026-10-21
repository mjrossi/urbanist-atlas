import { useSyncExternalStore } from 'react';

export type ThemeChoice = 'auto' | 'light' | 'dark';

const STORAGE_KEY = 'theme';

/**
 * `data-theme` on `<html>` is the source of truth. `/theme.js` also
 * re-applies it, so the toggle observes the attribute rather than
 * keeping its own copy.
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
  syncThemeColor(next);
}

/**
 * Points the browser chrome at the chosen theme: an explicit choice forces
 * its scheme's theme-color meta on and the other off. Keep in step with
 * the same function in public/theme.js.
 */
function syncThemeColor(choice: ThemeChoice) {
  for (const meta of document.querySelectorAll('meta[name="theme-color"][data-scheme]')) {
    const scheme = meta.getAttribute('data-scheme') ?? '';
    const media =
      choice === 'auto'
        ? `(prefers-color-scheme: ${scheme})`
        : scheme === choice
          ? 'all'
          : 'not all';
    meta.setAttribute('media', media);
  }
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

/** The family core's Auto · Light · Dark icon toggle. */
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
