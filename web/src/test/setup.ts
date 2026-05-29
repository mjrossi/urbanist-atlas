// jsdom doesn't ship window.matchMedia; components that use it
// (e.g. About.tsx's desktop force-open effect) throw on mount in
// tests without this polyfill. Default to "no match" so the
// production-only desktop path stays inert under jsdom.
if (typeof window !== 'undefined' && !window.matchMedia) {
  window.matchMedia = (query: string): MediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  });
}
