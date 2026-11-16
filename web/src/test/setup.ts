// jsdom has no window.matchMedia. Every query reports no match.
if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
  // The deprecated addListener/removeListener are omitted; app code uses addEventListener.
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }) as unknown as MediaQueryList;
}
