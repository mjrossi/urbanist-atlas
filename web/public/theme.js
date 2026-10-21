// Sets `data-theme` before first paint, so a stored choice never flashes
// the other theme. It is a static file, outside Vite's bundle, because a
// bundled module script is deferred until after first paint and the CSP
// (`script-src 'self'`) rules out an inline script. See CLAUDE.md, "The
// theme.js carve-out".
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

  // Keep in step with syncThemeColor in ThemeToggle.tsx.
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
  // Reveals the toggle: `:root[data-js] .theme-toggle[hidden]`.
  root.setAttribute('data-js', '');

  // Another tab changed the choice, or the page came back from the
  // back/forward cache with the theme it had when left.
  window.addEventListener('storage', (e) => {
    if (e.key === KEY || e.key === null) apply(read());
  });
  window.addEventListener('pageshow', (e) => {
    if (e.persisted) apply(read());
  });
})();
