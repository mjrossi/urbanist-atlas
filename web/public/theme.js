// The theme carve-out: the one piece of JS that is not in Vite's bundle.
//
// Loaded as a classic, synchronous <script src="/theme.js"> from <head> in
// index.html, so `data-theme` is on <html> before the first paint and a
// reader who chose Dark never sees a cream flash. It is a static file in
// public/ on purpose:
//   - a bundled module script is deferred, so it would run after the
//     first paint and flash the wrong theme;
//   - the CSP (web/public/_headers) is `script-src 'self'`, so an inline
//     <script> is not an option.
//
// This is the Lexicon Family core's theme.js contract, ported from
// mjrossi.com's public/scripts/theme.js. Everything here degrades to
// nothing: no script, no storage, or an unknown stored value all leave
// <html> without `data-theme`, which is Auto — the stylesheet's
// prefers-color-scheme block decides. The toggle itself is React
// (src/components/ThemeToggle.tsx); it writes the same storage key and
// attribute, and observes the attribute this script maintains.
(() => {
  const KEY = 'theme';
  const root = document.documentElement;

  const read = () => {
    try {
      const v = localStorage.getItem(KEY);
      return v === 'light' || v === 'dark' ? v : 'auto';
    } catch {
      return 'auto';
    }
  };

  // Same as syncThemeColor in ThemeToggle.tsx: an explicit choice forces
  // its scheme's theme-color meta on and the other off.
  const syncThemeColor = (choice) => {
    for (const meta of document.querySelectorAll(
      'meta[name="theme-color"][data-scheme]',
    )) {
      const scheme = meta.getAttribute('data-scheme');
      meta.setAttribute(
        'media',
        choice === 'auto'
          ? `(prefers-color-scheme: ${scheme})`
          : scheme === choice
            ? 'all'
            : 'not all',
      );
    }
  };

  const apply = (choice) => {
    if (choice === 'auto') root.removeAttribute('data-theme');
    else root.setAttribute('data-theme', choice);
    syncThemeColor(choice);
  };

  apply(read());
  // The core's reveal hook: `:root[data-js] .theme-toggle[hidden]`.
  root.setAttribute('data-js', '');

  // Re-read the stored choice: another tab changed it (`storage` fires
  // only in the tabs that did NOT make the change), or the page came
  // back from the back/forward cache with the theme it had when left.
  window.addEventListener('storage', (e) => {
    if (e.key === KEY || e.key === null) apply(read());
  });
  window.addEventListener('pageshow', (e) => {
    if (e.persisted) apply(read());
  });
})();
